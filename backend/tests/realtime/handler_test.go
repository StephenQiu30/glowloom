package realtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	realtime "github.com/StephenQiu30/lanverse/backend/internal/infra/realtime/application"
)

type processedStore struct {
	seen  map[string]bool
	calls int
}

func (s *processedStore) ProcessExternalOnce(ctx context.Context, _, eventID string, handle func(context.Context) error) (bool, error) {
	if s.seen[eventID] {
		return false, nil
	}
	s.calls++
	if err := handle(ctx); err != nil {
		return false, err
	}
	s.seen[eventID] = true
	return true, nil
}

type eventSink struct {
	events []realtime.Event
	err    error
}

func (s *eventSink) Publish(_ context.Context, event realtime.Event) error {
	if s.err != nil {
		return s.err
	}
	s.events = append(s.events, event)
	return nil
}

func TestOperationStatusEventRetriesAndDeduplicates(t *testing.T) {
	store := &processedStore{seen: make(map[string]bool)}
	sink := &eventSink{err: errors.New("redis unavailable")}
	handler := realtime.NewOperationStatusHandler(store, sink)
	eventID, projectID, operationID, targetID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	record := inbox.Record{
		Topic: "lanverse.operation.status_changed.v1",
		Key:   []byte(projectID),
		Value: []byte(`{"event_id":"` + eventID + `","event_type":"lanverse.operation.status_changed.v1","project_id":"` + projectID + `","aggregate":{"type":"operation","id":"` + operationID + `"},"data":{"target_type":"shot_frame","target_id":"` + targetID + `","status":"submitted"}}`),
	}
	if err := handler.Handle(t.Context(), record); !errors.Is(err, sink.err) {
		t.Fatalf("first attempt = %v, want Redis error", err)
	}
	if store.seen[eventID] {
		t.Fatal("failed Redis delivery marked event processed")
	}
	sink.err = nil
	if err := handler.Handle(t.Context(), record); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if err := handler.Handle(t.Context(), record); err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if store.calls != 2 || len(sink.events) != 1 {
		t.Fatalf("calls = %d, delivered = %d, want 2 and 1", store.calls, len(sink.events))
	}
	got := sink.events[0]
	if got.ID != eventID || got.ProjectID != projectID || got.Type != "operation.updated" || got.OperationID != operationID || got.TargetID != targetID || got.Status != "submitted" {
		t.Fatalf("projected event = %+v", got)
	}
}

func TestOperationStatusRejectsMismatchedRouting(t *testing.T) {
	store := &processedStore{seen: make(map[string]bool)}
	sink := &eventSink{}
	handler := realtime.NewOperationStatusHandler(store, sink)
	projectID := uuid.NewString()
	record := inbox.Record{
		Topic: "lanverse.operation.status_changed.v1",
		Key:   []byte(uuid.NewString()),
		Value: []byte(`{"event_id":"` + uuid.NewString() + `","event_type":"lanverse.operation.status_changed.v1","project_id":"` + projectID + `","aggregate":{"type":"operation","id":"` + uuid.NewString() + `"},"data":{"target_type":"shot_frame","status":"submitted"}}`),
	}
	if err := handler.Handle(t.Context(), record); !errors.Is(err, realtime.ErrInvalidEvent) {
		t.Fatalf("mismatched project key = %v", err)
	}
	if store.calls != 0 || len(sink.events) != 0 {
		t.Fatal("invalid event reached effect store")
	}
}
