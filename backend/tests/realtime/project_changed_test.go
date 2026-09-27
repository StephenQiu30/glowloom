package realtime_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	realtime "github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/application"
)

type projectChangedEnvelope struct {
	EventID    string `json:"event_id"`
	EventType  string `json:"event_type"`
	OccurredAt string `json:"occurred_at"`
	OrgID      string `json:"org_id"`
	ProjectID  string `json:"project_id"`
	Actor      struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	} `json:"actor"`
	Aggregate struct {
		Type     string `json:"type"`
		ID       string `json:"id"`
		Revision int64  `json:"revision"`
	} `json:"aggregate"`
	Data struct {
		Change string `json:"change"`
	} `json:"data"`
}

func newProjectChangedEnvelope() projectChangedEnvelope {
	projectID := uuid.NewString()
	envelope := projectChangedEnvelope{
		EventID: uuid.NewString(), EventType: realtime.ProjectChangedTopic,
		OccurredAt: "2026-09-27T08:00:00Z", OrgID: uuid.NewString(),
		ProjectID: projectID,
	}
	envelope.Actor.Kind = "user"
	envelope.Actor.ID = uuid.NewString()
	envelope.Aggregate.Type = "project"
	envelope.Aggregate.ID = projectID
	envelope.Aggregate.Revision = 1
	envelope.Data.Change = "created"
	return envelope
}

func unitProjectChangedRecord(t *testing.T, envelope projectChangedEnvelope) inbox.Record {
	t.Helper()
	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("encode project changed event: %v", err)
	}
	return inbox.Record{Topic: realtime.ProjectChangedTopic, Key: []byte(envelope.ProjectID), Value: payload}
}

func TestProjectChangedProjectsOnlySafeInvalidationFields(t *testing.T) {
	store := &processedStore{seen: make(map[string]bool)}
	sink := &eventSink{}
	handler := realtime.NewProjectChangedHandler(store, sink)
	envelope := newProjectChangedEnvelope()
	if err := handler.Handle(t.Context(), unitProjectChangedRecord(t, envelope)); err != nil {
		t.Fatalf("handle project changed event: %v", err)
	}
	if store.calls != 1 || !store.seen[envelope.EventID] || len(sink.events) != 1 {
		t.Fatalf("processed calls = %d, seen = %v, events = %d", store.calls, store.seen[envelope.EventID], len(sink.events))
	}
	got := sink.events[0]
	if got.ID != envelope.EventID || got.ProjectID != envelope.ProjectID || got.Type != "project.updated" ||
		got.Revision != 1 || got.Change != "created" {
		t.Fatalf("projected event = %+v", got)
	}
	if got.OperationID != "" || got.BatchID != "" || got.TargetType != "" ||
		got.TargetID != "" || got.Status != "" || len(got.Progress) != 0 {
		t.Fatalf("project event contains unrelated operation data: %+v", got)
	}
}

func TestProjectChangedRetriesFailedPublishAndDeduplicatesEventID(t *testing.T) {
	store := &processedStore{seen: make(map[string]bool)}
	sink := &eventSink{err: errors.New("redis unavailable")}
	handler := realtime.NewProjectChangedHandler(store, sink)
	envelope := newProjectChangedEnvelope()
	record := unitProjectChangedRecord(t, envelope)

	if err := handler.Handle(t.Context(), record); !errors.Is(err, sink.err) {
		t.Fatalf("first attempt = %v, want publish error", err)
	}
	if store.seen[envelope.EventID] || len(sink.events) != 0 {
		t.Fatal("failed publish marked event processed or delivered")
	}
	sink.err = nil
	if err := handler.Handle(t.Context(), record); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if err := handler.Handle(t.Context(), record); err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if store.calls != 2 || !store.seen[envelope.EventID] || len(sink.events) != 1 {
		t.Fatalf("processed calls = %d, seen = %v, events = %d; want 2, true, 1", store.calls, store.seen[envelope.EventID], len(sink.events))
	}
}

