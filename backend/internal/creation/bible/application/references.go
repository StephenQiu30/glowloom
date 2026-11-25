package application

import (
	"context"

	"github.com/google/uuid"
)

// CharacterReference freezes one confirmed stable character and immutable version.
type CharacterReference struct {
	CharacterID uuid.UUID
	VersionID   uuid.UUID
	Revision    int64
}

// CharacterReferenceReader is the own read boundary for explicit outer-layer conversion.
type CharacterReferenceReader interface {
	Reference(context.Context, Actor, uuid.UUID, uuid.UUID, *uuid.UUID) (CharacterReference, error)
}
