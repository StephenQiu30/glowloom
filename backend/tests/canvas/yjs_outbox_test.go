package canvas_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	outbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/outbox/application"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
)

type canvasOutboxEvent struct {
	event     outbox.Event
	published bool
}

func (s *canvasOutboxEvent) WithNext(_ context.Context, deliver func(outbox.Event) error) (bool, error) {
	if err := deliver(s.event); err != nil {
		return false, err
	}
	return true, nil
}

func (s *canvasOutboxEvent) Publish(_ context.Context, _ outbox.Event) error {
	s.published = true
	return nil
}

func TestYjsSaveAndRestoreEventsPassRelayValidation(t *testing.T) {
	database := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	store := canvasStore(database)
	doc, err := canvas.NewService(store).Create(t.Context(), actor, project, uuid.NewString(), canvas.CreateInput{Name: "Outbox acceptance"})
	if err != nil {
		t.Fatal(err)
	}
	node := resourceNode("text")
	node.Config.Text = ptr("before")
	_, err = canvasfixture.NewLegacySeeder(database, store).Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: []domain.Operation{{Type: "create_node", Node: &node}}})
	if err != nil {
		t.Fatal(err)
	}
	service := canvas.NewYjsService(store, yjsRuntime(t))
	initial, err := service.Initialize(t.Context(), actor, doc.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := service.Versions(t.Context(), actor, doc.ID, 0)
	if err != nil || len(versions) != 1 {
		t.Fatalf("versions: %v %v", versions, err)
	}
	accepted, err := service.Submit(t.Context(), actor, doc.ID, yjsEdit(t, initial, node.ID, "after", false))
	if err != nil || accepted.Status != "accepted" {
		t.Fatalf("save: %v %v", accepted, err)
	}
	_, err = service.RestoreVersion(t.Context(), actor, doc.ID, canvas.YjsRestore{ID: uuid.New(), SessionID: canvasfixture.ClientID, VersionID: versions[0].ID, Epoch: initial.Epoch, CommitSeq: accepted.CommitSeq})
	if err != nil {
		t.Fatal(err)
	}
	var events []struct {
		ID           string
		Topic        string
		PartitionKey string
		Payload      json.RawMessage
	}
	if err := database.Raw(`SELECT id::text,topic,partition_key,payload,create_time FROM infra.outbox WHERE topic='glowloom.canvas.collaboration_committed.v1' AND partition_key=? ORDER BY create_time`, doc.ID.String()).Scan(&events).Error; err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events: got %d, want save and restore", len(events))
	}
	for i, event := range events {
		sink := &canvasOutboxEvent{event: outbox.Event{ID: event.ID, Topic: event.Topic, PartitionKey: event.PartitionKey, Payload: event.Payload}}
		if sent, err := outbox.NewRelay(sink, sink).RunOnce(t.Context()); err != nil || !sent || !sink.published {
			t.Errorf("event %d rejected by relay: sent=%v err=%v", i, sent, err)
		}
		var body struct {
			Data struct {
				DocumentID uuid.UUID `json:"document_id"`
				Epoch      uuid.UUID `json:"epoch"`
				CommitSeq  string    `json:"commit_seq"`
				Revision   string    `json:"revision"`
			} `json:"data"`
		}
		if err := json.Unmarshal(event.Payload, &body); err != nil {
			t.Fatal(err)
		}
		if body.Data.DocumentID != doc.ID || body.Data.Epoch == uuid.Nil || body.Data.Revision == "" || body.Data.CommitSeq == "" {
			t.Errorf("event %d missing collaboration identity: %+v", i, body.Data)
		}
	}
}
