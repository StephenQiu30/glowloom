package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// UploadPolicy is frozen when a request is admitted, before reading any bytes.
type UploadPolicy struct {
	MaxImageBytes    int64 `json:"max_image_bytes"`
	MaxVideoBytes    int64 `json:"max_video_bytes"`
	MaxAudioBytes    int64 `json:"max_audio_bytes"`
	MaxModelBytes    int64 `json:"max_model_bytes"`
	MaxDocumentBytes int64 `json:"max_document_bytes"`
}

// Validate permits deployment limits to narrow the existing upload contract.
func (p UploadPolicy) Validate() error {
	for _, limit := range [][2]int64{{p.MaxImageBytes, MaxUploadImageBytes}, {p.MaxVideoBytes, MaxUploadVideoBytes}, {p.MaxAudioBytes, MaxUploadAudioBytes}, {p.MaxModelBytes, MaxUploadModelBytes}, {p.MaxDocumentBytes, MaxUploadDocumentBytes}} {
		if limit[0] < 1 || limit[0] > limit[1] {
			return ErrInvalidUpload
		}
	}
	return nil
}

// MaximumBytes bounds the envelope before its file kind is known.
func (p UploadPolicy) MaximumBytes() int64 {
	return max(p.MaxImageBytes, p.MaxVideoBytes, p.MaxAudioBytes, p.MaxModelBytes, p.MaxDocumentBytes)
}
func (p UploadPolicy) limit(kind domain.Kind) int64 {
	switch kind {
	case domain.KindImage:
		return p.MaxImageBytes
	case domain.KindVideo:
		return p.MaxVideoBytes
	case domain.KindAudio:
		return p.MaxAudioBytes
	case domain.KindModel:
		return p.MaxModelBytes
	case domain.KindDocument:
		return p.MaxDocumentBytes
	}
	return 0
}

// UploadTarget is server-only admission evidence. A browser never selects a
// profile, bucket, location, revision or upload policy.
type UploadTarget struct {
	SessionID                         uuid.UUID
	ProfileID, Bucket, ConfigRevision string
	ExpiresAt                         time.Time
	Policy                            UploadPolicy
}

// UploadAdmission pins authorized idempotent requests before reading their bodies.
type UploadAdmission interface {
	PinUpload(context.Context, identityapp.Principal, domain.LibraryScope, uuid.UUID) (UploadTarget, error)
}

// RoutedUploadObjects records intent before creating verified immutable bytes.
// Removal must retain a durable cleanup debt when the provider outcome is unknown.
type RoutedUploadObjects interface {
	CreateUploadObject(context.Context, UploadTarget, string, *Downloaded) (domain.ObjectLocation, error)
	RemoveUploadObject(context.Context, domain.ObjectLocation) error
}

// NewRoutedUploadService opts the existing relay pipeline into durable routing.
// The legacy constructors remain restricted to the original single MinIO bucket
// until all key-only consumers have been migrated and activation is accepted.
func NewRoutedUploadService(repo UploadRepository, personal PersonalUploadRepository, admission UploadAdmission, prober Prober, normalizer UploadNormalizer, renderer Renderer, objects RoutedUploadObjects, now func() time.Time) *UploadService {
	return &UploadService{repo: repo, personal: personal, admission: admission, prober: prober, normalizer: normalizer, renderer: renderer, routedObjects: objects, now: now, routed: true}
}

// BeginUpload authorizes again inside the persistent admission transaction. The
// HTTP handler invokes this after its slot and before MultipartReader or Read.
func (s *UploadService) BeginUpload(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope, key uuid.UUID) (UploadTarget, error) {
	if s == nil {
		return UploadTarget{}, ErrUnavailable
	}
	if s.routed && (s.admission == nil || s.routedObjects == nil || s.prober == nil || s.normalizer == nil || s.renderer == nil || s.now == nil) {
		return UploadTarget{}, ErrStorageNotReady
	}
	if s.admission == nil {
		if s.routedObjects != nil {
			return UploadTarget{}, ErrStorageNotReady
		}
		return UploadTarget{}, nil // original single-bucket construction only
	}
	if key == uuid.Nil || scope.Validate() != nil {
		return UploadTarget{}, ErrInvalidUpload
	}
	return s.admission.PinUpload(ctx, actor, scope, key)
}

func uploadScope(r UploadRequest) domain.LibraryScope {
	if r.Personal != nil {
		return domain.LibraryScope{Kind: domain.LibraryPersonal}
	}
	project := r.ProjectID
	return domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
}
