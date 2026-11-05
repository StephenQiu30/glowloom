package operation

import (
	"context"
	"time"

	"github.com/google/uuid"

	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

type publicationSigner struct {
	routed *mediaobjects.RoutedReadObjects
}

// PublicationMedia reuses configured physical readers without exposing private project routes.
func PublicationMedia(routed *mediaobjects.RoutedReadObjects) operation.PublicationMedia {
	return publicationSigner{routed: routed}
}
func (s publicationSigner) SignPublishedMedia(ctx context.Context, file operation.PublishedWorkFile) (string, time.Time, error) {
	const ttl = 5 * time.Minute
	if s.routed == nil || file.LocationID == uuid.Nil {
		return "", time.Time{}, media.ErrUnavailable
	}
	ref := mediadomain.ObjectRef{ProfileID: file.ProfileID, Bucket: file.Bucket, Key: file.ObjectKey}
	grant, err := s.routed.PresignGet(ctx, ref, ttl)
	return grant.URL, grant.ExpiresAt, err
}
