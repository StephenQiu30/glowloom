package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// MaxStorageUsageObjects bounds an exact private inventory. Exceeding it returns
// unavailable rather than a partial number that could be mistaken for a quota.
const MaxStorageUsageObjects = 50000

// MaxStorageUsagePrefixes bounds configured resource scans per library.
const MaxStorageUsagePrefixes = 80

// StorageUsage contains current physical occupancy, not a reservation or invoice.
type StorageUsage struct {
	CurrentActorID uuid.UUID           `json:"current_actor_id"`
	CurrentOrgID   uuid.UUID           `json:"current_org_id"`
	LibraryID      uuid.UUID           `json:"library_id"`
	Scope          domain.LibraryScope `json:"scope"`
	UsedBytes      int64               `json:"used_bytes"`
	ObjectCount    int                 `json:"object_count"`
	LimitBytes     *int64              `json:"limit_bytes" extensions:"x-nullable"`
	CalculatedAt   time.Time           `json:"calculated_at"`
}

// StorageUsageInventory is internal owning evidence. Its keys never reach a DTO.
type StorageUsageInventory struct {
	LibraryID        uuid.UUID
	Locations        []domain.ObjectRef
	PhysicalPrefixes []StorageUsagePrefix
}

// StorageUsageSource holds current authorization while the exact inventory is read.
type StorageUsageSource interface {
	InspectStorageUsage(context.Context, identityapp.Principal, domain.LibraryScope, func(StorageUsageInventory) error) error
}

// StorageUsageObject is private metadata used only to locate an actual object.
type StorageUsageObject struct {
	Ref      domain.ObjectRef
	ByteSize int64
}

// StorageUsageReader counts unique existing objects, including retained trash
// and unknown execution results. Listing sizes alone are never accounting facts.
type StorageUsageReader struct {
	source          StorageUsageSource
	physicalObjects PhysicalStorageUsageObjects
	clock           func() time.Time
}

// StorageUsage never publishes partial results or invents an unconfigured limit.
func (s *StorageUsageReader) StorageUsage(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope) (StorageUsage, error) {
	if s == nil || s.source == nil || s.physicalObjects == nil {
		return StorageUsage{}, ErrUnavailable
	}
	return s.physicalStorageUsage(ctx, actor, scope)
}
