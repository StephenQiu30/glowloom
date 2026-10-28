package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

// SubjectMediaFact preserves an owning binding proof independently of input role.
type SubjectMediaFact struct {
	BindingID     uuid.UUID
	BindingSHA256 string
	AssetID       uuid.UUID
	AssetRevision int64
	Kind          string
	SHA256        string
	ByteSize      int64
	DurationMS    *int64
}

// SubjectMediaAccess is transaction-bound by the composition root. Ordinary
// media remains project-owned; this port grants only a formally adopted binding.
type SubjectMediaAccess interface {
	ResolveQuote(context.Context, identity.Principal, uuid.UUID, uuid.UUID) (SubjectMediaFact, error)
	VerifyInput(context.Context, identity.Principal, uuid.UUID, uuid.UUID, domain.OperationInput) error
	SignedInputURL(context.Context, uuid.UUID, uuid.UUID, domain.OperationInput, time.Duration) (ProviderReference, error)
}
