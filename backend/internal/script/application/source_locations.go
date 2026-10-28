package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

// SourceObjectLocations joins storage admission and acceptance to the script
// owner's transaction. Implementations cannot select another route on recovery.
type SourceObjectLocations interface {
	Admit(context.Context, uuid.UUID, uuid.UUID, []domain.ObjectFact) (uuid.UUID, []domain.ObjectFact, error)
	Begin(context.Context, uuid.UUID, domain.ObjectFact) error
	Confirm(context.Context, uuid.UUID, domain.ObjectFact) error
	Accept(context.Context, uuid.UUID, []domain.ObjectFact) error
	BeginRemoval(context.Context, uuid.UUID, domain.ObjectFact) error
	ConfirmRemoval(context.Context, uuid.UUID, domain.ObjectFact) error
}
