// Package objects converts storage-specific failures at the private-byte boundary.
package objects

import (
	"context"
	"errors"
	"io"

	"github.com/minio/minio-go/v7"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
)

// Storage uses the configured private bucket; keys come only from frozen plans.
type Storage struct{ client *objectstorage.Client }

// NewStorage injects the existing immutable private-object client.
func NewStorage(client *objectstorage.Client) *Storage { return &Storage{client: client} }
func storageError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, objectstorage.ErrObjectExists) {
		return errors.Join(application.ErrObjectExists, err)
	}
	response := minio.ToErrorResponse(err)
	if response.Code == "NoSuchKey" || response.Code == "NoSuchObject" || response.Code == "NotFound" {
		return errors.Join(application.ErrObjectMissing, err)
	}
	return err
}

// PutIfAbsent preserves If-None-Match and exposes a stable already-present predicate.
func (s *Storage) PutIfAbsent(ctx context.Context, key string, reader io.Reader, size int64, mime, hash string) error {
	if s == nil || s.client == nil {
		return application.ErrUnavailable
	}
	return storageError(s.client.PutIfAbsent(ctx, key, reader, size, mime, hash))
}

// Get maps deferred MinIO read errors as well as immediate acquisition errors.
func (s *Storage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if s == nil || s.client == nil {
		return nil, application.ErrUnavailable
	}
	reader, err := s.client.Get(ctx, key)
	if err != nil {
		return nil, storageError(err)
	}
	return &privateReader{ReadCloser: reader}, nil
}

type privateReader struct{ io.ReadCloser }

func (r *privateReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	return n, storageError(err)
}

// Remove requires the caller's separate ownership and digest proof.
func (s *Storage) Remove(ctx context.Context, key string) error {
	if s == nil || s.client == nil {
		return application.ErrUnavailable
	}
	return storageError(s.client.Remove(ctx, key))
}
