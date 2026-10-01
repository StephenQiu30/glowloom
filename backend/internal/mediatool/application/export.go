// Package application owns authorized local export commands and worker ports.
package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	canvasdomain "github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

var (
	// ErrInvalidExport rejects malformed or nonrenderable timeline inputs.
	ErrInvalidExport = errors.New("invalid media export")
	// ErrNotFound hides jobs outside the current project and actor scope.
	ErrNotFound = errors.New("media export not found")
	// ErrConflict rejects stale revisions and changed idempotent requests.
	ErrConflict = errors.New("media export conflict")
	// ErrUnavailable reports missing local rendering dependencies.
	ErrUnavailable = errors.New("media export unavailable")
	// ErrCancelled rejects publication after durable cancellation wins the lock.
	ErrCancelled = errors.New("media export cancelled")
	// ErrWorkerBusy prevents overlapping attempts from inventing cessation evidence.
	ErrWorkerBusy = errors.New("media export worker active")
	// ErrNoAudio rejects extraction when no visible, audible source stream exists.
	ErrNoAudio = fmt.Errorf("%w: no audible source streams", ErrInvalidExport)
)

// TimelineReader freezes only a saved, authorized timeline under the enclosing
// command transaction. It resolves source node references into formal asset IDs.
type TimelineReader interface {
	FreezeTimeline(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, uuid.UUID, int64) (canvasdomain.TimelineConfig, error)
}

// DerivedMedia owns media metadata and review within its own context; the export
// repository uses this transaction-bound port rather than touching media tables.
type DerivedMedia interface {
	Asset(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (mediadomain.MediaAsset, error)
	Renditions(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) ([]mediadomain.Rendition, error)
	AuthorizeProject(context.Context, identityapp.Principal, uuid.UUID, bool) error
	Sources(context.Context, identityapp.Principal, uuid.UUID, []uuid.UUID) ([]mediadomain.MediaAsset, error)
	Store(context.Context, identityapp.Principal, mediadomain.MediaAsset, []mediadomain.Rendition) error
	Review(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, string, time.Time) error
	Reject(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, string, time.Time) error
}

// ExportPreview exposes one actual private result for the explicit owner review.
type ExportPreview struct {
	JobID     uuid.UUID              `json:"job_id"`
	Revision  int64                  `json:"revision"`
	SHA256    string                 `json:"sha256"`
	URL       string                 `json:"url"`
	ExpiresAt time.Time              `json:"expires_at"`
	Asset     mediaapp.AssetSummary  `json:"asset"`
	Waveform  *ExportWaveformPreview `json:"waveform,omitempty"`
}

// ExportWaveformPreview signs the actual audio rendition within one job review.
type ExportWaveformPreview struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
	Width     int32     `json:"width"`
	Height    int32     `json:"height"`
}

// CreateInput accepts saved source identity only, never a URL or local path.
type CreateInput struct {
	OutputKind domain.OutputKind `json:"output_kind,omitempty" enums:"video,audio" default:"video"`
	CanvasID   uuid.UUID         `json:"canvas_id"`
	NodeID     uuid.UUID         `json:"node_id"`
	Revision   int64             `json:"revision"`
}

// ReviewInput confirms the exact produced file at the current job revision.
type ReviewInput struct {
	ProjectID            uuid.UUID `json:"project_id"`
	Revision             int64     `json:"revision"`
	SHA256               string    `json:"sha256"`
	LocalReviewConfirmed bool      `json:"local_review_confirmed"`
}

// ControlInput targets a currently visible local job at its durable revision.
type ControlInput struct {
	ProjectID uuid.UUID `json:"project_id"`
	Revision  int64     `json:"revision"`
}

// Renderer produces a real, bounded MP4 or M4A from worker-owned local files.
type Renderer interface {
	Render(context.Context, domain.FrozenExport, map[uuid.UUID]string, func(int, string) error) (*mediaapp.Downloaded, error)
}

// WorkID fences a single durable attempt across retries and delayed activities.
type WorkID struct {
	JobID       uuid.UUID `json:"job_id"`
	Attempt     int       `json:"attempt"`
	ExecutionID uuid.UUID `json:"-"`
}

// Work contains private inputs and the initiating workspace identity.
type Work struct {
	Job    domain.ExportJob
	Actor  identityapp.Principal
	Frozen domain.FrozenExport
}

// Delivery is a committed outbox command, verified before Temporal delivery.
type Delivery struct {
	WorkID
	EventID   uuid.UUID `json:"event_id"`
	RequestID uuid.UUID `json:"request_id"`
	ActorID   uuid.UUID `json:"actor_id"`
	OrgID     uuid.UUID `json:"org_id"`
	ProjectID uuid.UUID `json:"project_id"`
	Action    string    `json:"action"`
}

// ListInput scopes restored job facts to a project and optional saved source.
type ListInput struct {
	ProjectID, CanvasID, NodeID, After uuid.UUID
	Limit                              int
}

// ExportPage contains a bounded page with a project-bound cursor.
type ExportPage struct {
	Items      []domain.ExportJob `json:"items"`
	NextCursor *string            `json:"next_cursor" extensions:"x-nullable"`
}

// ExportStore owns authorized commands and durable job facts.
type ExportStore interface {
	Create(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, CreateInput) (domain.ExportJob, error)
	Get(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (domain.ExportJob, error)
	List(context.Context, identityapp.Principal, ListInput) ([]domain.ExportJob, error)
	Control(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, uuid.UUID, int64, string) (domain.ExportJob, error)
	Review(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, ReviewInput) (domain.ExportJob, error)
	PreviewAsset(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (domain.ExportJob, mediadomain.MediaAsset, error)
	PreviewWaveform(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, int64, string) (mediadomain.Rendition, error)
	Frozen(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (domain.FrozenExport, error)
}
