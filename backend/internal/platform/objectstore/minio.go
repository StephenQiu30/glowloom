package objectstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var (
	ErrInvalidObjectDeclaration = errors.New("invalid object declaration")
	ErrObjectSizeMismatch       = errors.New("object size mismatch")
	ErrObjectChecksumMismatch   = errors.New("object checksum mismatch")
)

type Config struct {
	Endpoint, PublicEndpoint string
	AccessKey, SecretKey     string
	Bucket                   string
	Region                   string
	Secure, PublicSecure     bool
}

type Client struct {
	internal, public *minio.Client
	bucket           string
}

func Open(config Config) (*Client, error) {
	credential := credentials.NewStaticV4(config.AccessKey, config.SecretKey, "")
	internal, err := minio.New(config.Endpoint, &minio.Options{Creds: credential, Secure: config.Secure, Region: config.Region})
	if err != nil {
		return nil, fmt.Errorf("open MinIO client: %w", err)
	}
	public, err := minio.New(config.PublicEndpoint, &minio.Options{Creds: credential, Secure: config.PublicSecure, Region: config.Region})
	if err != nil {
		return nil, fmt.Errorf("open public MinIO client: %w", err)
	}
	return &Client{internal: internal, public: public, bucket: config.Bucket}, nil
}

func (client *Client) EnsureBucket(ctx context.Context) error {
	exists, err := client.internal.BucketExists(ctx, client.bucket)
	if err != nil {
		return fmt.Errorf("check MinIO bucket: %w", err)
	}
	if !exists {
		if err = client.internal.MakeBucket(ctx, client.bucket, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("create MinIO bucket: %w", err)
		}
	}
	return nil
}

func (client *Client) Ping(ctx context.Context) error {
	_, err := client.internal.BucketExists(ctx, client.bucket)
	return err
}

func (client *Client) PresignPut(ctx context.Context, objectKey string, expires time.Duration) (*url.URL, error) {
	value, err := client.public.PresignedPutObject(ctx, client.bucket, objectKey, expires)
	if err != nil {
		return nil, fmt.Errorf("presign object upload: %w", err)
	}
	return value, nil
}

func (client *Client) EnsurePrivateObject(
	ctx context.Context,
	objectKey string,
	contents []byte,
	mediaType, expectedSHA256 string,
) error {
	objectKey = strings.TrimSpace(objectKey)
	mediaType = strings.TrimSpace(mediaType)
	if client == nil || client.internal == nil || objectKey == "" || len(objectKey) > 1024 ||
		strings.HasPrefix(objectKey, "/") || strings.HasSuffix(objectKey, "/") ||
		strings.Contains(objectKey, "..") || strings.ContainsAny(objectKey, "\\\x00") ||
		len(contents) == 0 || mediaType == "" {
		return ErrInvalidObjectDeclaration
	}
	digest := sha256.Sum256(contents)
	if hex.EncodeToString(digest[:]) != expectedSHA256 {
		return ErrObjectChecksumMismatch
	}
	if _, err := client.internal.StatObject(ctx, client.bucket, objectKey, minio.StatObjectOptions{}); err == nil {
		return client.verifyPrivateObject(ctx, objectKey, int64(len(contents)), mediaType, expectedSHA256)
	} else if response := minio.ToErrorResponse(err); response.Code != "NoSuchKey" && response.Code != "NoSuchObject" &&
		response.StatusCode != 404 {
		return fmt.Errorf("inspect private object: %w", err)
	}
	options := minio.PutObjectOptions{ContentType: mediaType}
	options.SetMatchETagExcept("*")
	result, err := client.internal.PutObject(
		ctx, client.bucket, objectKey, bytes.NewReader(contents), int64(len(contents)),
		options,
	)
	if err != nil {
		response := minio.ToErrorResponse(err)
		if response.Code == "PreconditionFailed" || response.StatusCode == 412 {
			return client.verifyPrivateObject(ctx, objectKey, int64(len(contents)), mediaType, expectedSHA256)
		}
		return fmt.Errorf("write private object: %w", err)
	}
	if result.Key != objectKey || result.Size != int64(len(contents)) {
		return ErrObjectSizeMismatch
	}
	if err = client.verifyPrivateObject(ctx, objectKey, int64(len(contents)), mediaType, expectedSHA256); err != nil {
		return fmt.Errorf("verify private object: %w", err)
	}
	return nil
}

func (client *Client) verifyPrivateObject(
	ctx context.Context,
	objectKey string,
	expectedSize int64,
	expectedMediaType, expectedSHA256 string,
) error {
	info, err := client.internal.StatObject(ctx, client.bucket, objectKey, minio.StatObjectOptions{})
	if err != nil {
		return fmt.Errorf("inspect private object: %w", err)
	}
	if info.Size != expectedSize {
		return ErrObjectSizeMismatch
	}
	if info.ContentType != expectedMediaType {
		return ErrInvalidObjectDeclaration
	}
	_, err = client.ReadVerified(ctx, objectKey, expectedSize, expectedSHA256, expectedSize)
	return err
}

func (client *Client) ReadVerified(ctx context.Context, objectKey string, expectedSize int64, expectedSHA256 string, maxBytes int64) ([]byte, error) {
	if expectedSize < 1 || expectedSize > maxBytes {
		return nil, fmt.Errorf("%w: object size %d is outside the allowed range", ErrInvalidObjectDeclaration, expectedSize)
	}
	object, err := client.internal.GetObject(ctx, client.bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object: %w", err)
	}
	defer object.Close()
	contents, err := io.ReadAll(io.LimitReader(object, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read object: %w", err)
	}
	if int64(len(contents)) != expectedSize {
		return nil, fmt.Errorf("%w: got %d, expected %d", ErrObjectSizeMismatch, len(contents), expectedSize)
	}
	hash := sha256.Sum256(contents)
	if hex.EncodeToString(hash[:]) != expectedSHA256 {
		return nil, ErrObjectChecksumMismatch
	}
	return contents, nil
}
