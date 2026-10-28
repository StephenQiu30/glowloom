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

// GeneratedObjects writes generated originals and previews to one frozen operation.
type GeneratedObjects struct {
	registry  *platform.Registry
	locations application.WorkObjectLocations
}

// NewGeneratedObjects reuses the registry and durable work-location owner.
func NewGeneratedObjects(registry *platform.Registry, locations application.WorkObjectLocations) *GeneratedObjects {
	return &GeneratedObjects{registry: registry, locations: locations}
}

// CreateOperationObject records the write fence before any native provider SDK call.
func (s *GeneratedObjects) CreateOperationObject(ctx context.Context, intent application.StorageWorkIntent, key string, file *application.Downloaded) (domain.ObjectLocation, error) {
	if s == nil || s.registry == nil || s.locations == nil {
		return domain.ObjectLocation{}, application.ErrUnavailable
	}
	ref := domain.ObjectRef{ProfileID: intent.ProfileID, Bucket: intent.Bucket, Key: key}
	if intent.ID == uuid.Nil || intent.Owner.Validate() != nil || intent.Owner.Kind != "operation" || intent.State != "open" || strings.TrimSpace(intent.ConfigRevision) == "" || ref.Validate() != nil || !strings.HasPrefix(key, "projects/"+intent.Owner.OwnerID.String()+"/") || file == nil || file.File == nil || file.Size < 1 || file.Size > 512<<20 || len(file.SHA256) != 64 {
		return domain.ObjectLocation{}, application.ErrInvalidIngest
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
		return location, application.ErrObjectMismatch
	}
	if err := s.locations.ConfirmWorkLocation(ctx, intent.ID, location, application.ObjectInfo{Size: info.Size, ContentType: info.ContentType, SHA256: info.SHA256}); err != nil {
		return location, err
	}
	location.State = "present"
	location.ByteSize = &file.Size
	location.MIMEType = &file.MIMEType
	location.SHA256 = &file.SHA256
	return location, nil
}

// StatOperationObject verifies the persisted physical identity, never the current default.
func (s *GeneratedObjects) StatOperationObject(ctx context.Context, location domain.ObjectLocation) (application.ObjectInfo, error) {
	if s == nil || s.registry == nil || location.ID == uuid.Nil || location.Ref.Validate() != nil || (location.State != "present" && location.State != "accepted") {
		return application.ObjectInfo{}, application.ErrUnavailable
	}
	info, err := s.registry.Stat(ctx, transportRef(location.Ref))
	return application.ObjectInfo{Size: info.Size, ContentType: info.ContentType, SHA256: info.SHA256}, err
}
