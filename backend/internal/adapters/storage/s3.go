// Package storage provides an S3-compatible storage adapter.
// Supports AWS S3, Cloudflare R2, MinIO, and any S3-compatible API.
package storage

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

// S3Adapter implements outbound.StorageAdapter for any S3-compatible backend.
type S3Adapter struct {
	client         *s3.Client
	presignClient  *s3.PresignClient
	bucket         string
	publicBaseURL  string
}

// Factory caches adapters per-tenant to avoid recreating S3 clients.
type Factory struct {
	mu    sync.RWMutex
	cache map[string]*S3Adapter // keyed by bucket
}

func NewFactory() *Factory {
	return &Factory{cache: make(map[string]*S3Adapter)}
}

func (f *Factory) For(cfg outbound.StorageConfig) (outbound.StorageAdapter, error) {
	f.mu.RLock()
	if a, ok := f.cache[cfg.Bucket]; ok {
		f.mu.RUnlock()
		return a, nil
	}
	f.mu.RUnlock()

	adapter, err := newS3Adapter(cfg)
	if err != nil {
		return nil, err
	}

	f.mu.Lock()
	f.cache[cfg.Bucket] = adapter
	f.mu.Unlock()

	return adapter, nil
}

func newS3Adapter(cfg outbound.StorageConfig) (*S3Adapter, error) {
	creds := credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretKey, "")

	awsCfg, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(creds),
	)
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}

	opts := []func(*s3.Options){
		func(o *s3.Options) {
			o.UsePathStyle = cfg.ForcePathStyle
		},
	}

	if cfg.Endpoint != "" {
		opts = append(opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		})
	}

	client := s3.NewFromConfig(awsCfg, opts...)

	return &S3Adapter{
		client:        client,
		presignClient: s3.NewPresignClient(client),
		bucket:        cfg.Bucket,
		publicBaseURL: cfg.PublicBaseURL,
	}, nil
}

func (a *S3Adapter) Upload(ctx context.Context, key string, r io.Reader, size int64, contentType string) (string, error) {
	_, err := a.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(a.bucket),
		Key:           aws.String(key),
		Body:          r,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
		ACL:           types.ObjectCannedACLPrivate,
	})
	if err != nil {
		return "", fmt.Errorf("s3 put: %w", err)
	}

	url := a.publicURL(key)
	return url, nil
}

func (a *S3Adapter) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := a.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 get: %w", err)
	}
	return out.Body, nil
}

func (a *S3Adapter) Delete(ctx context.Context, key string) error {
	_, err := a.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(key),
	})
	return err
}

func (a *S3Adapter) PresignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	req, err := a.presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("presign: %w", err)
	}
	return req.URL, nil
}

func (a *S3Adapter) Exists(ctx context.Context, key string) (bool, error) {
	_, err := a.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return false, nil // treat all errors as not-found for simplicity
	}
	return true, nil
}

func (a *S3Adapter) publicURL(key string) string {
	if a.publicBaseURL != "" {
		return a.publicBaseURL + "/" + key
	}
	return fmt.Sprintf("https://%s.s3.amazonaws.com/%s", a.bucket, key)
}
