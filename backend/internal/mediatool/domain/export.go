// Package domain defines local media processing jobs independently of AI quotes.
package domain

import (
	"time"

	"github.com/google/uuid"

	canvasdomain "github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
)

// Status is a durable local export lifecycle, including explicit output review.
type Status string

// OutputKind selects one immutable local encoding contract.
type OutputKind string

// The request default is video; audio produces M4A and an actual waveform.
const (
	OutputVideo OutputKind = "video"
	OutputAudio OutputKind = "audio"
)

// Effective applies the creation request's documented video default.
func (k OutputKind) Effective() OutputKind {
	if k == "" {
		return OutputVideo
	}
	return k
}

// Valid reports whether a resolved output kind has a supported encoding.
func (k OutputKind) Valid() bool { return k == OutputVideo || k == OutputAudio }

// Export statuses never imply supplier generation or billing facts.
const (
	Queued          Status = "queued"
	Running         Status = "running"
	ReviewRequired  Status = "review_required"
	Succeeded       Status = "succeeded"
	Failed          Status = "failed"
	CancelRequested Status = "cancel_requested"
	Cancelled       Status = "cancelled"
)

// Source freezes the identity and revision of the saved timeline node.
type Source struct {
	CanvasID uuid.UUID `json:"canvas_id"`
	NodeID   uuid.UUID `json:"node_id"`
	Revision int64     `json:"revision"`
}

// ExportJob is the safe public projection; private inputs and object keys remain
// within the local worker. Progress uses integer percentage units, 0 through 100.
type ExportJob struct {
	ID          uuid.UUID  `json:"id"`
	ProjectID   uuid.UUID  `json:"project_id"`
	OutputKind  OutputKind `json:"output_kind" enums:"video,audio"`
	Source      Source     `json:"source"`
	Status      Status     `json:"status"`
	Stage       string     `json:"stage"`
	Progress    int        `json:"progress"`
	Attempt     int        `json:"attempt"`
	Revision    int64      `json:"revision"`
	AssetID     *uuid.UUID `json:"asset_id" extensions:"x-nullable"`
	SHA256      *string    `json:"sha256" extensions:"x-nullable"`
	FailureCode *string    `json:"failure_code" extensions:"x-nullable"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// FrozenSource binds a private original's verified metadata to its asset ID.
type FrozenSource struct {
	AssetID    uuid.UUID `json:"asset_id"`
	Revision   int64     `json:"revision"`
	Kind       string    `json:"kind"`
	ObjectKey  string    `json:"object_key"`
	MIMEType   string    `json:"mime_type"`
	ByteSize   int64     `json:"byte_size"`
	SHA256     string    `json:"sha256"`
	DurationMS *int32    `json:"duration_ms"`
	Width      *int32    `json:"width"`
	Height     *int32    `json:"height"`
}

// FrozenExport is the immutable rendering input stored with the local job.
type FrozenExport struct {
	OutputKind OutputKind                  `json:"output_kind"`
	Timeline   canvasdomain.TimelineConfig `json:"timeline"`
	Inputs     []FrozenSource              `json:"inputs"`
}
