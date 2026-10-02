package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// ProjectCopyScriptOwner owns complete history snapshots and SQL-only registration.
type ProjectCopyScriptOwner interface {
	ReferencedMedia(context.Context, identityapp.Principal, ProjectCopyBinding) ([]uuid.UUID, error)
	Freeze(context.Context, identityapp.Principal, ProjectCopyBinding, map[uuid.UUID]uuid.UUID, time.Time) (domain.ProjectCopyScriptSnapshot, error)
	Register(context.Context, identityapp.Principal, ProjectCopyBinding, domain.ProjectCopyScriptSnapshot) (domain.ProjectCopyScriptReceipt, error)
	FinishCleanup(context.Context, identityapp.Principal, ProjectCopyBinding, domain.ProjectCopyScriptSnapshot) error
}

// ProjectCopyScriptTransfer synchronously owns actual private-byte calls outside SQL.
type ProjectCopyScriptTransfer interface {
	Transfer(context.Context, identityapp.Principal, ProjectCopyBinding, domain.ProjectCopyScriptSnapshot) error
	Cleanup(context.Context, identityapp.Principal, ProjectCopyBinding, domain.ProjectCopyScriptSnapshot) error
}

// ProjectCopyScriptTransferFactory captures one claimed worker and cleanup phase.
type ProjectCopyScriptTransferFactory func(domain.ProjectCopyJob, uuid.UUID, bool) ProjectCopyScriptTransfer

// ProjectCopyScriptTransferError preserves uncertain object outcomes for recovery.
type ProjectCopyScriptTransferError struct {
	Code                string
	NeedsReconciliation bool
	Cause               error
}

func (e *ProjectCopyScriptTransferError) Error() string { return e.Code }
func (e *ProjectCopyScriptTransferError) Unwrap() error { return e.Cause }
