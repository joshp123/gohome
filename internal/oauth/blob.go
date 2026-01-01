package oauth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	configv1 "github.com/joshp123/gohome/proto/gen/config/v1"
)

var ErrBlobNotFound = errors.New("oauth blob not found")

// BlobStore handles state mirroring to object storage.
type BlobStore interface {
	Load(ctx context.Context, provider string) ([]byte, error)
	Save(ctx context.Context, provider string, data []byte) error
}

type S3Store struct {
	client *minio.Client
	bucket string
	prefix string
}

func NewS3Store(cfg *configv1.OAuthConfig) (*S3Store, error) {
	if cfg == nil {
		return nil, fmt.Errorf("missing oauth config")
	}

	endpoint := strings.TrimSpace(cfg.BlobEndpoint)
	bucket := strings.TrimSpace(cfg.BlobBucket)
	prefix := strings.TrimSpace(cfg.BlobPrefix)
	accessKeyFile := strings.TrimSpace(cfg.BlobAccessKeyFile)
	secretKeyFile := strings.TrimSpace(cfg.BlobSecretKeyFile)
	region := strings.TrimSpace(cfg.BlobRegion)

	if endpoint == "" || bucket == "" || accessKeyFile == "" || secretKeyFile == "" {
		return nil, fmt.Errorf("missing blob configuration")
	}

	accessKey, err := readSecretFile(accessKeyFile)
	if err != nil {
		return nil, fmt.Errorf("read blob access key: %w", err)
	}
	secretKey, err := readSecretFile(secretKeyFile)
	if err != nil {
		return nil, fmt.Errorf("read blob secret key: %w", err)
	}

	host, secure, err := parseEndpoint(endpoint)
	if err != nil {
		return nil, err
	}

	client, err := minio.New(host, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: secure,
		Region: region,
	})
	if err != nil {
		return nil, fmt.Errorf("init s3 client: %w", err)
	}

	if prefix == "" {
		prefix = "gohome/oauth"
	}

	return &S3Store{client: client, bucket: bucket, prefix: prefix}, nil
}

func (s *S3Store) Load(ctx context.Context, provider string) ([]byte, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, s.key(provider), minio.GetObjectOptions{})
	if err != nil {
		return nil, s.wrapError(err)
	}
	defer obj.Close()

	if _, err := obj.Stat(); err != nil {
		return nil, s.wrapError(err)
	}

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("read blob: %w", err)
	}
	return data, nil
}

func (s *S3Store) Save(ctx context.Context, provider string, data []byte) error {
	reader := bytes.NewReader(data)
	_, err := s.client.PutObject(ctx, s.bucket, s.key(provider), reader, int64(reader.Len()), minio.PutObjectOptions{
		ContentType: "application/json",
	})
	if err != nil {
		return s.wrapError(err)
	}
	return nil
}

func (s *S3Store) key(provider string) string {
	return path.Join(s.prefix, provider+".json")
}

func (s *S3Store) wrapError(err error) error {
	resp := minio.ToErrorResponse(err)
	if resp.Code == "NoSuchKey" {
		return ErrBlobNotFound
	}
	return err
}

func parseEndpoint(raw string) (string, bool, error) {
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", false, fmt.Errorf("parse endpoint: %w", err)
		}
		if u.Host == "" {
			return "", false, fmt.Errorf("invalid endpoint: %q", raw)
		}
		return u.Host, u.Scheme == "https", nil
	}
	return raw, true, nil
}

func readSecretFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
