package operation

import (
	"context"
	"time"

	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

type publicationSigner struct {
	routed *mediaobjects.RoutedReadObjects
	legacy *objectstorage.Client
}

// PublicationMedia reuses configured physical readers without exposing private project routes.
func PublicationMedia(routed *mediaobjects.RoutedReadObjects, legacy *objectstorage.Client) operation.PublicationMedia {
	return publicationSigner{routed: routed, legacy: legacy}
}
func (s publicationSigner) SignPublishedMedia(ctx context.Context, file operation.PublishedWorkFile) (string, time.Time, error) {
	const ttl = 5 * time.Minute
	if file.LocationID.String() != "00000000-0000-0000-0000-000000000000" {
		if s.routed == nil {
			return "", time.Time{}, media.ErrUnavailable
		}
		ref := mediadomain.ObjectRef{ProfileID: file.ProfileID, Bucket: file.Bucket, Key: file.ObjectKey}
		grant, err := s.routed.PresignGet(ctx, ref, ttl)
		return grant.URL, grant.ExpiresAt, err
	}
	if s.legacy == nil {
		return "", time.Time{}, media.ErrUnavailable
	}
	url, err := s.legacy.PresignGet(ctx, file.ObjectKey, ttl)
	return url, time.Now().Add(ttl), err
}
