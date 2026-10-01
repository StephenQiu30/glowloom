package application

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

// DepthCreateInput selects only an existing saved formal video node.
type DepthCreateInput struct {
	CanvasID uuid.UUID `json:"canvas_id"`
	NodeID   uuid.UUID `json:"node_id"`
	Revision int64     `json:"revision"`
}

// DepthWorkID binds durable delivery to one physical worker; execution is internal.
type DepthWorkID struct {
	JobID       uuid.UUID `json:"job_id"`
	Attempt     int       `json:"attempt"`
	ExecutionID uuid.UUID `json:"execution_id"`
	Reconcile   bool      `json:"reconcile"`
}

// DepthDelivery is an exact permanently committed command, not a free execution request.
type DepthDelivery struct {
	DepthWorkID
	EventID   uuid.UUID `json:"event_id"`
	RequestID uuid.UUID `json:"request_id"`
	ActorID   uuid.UUID `json:"actor_id"`
	OrgID     uuid.UUID `json:"org_id"`
	ProjectID uuid.UUID `json:"project_id"`
	Action    string    `json:"action"`
}

// DepthPage returns authorized durable facts for refresh recovery.
type DepthPage struct {
	Items          []domain.DepthJob `json:"items"`
	NextCursor     *string           `json:"next_cursor" extensions:"x-nullable"`
	CurrentActorID uuid.UUID         `json:"current_actor_id"`
	CurrentOrgID   uuid.UUID         `json:"current_org_id"`
}

