package media_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	mediaobjects "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/objectstorage"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
)

type generatedAudioLedger struct {
	intent mediaapp.StorageWorkIntent
	rows   map[string]domain.ObjectLocation
	trace  []string
	wrong  bool
	deny   bool
}

func (l *generatedAudioLedger) ReserveWorkLocation(_ context.Context, id uuid.UUID, key string) (domain.ObjectLocation, error) {
	l.trace = append(l.trace, "reserve")
	if id != l.intent.ID {
		return domain.ObjectLocation{}, mediaapp.ErrStorageRouteConflict
	}
	if l.rows == nil {
		l.rows = map[string]domain.ObjectLocation{}
	}
	if row, ok := l.rows[key]; ok {
		return row, nil
	}
	row := domain.ObjectLocation{ID: uuid.New(), Ref: domain.ObjectRef{ProfileID: l.intent.ProfileID, Bucket: l.intent.Bucket, Key: key}, State: "reserved"}
	if l.wrong {
		row.Ref.ProfileID = "different-profile"
	}
	l.rows[key] = row
	return row, nil
}

func (l *generatedAudioLedger) BeginWorkWrite(ctx context.Context, _ uuid.UUID, _ domain.ObjectLocation) error {
	l.trace = append(l.trace, "begin")
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 5*time.Minute || time.Until(deadline) <= 0 || l.deny {
		return mediaapp.ErrStorageRouteConflict
	}
	return nil
}

func (l *generatedAudioLedger) ConfirmWorkLocation(_ context.Context, _ uuid.UUID, location domain.ObjectLocation, info mediaapp.ObjectInfo) error {
	l.trace = append(l.trace, "confirm")
	location.State = "present"
	location.ByteSize, location.MIMEType, location.SHA256 = &info.Size, &info.ContentType, &info.SHA256
	l.rows[location.Ref.Key] = location
	return nil
}

func (*generatedAudioLedger) BeginWorkRemoval(context.Context, uuid.UUID, domain.ObjectLocation) (bool, error) {
	return false, errors.New("ingest must not compensate an owned voice job")
}

func (*generatedAudioLedger) ConfirmWorkRemoval(context.Context, uuid.UUID, domain.ObjectLocation) error {
	return errors.New("ingest must not clear unknown storage debt")
}

func TestPersonalGeneratedAudioObjectsUseFrozenProfileAndDurableWriteFence(t *testing.T) {
	oldObjects, oldServer := newCOSStageFixture(t)
	newObjects, newServer := newCOSStageFixture(t)
	cfg := stageConfiguration(t, oldServer.URL, newServer.URL)
	cfg.DefaultWriteProfile = "stage-new"
	registry, err := objectstorage.OpenProfiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	r, file := generatedAudioInput(t)
	r.Intent.ProfileID, r.Intent.Bucket = "stage-old", "fixture-123"
	ledger := &generatedAudioLedger{intent: r.Intent}
	objects := mediaobjects.NewPersonalGeneratedAudioObjects(registry, ledger)
	first, err := objects.CreateGeneratedAudioObject(t.Context(), r.Intent, r.ObjectKey(), file)
	if err != nil || first.State != "present" || first.Ref.ProfileID != "stage-old" || len(oldObjects.objects) != 1 || len(newObjects.objects) != 0 || !reflect.DeepEqual(ledger.trace, []string{"reserve", "begin", "confirm"}) {
		t.Fatal("frozen profile or write ledger order changed", err, ledger.trace)
	}
	replay, err := objects.CreateGeneratedAudioObject(t.Context(), r.Intent, r.ObjectKey(), file)
	if err != nil || replay.ID != first.ID || replay.Ref != first.Ref || len(oldObjects.objects) != 1 {
		t.Fatal("same physical object was overwritten or rebound", err)
	}
	for _, entry := range oldObjects.objects {
		if entry.sha != file.SHA256 || entry.mime != file.MIMEType || int64(len(entry.body)) != file.Size {
			t.Fatal("official SDK metadata did not preserve verified bytes")
		}
	}
}

func TestPersonalGeneratedAudioObjectsRejectWrongLocationAndCancelledWriteBeforeSDK(t *testing.T) {
	fixture, server := newCOSStageFixture(t)
	_, other := newCOSStageFixture(t)
	registry, err := objectstorage.OpenProfiles(stageConfiguration(t, server.URL, other.URL))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	r, file := generatedAudioInput(t)
	r.Intent.ProfileID, r.Intent.Bucket = "stage-old", "fixture-123"
	for _, wrong := range []bool{true, false} {
		ledger := &generatedAudioLedger{intent: r.Intent, wrong: wrong, deny: !wrong}
		objects := mediaobjects.NewPersonalGeneratedAudioObjects(registry, ledger)
		if _, err := objects.CreateGeneratedAudioObject(t.Context(), r.Intent, r.ObjectKey(), file); err == nil || len(fixture.objects) != 0 {
			t.Fatal("foreign or cancelled work reached SDK write", err)
		}
	}
}
