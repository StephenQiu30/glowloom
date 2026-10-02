package media_test

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
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
	sizes   map[string]int64
	err     error
	checked []string
}

func (s *usageObjects) ListMetadata(context.Context, string, int) ([]mediaapp.StorageUsageObject, error) {
	return s.listed, s.err
}

func (s *usageObjects) Stat(_ context.Context, key string) (int64, bool, error) {
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
	source := usageSource{inventory: mediaapp.StorageUsageInventory{LibraryID: id, Prefixes: []string{prefix}, ObjectKeys: []string{one, absent}}}
	objects := &usageObjects{listed: []mediaapp.StorageUsageObject{{Key: one, ByteSize: 9000}, {Key: two, ByteSize: 8000}}, sizes: map[string]int64{one: 21, two: 35}}
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
		"foreign key":         {listed: []mediaapp.StorageUsageObject{{Key: "personal/foreign/file", ByteSize: 1}}},
		"duplicate inventory": {listed: []mediaapp.StorageUsageObject{{Key: prefix + "a", ByteSize: 1}, {Key: prefix + "a", ByteSize: 1}}},
		"negative metadata":   {listed: []mediaapp.StorageUsageObject{{Key: prefix + "a", ByteSize: -1}}},
		"negative stat":       {listed: []mediaapp.StorageUsageObject{{Key: prefix + "a", ByteSize: 1}}, sizes: map[string]int64{prefix + "a": -1}},
		"sum overflow":        {listed: []mediaapp.StorageUsageObject{{Key: prefix + "a", ByteSize: 1}, {Key: prefix + "b", ByteSize: 1}}, sizes: map[string]int64{prefix + "a": math.MaxInt64, prefix + "b": 1}},
		"partial transport":   {listed: []mediaapp.StorageUsageObject{{Key: prefix + "a", ByteSize: 1}}, err: errors.New("transport unavailable")},
	} {
		t.Run(name, func(t *testing.T) {
			reader := mediaapp.NewStorageUsageReader(usageSource{inventory: mediaapp.StorageUsageInventory{LibraryID: id, Prefixes: []string{prefix}}}, objects, time.Now)
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
