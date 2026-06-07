package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// R2Store is a thin wrapper around Cloudflare R2 (S3-compatible).
type R2Store struct {
	client *s3.Client
	bucket string
}

// NewR2 creates an R2Store pointed at the given Cloudflare account.
// The endpoint is derived from accountID: https://{accountID}.r2.cloudflarestorage.com
func NewR2(accountID, accessKeyID, secretAccessKey, bucket string) (*R2Store, error) {
	if accountID == "" || accessKeyID == "" || secretAccessKey == "" || bucket == "" {
		return nil, fmt.Errorf("storage: all R2 credentials and bucket name are required")
	}
	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID)
	client := s3.New(s3.Options{
		Region:      "auto",
		BaseEndpoint: aws.String(endpoint),
		Credentials: credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
	})
	return &R2Store{client: client, bucket: bucket}, nil
}

// Put uploads data to the given key.
func (s *R2Store) Put(ctx context.Context, key string, data []byte) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
	})
	if err != nil {
		return fmt.Errorf("r2 put %q: %w", key, err)
	}
	return nil
}

// Get downloads and returns the object at key.
func (s *R2Store) Get(ctx context.Context, key string) ([]byte, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("r2 get %q: %w", key, err)
	}
	defer out.Body.Close()
	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("r2 read %q: %w", key, err)
	}
	return data, nil
}

// Delete removes the object at key. Returns nil if the object does not exist.
func (s *R2Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("r2 delete %q: %w", key, err)
	}
	return nil
}
