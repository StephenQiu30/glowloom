package objectstorage

import (
	"context"
	"io"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	platformobjects "github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
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

// RoutedTeamAvatarSigner signs only the immutable profile and bucket selected
// by the authorized team avatar reader.
type RoutedTeamAvatarSigner struct{ objects *RoutedReadObjects }

// NewRoutedTeamAvatarSigner shares the process registry used by media routes.
func NewRoutedTeamAvatarSigner(registry *platformobjects.Registry) *RoutedTeamAvatarSigner {
	return &RoutedTeamAvatarSigner{objects: NewRoutedReadObjects(registry)}
}

// PresignLocationGet signs a short-lived read for the exact accepted avatar location.
func (s *RoutedTeamAvatarSigner) PresignLocationGet(ctx context.Context, profileID, bucket, key string, ttl time.Duration) (workspaceapp.TeamAvatarGrant, error) {
	if s == nil || s.objects == nil {
		return workspaceapp.TeamAvatarGrant{}, application.ErrUnavailable
	}
	grant, err := s.objects.PresignGet(ctx, domain.ObjectRef{ProfileID: profileID, Bucket: bucket, Key: key}, ttl)
	return workspaceapp.TeamAvatarGrant{URL: grant.URL, ExpiresAt: grant.ExpiresAt}, err
}

var _ workspaceapp.RoutedTeamAvatarSigner = (*RoutedTeamAvatarSigner)(nil)
