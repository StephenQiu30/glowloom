package application

import (
	"context"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

// SavedMediaSourceReader resolves the exact persisted media node for admission.
type SavedMediaSourceReader interface {
	FreezeMediaSource(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, uuid.UUID, int64) (uuid.UUID, error)
}
