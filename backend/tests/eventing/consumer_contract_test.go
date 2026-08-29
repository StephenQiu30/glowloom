package eventing_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	eventingapp "github.com/StephenQiu30/lanverse/backend/internal/eventing/application"
	eventing "github.com/StephenQiu30/lanverse/backend/internal/eventing/domain"
)

func TestConsumerUsesInboxAndRevisionFencingBeforeProjection(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 27, 10, 0, 0, 0, time.UTC)
	repository := &consumerRepository{decisions: []eventingapp.InboxClaim{
		{Disposition: eventingapp.InboxAcquired, ClaimToken: "inbox-claim", Attempt: 1},
		{Disposition: eventingapp.InboxDuplicate},
		{Disposition: eventingapp.InboxStale},
	}}
	processor := &recordingProcessor{}
	consumer := newContractConsumer(repository, processor, &recordingBroker{}, now)
	message := incomingEnvelope(t, eventingFixture(t), 17)

	for attempt := 0; attempt < 3; attempt++ {
		result, err := consumer.Handle(context.Background(), message)
		if err != nil || !result.Ack {
			t.Fatalf("delivery %d was not acknowledged: result=%#v error=%v", attempt, result, err)
		}
	}
	if processor.calls != 1 || repository.completed != 1 {
		t.Fatalf("duplicate or stale event reached projector: processor=%d complete=%d", processor.calls, repository.completed)
	}
	if repository.acquired[0].Group != "lanverse.search-projector.v1" || repository.acquired[0].Message.Offset != 17 {
		t.Fatalf("consumer group or Kafka coordinate was not persisted: %#v", repository.acquired[0])
	}
}

func TestConsumerSendsPoisonMessageToIsolatedDLQWithoutLeakingInvalidBody(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 27, 10, 30, 0, 0, time.UTC)
	repository := &consumerRepository{decisions: []eventingapp.InboxClaim{{
		Disposition: eventingapp.InboxAcquired, ClaimToken: "poison-claim", Attempt: 1,
	}}}
	processor := &recordingProcessor{err: eventingapp.Permanent(errors.New("unsupported projection contract"))}
	broker := &recordingBroker{}
	consumer := newContractConsumer(repository, processor, broker, now)
	result, err := consumer.Handle(context.Background(), incomingEnvelope(t, eventingFixture(t), 19))
	if err != nil || !result.Ack || repository.deadLetters != 1 || len(broker.messages) != 1 {
		t.Fatalf("valid poison event did not reach DLQ: result=%#v error=%v repository=%#v broker=%#v", result, err, repository, broker)
	}
	if broker.messages[0].Topic != "lanverse.business.storygraph-version.dlq.v1" ||
		!strings.Contains(string(broker.messages[0].Value), eventingFixture(t).EventID) {
		t.Fatalf("DLQ identity or topic is wrong: %#v", broker.messages[0])
	}

	invalid := []byte(`{"prompt":"do not persist this secret","access_token":"private-token"}`)
	result, err = consumer.Handle(context.Background(), eventingapp.IncomingMessage{
		Topic: "lanverse.business.storygraph-version.v1", Partition: 0, Offset: 20, Key: "invalid", Value: invalid,
	})
	if err != nil || !result.Ack || repository.rejected != 1 || len(broker.messages) != 2 {
		t.Fatalf("invalid poison event did not reach safe DLQ: result=%#v error=%v", result, err)
	}
	invalidDLQ := string(broker.messages[1].Value)
	if strings.Contains(invalidDLQ, "do not persist") || strings.Contains(invalidDLQ, "private-token") ||
		!strings.Contains(invalidDLQ, `"replayable":false`) {
		t.Fatalf("invalid message body leaked into DLQ: %s", invalidDLQ)
	}
}

