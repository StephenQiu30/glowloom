package media_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/google/uuid"

	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	scriptobjects "github.com/StephenQiu30/glowloom-server/backend/internal/script/adapter/objects"
	script "github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

type scriptLocationFence struct {
	location mediadomain.ObjectLocation
	denied   bool
}

func (s *scriptLocationFence) ReadLocation(_ context.Context, id uuid.UUID) (mediadomain.ObjectLocation, error) {
	if id != s.location.ID {
		return mediadomain.ObjectLocation{}, media.ErrObjectMismatch
	}
	return s.location, nil
}
func (s *scriptLocationFence) ReadWritableWorkLocation(ctx context.Context, id uuid.UUID) (mediadomain.ObjectLocation, error) {
	if s.denied {
		return mediadomain.ObjectLocation{}, media.ErrStorageRouteConflict
	}
	return s.ReadLocation(ctx, id)
}
func (s *scriptLocationFence) ReadRemovableWorkLocation(ctx context.Context, id uuid.UUID) (mediadomain.ObjectLocation, error) {
	return s.ReadWritableWorkLocation(ctx, id)
}
func (*scriptLocationFence) ResolveLegacyRef(context.Context, string) (mediadomain.ObjectRef, error) {
	return mediadomain.ObjectRef{}, media.ErrStorageNotReady
}

func TestScriptRoutedSDKUsesFrozenIdentityAndRejectsBareWrites(t *testing.T) {
	first, old := newCOSStageFixture(t)
	second, newer := newCOSStageFixture(t)
	cfg := stageConfiguration(t, old.URL, newer.URL)
	cfg.DefaultWriteProfile = "stage-new"
	r, err := objectstorage.OpenProfiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	body := []byte("脚本真实字节")
	fact := domain.ObjectFact{LocationID: uuid.New(), Key: "projects/frozen/script/original", SHA256: domain.ContentSHA(body), ByteSize: int64(len(body)), MIME: "text/plain"}
	fence := &scriptLocationFence{location: mediadomain.ObjectLocation{ID: fact.LocationID, Ref: mediadomain.ObjectRef{ProfileID: "stage-old", Bucket: "fixture-123", Key: fact.Key}, State: "reserved"}}
	objects := scriptobjects.NewRoutedStorage(r, fence)
	if err := objects.PutObjectIfAbsent(t.Context(), fact, bytes.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	reader, err := objects.GetObject(t.Context(), fact)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || !bytes.Equal(got, body) || len(first.objects) != 1 || len(second.objects) != 0 {
		t.Fatal("script SDK followed current default", err)
	}
	if err := objects.PutIfAbsent(t.Context(), fact.Key, bytes.NewReader(body), fact.ByteSize, fact.MIME, fact.SHA256); !errors.Is(err, script.ErrUnavailable) {
		t.Fatal("bare-key write reached native default", err)
	}
	fence.denied = true
	if err := objects.RemoveObject(t.Context(), fact); err == nil || len(first.objects) != 1 {
		t.Fatal("cancelled/unfenced cleanup reached SDK", err)
	}
	if _, err := objects.Get(t.Context(), fact.Key); !errors.Is(err, media.ErrStorageNotReady) {
		t.Fatal("unproved legacy key followed active profile", err)
	}
}