func TestProjectChangedRejectsInvalidEnvelopeBeforeEffects(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*projectChangedEnvelope, *inbox.Record)
	}{
		{"topic", func(_ *projectChangedEnvelope, record *inbox.Record) { record.Topic = "other" }},
		{"partition key", func(_ *projectChangedEnvelope, record *inbox.Record) { record.Key = []byte(uuid.NewString()) }},
		{"event type", func(envelope *projectChangedEnvelope, _ *inbox.Record) { envelope.EventType = "other" }},
		{"event ID", func(envelope *projectChangedEnvelope, _ *inbox.Record) { envelope.EventID = "invalid" }},
		{"aggregate type", func(envelope *projectChangedEnvelope, _ *inbox.Record) { envelope.Aggregate.Type = "operation" }},
		{"aggregate ID", func(envelope *projectChangedEnvelope, _ *inbox.Record) { envelope.Aggregate.ID = uuid.NewString() }},
		{"zero revision", func(envelope *projectChangedEnvelope, _ *inbox.Record) { envelope.Aggregate.Revision = 0 }},
		{"negative revision", func(envelope *projectChangedEnvelope, _ *inbox.Record) { envelope.Aggregate.Revision = -1 }},
		{"missing change", func(envelope *projectChangedEnvelope, _ *inbox.Record) { envelope.Data.Change = "" }},
		{"unknown change", func(envelope *projectChangedEnvelope, _ *inbox.Record) { envelope.Data.Change = "unknown" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &processedStore{seen: make(map[string]bool)}
			sink := &eventSink{}
			handler := realtime.NewProjectChangedHandler(store, sink)
			envelope := newProjectChangedEnvelope()
			record := unitProjectChangedRecord(t, envelope)
			test.mutate(&envelope, &record)
			payload, err := json.Marshal(envelope)
			if err != nil {
				t.Fatalf("encode changed envelope: %v", err)
			}
			record.Value = payload
			if err := handler.Handle(t.Context(), record); !errors.Is(err, realtime.ErrInvalidEvent) {
				t.Fatalf("invalid event error = %v, want ErrInvalidEvent", err)
			}
			if store.calls != 0 || len(store.seen) != 0 || len(sink.events) != 0 {
				t.Fatalf("invalid event reached effects: processed calls = %d, seen = %d, events = %d", store.calls, len(store.seen), len(sink.events))
			}
		})
	}
}

func TestRealtimeHandlerDispatchesKnownTopicsAndRejectsUnknown(t *testing.T) {
	store := &processedStore{seen: make(map[string]bool)}
	sink := &eventSink{}
	handler := realtime.NewHandler(store, sink)

	operationEventID, operationProjectID, operationID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	operationRecord := inbox.Record{
		Topic: realtime.OperationStatusTopic,
		Key:   []byte(operationProjectID),
		Value: []byte(`{"event_id":"` + operationEventID + `","event_type":"` + realtime.OperationStatusTopic + `","project_id":"` + operationProjectID + `","aggregate":{"type":"operation","id":"` + operationID + `"},"data":{"target_type":"shot_frame","status":"submitted"}}`),
	}
	if err := handler.Handle(t.Context(), operationRecord); err != nil {
		t.Fatalf("dispatch operation event: %v", err)
	}
	if store.calls != 1 || len(sink.events) != 1 || sink.events[0].ID != operationEventID ||
		sink.events[0].ProjectID != operationProjectID || sink.events[0].Type != "operation.updated" ||
		sink.events[0].OperationID != operationID || sink.events[0].Status != "submitted" {
		t.Fatalf("operation dispatch: calls = %d, events = %+v", store.calls, sink.events)
	}

	projectEnvelope := newProjectChangedEnvelope()
	if err := handler.Handle(t.Context(), unitProjectChangedRecord(t, projectEnvelope)); err != nil {
		t.Fatalf("dispatch project event: %v", err)
	}
	if store.calls != 2 || len(sink.events) != 2 || sink.events[1].ID != projectEnvelope.EventID ||
		sink.events[1].ProjectID != projectEnvelope.ProjectID || sink.events[1].Type != "project.updated" ||
		sink.events[1].Revision != 1 || sink.events[1].Change != "created" {
		t.Fatalf("project dispatch: calls = %d, events = %+v", store.calls, sink.events)
	}

	unknown := unitProjectChangedRecord(t, newProjectChangedEnvelope())
	unknown.Topic = "lanverse.unknown.v1"
	if err := handler.Handle(t.Context(), unknown); !errors.Is(err, realtime.ErrInvalidEvent) {
		t.Fatalf("unknown topic error = %v, want ErrInvalidEvent", err)
	}
	if store.calls != 2 || len(store.seen) != 2 || len(sink.events) != 2 {
		t.Fatalf("unknown topic reached effects: calls = %d, seen = %d, events = %d", store.calls, len(store.seen), len(sink.events))
	}
}
