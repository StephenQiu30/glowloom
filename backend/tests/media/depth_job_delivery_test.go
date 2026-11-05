package media_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	current "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/task"

	"github.com/google/uuid"

	inboxpg "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/adapter/postgres"
	inboxapp "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
	toolevent "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/event"
	toolapp "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
)

type depthDeliveryRecorder struct {
	calls int
	fail  bool
}

func (s *depthDeliveryRecorder) Deliver(context.Context, toolapp.DepthDelivery) error {
	s.calls++
	if s.fail {
		return errors.New("controlled scheduling failure")
	}
	return nil
}

func TestDepthJobPGExactOutboxAndInboxDelivery(t *testing.T) {
	f := depthHTTP(t)
	j := f.create(t)
	var row struct {
		ID           uuid.UUID
		PartitionKey string
		Payload      []byte
	}
	if err := f.db.Raw(`SELECT id,partition_key,payload FROM infra.outbox WHERE topic=? AND partition_key=?`, toolevent.DepthTopic, j.ID.String()).Scan(&row).Error; err != nil || row.ID == uuid.Nil {
		t.Fatal("missing exact depth outbox", err)
	}
	starter := &depthDeliveryRecorder{fail: true}
	h := toolevent.NewDepthHandler(inboxpg.NewStore(f.db), f.workerStore, starter)
	record := inboxapp.Record{Topic: toolevent.DepthTopic, Key: []byte(row.PartitionKey), Value: row.Payload}
	var body map[string]any
	if err := json.Unmarshal(row.Payload, &body); err != nil {
		t.Fatal(err)
	}
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatal("outbox data missing")
	}
	data["project_id"] = uuid.NewString()
	injected, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Handle(t.Context(), inboxapp.Record{Topic: record.Topic, Key: record.Key, Value: injected}); !errors.Is(err, toolapp.ErrInvalidDepthInput) {
		t.Fatal("forged outbox command acknowledged", err)
	}
	if starter.calls != 0 {
		t.Fatal("forged payload reached physical scheduler")
	}
	if err := h.Handle(t.Context(), record); err == nil || starter.calls != 1 {
		t.Fatal("scheduling failure acknowledged", err)
	}
	starter.fail = false
	if err := h.Handle(t.Context(), record); err != nil || starter.calls != 2 {
		t.Fatal("failed delivery could not be recovered", err)
	}
	if err := h.Handle(t.Context(), record); err != nil || starter.calls != 2 {
		t.Fatal("duplicate permanent inbox scheduling", err)
	}
}

func TestDepthCancelAdmissionIsDurableAndDeduplicated(t *testing.T) {
	f := depthHTTP(t)
	j := f.create(t)
	key := uuid.New()
	first, err := f.store.Control(t.Context(), f.actor, f.project, j.ID, key, j.Revision, "cancel")
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.store.Control(t.Context(), f.actor, f.project, j.ID, key, j.Revision, "cancel")
	if err != nil || second.Revision != first.Revision {
		t.Fatal("cancel replay", err)
	}
	var count int
	if err := f.db.Raw("SELECT count(*) FROM infra.task_work WHERE kind=? AND reference->>'job_id'=?", current.Depth, j.ID.String()).Scan(&count).Error; err != nil || count != 2 {
		t.Fatal("expected distinct start and cancel admissions", count, err)
	}
}
