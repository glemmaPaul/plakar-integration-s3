package common

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3SeekableReader implements io.ReadSeekCloser and io.ReaderAt (heavily inspired by minio-go)
type S3SeekableReader struct {
	ctx    context.Context
	client S3Client
	bucket string
	key    string

	offset    int64
	totalSize int64
	body      io.ReadCloser
	mu        sync.Mutex
}

// NewS3SeekableReader initializes the reader and fetches file metadata (size)
// NOTE: Perhaps we have to let the HeadObject responsibility to the caller
func NewS3SeekableReader(ctx context.Context, client S3Client, bucket, key string) (*S3SeekableReader, error) {
	head, err := client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}

	return &S3SeekableReader{
		ctx:       ctx,
		client:    client,
		bucket:    bucket,
		key:       key,
		totalSize: *head.ContentLength,
		offset:    0,
	}, nil
}

// Read reads from the current offset. It keeps the connection open.
func (s *S3SeekableReader) Read(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.offset >= s.totalSize {
		return 0, io.EOF
	}

	if s.body == nil {
		if err := s.openStream(); err != nil {
			return 0, err
		}
	}

	n, err := s.body.Read(p)
	s.offset += int64(n)

	if err != nil {
		s.closeStream()
	}

	return n, err
}

// Seek sets the offset for the next Read
func (s *S3SeekableReader) Seek(offset int64, whence int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var newOffset int64
	switch whence {
	case io.SeekStart:
		newOffset = offset
	case io.SeekCurrent:
		newOffset = s.offset + offset
	case io.SeekEnd:
		newOffset = s.totalSize + offset
	default:
		return 0, fmt.Errorf("invalid whence: %d", whence)
	}

	if newOffset < 0 {
		return 0, fmt.Errorf("seek before start")
	}

	// Optimization: If offset hasn't changed, do nothing
	if newOffset == s.offset {
		return newOffset, nil
	}

	// We need to close the current stream and open a new one with the new offset.
	s.closeStream()
	s.offset = newOffset

	return s.offset, nil
}

func (s *S3SeekableReader) ReadAt(p []byte, off int64) (int, error) {

	if off >= s.totalSize {
		return 0, io.EOF
	}

	// Calculate range: bytes=start-end
	end := off + int64(len(p)) - 1
	rangeHeader := fmt.Sprintf("bytes=%d-%d", off, end)

	out, err := s.client.GetObject(s.ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    &s.key,
		Range:  aws.String(rangeHeader),
	})
	if err != nil {
		return 0, err
	}
	defer out.Body.Close()

	// ReadFull ensures we get the exact bytes requested (or error)
	return io.ReadFull(out.Body, p)
}

func (s *S3SeekableReader) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeStream()
}

// Helpers
func (s *S3SeekableReader) openStream() error {
	rangeHeader := fmt.Sprintf("bytes=%d-", s.offset) // Request from offset to end

	resp, err := s.client.GetObject(s.ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    &s.key,
		Range:  aws.String(rangeHeader),
	})
	if err != nil {
		return err
	}
	s.body = resp.Body
	return nil
}

func (s *S3SeekableReader) closeStream() error {
	if s.body != nil {
		err := s.body.Close()
		s.body = nil
		return err
	}
	return nil
}