// DepthStore owns depth commands and transaction-bound pending media publication.
type DepthStore interface {
	Create(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, DepthCreateInput) (domain.DepthJob, error)
	Get(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (domain.DepthJob, error)
	List(context.Context, identityapp.Principal, ListInput) ([]domain.DepthJob, error)
	Control(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, uuid.UUID, int64, string) (domain.DepthJob, error)
	Review(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID, ReviewInput) (domain.DepthJob, error)
	PreviewAsset(context.Context, identityapp.Principal, uuid.UUID, uuid.UUID) (domain.DepthJob, mediadomain.MediaAsset, error)
}

// DepthPreview binds inspection to exact private result bytes and job revision.
type DepthPreview struct {
	JobID     uuid.UUID             `json:"job_id"`
	Revision  int64                 `json:"revision"`
	SHA256    string                `json:"sha256"`
	URL       string                `json:"url"`
	ExpiresAt time.Time             `json:"expires_at"`
	Asset     mediaapp.AssetSummary `json:"asset"`
}

// DepthVerifier independently decodes and counts the complete silent MP4 bytes.
type DepthVerifier interface {
	Verify(context.Context, *mediaapp.Downloaded) (DepthVideoFacts, error)
}

// DepthObjects extends exact private storage with owned unpublished result removal.
type DepthObjects interface {
	PrivateObjects
	Remove(context.Context, string) error
}

// DepthDerivedMedia adds only the removal authorization needed by this consumer.
type DepthDerivedMedia interface {
	DerivedMedia
	AuthorizeRemoval(context.Context, identityapp.Principal, mediaapp.DerivedRemoval) error
}

// DepthObject freezes one server-selected result before any external write.
type DepthObject struct {
	Kind          string `json:"kind"`
	ObjectKey     string `json:"object_key"`
	SHA256        string `json:"sha256"`
	ByteSize      int64  `json:"byte_size"`
	MIMEType      string `json:"mime_type"`
	WriteStarted  bool   `json:"-"`
	DeleteStarted bool   `json:"-"`
	Status        string `json:"-"`
}

// DepthArtifact is a permanent private output manifest, never a public DTO.
type DepthArtifact struct {
	JobID       uuid.UUID               `json:"job_id"`
	Attempt     int                     `json:"attempt"`
	InputSHA256 string                  `json:"input_sha256"`
	Receipt     DepthReceipt            `json:"receipt"`
	Asset       mediadomain.MediaAsset  `json:"asset"`
	Renditions  []mediadomain.Rendition `json:"renditions"`
	Objects     []DepthObject           `json:"objects"`
}

// DepthWork carries only the currently fenced owner and its frozen source.
type DepthWork struct {
	Job          domain.DepthJob
	Actor        identityapp.Principal
	Frozen       domain.FrozenDepth
	ProcessState domain.DepthProcessState
}

// DepthWorkerStore persists exact native cessation and immutable result intents.
// External file and object IO never runs within its transaction callbacks.
type DepthWorkerStore interface {
	Claim(context.Context, DepthWorkID) (DepthWork, error)
	Phase(context.Context, DepthWorkID, string) error
	StartProcess(context.Context, DepthWorkID) error
	EndProcess(context.Context, DepthWorkID) error
	FreezeArtifact(context.Context, DepthWorkID, DepthArtifact) error
	Artifact(context.Context, DepthWorkID) (DepthArtifact, bool, error)
	Objects(context.Context, DepthWorkID) ([]DepthObject, error)
	BeginWrite(context.Context, DepthWorkID, string) error
	ConfirmObject(context.Context, DepthWorkID, string) error
	BeginRemove(context.Context, DepthWorkID, string) error
	ConfirmRemoved(context.Context, DepthWorkID, string) error
	Commit(context.Context, DepthWorkID) (domain.DepthJob, error)
	Finish(context.Context, DepthWorkID, bool, bool, string) (domain.DepthJob, error)
	Interrupt(context.Context, DepthWorkID) error
}

// ValidateDepthReceipt rejects a typed runner result without closed native evidence.
// The worker additionally hashes every file and independently probes actual bytes.
func ValidateDepthReceipt(f domain.FrozenDepth, o *DepthOutput) error {
	if f.Validate() != nil || o == nil || o.File == nil || o.File.Size < 1 || o.File.Size > 500<<20 || o.File.MIMEType != "video/mp4" || !domain.ValidDepthSHA(o.File.SHA256) {
		return ErrDepthOutputInvalid
	}
	r, p := o.Receipt, o.Probe
	if r.ProfileID != domain.DepthProfileID || r.VDACommit != domain.DepthVDACommit || r.ModelSHA256 != domain.DepthModelSHA256 || r.SourceTreeSHA256 != domain.DepthSourceTreeSHA256 || r.InputSHA256 != f.Input.SHA256 || r.Device != "mps" || r.RuntimeVersion == "" || len(r.RuntimeVersion) > 256 || !r.Native.ProcessGroupJoined || r.Native.ElapsedMS < 1 || r.Native.PeakRSSBytes < 1 || r.Native.RSSLimitBytes < 1<<20 || r.Native.RSSLimitBytes > 8<<30 || r.Native.PeakRSSBytes > r.Native.RSSLimitBytes {
		return ErrDepthOutputInvalid
	}
	for _, v := range []DepthVideoFacts{r.Source, r.Prepared, r.Output} {
		if v.Width < 1 || v.Height < 1 || v.FrameCount < 1 || math.IsNaN(v.FPS) || math.IsInf(v.FPS, 0) || v.FPS <= 0 || v.DurationMS < 1 || v.DurationMS > 15100 {
			return ErrDepthOutputInvalid
		}
	}
	if r.Source.Width != *f.Input.Width || r.Source.Height != *f.Input.Height || math.Abs(float64(r.Source.DurationMS-int64(*f.Input.DurationMS))) > 1000/r.Source.FPS+100 {
		return ErrDepthOutputInvalid
	}
	for _, v := range []DepthVideoFacts{r.Prepared, r.Output} {
		if math.Abs(float64(v.DurationMS)-float64(v.FrameCount)*1000/v.FPS) > 1000/v.FPS+25 {
			return ErrDepthOutputInvalid
		}
	}
	if r.Prepared.FrameCount > 453 || r.Prepared.FPS > 30 || r.Prepared.Width > 960 || r.Prepared.Height > 960 || r.Output.Width != 1920 || r.Output.Height != 1080 || r.Output.FrameCount != r.Prepared.FrameCount || math.Abs(r.Output.FPS-r.Prepared.FPS) > 0.00001 || math.Abs(float64(r.Output.DurationMS-r.Prepared.DurationMS)) > 1000/r.Prepared.FPS+25 || math.Abs(float64(r.Source.DurationMS-r.Prepared.DurationMS)) > 1000/r.Prepared.FPS+100 {
		return ErrDepthOutputInvalid
	}
	if p.Kind != mediadomain.KindVideo || p.Extension != "mp4" || p.Width == nil || p.Height == nil || *p.Width != 1920 || *p.Height != 1080 || p.AudioChannels != nil || p.Codec == nil || *p.Codec != "h264" || p.FPS == nil || math.IsNaN(*p.FPS) || math.IsInf(*p.FPS, 0) || math.Abs(*p.FPS-r.Output.FPS) > 0.00001 || p.DurationMS == nil || math.Abs(float64(int64(*p.DurationMS)-r.Output.DurationMS)) > 1000/r.Output.FPS+25 {
		return ErrDepthOutputInvalid
	}
	return nil
}
