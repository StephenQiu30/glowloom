package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

// ImportCommand freezes the original order, observed draft and rights acceptance.
type ImportCommand struct {
	ProjectID        uuid.UUID   `json:"project_id"`
	Key              uuid.UUID   `json:"key"`
	RequestID        uuid.UUID   `json:"request_id"`
	ExpectedRevision int64       `json:"expected_revision"`
	BaseVersionID    *uuid.UUID  `json:"base_version_id,omitempty"`
	AssetIDs         []uuid.UUID `json:"asset_ids"`
	RightsConfirmed  bool        `json:"rights_confirmed"`
}

// ImportFile exposes a frozen original and its latest actual extraction outcome.
type ImportFile struct {
	Position    int                 `json:"position"`
	AssetID     uuid.UUID           `json:"asset_id"`
	FileName    string              `json:"file_name"`
	Status      string              `json:"status"`
	FailureCode string              `json:"failure_code,omitempty"`
	Warnings    []ExtractionWarning `json:"warnings"`
	SourceID    *uuid.UUID          `json:"source_id,omitempty"`
	LineageID   *uuid.UUID          `json:"source_lineage_id,omitempty"`
	Attempt     int                 `json:"attempt"`
}

// ImportJob is a safe refreshable state; an accepted cancel is not a stopped call.
type ImportJob struct {
	ID                      uuid.UUID    `json:"id"`
	ProjectID               uuid.UUID    `json:"project_id"`
	Revision                int64        `json:"revision"`
	Attempt                 int          `json:"attempt"`
	Status                  string       `json:"status"`
	Stage                   string       `json:"stage"`
	ExpectedScriptRevision  int64        `json:"expected_script_revision"`
	LatestScriptRevision    int64        `json:"latest_script_revision"`
	LatestVersionID         *uuid.UUID   `json:"latest_version_id,omitempty"`
	CancellationRequested   bool         `json:"cancellation_requested"`
	ReconciliationRequested bool         `json:"reconciliation_requested"`
	NeedsReconciliation     bool         `json:"needs_reconciliation"`
	ActiveIO                bool         `json:"active_io"`
	Retryable               bool         `json:"retryable"`
	CanControl              bool         `json:"can_control"`
	FailureCode             string       `json:"failure_code,omitempty"`
	Files                   []ImportFile `json:"files"`
	CreatedAt               time.Time    `json:"created_at"`
	UpdatedAt               time.Time    `json:"updated_at"`
}

// ImportPage includes only the authenticated caller's scope for exact-key recovery.
type ImportPage struct {
	CurrentActorID uuid.UUID   `json:"current_actor_id"`
	CurrentOrgID   uuid.UUID   `json:"current_org_id"`
	Items          []ImportJob `json:"items"`
	NextAfter      *int64      `json:"next_after,omitempty"`
}

// ImportControl permanently binds one current controller and one observed revision.
type ImportControl struct {
	ProjectID, JobID, Key, RequestID uuid.UUID
	ExpectedRevision                 int64
	Action                           string
}

// ImportWork identifies one immutable extraction/publication attempt.
type ImportWork struct {
	JobID   uuid.UUID `json:"job_id"`
	Attempt int       `json:"attempt"`
}

// ImportDelivery is verified against the permanent command and actual outbox.
type ImportDelivery struct {
	ImportWork
	EventID   uuid.UUID `json:"event_id"`
	RequestID uuid.UUID `json:"request_id"`
	ActorID   uuid.UUID `json:"actor_id"`
	OrgID     uuid.UUID `json:"org_id"`
	ProjectID uuid.UUID `json:"project_id"`
	Action    string    `json:"action"`
}

// FrozenImportFile retains the exact owning media facts, never a client path.
type FrozenImportFile struct {
	Position int                     `json:"position"`
	SourceID uuid.UUID               `json:"source_id"`
	Source   mediaapp.DocumentSource `json:"source"`
}

