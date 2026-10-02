package application

import (
	"context"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
)

// CharacterReference is script's minimal confirmed identity/version projection.
type CharacterReference struct {
	CharacterID uuid.UUID
	VersionID   uuid.UUID
	Revision    int64
}

// CharacterReferences resolves approved current or historically confirmed pinned content.
type CharacterReferences interface {
	Reference(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, *uuid.UUID) (CharacterReference, error)
}

// BibleLookScope is script's own formal episode/scene applicability contract.
type BibleLookScope struct {
	EpisodeID uuid.UUID
	SceneKey  *uuid.UUID
}
