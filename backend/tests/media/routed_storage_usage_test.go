package media_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

type physicalUsageObjects struct {
	listed      map[mediaapp.StorageUsagePrefix][]mediaapp.StorageUsageObject
	sizes       map[domain.ObjectRef]int64
	failProfile string
	statCalls   int
}

func (s *physicalUsageObjects) ListMetadata(_ context.Context, p mediaapp.StorageUsagePrefix, _ int) ([]mediaapp.StorageUsageObject, error) {
	if p.ProfileID == s.failProfile {
		return nil, errors.New("required profile listing failed")
	}
	return s.listed[p], nil
}
func (s *physicalUsageObjects) Stat(_ context.Context, r domain.ObjectRef) (int64, bool, error) {
	s.statCalls++
	n, ok := s.sizes[r]
	return n, ok, nil
}
func TestRoutedStorageUsageCountsSameKeyCopiesAndRetainedObjectsSeparately(t *testing.T) {
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New()}
	scope := domain.LibraryScope{Kind: domain.LibraryPersonal}
	id, _ := scope.Identity(actor.OrgID, actor.ID)
	prefix := "personal/" + actor.OrgID.String() + "/" + actor.ID.String() + "/image/"
	minio := domain.ObjectRef{ProfileID: "minio-main", Bucket: "storage-test", Key: prefix + "same.png"}
	cos := minio
	cos.ProfileID = "cos-main"
	retained := minio
	retained.Key = prefix + "retained-trash.png"
	intent := minio
	intent.Key = "library-packages/owned-job/archive.zip"
	absent := cos
	absent.Key = prefix + "deleted.png"
	p1 := mediaapp.StorageUsagePrefix{ProfileID: minio.ProfileID, Bucket: minio.Bucket, Prefix: prefix}
	p2 := p1
	p2.ProfileID = cos.ProfileID
	inventory := mediaapp.StorageUsageInventory{LibraryID: id, Locations: []domain.ObjectRef{minio, cos, absent, minio, intent}, PhysicalPrefixes: []mediaapp.StorageUsagePrefix{p1, p2}}
	objects := &physicalUsageObjects{listed: map[mediaapp.StorageUsagePrefix][]mediaapp.StorageUsageObject{p1: {{Ref: minio, ByteSize: 900}, {Ref: retained, ByteSize: 999}}, p2: {{Ref: cos, ByteSize: 900}}}, sizes: map[domain.ObjectRef]int64{minio: 21, cos: 21, retained: 35, intent: 11}}
	got, err := mediaapp.NewRoutedStorageUsageReader(usageSource{inventory: inventory}, objects, time.Now).StorageUsage(t.Context(), actor, scope)
	if err != nil || got.UsedBytes != 88 || got.ObjectCount != 4 || got.LimitBytes != nil || objects.statCalls != 5 {
		t.Fatal("physical copies collapsed or trash omitted", got, err)
	}
	objects.failProfile = "cos-main"
	got, err = mediaapp.NewRoutedStorageUsageReader(usageSource{inventory: inventory}, objects, time.Now).StorageUsage(t.Context(), actor, scope)
	if !errors.Is(err, mediaapp.ErrUnavailable) || got.UsedBytes != 0 || got.ObjectCount != 0 {
		t.Fatal("partial profile total published", got, err)
	}
	objects.failProfile = ""
	objects.listed[p2] = []mediaapp.StorageUsageObject{{Ref: minio, ByteSize: 21}}
	if _, err := mediaapp.NewRoutedStorageUsageReader(usageSource{inventory: inventory}, objects, time.Now).StorageUsage(t.Context(), actor, scope); !errors.Is(err, mediaapp.ErrUnavailable) {
		t.Fatal("provider identity mismatch accepted", err)
	}
}

func TestRoutedStorageUsageBoundsCombinedProfilesBeforePublishingTotals(t *testing.T) {
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New()}
	scope := domain.LibraryScope{Kind: domain.LibraryPersonal}
	id, _ := scope.Identity(actor.OrgID, actor.ID)
	prefix := "personal/" + actor.OrgID.String() + "/" + actor.ID.String() + "/image/"
	p1 := mediaapp.StorageUsagePrefix{ProfileID: "minio-main", Bucket: "storage-test", Prefix: prefix}
	p2 := p1
	p2.ProfileID = "cos-main"
	objects := &physicalUsageObjects{listed: map[mediaapp.StorageUsagePrefix][]mediaapp.StorageUsageObject{}}
	for index, p := range []mediaapp.StorageUsagePrefix{p1, p2} {
		count := 25000 + index
		for i := range count {
			objects.listed[p] = append(objects.listed[p], mediaapp.StorageUsageObject{Ref: domain.ObjectRef{ProfileID: p.ProfileID, Bucket: p.Bucket, Key: prefix + fmt.Sprint(i) + ".png"}, ByteSize: 1})
		}
	}
	source := usageSource{inventory: mediaapp.StorageUsageInventory{LibraryID: id, PhysicalPrefixes: []mediaapp.StorageUsagePrefix{p1, p2}}}
	got, err := mediaapp.NewRoutedStorageUsageReader(source, objects, time.Now).StorageUsage(t.Context(), actor, scope)
	if !errors.Is(err, mediaapp.ErrUnavailable) || got.UsedBytes != 0 || got.ObjectCount != 0 || objects.statCalls != 0 {
		t.Fatal("combined profile limit returned a partial inventory", got, err)
	}
}
