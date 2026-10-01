// Package objectstorage adapts private object errors to media application contracts.
package objectstorage

import (
	"context"
	"errors"
	"io"

	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	platformobjects "github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

// ProjectCopyObjects retains exact-key access while translating conditional creation.
type ProjectCopyObjects struct{ client *platformobjects.Client }

// NewProjectCopyObjects injects the configured private bucket client.
func NewProjectCopyObjects(client *platformobjects.Client) *ProjectCopyObjects {
	return &ProjectCopyObjects{client: client}
}

// Get opens the private bytes selected by a media-owned snapshot.
func (s *ProjectCopyObjects) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return s.client.Get(ctx, key)
}

// Exists distinguishes absence from transport or authorization failure.
func (s *ProjectCopyObjects) Exists(ctx context.Context, key string) (bool, error) {
	return s.client.Exists(ctx, key)
}

// PutIfAbsent preserves the immutable key and stable application existence error.
func (s *ProjectCopyObjects) PutIfAbsent(ctx context.Context, key string, reader io.Reader, size int64, contentType, digest string) error {
	err := s.client.PutIfAbsent(ctx, key, reader, size, contentType, digest)
	if errors.Is(err, platformobjects.ErrObjectExists) {
		return application.ErrObjectAlreadyExists
	}
	return err
}

// Remove deletes only the exact key already verified by the copy coordinator.
func (s *ProjectCopyObjects) Remove(ctx context.Context, key string) error {
	return s.client.Remove(ctx, key)
}
