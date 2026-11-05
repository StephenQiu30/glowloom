package script_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

type locatedSourcePersistence struct{ sourcePersistence }

func (s *locatedSourcePersistence) BeginWrite(ctx context.Context, actor identity.Principal, p app.WritePlan) (app.PendingWrite, error) {
	for i := range p.Objects {
		p.Objects[i].LocationID = uuid.NewSHA1(p.Command.Key, []byte(p.Objects[i].Key))
	}
	return s.sourcePersistence.BeginWrite(ctx, actor, p)
}

type locatedSourceObjects struct {
	data map[uuid.UUID][]byte
	bare int
}

func (o *locatedSourceObjects) GetObject(_ context.Context, f domain.ObjectFact) (io.ReadCloser, error) {
	if f.LocationID == uuid.Nil {
		o.bare++
		return nil, app.ErrUnavailable
	}
	b, ok := o.data[f.LocationID]
	if !ok {
		return nil, app.ErrObjectMissing
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}
func (o *locatedSourceObjects) PutObjectIfAbsent(_ context.Context, f domain.ObjectFact, r io.Reader) error {
	if f.LocationID == uuid.Nil {
		o.bare++
		return app.ErrUnavailable
	}
	b, err := io.ReadAll(r)
	if err == nil {
		o.data[f.LocationID] = b
	}
	return err
}
func (o *locatedSourceObjects) RemoveObject(_ context.Context, f domain.ObjectFact) error {
	delete(o.data, f.LocationID)
	return nil
}

func TestScriptLocatedObjectsKeepSemanticManifestAndNeverFallBackToBareKey(t *testing.T) {
	actor, cmd := sourceCommand()
	store := &locatedSourcePersistence{}
	objects := &locatedSourceObjects{data: map[uuid.UUID][]byte{}}
	got, err := app.NewSourceService(store, objects, time.Now).Write(t.Context(), actor, cmd)
	if err != nil || got.VersionID == uuid.Nil || objects.bare != 0 || len(objects.data) != 4 {
		t.Fatal("script lost fixed identity between admission and I/O", err, objects.bare, len(objects.data))
	}
	f := store.pending.Plan.Objects[0]
	raw, err := json.Marshal(f)
	if err != nil || bytes.Contains(raw, []byte(f.LocationID.String())) {
		t.Fatal("physical migration changed legacy semantic manifest", err)
	}
}
