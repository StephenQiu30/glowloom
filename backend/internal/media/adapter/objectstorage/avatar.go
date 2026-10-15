package objectstorage

import (
	"context"
	"io"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	platformobjects "github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// AvatarObjects adapts the existing private bucket without another storage connection.
type AvatarObjects struct {
	objects *ProjectCopyObjects
	client  *platformobjects.Client
}

// NewAvatarObjects reuses the configured media bucket client.
func NewAvatarObjects(c *platformobjects.Client) *AvatarObjects {
	return &AvatarObjects{objects: NewProjectCopyObjects(c), client: c}
}

// PutIfAbsent conditionally creates the unique avatar output.
func (s *AvatarObjects) PutIfAbsent(ctx context.Context, key string, r io.Reader, size int64, mime, hash string) error {
	return s.objects.PutIfAbsent(ctx, key, r, size, mime, hash)
}

// Remove cleans up only the uncommitted unique upload key.
func (s *AvatarObjects) Remove(ctx context.Context, key string) error {
	return s.objects.Remove(ctx, key)
}

// Stat reads actual stored image metadata before publication.
func (s *AvatarObjects) Stat(ctx context.Context, key string) (application.ObjectInfo, error) {
	i, err := s.client.Stat(ctx, key)
	return application.ObjectInfo{Size: i.Size, ContentType: i.ContentType, SHA256: i.SHA256}, err
}
