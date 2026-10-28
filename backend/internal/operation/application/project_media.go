package application

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ProjectMediaObject is the exact authorized immutable input and physical owner.
// A missing LocationID can use only an explicitly configured original reader.
type ProjectMediaObject struct {
	AssetID, ProjectID                         uuid.UUID
	LocationID                                 *uuid.UUID
	ProfileID, Bucket, Key, SHA256, Kind, Role string
	ByteSize                                   int64
}

// ProjectMediaSigner consumes proven same-project media, never client-selected keys.
type ProjectMediaSigner interface {
	SignProjectMedia(context.Context, ProjectMediaObject, time.Duration) (ProviderReference, error)
}
