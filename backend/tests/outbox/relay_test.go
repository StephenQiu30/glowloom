package outbox_test

import (
	"context"
	"errors"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/application"
)

type memoryStore struct {
	event   application.Event
	pending bool
}

func (s *memoryStore) WithNext(_ context.Context, deliver func(application.Event) error) (bool, error) {
	if !s.pending {
		return false, nil
	}
	if err := deliver(s.event); err != nil {
		return false, err
	}
	s.pending = false
	return true, nil
}

type recordingPublisher struct {
	events []application.Event
	err    error
}

func (p *recordingPublisher) Publish(_ context.Context, event application.Event) error {
	if p.err != nil {
		return p.err
	}
	p.events = append(p.events, event)
	return nil
}

func TestRelayRetriesAfterPublishFailure(t *testing.T) {
	event := application.Event{
		ID:           "event-1",
		Topic:        "lanverse.operation.status_changed.v1",
		PartitionKey: "test-project",
		Payload:      []byte(`{"event_id":"event-1","event_type":"lanverse.operation.status_changed.v1"}`),
	}
	store := &memoryStore{event: event, pending: true}
	wantErr := errors.New("broker unavailable")
	publisher := &recordingPublisher{err: wantErr}
	relay := application.NewRelay(store, publisher)

	if sent, err := relay.RunOnce(t.Context()); sent || !errors.Is(err, wantErr) {
		t.Fatalf("first delivery = (%t, %v), want failure", sent, err)
	}
	if !store.pending {
		t.Fatal("failed publish acknowledged the event")
	}

	publisher.err = nil
	if sent, err := relay.RunOnce(t.Context()); !sent || err != nil {
		t.Fatalf("retry = (%t, %v), want success", sent, err)
	}
	if len(publisher.events) != 1 || publisher.events[0].ID != event.ID {
		t.Fatalf("published events = %+v, want one matching event", publisher.events)
	}
	if sent, err := relay.RunOnce(t.Context()); sent || err != nil {
		t.Fatalf("empty queue = (%t, %v), want no event", sent, err)
	}
}

func TestRelayRejectsMismatchedEventEnvelope(t *testing.T) {
	store := &memoryStore{
		pending: true,
		event: application.Event{
			ID:           "event-1",
			Topic:        "lanverse.operation.status_changed.v1",
			PartitionKey: "test-project",
			Payload:      []byte(`{"event_id":"different","event_type":"lanverse.operation.status_changed.v1"}`),
		},
	}
	publisher := &recordingPublisher{}
	sent, err := application.NewRelay(store, publisher).RunOnce(t.Context())
	if sent || !errors.Is(err, application.ErrInvalidEnvelope) {
		t.Fatalf("invalid envelope delivery = (%t, %v)", sent, err)
	}
	if !store.pending || len(publisher.events) != 0 {
		t.Fatal("invalid envelope was acknowledged or published")
	}
}
