package common

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func generateRandomBody(size int64) string {
	randomBody := make([]byte, size)
	// fill with random bytes
	rand.Read(randomBody)
	return string(randomBody)
}

func generateRandomSize(t *testing.T) int64 {
	randSize, err := rand.Int(rand.Reader, big.NewInt(1000))
	size := randSize.Int64()
	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}
	return size
}

// MockS3Client implements the S3 API using function fields
type MockS3Client struct {
	HeadObjectFunc   func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	GetObjectFunc    func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	HeadBucketFunc   func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
	PutObjectFunc    func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	DeleteObjectFunc func(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

func (m *MockS3Client) HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	return m.HeadObjectFunc(ctx, params, optFns...)
}

func (m *MockS3Client) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return m.GetObjectFunc(ctx, params, optFns...)
}

func (m *MockS3Client) HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	return m.HeadBucketFunc(ctx, params, optFns...)
}

func (m *MockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	return m.PutObjectFunc(ctx, params, optFns...)
}

func (m *MockS3Client) DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	return m.DeleteObjectFunc(ctx, params, optFns...)
}

// TestBucketExists_Success tests the case where the bucket exists.
func TestBucketExists_Success(t *testing.T) {
	client := &MockS3Client{
		HeadBucketFunc: func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
			return &s3.HeadBucketOutput{}, nil
		},
	}

	exists, err := BucketExists(context.TODO(), client, "my-existing-bucket")

	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}
	if !exists {
		t.Errorf("expected bucket to exist, but it did not")
	}
}

func Test_BucketExists_Bucket_Not_Found(t *testing.T) {
	// client := newTestS3ClientOld(t, func(req *http.Request) (*http.Response, error) {
	// 	// To trigger the SDK's *types.NotFound error, we must return a 404
	// 	// with a specific XML error body that the SDK knows how to parse, according to S3 error response format.
	// 	return &http.Response{
	// 		StatusCode: http.StatusNotFound,
	// 		Body:       io.NopCloser(strings.NewReader(`<Error><Code>NotFound</Code></Error>`)),
	// 	}, nil
	// })

	client := &MockS3Client{
		HeadBucketFunc: func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
			return nil, &types.NotFound{
				Message: aws.String("Bucket not found"),
			}
		},
	}
	exists, err := BucketExists(context.TODO(), client, "my-new-bucket")

	if err != nil {
		t.Fatalf("expected no error for a 'NotFound' case, but got: %v", err)
	}
	if exists {
		t.Errorf("expected bucket not to exist, but it did")
	}
}

func TestBucketExists_AccessDenied(t *testing.T) {
	// client := newTestS3ClientOld(t, func(req *http.Request) (*http.Response, error) {
	// 	// Simulate a different error, like a 403 Forbidden.
	// 	return &http.Response{
	// 		StatusCode: http.StatusForbidden,
	// 		Body:       io.NopCloser(strings.NewReader(`<Error><Code>AccessDenied</Code></Error>`)),
	// 	}, nil
	// })
	client := &MockS3Client{
		HeadBucketFunc: func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
			// Simulate a different error, like a 403 Forbidden.
			return nil, &types.AccessDenied{
				Message: aws.String("Access denied"),
			}
		},
	}

	exists, err := BucketExists(context.TODO(), client, "some-bucket")

	if err == nil {
		t.Fatalf("expected an error, but got nil")
	}
	if exists {
		t.Errorf("expected bucket not to exist on error, but it did")
	}
}

func Test_ObjectExists_Success(t *testing.T) {
	client := &MockS3Client{
		HeadObjectFunc: func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
			return &s3.HeadObjectOutput{
				ContentLength: aws.Int64(100),
			}, nil
		},
	}

	exists, err := ObjectExists(context.TODO(), client, "my-existing-bucket", "my-existing-object")

	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}
	if !exists {
		t.Errorf("expected object to exist, but it did not")
	}
}

func Test_ObjectExists_Object_Not_Found(t *testing.T) {
	client := &MockS3Client{
		HeadObjectFunc: func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
			return nil, &types.NotFound{
				Message: aws.String("Object not found"),
			}
		},
	}

	exists, err := ObjectExists(context.TODO(), client, "my-existing-bucket", "my-existing-object")

	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}
	if exists {
		t.Errorf("expected object not to exist, but it did")
	}
}

func Test_NewS3SeekableFileReader_Success(t *testing.T) {
	size := generateRandomSize(t)
	body := generateRandomBody(size)

	client := &MockS3Client{
		HeadObjectFunc: func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
			return &s3.HeadObjectOutput{
				ContentLength: aws.Int64(size),
			}, nil
		},
		GetObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{
				Body: io.NopCloser(strings.NewReader(body)),
			}, nil
		},
	}
	seekableFileReader, err := NewS3SeekableReader(context.TODO(), client, "my-existing-bucket", "my-existing-object")
	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}

	readData := make([]byte, size)
	n, err := seekableFileReader.Read(readData)
	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}
	if int(n) != int(size) {
		t.Errorf("expected to read %d bytes, but got: %d", size, n)
	}
}

func Test_NewS3SeekableFileReader_Read_Error(t *testing.T) {
	//size := generateRandomSize(t)
	client := &MockS3Client{
		HeadObjectFunc: func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
			// Error case
			return nil, fmt.Errorf("error")
		},
		GetObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return nil, fmt.Errorf("error")
		},
	}
	seekableFileReader, err := NewS3SeekableReader(context.TODO(), client, "my-existing-bucket", "my-non-existing-object")

	if err == nil {
		t.Fatalf("expected an error, but got: %v", err)
	}

	if seekableFileReader != nil {
		t.Fatalf("expected a nil S3SeekableReader, but got: %v", seekableFileReader)
	}
}

func Test_NewS3SeekableFileReader_ReadAt_Success(t *testing.T) {
	size := generateRandomSize(t)
	body := generateRandomBody(size)
	// client := newTestS3ClientOld(t, func(req *http.Request) (*http.Response, error) {
	// 	return &http.Response{
	// 		StatusCode: http.StatusOK,
	// 		Body:       io.NopCloser(strings.NewReader(body)),
	// 	}, nil
	// })
	client := &MockS3Client{
		HeadObjectFunc: func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
			return &s3.HeadObjectOutput{
				ContentLength: aws.Int64(size),
			}, nil
		},
		GetObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{
				Body: io.NopCloser(strings.NewReader(body)),
			}, nil
		},
	}
	seekableFileReader, err := NewS3SeekableReader(context.TODO(), client, "my-existing-bucket", "my-existing-object")

	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}

	n, err := seekableFileReader.ReadAt(make([]byte, size), 0)
	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}

	if n != int(size) {
		t.Errorf("expected to read %d bytes, but got: %d", size, n)
	}
}
