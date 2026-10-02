package application

import (
	"context"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	canvasapp "github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// ProjectCopyInput freezes a complete source revision behind a durable request key.
type ProjectCopyInput struct {
	SourceProjectID  uuid.UUID                 `json:"source_project_id"`
	ExpectedRevision int64                     `json:"expected_revision"`
	TargetName       string                    `json:"target_name"`
	Placement        *CopyPlacementExpectation `json:"placement,omitempty"`
	IdempotencyKey   uuid.UUID                 `json:"-"`
	RequestID        string                    `json:"-"`
}

// Validate rejects unscoped, stale-shaped or incomplete public admission requests.
func (i ProjectCopyInput) Validate() error {
	request, err := uuid.Parse(i.RequestID)
	if i.SourceProjectID == uuid.Nil || i.ExpectedRevision < 1 || i.ExpectedRevision > math.MaxInt32 || i.IdempotencyKey == uuid.Nil || err != nil || request == uuid.Nil || request.String() != i.RequestID || strings.TrimSpace(i.TargetName) == "" || !utf8.ValidString(i.TargetName) || utf8.RuneCountInString(i.TargetName) > 50 || strings.ContainsRune(i.TargetName, 0) {
		return domain.ErrInvalidProjectCopy
	}
	if i.Placement != nil && i.Placement.Validate() != nil {
		return domain.ErrInvalidProjectCopy
	}
	return nil
}

// ProjectCopyMediaOwner retains media-owned snapshots, objects and complete receipts.
type ProjectCopyMediaOwner interface {
	mediaapp.ProjectCopyObjectRepository
	Freeze(context.Context, identityapp.Principal, mediaapp.ProjectCopyBinding, time.Time) (mediaapp.ProjectCopySnapshot, error)
	Register(context.Context, identityapp.Principal, mediaapp.ProjectCopyBinding, mediaapp.ProjectCopySnapshot) (mediaapp.ProjectCopyReceipt, error)
	FinishCleanup(context.Context, identityapp.Principal, mediaapp.ProjectCopyBinding, mediaapp.ProjectCopySnapshot) error
}

// ProjectCopyHistoricalMediaOwner freezes explicit retained document references.
// Ordinary media reads and legacy admissions retain their original live scope.
type ProjectCopyHistoricalMediaOwner interface {
	FreezeWithReferences(context.Context, identityapp.Principal, mediaapp.ProjectCopyBinding, time.Time, []uuid.UUID) (mediaapp.ProjectCopySnapshot, error)
}

// ProjectCopyCanvasOwner owns complete graph snapshots and private graph copies.
type ProjectCopyCanvasOwner interface {
	Freeze(context.Context, identityapp.Principal, canvasapp.ProjectCopyBinding, map[uuid.UUID]uuid.UUID) (canvasapp.ProjectCopySnapshot, error)
	Copy(context.Context, identityapp.Principal, canvasapp.ProjectCopyBinding, canvasapp.ProjectCopySnapshot) (canvasapp.ProjectCopyReceipt, error)
	Cleanup(context.Context, identityapp.Principal, canvasapp.ProjectCopyBinding, canvasapp.ProjectCopySnapshot) error
}

// ProjectCopyBudgetOwner creates a fresh zero budget without inspecting the source ledger.
type ProjectCopyBudgetOwner interface {
	CreateCopyTargetBudget(context.Context, identityapp.Principal, uuid.UUID) error
	VerifyCopyTargetBudget(context.Context, identityapp.Principal, uuid.UUID) error
}

// ProjectCopyCoverOwner proves source and private target images through media ownership.
type ProjectCopyCoverOwner interface {
	Freeze(context.Context, identityapp.Principal, uuid.UUID, *uuid.UUID) error
	Verify(context.Context, identityapp.Principal, mediaapp.ProjectCopyBinding, *uuid.UUID, *uuid.UUID) error
}

// ProjectCopyOwners are concrete owning-module application boundaries on one transaction.
type ProjectCopyOwners struct {
	Media  ProjectCopyMediaOwner
	Canvas ProjectCopyCanvasOwner
	Budget ProjectCopyBudgetOwner
	Cover  ProjectCopyCoverOwner
}

// ProjectCopyAdmissionStore owns authorization, atomic freezing and safe job reads.
type ProjectCopyAdmissionStore interface {
	Create(context.Context, identityapp.Principal, ProjectCopyInput, time.Time) (domain.ProjectCopyJob, error)
	Find(context.Context, identityapp.Principal, uuid.UUID) (domain.ProjectCopyJob, error)
	Change(context.Context, identityapp.Principal, uuid.UUID, string, int64, uuid.UUID, string) (domain.ProjectCopyJob, error)
}

// ProjectCopyService validates complete-copy public requests before persistence.
type ProjectCopyService struct {
	store ProjectCopyAdmissionStore
	now   func() time.Time
}

// NewProjectCopyService injects admission persistence and its clock.
func NewProjectCopyService(store ProjectCopyAdmissionStore, now func() time.Time) *ProjectCopyService {
	return &ProjectCopyService{store: store, now: now}
}

// Create admits a frozen whole-content copy; a successful response is not publication.
func (s *ProjectCopyService) Create(ctx context.Context, actor identityapp.Principal, input ProjectCopyInput) (domain.ProjectCopyJob, error) {
	if !canManageModelDefaults(actor) {
		return domain.ProjectCopyJob{}, identityapp.ErrForbidden
	}
	input.TargetName = strings.TrimSpace(input.TargetName)
	if err := input.Validate(); err != nil {
		return domain.ProjectCopyJob{}, err
	}
	if s == nil || s.store == nil || s.now == nil {
		return domain.ProjectCopyJob{}, ErrProjectDependencyUnavailable
	}
	return s.store.Create(ctx, actor, input, s.now().UTC())
}

// Get returns an authorized copy job, never private snapshot contents or object keys.
func (s *ProjectCopyService) Get(ctx context.Context, actor identityapp.Principal, id uuid.UUID) (domain.ProjectCopyJob, error) {
	if !canManageModelDefaults(actor) {
		return domain.ProjectCopyJob{}, identityapp.ErrForbidden
	}
	if s == nil || s.store == nil || id == uuid.Nil {
		return domain.ProjectCopyJob{}, ErrProjectDependencyUnavailable
	}
	return s.store.Find(ctx, actor, id)
}

// Change records cancellation or known-outcome retry under a durable CAS request.
func (s *ProjectCopyService) Change(ctx context.Context, actor identityapp.Principal, id uuid.UUID, action string, expected int64, key uuid.UUID, requestID string) (domain.ProjectCopyJob, error) {
	if !canManageModelDefaults(actor) {
		return domain.ProjectCopyJob{}, identityapp.ErrForbidden
	}
	if id == uuid.Nil || key == uuid.Nil || expected < 1 || expected >= math.MaxInt32 || (action != "cancel" && action != "retry" && action != "reconcile") {
		return domain.ProjectCopyJob{}, domain.ErrInvalidProjectCopy
	}
	if s == nil || s.store == nil {
		return domain.ProjectCopyJob{}, ErrProjectDependencyUnavailable
	}
	request, err := uuid.Parse(requestID)
	if err != nil || request == uuid.Nil || request.String() != requestID {
		return domain.ProjectCopyJob{}, domain.ErrInvalidProjectCopy
	}
	return s.store.Change(ctx, actor, id, action, expected, key, requestID)
}
