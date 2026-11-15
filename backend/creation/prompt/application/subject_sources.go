package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
)

// SubjectSources exposes only fixed subject versions and their formal bindings.
// Missing means that no unified Subject identity exists; access failures must
// return an error instead of falling back to a legacy identity.
type SubjectSources interface {
	ResolveSubject(context.Context, Actor, uuid.UUID, string, string) (source domain.ReferenceSource, found bool, err error)
	ValidateBinding(context.Context, Actor, uuid.UUID, domain.SourceMedia, bool) error
}

// CanvasSubjectSources resolves exact accepted canvas-local subject input revisions.
type CanvasSubjectSources interface {
	ResolveCanvasSubject(context.Context, Actor, uuid.UUID, string, string) (domain.ReferenceSource, error)
}