// ImportFileResult is immutable per attempt, excluding private editable bodies.
type ImportFileResult struct {
	Position    int                 `json:"position"`
	Status      string              `json:"status"`
	FailureCode string              `json:"failure_code,omitempty"`
	RichSHA256  string              `json:"rich_sha256,omitempty"`
	ContentHash string              `json:"content_hash,omitempty"`
	CharCount   int                 `json:"char_count"`
	Encoding    string              `json:"encoding,omitempty"`
	Mapping     []SourceMapping     `json:"mapping,omitempty"`
	Warnings    []ExtractionWarning `json:"warnings"`
}

// ImportRecord carries private durable facts only within this owning module.
type ImportRecord struct {
	Job            ImportJob
	Actor          identityapp.Principal
	Files          []FrozenImportFile
	Results        []ImportFileResult
	BaseVersionID  *uuid.UUID
	PublicationKey uuid.UUID
	OwnerID        *uuid.UUID
	IOState        string
}

// ImportStore keeps command admission and read/control contracts in owner transactions.
type ImportStore interface {
	CreateImport(context.Context, identityapp.Principal, ImportCommand, time.Time) (ImportJob, error)
	ListImports(context.Context, identityapp.Principal, uuid.UUID, int64, int) (ImportPage, error)
	Import(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (ImportJob, error)
	ControlImport(context.Context, identityapp.Principal, ImportControl, time.Time) (ImportJob, error)
}

// ImportService never parses originals in a public request or launches detached work.
type ImportService struct {
	store ImportStore
	now   func() time.Time
}

// NewImportService injects permanent admission/query/control and a clock.
func NewImportService(store ImportStore, now func() time.Time) *ImportService {
	return &ImportService{store: store, now: now}
}

// Create admits an exact ordered batch; no source version is published at acceptance.
func (s *ImportService) Create(ctx context.Context, actor identityapp.Principal, in ImportCommand) (ImportJob, error) {
	if s == nil || s.store == nil || s.now == nil {
		return ImportJob{}, ErrUnavailable
	}
	if in.ProjectID == uuid.Nil || in.Key == uuid.Nil || in.RequestID == uuid.Nil || in.ExpectedRevision < 0 || in.BaseVersionID != nil && *in.BaseVersionID == uuid.Nil {
		return ImportJob{}, domain.ErrInvalidSource
	}
	if err := domain.ValidateImportFiles(in.AssetIDs, in.RightsConfirmed); err != nil {
		return ImportJob{}, err
	}
	return s.store.CreateImport(ctx, actor, in, s.now().UTC().Truncate(time.Microsecond))
}

// List makes unknown acceptance recoverable after an authorized refresh.
func (s *ImportService) List(ctx context.Context, actor identityapp.Principal, project uuid.UUID, after int64, limit int) (ImportPage, error) {
	if s == nil || s.store == nil {
		return ImportPage{}, ErrUnavailable
	}
	if project == uuid.Nil || after < 0 || after > 100000 || limit < 1 || limit > 100 {
		return ImportPage{}, domain.ErrInvalidSource
	}
	return s.store.ListImports(ctx, actor, project, after, limit)
}

// Get returns the current owning facts after current project authorization.
func (s *ImportService) Get(ctx context.Context, actor identityapp.Principal, project, id uuid.UUID) (ImportJob, error) {
	if s == nil || s.store == nil {
		return ImportJob{}, ErrUnavailable
	}
	if project == uuid.Nil || id == uuid.Nil {
		return ImportJob{}, domain.ErrInvalidSource
	}
	return s.store.Import(ctx, actor, project, id)
}

// Control records explicit intent; only a proven stopped consumer may cancel or retry.
func (s *ImportService) Control(ctx context.Context, actor identityapp.Principal, in ImportControl) (ImportJob, error) {
	if s == nil || s.store == nil || s.now == nil {
		return ImportJob{}, ErrUnavailable
	}
	if in.ProjectID == uuid.Nil || in.JobID == uuid.Nil || in.Key == uuid.Nil || in.RequestID == uuid.Nil || in.ExpectedRevision < 1 || in.Action != "cancel" && in.Action != "retry" && in.Action != "reconcile" {
		return ImportJob{}, domain.ErrInvalidSource
	}
	return s.store.ControlImport(ctx, actor, in, s.now().UTC().Truncate(time.Microsecond))
}
