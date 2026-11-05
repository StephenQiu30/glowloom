package media_test

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type usageSource struct {
	inventory mediaapp.StorageUsageInventory
	err       error
}

func (s usageSource) InspectStorageUsage(_ context.Context, _ identityapp.Principal, _ domain.LibraryScope, consume func(mediaapp.StorageUsageInventory) error) error {
	if s.err != nil {
		return s.err
	}
	return consume(s.inventory)
}

type usageObjects struct {
	listed  []mediaapp.StorageUsageObject
	sizes   map[domain.ObjectRef]int64
	err     error
	checked []domain.ObjectRef
}

func (s *usageObjects) ListMetadata(context.Context, mediaapp.StorageUsagePrefix, int) ([]mediaapp.StorageUsageObject, error) {
	return s.listed, s.err
}

func (s *usageObjects) Stat(_ context.Context, key domain.ObjectRef) (int64, bool, error) {
	s.checked = append(s.checked, key)
	size, exists := s.sizes[key]
	return size, exists, s.err
}

func TestMediaStorageUsageCountsActualUniqueObjectsAndKeepsUnconfiguredLimitNull(t *testing.T) {
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: "producer"}
	scope := domain.LibraryScope{Kind: domain.LibraryPersonal}
	id, _ := scope.Identity(actor.OrgID, actor.ID)
	prefix := "personal/" + actor.OrgID.String() + "/" + actor.ID.String() + "/image/"
	one, two, absent := prefix+"a.png", prefix+"b.png", prefix+"removed.png"
	source := usageSource{inventory: mediaapp.StorageUsageInventory{LibraryID: id, PhysicalPrefixes: []mediaapp.StorageUsagePrefix{{ProfileID: "cos-first", Bucket: "storage-test-1250000000", Prefix: prefix}}, Locations: []domain.ObjectRef{usageRef(one), usageRef(absent)}}}
	objects := &usageObjects{listed: []mediaapp.StorageUsageObject{{Ref: usageRef(one), ByteSize: 9000}, {Ref: usageRef(two), ByteSize: 8000}}, sizes: map[domain.ObjectRef]int64{usageRef(one): 21, usageRef(two): 35}}
	reader := mediaapp.NewStorageUsageReader(source, objects, func() time.Time { return domainNow })
	result, err := reader.StorageUsage(t.Context(), actor, scope)
	if err != nil || result.UsedBytes != 56 || result.ObjectCount != 2 || result.LimitBytes != nil || result.CurrentActorID != actor.ID || result.CurrentOrgID != actor.OrgID || result.CalculatedAt != domainNow || len(objects.checked) != 3 {
		t.Fatal("listing sizes or duplicate catalog links became accounting truth", result, objects.checked, err)
	}
}

func TestMediaStorageUsageRejectsPartialForeignOverflowAndMissingAuthorities(t *testing.T) {
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: "producer"}
	scope := domain.LibraryScope{Kind: domain.LibraryPersonal}
	id, _ := scope.Identity(actor.OrgID, actor.ID)
	prefix := "personal/" + actor.OrgID.String() + "/" + actor.ID.String() + "/image/"
	for name, objects := range map[string]*usageObjects{
		"foreign key":         {listed: []mediaapp.StorageUsageObject{{Ref: usageRef("personal/foreign/file"), ByteSize: 1}}},
		"duplicate inventory": {listed: []mediaapp.StorageUsageObject{{Ref: usageRef(prefix + "a"), ByteSize: 1}, {Ref: usageRef(prefix + "a"), ByteSize: 1}}},
		"negative metadata":   {listed: []mediaapp.StorageUsageObject{{Ref: usageRef(prefix + "a"), ByteSize: -1}}},
		"negative stat":       {listed: []mediaapp.StorageUsageObject{{Ref: usageRef(prefix + "a"), ByteSize: 1}}, sizes: map[domain.ObjectRef]int64{usageRef(prefix + "a"): -1}},
		"sum overflow":        {listed: []mediaapp.StorageUsageObject{{Ref: usageRef(prefix + "a"), ByteSize: 1}, {Ref: usageRef(prefix + "b"), ByteSize: 1}}, sizes: map[domain.ObjectRef]int64{usageRef(prefix + "a"): math.MaxInt64, usageRef(prefix + "b"): 1}},
		"partial transport":   {listed: []mediaapp.StorageUsageObject{{Ref: usageRef(prefix + "a"), ByteSize: 1}}, err: errors.New("transport unavailable")},
	} {
		t.Run(name, func(t *testing.T) {
			reader := mediaapp.NewStorageUsageReader(usageSource{inventory: mediaapp.StorageUsageInventory{LibraryID: id, PhysicalPrefixes: []mediaapp.StorageUsagePrefix{{ProfileID: "cos-first", Bucket: "storage-test-1250000000", Prefix: prefix}}}}, objects, time.Now)
			result, err := reader.StorageUsage(t.Context(), actor, scope)
			if !errors.Is(err, mediaapp.ErrUnavailable) || !reflect.DeepEqual(result, mediaapp.StorageUsage{}) {
				t.Fatal("invalid inventory returned partial totals", result, err)
			}
		})
	}
	reader := mediaapp.NewStorageUsageReader(usageSource{err: identityapp.ErrForbidden}, &usageObjects{}, time.Now)
	if _, err := reader.StorageUsage(t.Context(), actor, scope); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("authorizer failure was weakened", err)
	}
	if _, err := mediaapp.NewStorageUsageReader(nil, nil, time.Now).StorageUsage(t.Context(), actor, scope); !errors.Is(err, mediaapp.ErrUnavailable) {
		t.Fatal("missing owner returned zero", err)
	}
}

func usageRef(key string) domain.ObjectRef {
	return domain.ObjectRef{ProfileID: "cos-first", Bucket: "storage-test-1250000000", Key: key}
}
