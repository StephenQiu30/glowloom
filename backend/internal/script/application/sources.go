package application

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

// Public use-case errors are stable predicates; underlying owner failures stay wrapped.
var (
	ErrNotFound             = errors.New("script not found")
	ErrConflict             = errors.New("script revision conflict")
	ErrIdempotencyConflict  = errors.New("script idempotency conflict")
	ErrUnavailable          = errors.New("script dependency unavailable")
	ErrNeedsReconciliation  = errors.New("script object reconciliation required")
	ErrConfirmationRequired = errors.New("script impact confirmation required")
	ErrContextUnavailable   = errors.New("script downstream context unavailable")
	ErrObjectMissing        = errors.New("script object missing")
	ErrObjectExists         = errors.New("script object exists")
	ErrObjectMismatch       = errors.New("script object integrity mismatch")
)

// ProjectAccess retains workspace locks in the caller-owned transaction.
type ProjectAccess interface {
	Authorize(context.Context, identityapp.Principal, uuid.UUID, bool) (workspaceapp.ProjectContentAccess, error)
	TouchContent(context.Context, identityapp.Principal, uuid.UUID, int64) (int64, error)
}

// SourceAssetReader uses current owning media authorization, never client paths.
type SourceAssetReader interface {
	Freeze(context.Context, identityapp.Principal, uuid.UUID, []uuid.UUID) ([]mediaapp.DocumentSource, error)
	Open(context.Context, identityapp.Principal, uuid.UUID, mediaapp.DocumentSource) (*mediaapp.Downloaded, error)
}

// SourceInput is one complete editor input; legacy HTML is normalized at the adapter.
type SourceInput struct {
	Kind         string                  `json:"source_kind"`
	Title        string                  `json:"title"`
	Status       string                  `json:"status"`
	Document     domain.RichDocument     `json:"document"`
	OriginalHTML *string                 `json:"original_html,omitempty"`
	Provenance   domain.SourceProvenance `json:"provenance"`
}

// SourceCommand is a whole atomic editor command, including the complete reorder set.
type SourceCommand struct {
	ProjectID        uuid.UUID     `json:"project_id"`
	Key              uuid.UUID     `json:"key"`
	RequestID        uuid.UUID     `json:"request_id"`
	Action           string        `json:"action"`
	ExpectedRevision int64         `json:"expected_revision"`
	BaseVersionID    *uuid.UUID    `json:"base_version_id,omitempty"`
	LineageID        *uuid.UUID    `json:"source_lineage_id,omitempty"`
	RightsConfirmed  bool          `json:"rights_confirmed"`
	Sources          []SourceInput `json:"sources,omitempty"`
	Order            []uuid.UUID   `json:"source_lineage_ids,omitempty"`
}

// SourceChange explicitly relates the old snapshot to its new stable-lineage input.
type SourceChange struct {
	LineageID   uuid.UUID  `json:"source_lineage_id"`
	OldSourceID *uuid.UUID `json:"old_source_id,omitempty"`
	NewSourceID *uuid.UUID `json:"new_source_id,omitempty"`
}

// SourceReceipt is the permanent original result, not a regenerated current head.
type SourceReceipt struct {
	ScriptRevision  int64          `json:"script_revision"`
	ProjectRevision int64          `json:"project_revision"`
	VersionID       uuid.UUID      `json:"version_id"`
	SplitSetID      uuid.UUID      `json:"split_set_id"`
	Mappings        []SourceChange `json:"source_mappings"`
	Changed         bool           `json:"changed"`
	Duplicate       bool           `json:"duplicate"`
}

// SourceBase contains immutable facts only; rich bodies remain private objects.
type SourceBase struct {
	State   domain.ProjectState
	Version *domain.ScriptVersion
	Sources []domain.SourceRecord
}

// WritePlan freezes all identifiers and byte manifests before private writes occur.
type WritePlan struct {
	Command       SourceCommand         `json:"command"`
	RequestHash   string                `json:"request_hash"`
	ActorID       uuid.UUID             `json:"actor_id"`
	OrgID         uuid.UUID             `json:"org_id"`
	NewSources    []domain.SourceRecord `json:"new_sources"`
	Version       domain.ScriptVersion  `json:"version"`
	Candidate     domain.SplitSet       `json:"candidate"`
	Mappings      []SourceChange        `json:"mappings"`
	Objects       []domain.ObjectFact   `json:"objects"`
	CreatedAt     time.Time             `json:"created_at"`
	Reuse         bool                  `json:"reuse,omitempty"`
	HeadUnchanged bool                  `json:"head_unchanged,omitempty"`
}

// PendingWrite retains an immutable plan and optional immutable first result.
type PendingWrite struct {
	Plan    WritePlan
	Receipt *SourceReceipt
}

// SourceStore owns authorized CAS and permanent plans/results in short transactions.
type SourceStore interface {
	LoadBase(context.Context, identityapp.Principal, uuid.UUID, *uuid.UUID) (SourceBase, error)
	FindCommand(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, string) (*PendingWrite, error)
	BeginWrite(context.Context, identityapp.Principal, WritePlan) (PendingWrite, error)
	FinishWrite(context.Context, identityapp.Principal, WritePlan) (SourceReceipt, error)
	SourceIOStore
}

// PrivateObjects preserves immutable put and bounded actual-read integrity proofs.
type PrivateObjects interface {
	PutIfAbsent(context.Context, string, io.Reader, int64, string, string) error
	Get(context.Context, string) (io.ReadCloser, error)
	Remove(context.Context, string) error
}

// SourceService coordinates the actual private bytes and owning persistence port.
type SourceService struct {
	store   SourceStore
	objects PrivateObjects
	now     func() time.Time
	io      *sourceIORegistry
}

// NewSourceService injects the current authorized store, private bytes and clock.
func NewSourceService(store SourceStore, objects PrivateObjects, now func() time.Time) *SourceService {
	return &SourceService{store: store, objects: objects, now: now, io: &sourceIORegistry{entries: make(map[uuid.UUID]*sourceIO)}}
}

// NewSourceServiceSharingIO keeps separate publication stores under one actual
// registry. Recovery can retain and prove an already exited call even when its
// first durable exit update fails; absence from a registry never proves exit.
func NewSourceServiceSharingIO(store SourceStore, objects PrivateObjects, now func() time.Time, owner *SourceService) *SourceService {
	if owner == nil || owner.io == nil {
		return nil
	}
	return &SourceService{store: store, objects: objects, now: now, io: owner.io}
}

func commandHash(input SourceCommand) (string, error) {
	// Request-ID is trace identity; permanent idempotency binds the exact business input.
	input.RequestID = uuid.Nil
	data, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return domain.ContentSHA(data), nil
}
