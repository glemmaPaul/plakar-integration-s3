package common

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	transfertypes "github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

type S3Client interface {
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

func Connect(location *url.URL, useSsl, insecure bool, accessKeyID, secretAccessKey string) (*s3.Client, error) {
	creds := credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, "")

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		// TODO: make configurable (And perhaps omittable)
		config.WithRegion("eu-west-1"),
		config.WithCredentialsProvider(creds),
	)
	if err != nil {
		return nil, err
	}
	loggingEnabled := true

	return s3.NewFromConfig(cfg, buildOptions(location, useSsl, insecure, "plakar/1.0", loggingEnabled)...), nil
}

func buildOptions(location *url.URL, useSsl, insecure bool, userAgent string, loggingEnabled bool) []func(*s3.Options) {
	return []func(*s3.Options){
		withEndpoint(location, useSsl),
		withHTTPClient(insecure),
		withUserAgent(userAgent),
		withLogging(loggingEnabled),
		func(o *s3.Options) {
			// Make sure to use path style for S3 compatibility
			o.UsePathStyle = true
		},
	}
}

func withLogging(loggingEnabled bool) func(*s3.Options) {
	return func(o *s3.Options) {
		if loggingEnabled {
			o.ClientLogMode = aws.LogRequestWithBody | aws.LogResponseWithBody
		}
	}
}

func withEndpoint(location *url.URL, useSsl bool) func(*s3.Options) {
	return func(o *s3.Options) {
		scheme := "http"
		if useSsl {
			scheme = "https"
		}

		if location.Host != "" {
			o.BaseEndpoint = aws.String(fmt.Sprintf("%s://%s", scheme, location.Host))
		}
	}
}

func withHTTPClient(insecure bool) func(*s3.Options) {
	return func(o *s3.Options) {
		if insecure {
			tr := &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}
			o.HTTPClient = &http.Client{Transport: tr}
		}
	}
}

func withUserAgent(ua string) func(*s3.Options) {
	return func(o *s3.Options) {
		o.APIOptions = append(o.APIOptions, func(stack *middleware.Stack) error {
			return stack.Build.Add(middleware.BuildMiddlewareFunc("AttachUserAgent",
				func(ctx context.Context, in middleware.BuildInput, next middleware.BuildHandler) (middleware.BuildOutput, middleware.Metadata, error) {
					if req, ok := in.Request.(*smithyhttp.Request); ok {
						req.Header.Set("User-Agent", ua)
					}
					return next.HandleBuild(ctx, in)
				}), middleware.After)
		})
	}
}

func BucketExists(ctx context.Context, client S3Client, bucket string) (bool, error) {
	bucketResponse, err := client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})
	fmt.Printf("HEAD BUCKET: Metadata=%+v Error=%v\n", bucketResponse, err)
	if err != nil {
		var notFoundErr *types.NotFound
		if errors.As(err, &notFoundErr) {
			// The bucket does not exist, but an expected error
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func ObjectExists(ctx context.Context, client S3Client, bucket, key string) (bool, error) {
	_, err := client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		var notFoundErr *types.NotFound
		if errors.As(err, &notFoundErr) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func NewPutObjectInput(bucket, key string, body io.Reader, storageClass types.StorageClass) (*s3.PutObjectInput, error) {
	content, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	hash := md5.Sum(content)
	md5string := base64.StdEncoding.EncodeToString(hash[:])

	return &s3.PutObjectInput{
		Bucket:       aws.String(bucket),
		Key:          aws.String(key),
		Body:         bytes.NewReader(content),
		StorageClass: storageClass,
		ContentMD5:   aws.String(md5string),
	}, nil
}

func PutObjectSigned(ctx context.Context, client S3Client, bucket, key string, body io.Reader, storageClass types.StorageClass) (*s3.PutObjectOutput, error) {
	/*
		Put object signed with md5 checksum.
	*/
	putObjectInput, err := NewPutObjectInput(bucket, key, body, storageClass)
	if err != nil {
		return nil, fmt.Errorf("create put object input: %w", err)
	}
	return client.PutObject(ctx, putObjectInput)
}

func UploadObject(ctx context.Context, client transfermanager.Client, bucket, key string, body io.Reader, storageClass types.StorageClass) (*transfermanager.UploadObjectOutput, error) {
	putObjectInput := &transfermanager.UploadObjectInput{
		Bucket:            aws.String(bucket),
		Key:               aws.String(key),
		Body:              body,
		StorageClass:      transfertypes.StorageClass(storageClass),
		ChecksumAlgorithm: transfertypes.ChecksumAlgorithmSha256,
	}
	return client.UploadObject(ctx, putObjectInput)
}
