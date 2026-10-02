package application

import (
	"context"
	"fmt"
	"math"
	"path"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

// MaxStorageUsageObjects bounds an exact private inventory. Exceeding it returns
// unavailable rather than a partial number that could be mistaken for a quota.
const MaxStorageUsageObjects = 50000

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
	LibraryID  uuid.UUID
	Prefixes   []string
	ObjectKeys []string
}

// StorageUsageSource holds current authorization while the exact inventory is read.
type StorageUsageSource interface {
	InspectStorageUsage(context.Context, identityapp.Principal, domain.LibraryScope, func(StorageUsageInventory) error) error
}

// StorageUsageIntentSource supplies keys from other installed owning modules.
// Implementations authorize their own rows using the caller's transaction.
type StorageUsageIntentSource interface {
	StorageUsageKeys(context.Context, identityapp.Principal, domain.LibraryScope) ([]string, error)
}

// StorageUsageObject is private metadata used only to locate an actual object.
type StorageUsageObject struct {
	Key      string
	ByteSize int64
}

// StorageUsageObjects keeps absence distinct from a failed or partial query.
type StorageUsageObjects interface {
	ListMetadata(context.Context, string, int) ([]StorageUsageObject, error)
	Stat(context.Context, string) (size int64, exists bool, err error)
}

// StorageUsageReader counts unique existing objects, including retained trash
// and unknown execution results. Listing sizes alone are never accounting facts.
type StorageUsageReader struct {
	source  StorageUsageSource
	objects StorageUsageObjects
	clock   func() time.Time
}

// NewStorageUsageReader explicitly injects the owner and private object reader.
func NewStorageUsageReader(source StorageUsageSource, objects StorageUsageObjects, clock func() time.Time) *StorageUsageReader {
	if clock == nil {
		clock = time.Now
	}
	return &StorageUsageReader{source: source, objects: objects, clock: clock}
}

// StorageUsage never publishes partial results or invents an unconfigured limit.
func (s *StorageUsageReader) StorageUsage(ctx context.Context, actor identityapp.Principal, scope domain.LibraryScope) (StorageUsage, error) {
	if s == nil || s.source == nil || s.objects == nil {
		return StorageUsage{}, ErrUnavailable
	}
	id, err := scope.Identity(actor.OrgID, actor.ID)
	if err != nil {
		return StorageUsage{}, err
	}
	// This display query cannot hold current authority locks indefinitely on an
	// unhealthy object service. A deadline produces an error, never a subtotal.
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	var result StorageUsage
	err = s.source.InspectStorageUsage(ctx, actor, scope, func(inventory StorageUsageInventory) error {
		if inventory.LibraryID != id || len(inventory.Prefixes) == 0 || len(inventory.Prefixes) > 16 || len(inventory.ObjectKeys) > MaxStorageUsageObjects {
			return ErrUnavailable
		}
		keys := make(map[string]bool)
		for _, key := range inventory.ObjectKeys {
			if !storageUsageKey(key) {
				return ErrUnavailable
			}
			keys[key] = true
		}
		base := "personal/" + actor.OrgID.String() + "/" + actor.ID.String() + "/"
		if scope.Kind == domain.LibraryProject {
			base = "projects/" + scope.ProjectID.String() + "/"
		}
		prefixes := make(map[string]bool)
		for _, prefix := range inventory.Prefixes {
			if !strings.HasPrefix(prefix, base) || !strings.HasSuffix(prefix, "/") || !storageUsageKey(strings.TrimSuffix(prefix, "/")) || prefixes[prefix] {
				return ErrUnavailable
			}
			prefixes[prefix] = true
			listed, err := s.objects.ListMetadata(ctx, prefix, MaxStorageUsageObjects)
			if err != nil {
				return fmt.Errorf("%w: list private media usage: %w", ErrUnavailable, err)
			}
			seen := make(map[string]bool)
			for _, object := range listed {
				if !storageUsageKey(object.Key) || !strings.HasPrefix(object.Key, prefix) || object.ByteSize < 0 || seen[object.Key] {
					return ErrUnavailable
				}
				seen[object.Key], keys[object.Key] = true, true
				if len(keys) > MaxStorageUsageObjects {
					return ErrUnavailable
				}
			}
		}
		ordered := make([]string, 0, len(keys))
		for key := range keys {
			ordered = append(ordered, key)
		}
		sort.Strings(ordered)
		var total int64
		count := 0
		for _, key := range ordered {
			if err := ctx.Err(); err != nil {
				return err
			}
			size, exists, err := s.objects.Stat(ctx, key)
			if err != nil {
				return fmt.Errorf("%w: inspect private media usage: %w", ErrUnavailable, err)
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

func storageUsageKey(key string) bool {
	return len(key) > 0 && len(key) <= 4096 && utf8.ValidString(key) && !strings.ContainsAny(key, "\\\x00\r\n") && !strings.HasPrefix(key, "/") && path.Clean(key) == key && !strings.Contains(key, "../") && key != ".." && key != "."
}
