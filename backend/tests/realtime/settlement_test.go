package realtime_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	realtime "github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/application"
)

type settlementEnvelope struct {
	EventID    string `json:"event_id"`
	EventType  string `json:"event_type"`
	OccurredAt string `json:"occurred_at"`
	OrgID      string `json:"org_id"`
	ProjectID  string `json:"project_id"`
	Actor      struct {
		Kind string  `json:"kind"`
		ID   *string `json:"id"`
	} `json:"actor"`
	Aggregate struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"aggregate"`
	Data struct {
		OperationID     string `json:"operation_id"`
		AvailableMicros int64  `json:"available_micros"`
		BudgetOverrun   bool   `json:"budget_overrun"`
		ChargeMicros    int64  `json:"charge_micros"`
	} `json:"data"`
}

func newSettlementEnvelope() settlementEnvelope {
	envelope := settlementEnvelope{
		EventID: uuid.NewString(), EventType: realtime.BillingSettledTopic,
		OccurredAt: "2026-09-28T08:00:00Z", OrgID: uuid.NewString(),
		ProjectID: uuid.NewString(),
	}
	envelope.Actor.Kind = "system"
	envelope.Aggregate.Type = "operation"
	envelope.Aggregate.ID = uuid.NewString()
	envelope.Data.OperationID = envelope.Aggregate.ID
	envelope.Data.AvailableMicros = 400
	envelope.Data.ChargeMicros = 100
	return envelope
}

func settlementRecord(t *testing.T, envelope settlementEnvelope) inbox.Record {
	t.Helper()
	value, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("encode settlement: %v", err)
	}
	return inbox.Record{Topic: realtime.BillingSettledTopic, Key: []byte(envelope.ProjectID), Value: value}
}

func TestBillingSettlementProjectsAvailableBalanceAndDeduplicates(t *testing.T) {
	processed := &processedStore{seen: make(map[string]bool)}
	sink := &eventSink{err: errors.New("redis unavailable")}
	handler := realtime.NewBillingSettledHandler(processed, sink)
	envelope := newSettlementEnvelope()
	record := settlementRecord(t, envelope)
	if err := handler.Handle(t.Context(), record); !errors.Is(err, sink.err) {
		t.Fatalf("first publish = %v, want Redis error", err)
	}
	if processed.seen[envelope.EventID] {
		t.Fatal("failed publish marked event processed")
	}
	sink.err = nil
	for range 2 {
		if err := handler.Handle(t.Context(), record); err != nil {
			t.Fatalf("retry settlement: %v", err)
		}
	}
	if processed.calls != 2 || len(sink.events) != 1 {
		t.Fatalf("processed calls = %d, published = %d", processed.calls, len(sink.events))
	}
	got := sink.events[0]
	if got.ID != envelope.EventID || got.ProjectID != envelope.ProjectID ||
		got.Type != "budget.updated" || got.AvailableMicros != envelope.Data.AvailableMicros {
		t.Fatalf("settlement projection = %+v", got)
	}
}

func TestBillingSettlementProjectsNegativeBalanceAfterOverrun(t *testing.T) {
	processed := &processedStore{seen: make(map[string]bool)}
	sink := &eventSink{}
	envelope := newSettlementEnvelope()
	envelope.Data.AvailableMicros = -25
	envelope.Data.BudgetOverrun = true
	if err := realtime.NewBillingSettledHandler(processed, sink).Handle(t.Context(), settlementRecord(t, envelope)); err != nil {
		t.Fatalf("project overrun settlement: %v", err)
	}
	if len(sink.events) != 1 || sink.events[0].AvailableMicros != -25 {
		t.Fatalf("overrun projection = %+v", sink.events)
	}
}

func TestBillingSettlementRejectsInvalidEnvelopeBeforeEffects(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*settlementEnvelope, *inbox.Record)
	}{
		{"partition key", func(_ *settlementEnvelope, record *inbox.Record) { record.Key = []byte(uuid.NewString()) }},
		{"event type", func(envelope *settlementEnvelope, _ *inbox.Record) { envelope.EventType = "other" }},
		{"aggregate type", func(envelope *settlementEnvelope, _ *inbox.Record) { envelope.Aggregate.Type = "budget" }},
		{"aggregate ID", func(envelope *settlementEnvelope, _ *inbox.Record) { envelope.Aggregate.ID = uuid.NewString() }},
		{"actor", func(envelope *settlementEnvelope, _ *inbox.Record) { envelope.Actor.Kind = "user" }},
		{"overrun mismatch", func(envelope *settlementEnvelope, _ *inbox.Record) { envelope.Data.AvailableMicros = -1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			processed := &processedStore{seen: make(map[string]bool)}
			sink := &eventSink{}
			handler := realtime.NewBillingSettledHandler(processed, sink)
			envelope := newSettlementEnvelope()
			record := settlementRecord(t, envelope)
			test.mutate(&envelope, &record)
			record.Value = settlementRecord(t, envelope).Value
			if err := handler.Handle(t.Context(), record); !errors.Is(err, realtime.ErrInvalidEvent) {
				t.Fatalf("invalid settlement = %v, want ErrInvalidEvent", err)
			}
			if processed.calls != 0 || len(sink.events) != 0 {
				t.Fatal("invalid settlement reached effects")
			}
		})
	}
}

func TestLegacyBillingSettlementWithoutAvailableBalanceResyncs(t *testing.T) {
	envelope := newSettlementEnvelope()
	record := settlementRecord(t, envelope)
	var body map[string]any
	if err := json.Unmarshal(record.Value, &body); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	data := body["data"].(map[string]any)
	delete(data, "available_micros")
	value, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode missing field: %v", err)
	}
	record.Value = value
	processed := &processedStore{seen: make(map[string]bool)}
	sink := &eventSink{}
	if err := realtime.NewBillingSettledHandler(processed, sink).Handle(t.Context(), record); err != nil {
		t.Fatalf("project legacy settlement: %v", err)
	}
	if processed.calls != 1 || len(sink.events) != 1 ||
		sink.events[0].ID != envelope.EventID || sink.events[0].ProjectID != envelope.ProjectID ||
		sink.events[0].Type != "resync" {
		t.Fatalf("legacy settlement projection = %+v, calls = %d", sink.events, processed.calls)
	}
}

func TestBillingSettlementRejectsNullBalanceBeforeEffects(t *testing.T) {
	record := settlementRecord(t, newSettlementEnvelope())
	var body map[string]any
	if err := json.Unmarshal(record.Value, &body); err != nil {
		t.Fatalf("decode settlement: %v", err)
	}
	body["data"].(map[string]any)["available_micros"] = nil
	value, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode null balance: %v", err)
	}
	record.Value = value
	processed := &processedStore{seen: make(map[string]bool)}
	sink := &eventSink{}
	if err := realtime.NewBillingSettledHandler(processed, sink).Handle(t.Context(), record); !errors.Is(err, realtime.ErrInvalidEvent) {
		t.Fatalf("null balance = %v, want ErrInvalidEvent", err)
	}
	if processed.calls != 0 || len(sink.events) != 0 {
		t.Fatal("null balance reached effects")
	}
}

func TestRealtimeHandlerDispatchesSettlement(t *testing.T) {
	processed := &processedStore{seen: make(map[string]bool)}
	sink := &eventSink{}
	envelope := newSettlementEnvelope()
	if err := realtime.NewHandler(processed, sink).Handle(t.Context(), settlementRecord(t, envelope)); err != nil {
		t.Fatalf("dispatch settlement: %v", err)
	}
	if len(sink.events) != 1 || sink.events[0].Type != "budget.updated" ||
		sink.events[0].AvailableMicros != envelope.Data.AvailableMicros {
		t.Fatalf("dispatched events = %+v", sink.events)
	}
}
