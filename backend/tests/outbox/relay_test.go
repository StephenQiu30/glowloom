package outbox_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/infra/outbox/application"
)

type memoryStore struct {
	event   application.Event
	pending bool
	dead    bool
	class   string
}

func (s *memoryStore) ClaimNext(_ context.Context, _ time.Time, _ time.Duration) (application.Event, bool, error) {
	if !s.pending {
		return application.Event{}, false, nil
	}
	s.event.AttemptCount++
	s.event.ClaimToken = "lease"
	return s.event, true, nil
}

func (s *memoryStore) Acknowledge(_ context.Context, _ application.Event, _ time.Time) error {
	s.pending = false
	return nil
}

func (s *memoryStore) Retry(_ context.Context, event application.Event, class string, max int, _, _ time.Time) (bool, error) {
	s.class = class
	if event.AttemptCount >= max {
		s.dead, s.pending = true, false
	}
	return s.dead, nil
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
		Topic:        "glowloom.operation.status_changed.v1",
		PartitionKey: "test-project",
		Payload:      []byte(`{"event_id":"event-1","event_type":"glowloom.operation.status_changed.v1"}`),
	}
	store := &memoryStore{event: event, pending: true}
	publisher := &recordingPublisher{err: errors.New("broker unavailable")}
	relay := application.NewRelay(store, publisher)

	if sent, err := relay.RunOnce(t.Context()); sent || err != nil {
		t.Fatalf("first delivery = (%t, %v), want durable retry", sent, err)
	}
	if !store.pending || store.class != "broker_publish_failed" {
		t.Fatal("failed publish was not kept pending with a retry class")
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
			Topic:        "glowloom.operation.status_changed.v1",
			PartitionKey: "test-project",
			Payload:      []byte(`{"event_id":"different","event_type":"glowloom.operation.status_changed.v1"}`),
		},
	}
	publisher := &recordingPublisher{}
	sent, err := application.NewRelay(store, publisher).RunOnce(t.Context())
	if sent || err != nil {
		t.Fatalf("invalid envelope delivery = (%t, %v)", sent, err)
	}
	if store.pending || !store.dead || store.class != "invalid_envelope" || len(publisher.events) != 0 {
		t.Fatal("invalid envelope was not dead-lettered immediately or was published")
	}
}
