package eventing_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	eventingapp "github.com/StephenQiu30/lanverse/backend/internal/eventing/application"
	eventing "github.com/StephenQiu30/lanverse/backend/internal/eventing/domain"
)

func TestPublisherRetriesUnknownBrokerOutcomeWithSameEventIdentity(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 27, 9, 0, 0, 0, time.UTC)
	repository := &publisherRepository{event: eventing.OutboxEvent{
		ID: "00000000-0000-0000-0000-000000000301", EventType: eventing.StoryGraphVersionPublished,
		EventVersion: 1, WorkspaceID: "00000000-0000-0000-0000-000000000302",
		ProjectID: "00000000-0000-0000-0000-000000000303", AggregateKind: "storygraph",
		AggregateID: "00000000-0000-0000-0000-000000000303", AggregateRevision: 3,
		SourceReceiptID: "00000000-0000-0000-0000-000000000305",
		Payload:         json.RawMessage(`{"content_hash":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","owner_set_hash":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","topology_hash":"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","version_id":"00000000-0000-0000-0000-000000000304","version_no":3}`),
		OccurredAt:      now,
	}}
	broker := &recordingBroker{failures: 1}
	publisher := eventingapp.NewPublisher(repository, broker, eventingapp.StaticTopics{
		eventing.StoryGraphVersionPublished: "lanverse.business.storygraph-version.v1",
	}, eventingapp.PublisherConfig{
		Now: func() time.Time { return now }, NewID: func() string { return "claim-token" },
		Lease: time.Minute, BatchSize: 10,
	})

	if _, err := publisher.PublishOnce(context.Background()); err == nil {
		t.Fatal("unknown broker outcome must be reported")
	}
	if repository.published || repository.attempts != 1 {
		t.Fatalf("unknown outcome must remain retryable: %#v", repository)
	}
	published, err := publisher.PublishOnce(context.Background())
	if err != nil || published != 1 || !repository.published || repository.attempts != 2 {
		t.Fatalf("retry did not publish the original event: published=%d error=%v repo=%#v", published, err, repository)
	}
	if len(broker.messages) != 2 || broker.messages[0].Key != repository.event.ID || broker.messages[1].Key != repository.event.ID ||
		string(broker.messages[0].Value) != string(broker.messages[1].Value) {
		t.Fatalf("retry changed Kafka identity or envelope: %#v", broker.messages)
	}
	decoded, err := eventing.DecodeEnvelope(broker.messages[1].Value)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.EventID != repository.event.ID || decoded.TraceContext.RequestID != repository.event.SourceReceiptID {
		t.Fatalf("publisher did not preserve source identity: %#v", decoded)
	}
}

type publisherRepository struct {
	event      eventing.OutboxEvent
	claimed    bool
	published  bool
	attempts   int
	lastError  string
	claimToken string
}

func (repo *publisherRepository) ClaimPending(_ context.Context, _ time.Time, _ time.Duration, _ int, newID func() string) ([]eventingapp.ClaimedOutbox, error) {
	if repo.published || repo.claimed {
		return nil, nil
	}
	repo.claimed = true
	repo.attempts++
	repo.claimToken = newID()
	return []eventingapp.ClaimedOutbox{{Event: repo.event, ClaimToken: repo.claimToken}}, nil
}

func (repo *publisherRepository) MarkPublished(_ context.Context, eventID, claimToken string, _ time.Time) error {
	if eventID != repo.event.ID || claimToken != repo.claimToken || !repo.claimed {
		return errors.New("invalid publisher claim")
	}
	repo.claimed = false
	repo.published = true
	return nil
}

func (repo *publisherRepository) ReleaseClaim(_ context.Context, eventID, claimToken, message string) error {
	if eventID != repo.event.ID || claimToken != repo.claimToken || !repo.claimed {
		return errors.New("invalid publisher claim")
	}
	repo.claimed = false
	repo.lastError = message
	return nil
}

type recordingBroker struct {
	failures int
	messages []eventingapp.Message
}

func (broker *recordingBroker) Publish(_ context.Context, message eventingapp.Message) error {
	message.Value = append([]byte(nil), message.Value...)
	broker.messages = append(broker.messages, message)
	if broker.failures > 0 {
		broker.failures--
		return errors.New("broker outcome unknown")
	}
	return nil
}
