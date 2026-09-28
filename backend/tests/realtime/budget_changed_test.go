package realtime_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	realtime "github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/application"
)

type budgetChangedEnvelope struct {
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
		LimitMicros     int64 `json:"limit_micros"`
		AvailableMicros int64 `json:"available_micros"`
		IsOverrun       bool  `json:"is_overrun"`
	} `json:"data"`
}

func newBudgetChangedEnvelope() budgetChangedEnvelope {
	envelope := budgetChangedEnvelope{
		EventID: uuid.NewString(), EventType: realtime.BudgetChangedTopic,
		OccurredAt: "2026-09-27T08:00:00Z", OrgID: uuid.NewString(),
		ProjectID: uuid.NewString(),
	}
	envelope.Actor.Kind = "user"
	envelope.Actor.ID = uuid.NewString()
	envelope.Aggregate.Type = "budget"
	envelope.Aggregate.ID = uuid.NewString()
	envelope.Aggregate.Revision = 2
	envelope.Data.LimitMicros = 10_000_000
	envelope.Data.AvailableMicros = 4_000_000
	return envelope
}

func budgetRecord(t *testing.T, envelope budgetChangedEnvelope) inbox.Record {
	t.Helper()
	value, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("encode budget event: %v", err)
	}
	return inbox.Record{Topic: realtime.BudgetChangedTopic, Key: []byte(envelope.ProjectID), Value: value}
}

func TestBudgetChangedProjectsAvailableBalanceAndDeduplicates(t *testing.T) {
	processed := &processedStore{seen: make(map[string]bool)}
	sink := &eventSink{err: errors.New("redis unavailable")}
	handler := realtime.NewBudgetChangedHandler(processed, sink)
	envelope := newBudgetChangedEnvelope()
	record := budgetRecord(t, envelope)
	if err := handler.Handle(t.Context(), record); !errors.Is(err, sink.err) {
		t.Fatalf("first publish = %v, want Redis error", err)
	}
	if processed.seen[envelope.EventID] {
		t.Fatal("failed publish marked event processed")
	}
	sink.err = nil
	for range 2 {
		if err := handler.Handle(t.Context(), record); err != nil {
			t.Fatalf("retry budget event: %v", err)
		}
	}
	if processed.calls != 2 || len(sink.events) != 1 {
		t.Fatalf("processed calls = %d, published = %d", processed.calls, len(sink.events))
	}
	got := sink.events[0]
	if got.ID != envelope.EventID || got.ProjectID != envelope.ProjectID ||
		got.Type != "budget.updated" || got.AvailableMicros != envelope.Data.AvailableMicros {
		t.Fatalf("budget projection = %+v", got)
	}
}

func TestBudgetChangedRejectsInvalidEnvelopeBeforeEffects(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*budgetChangedEnvelope, *inbox.Record)
	}{
		{"partition key", func(_ *budgetChangedEnvelope, record *inbox.Record) { record.Key = []byte(uuid.NewString()) }},
		{"event type", func(envelope *budgetChangedEnvelope, _ *inbox.Record) { envelope.EventType = "other" }},
		{"missing project", func(envelope *budgetChangedEnvelope, _ *inbox.Record) { envelope.ProjectID = "" }},
		{"missing aggregate", func(envelope *budgetChangedEnvelope, _ *inbox.Record) { envelope.Aggregate.ID = "" }},
		{"wrong aggregate", func(envelope *budgetChangedEnvelope, _ *inbox.Record) { envelope.Aggregate.Type = "project" }},
		{"zero revision", func(envelope *budgetChangedEnvelope, _ *inbox.Record) { envelope.Aggregate.Revision = 0 }},
		{"missing actor", func(envelope *budgetChangedEnvelope, _ *inbox.Record) { envelope.Actor.ID = "" }},
		{"wrong actor", func(envelope *budgetChangedEnvelope, _ *inbox.Record) { envelope.Actor.Kind = "system" }},
		{"missing time", func(envelope *budgetChangedEnvelope, _ *inbox.Record) { envelope.OccurredAt = "" }},
		{"negative balance without overrun", func(envelope *budgetChangedEnvelope, _ *inbox.Record) { envelope.Data.AvailableMicros = -1 }},
		{"balance exceeds limit", func(envelope *budgetChangedEnvelope, _ *inbox.Record) {
			envelope.Data.AvailableMicros = envelope.Data.LimitMicros + 1
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			processed := &processedStore{seen: make(map[string]bool)}
			sink := &eventSink{}
			handler := realtime.NewBudgetChangedHandler(processed, sink)
			envelope := newBudgetChangedEnvelope()
			record := budgetRecord(t, envelope)
			test.mutate(&envelope, &record)
			record.Value = budgetRecord(t, envelope).Value
			if err := handler.Handle(t.Context(), record); !errors.Is(err, realtime.ErrInvalidEvent) {
				t.Fatalf("invalid event = %v, want ErrInvalidEvent", err)
			}
			if processed.calls != 0 || len(sink.events) != 0 {
				t.Fatal("invalid event reached effects")
			}
		})
	}
}

func TestRealtimeHandlerDispatchesBudgetChanged(t *testing.T) {
	processed := &processedStore{seen: make(map[string]bool)}
	sink := &eventSink{}
	envelope := newBudgetChangedEnvelope()
	if err := realtime.NewHandler(processed, sink).Handle(t.Context(), budgetRecord(t, envelope)); err != nil {
		t.Fatalf("dispatch budget changed: %v", err)
	}
	if len(sink.events) != 1 || sink.events[0].Type != "budget.updated" ||
		sink.events[0].AvailableMicros != envelope.Data.AvailableMicros {
		t.Fatalf("dispatched events = %+v", sink.events)
	}
}

func TestBudgetChangedRejectsMissingOverrunFlagBeforeEffects(t *testing.T) {
	record := budgetRecord(t, newBudgetChangedEnvelope())
	var body map[string]any
	if err := json.Unmarshal(record.Value, &body); err != nil {
		t.Fatalf("decode budget event: %v", err)
	}
	delete(body["data"].(map[string]any), "is_overrun")
	value, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode missing overrun flag: %v", err)
	}
	record.Value = value
	processed := &processedStore{seen: make(map[string]bool)}
	sink := &eventSink{}
	if err := realtime.NewBudgetChangedHandler(processed, sink).Handle(t.Context(), record); !errors.Is(err, realtime.ErrInvalidEvent) {
		t.Fatalf("missing overrun flag = %v, want ErrInvalidEvent", err)
	}
	if processed.calls != 0 || len(sink.events) != 0 {
		t.Fatal("missing overrun flag reached effects")
	}
}