func TestConsumerAcknowledgesOwnerlessEventAfterNonReplayableDeadLetter(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 29, 15, 0, 0, 0, time.UTC)
	repository := &consumerRepository{acquireErr: eventingapp.Permanent(eventingapp.ErrEventOwnerNotFound)}
	processor := &recordingProcessor{}
	broker := &recordingBroker{}
	consumer := newContractConsumer(repository, processor, broker, now)

	result, err := consumer.Handle(context.Background(), incomingEnvelope(t, eventingFixture(t), 23))
	if err != nil || !result.Ack {
		t.Fatalf("ownerless event was not acknowledged: result=%#v error=%v", result, err)
	}
	if processor.calls != 0 || repository.rejected != 1 || repository.deadLetters != 0 || len(broker.messages) != 1 {
		t.Fatalf("ownerless event crossed the projection boundary: processor=%#v repository=%#v broker=%#v", processor, repository, broker)
	}
	deadLetter := string(broker.messages[0].Value)
	if !strings.Contains(deadLetter, `"failure_code":"event_owner_not_found"`) ||
		!strings.Contains(deadLetter, `"replayable":false`) ||
		!strings.Contains(deadLetter, eventingFixture(t).EventID) {
		t.Fatalf("ownerless event DLQ contract is wrong: %s", deadLetter)
	}
}

func newContractConsumer(repository eventingapp.InboxRepository, processor eventingapp.Processor, broker eventingapp.PublisherBroker, now time.Time) *eventingapp.Consumer {
	return eventingapp.NewConsumer(repository, processor, broker, eventingapp.ConsumerTopics{
		BusinessToDLQ: map[string]string{
			"lanverse.business.storygraph-version.v1": "lanverse.business.storygraph-version.dlq.v1",
		},
	}, eventingapp.ConsumerConfig{
		Group: "lanverse.search-projector.v1", Now: func() time.Time { return now },
		NewID: func() string { return "inbox-generated-id" }, Lease: time.Minute, MaxAttempts: 3,
	})
}

func incomingEnvelope(t *testing.T, envelope eventing.Envelope, offset int64) eventingapp.IncomingMessage {
	t.Helper()
	encoded, err := eventing.EncodeEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return eventingapp.IncomingMessage{
		Topic: "lanverse.business.storygraph-version.v1", Partition: 0, Offset: offset,
		Key: envelope.EventID, Value: encoded,
	}
}

type consumerRepository struct {
	decisions   []eventingapp.InboxClaim
	acquireErr  error
	acquired    []eventingapp.InboxDelivery
	completed   int
	deadLetters int
	rejected    int
}

func (repo *consumerRepository) Acquire(_ context.Context, delivery eventingapp.InboxDelivery, _ time.Time, _ time.Duration, _ func() string) (eventingapp.InboxClaim, error) {
	repo.acquired = append(repo.acquired, delivery)
	if repo.acquireErr != nil {
		return eventingapp.InboxClaim{}, repo.acquireErr
	}
	if len(repo.decisions) == 0 {
		return eventingapp.InboxClaim{}, errors.New("unexpected acquire")
	}
	decision := repo.decisions[0]
	repo.decisions = repo.decisions[1:]
	return decision, nil
}

func (repo *consumerRepository) Complete(_ context.Context, _ eventingapp.InboxDelivery, claimToken string, _ time.Time) error {
	if claimToken == "" {
		return errors.New("missing claim token")
	}
	repo.completed++
	return nil
}

func (repo *consumerRepository) MarkDeadLetter(_ context.Context, _ eventingapp.InboxDelivery, claimToken string, _ eventingapp.DeadLetter, _ time.Time) error {
	if claimToken == "" {
		return errors.New("missing claim token")
	}
	repo.deadLetters++
	return nil
}

func (repo *consumerRepository) RecordRejected(_ context.Context, _ eventingapp.DeadLetter, _ time.Time) error {
	repo.rejected++
	return nil
}

type recordingProcessor struct {
	calls int
	err   error
}

func (processor *recordingProcessor) Process(_ context.Context, _ eventing.Envelope) error {
	processor.calls++
	return processor.err
}
