package outbox_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/application"
)

type queuedStore struct {
	events []application.Event
	idle   chan struct{}
	err    error
}

func (s *queuedStore) WithNext(_ context.Context, deliver func(application.Event) error) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	if len(s.events) == 0 {
		select {
		case s.idle <- struct{}{}:
		default:
		}
		return false, nil
	}
	event := s.events[0]
	if err := deliver(event); err != nil {
		return false, err
	}
	s.events = s.events[1:]
	return true, nil
}

func TestRelayRunDrainsBacklogAndStopsOnCancellation(t *testing.T) {
	newEvent := func(id string) application.Event {
		return application.Event{ID: id, Topic: "lanverse.operation.status_changed.v1", PartitionKey: "project", Payload: []byte(`{"event_id":"` + id + `","event_type":"lanverse.operation.status_changed.v1"}`)}
	}
	store := &queuedStore{events: []application.Event{newEvent("one"), newEvent("two")}, idle: make(chan struct{}, 1)}
	publisher := &recordingPublisher{}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- application.NewRelay(store, publisher).Run(ctx) }()
	select {
	case <-store.idle:
		cancel()
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("relay did not drain backlog")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("relay stop: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("relay did not stop after cancellation")
	}
	if len(publisher.events) != 2 || publisher.events[0].ID != "one" || publisher.events[1].ID != "two" {
		t.Fatalf("published events = %+v", publisher.events)
	}
}

func TestRelayRunReportsDeliveryFailure(t *testing.T) {
	wantErr := errors.New("database unavailable")
	store := &queuedStore{err: wantErr}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := application.NewRelay(store, &recordingPublisher{}).Run(ctx); !errors.Is(err, wantErr) {
		t.Fatalf("relay failure = %v, want database error", err)
	}
}
