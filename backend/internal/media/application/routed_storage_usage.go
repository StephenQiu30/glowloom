package application

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// StorageUsagePrefix identifies an explicitly admitted historical physical bucket.
// Inventory never derives these buckets from the current default write route.
type StorageUsagePrefix struct{ ProfileID, Bucket, Prefix string }

// PhysicalStorageUsageObjects keeps complete listing and explicit absence separate.
type PhysicalStorageUsageObjects interface {
	ListMetadata(context.Context, StorageUsagePrefix, int) ([]StorageUsageObject, error)
	Stat(context.Context, domain.ObjectRef) (int64, bool, error)
}

// NewRoutedStorageUsageReader injects exact physical inventory and metadata readers.
func NewRoutedStorageUsageReader(source StorageUsageSource, objects PhysicalStorageUsageObjects, clock func() time.Time) *StorageUsageReader {
	if clock == nil {
		clock = time.Now
	}
	return &StorageUsageReader{source: source, physicalObjects: objects, clock: clock}
}

func (s *StorageUsageReader) physicalStorageUsage(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope) (StorageUsage, error) {
	if s.source == nil {
		return StorageUsage{}, ErrUnavailable
	}
	id, err := scope.Identity(actor.OrgID, actor.ID)
	if err != nil {
		return StorageUsage{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	var result StorageUsage
	err = s.source.InspectStorageUsage(ctx, actor, scope, func(inventory StorageUsageInventory) error {
		if inventory.LibraryID != id || len(inventory.ObjectKeys) != 0 || len(inventory.Prefixes) != 0 || len(inventory.PhysicalPrefixes) < 1 || len(inventory.PhysicalPrefixes) > 80 || len(inventory.Locations) > MaxStorageUsageObjects {
			return ErrUnavailable
		}
		base := "personal/" + actor.OrgID.String() + "/" + actor.ID.String() + "/"
		if scope.Kind == domain.LibraryProject {
			base = "projects/" + scope.ProjectID.String() + "/"
		}
		refs := make(map[domain.ObjectRef]bool)
		admitted := make(map[StorageUsagePrefix]bool)
		for _, prefix := range inventory.PhysicalPrefixes {
			if admitted[prefix] || !strings.HasPrefix(prefix.Prefix, base) || !strings.HasSuffix(prefix.Prefix, "/") || (domain.ObjectRef{ProfileID: prefix.ProfileID, Bucket: prefix.Bucket, Key: strings.TrimSuffix(prefix.Prefix, "/")}).Validate() != nil {
				return ErrUnavailable
			}
			admitted[prefix] = true
		}
		for _, ref := range inventory.Locations {
			if ref.Validate() != nil {
				return ErrUnavailable
			}
			covered := false
			for prefix := range admitted {
				if ref.ProfileID == prefix.ProfileID && ref.Bucket == prefix.Bucket {
					covered = true
					break
				}
			}
			if !covered {
				return ErrUnavailable
			}
			refs[ref] = true
		}
		for _, prefix := range inventory.PhysicalPrefixes {
			listed, err := s.physicalObjects.ListMetadata(ctx, prefix, MaxStorageUsageObjects)
			if err != nil {
				return fmt.Errorf("%w: list physical usage: %w", ErrUnavailable, err)
			}
			seen := make(map[domain.ObjectRef]bool, len(listed))
			for _, object := range listed {
				ref := object.Ref
				if ref.Validate() != nil || ref.ProfileID != prefix.ProfileID || ref.Bucket != prefix.Bucket || !strings.HasPrefix(ref.Key, prefix.Prefix) || object.ByteSize < 0 || seen[ref] {
					return ErrUnavailable
				}
				seen[ref], refs[ref] = true, true
				if len(refs) > MaxStorageUsageObjects {
					return ErrUnavailable
				}
			}
		}
		ordered := make([]domain.ObjectRef, 0, len(refs))
		for ref := range refs {
			ordered = append(ordered, ref)
		}
		sort.Slice(ordered, func(i, j int) bool {
			a, b := ordered[i], ordered[j]
			if a.ProfileID != b.ProfileID {
				return a.ProfileID < b.ProfileID
			}
			if a.Bucket != b.Bucket {
				return a.Bucket < b.Bucket
			}
			return a.Key < b.Key
		})
		var total int64
		count := 0
		for _, ref := range ordered {
			if err := ctx.Err(); err != nil {
				return err
			}
			size, exists, err := s.physicalObjects.Stat(ctx, ref)
			if err != nil {
				return fmt.Errorf("%w: stat physical usage: %w", ErrUnavailable, err)
			}
			if !exists {
				continue
			}
			if size < 0 || size > math.MaxInt64-total {
				return ErrUnavailable
			}
			total += size
			count++
		}
		result = StorageUsage{CurrentActorID: actor.ID, CurrentOrgID: actor.OrgID, LibraryID: id, Scope: scope, UsedBytes: total, ObjectCount: count, CalculatedAt: s.clock().UTC()}
		return nil
	})
	if err != nil {
		return StorageUsage{}, err
	}
	return result, nil
}

// StorageUsageBucket is a declared readable historical bucket, not a write route.
type StorageUsageBucket struct {
	ProfileID, Bucket string
	// ResourceIdentity binds the configured endpoint namespace to the persisted profile.
	ResourceIdentity string
}

// StorageUsageLocationSource supplies scope-authorized intents from installed owners.
type StorageUsageLocationSource interface {
	StorageUsageLocations(context.Context, identityapp.Principal, domain.LibraryScope) ([]domain.ObjectRef, error)
}
