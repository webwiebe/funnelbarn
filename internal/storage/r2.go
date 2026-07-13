package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// R2Store is a thin wrapper around Cloudflare R2 (S3-compatible).
type R2Store struct {
	client *s3.Client
	bucket string
}

// NewR2 creates an R2Store. endpoint must be the full R2 endpoint URL
// (e.g. https://<accountID>.eu.r2.cloudflarestorage.com for EU jurisdiction buckets).
func NewR2(endpoint, accessKeyID, secretAccessKey, bucket string) (*R2Store, error) {
	if endpoint == "" || accessKeyID == "" || secretAccessKey == "" || bucket == "" {
		return nil, fmt.Errorf("storage: R2 endpoint, credentials and bucket name are required")
	}
	client := s3.New(s3.Options{
		Region:       "auto",
		BaseEndpoint: aws.String(endpoint),
		Credentials:  credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
	})
	return &R2Store{client: client, bucket: bucket}, nil
}

// Put uploads data to the given key.
func (s *R2Store) Put(ctx context.Context, key string, data []byte) error {
	ctx, span := tracing.StartSpan(ctx, "storage.r2.put",
		attribute.String("r2.bucket", s.bucket),
		attribute.String("r2.key", key),
		attribute.Int("r2.bytes", len(data)),
	)
	defer span.End()

	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
	})
	if err != nil {
		err = fmt.Errorf("r2 put %q: %w", key, err)
		tracing.RecordError(span, err)
		return err
	}
	return nil
}

// Get downloads and returns the object at key.
func (s *R2Store) Get(ctx context.Context, key string) ([]byte, error) {
	ctx, span := tracing.StartSpan(ctx, "storage.r2.get",
		attribute.String("r2.bucket", s.bucket),
		attribute.String("r2.key", key),
	)
	defer span.End()

	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		err = fmt.Errorf("r2 get %q: %w", key, err)
		tracing.RecordError(span, err)
		return nil, err
	}
	defer out.Body.Close()
	data, err := io.ReadAll(out.Body)
	if err != nil {
		err = fmt.Errorf("r2 read %q: %w", key, err)
		tracing.RecordError(span, err)
		return nil, err
	}
	span.SetAttributes(attribute.Int("r2.bytes", len(data)))
	return data, nil
}

// Delete removes the object at key. Returns nil if the object does not exist.
func (s *R2Store) Delete(ctx context.Context, key string) error {
	ctx, span := tracing.StartSpan(ctx, "storage.r2.delete",
		attribute.String("r2.bucket", s.bucket),
		attribute.String("r2.key", key),
	)
	defer span.End()

	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		err = fmt.Errorf("r2 delete %q: %w", key, err)
		tracing.RecordError(span, err)
		return err
	}
	return nil
}
