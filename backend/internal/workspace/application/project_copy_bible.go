package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// ProjectCopyBibleOwner owns complete immutable content and SQL-only publication.
type ProjectCopyBibleOwner interface {
	ReferencedMediaFacts(context.Context, identityapp.Principal, ProjectCopyBinding) ([]mediaapp.ReferenceFact, error)
	Freeze(context.Context, identityapp.Principal, ProjectCopyBinding, map[uuid.UUID]uuid.UUID, time.Time) (domain.ProjectCopyBibleSnapshot, error)
	Register(context.Context, identityapp.Principal, ProjectCopyBinding, domain.ProjectCopyBibleSnapshot) (domain.ProjectCopyBibleReceipt, error)
	Verify(context.Context, identityapp.Principal, ProjectCopyBinding, domain.ProjectCopyBibleSnapshot) (domain.ProjectCopyBibleReceipt, error)
	FinishCleanup(context.Context, identityapp.Principal, ProjectCopyBinding, domain.ProjectCopyBibleSnapshot) error
}

// ProjectCopyReferenceMediaOwner accepts only exact owning historical media facts.
type ProjectCopyReferenceMediaOwner interface {
	FreezeWithReferenceFacts(context.Context, identityapp.Principal, mediaapp.ProjectCopyBinding, time.Time, []uuid.UUID, []mediaapp.ReferenceFact) (mediaapp.ProjectCopySnapshot, error)
}

// ProjectCopyBibleTransfer verifies actual media outside the SQL transaction.
type ProjectCopyBibleTransfer interface {
	Transfer(context.Context, identityapp.Principal, ProjectCopyBinding, domain.ProjectCopyBibleSnapshot) error
	Cleanup(context.Context, identityapp.Principal, ProjectCopyBinding, domain.ProjectCopyBibleSnapshot) error
}

// ProjectCopyBibleTransferFactory binds verification to the exact stage and worker.
type ProjectCopyBibleTransferFactory func(domain.ProjectCopyJob, uuid.UUID, bool) ProjectCopyBibleTransfer

// ProjectCopyBibleExecutionStore atomically advances the optional complete history stage.
type ProjectCopyBibleExecutionStore interface {
	CompleteBible(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (domain.ProjectCopyJob, error)
}

// ProjectCopyBibleTransferError retains unknown foreign byte results for reconciliation.
type ProjectCopyBibleTransferError struct {
	Code                string
	NeedsReconciliation bool
	Cause               error
}

func (e *ProjectCopyBibleTransferError) Error() string { return e.Code }
func (e *ProjectCopyBibleTransferError) Unwrap() error { return e.Cause }
