package application

import (
	"context"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// CharacterReference freezes one confirmed stable character and immutable version.
type CharacterReference struct {
	CharacterID uuid.UUID
	VersionID   uuid.UUID
	Revision    int64
}

// CharacterReferenceReader is the own read boundary for explicit outer-layer conversion.
type CharacterReferenceReader interface {
	Reference(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, *uuid.UUID) (CharacterReference, error)
}
