package objectstorage

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	platform "github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

// PersonalGeneratedAudioObjects retains the same physical route and durable
// write fencing used by provider staging. The voice owner handles cancellation.
type PersonalGeneratedAudioObjects struct {
	registry  *platform.Registry
	locations application.WorkObjectLocations
}

// NewPersonalGeneratedAudioObjects injects immutable profiles and the work ledger.
func NewPersonalGeneratedAudioObjects(registry *platform.Registry, locations application.WorkObjectLocations) *PersonalGeneratedAudioObjects {
	return &PersonalGeneratedAudioObjects{registry: registry, locations: locations}
}

// CreateGeneratedAudioObject reserves before network I/O and verifies the exact
// original or waveform. Existing matching objects are reusable after a crash;
// unknown writes are never removed or interpreted as a completed provider call.
func (s *PersonalGeneratedAudioObjects) CreateGeneratedAudioObject(ctx context.Context, intent application.StorageWorkIntent, key string, file *application.Downloaded) (domain.ObjectLocation, error) {
	if s == nil || s.registry == nil || s.locations == nil {
		return domain.ObjectLocation{}, application.ErrUnavailable
	}
	ref := domain.ObjectRef{ProfileID: intent.ProfileID, Bucket: intent.Bucket, Key: key}
	if intent.ID == uuid.Nil || intent.Owner.Validate() != nil || intent.Owner.Kind != "subject-voice" ||
		intent.State != "open" || strings.TrimSpace(intent.ConfigRevision) == "" || ref.Validate() != nil ||
		!strings.HasPrefix(key, "personal/") || !strings.Contains(key, "/audio/") ||
		file == nil || file.File == nil || file.Size < 1 || file.Size > application.MaxUploadAudioBytes ||
		len(file.SHA256) != 64 || (file.MIMEType != "audio/mpeg" && file.MIMEType != "image/png") {
		return domain.ObjectLocation{}, application.ErrInvalidGeneratedAudio
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	location, err := s.locations.ReserveWorkLocation(ctx, intent.ID, key)
	if err != nil {
		return domain.ObjectLocation{}, err
	}
	if location.ID == uuid.Nil || location.Ref != ref || (location.State != "reserved" && location.State != "present" && location.State != "unknown") {
		return location, application.ErrStorageIdentityConflict
	}
	if err := s.locations.BeginWorkWrite(ctx, intent.ID, location); err != nil {
		return location, err
	}
	if _, err := file.File.Seek(0, io.SeekStart); err != nil {
		return location, err
	}
	// SDK transports may close an io.ReadCloser request body. A bounded section
	// exposes seeking for retries without transferring ownership of the file.
	body := io.NewSectionReader(file.File, 0, file.Size)
	err = s.registry.PutIfAbsent(ctx, transportRef(ref), body, file.Size, file.MIMEType, file.SHA256)
	if err != nil && !errors.Is(err, platform.ErrObjectExists) {
		location.State = "unknown"
		return location, err
	}
	info, err := s.registry.Stat(ctx, transportRef(ref))
	if err != nil {
		location.State = "unknown"
		return location, err
	}
	if info.Size != file.Size || info.ContentType != file.MIMEType || info.SHA256 != file.SHA256 {
		location.State = "unknown"
		return location, application.ErrObjectMismatch
	}
	if err := s.locations.ConfirmWorkLocation(ctx, intent.ID, location, application.ObjectInfo{Size: info.Size, ContentType: info.ContentType, SHA256: info.SHA256}); err != nil {
		location.State = "unknown"
		return location, err
	}
	location.State = "present"
	location.ByteSize, location.MIMEType, location.SHA256 = &info.Size, &info.ContentType, &info.SHA256
	return location, nil
}
