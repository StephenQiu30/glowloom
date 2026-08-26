package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	eventing "github.com/StephenQiu30/lanverse/backend/internal/eventing/domain"
)

const deadLetterSchemaV1 = "lanverse.dead-letter.v1"

type IncomingMessage struct {
	Topic     string
	Partition int32
	Offset    int64
	Key       string
	Value     []byte
}

type InboxDelivery struct {
	Group    string
	Message  IncomingMessage
	Envelope eventing.Envelope
}

type InboxDisposition string

const (
	InboxAcquired  InboxDisposition = "acquired"
	InboxDuplicate InboxDisposition = "duplicate"
	InboxStale     InboxDisposition = "stale"
	InboxBusy      InboxDisposition = "busy"
)

type InboxClaim struct {
	Disposition InboxDisposition
	ClaimToken  string
	Attempt     int
}

type DeadLetter struct {
	ID                string          `json:"dead_letter_id"`
	Schema            string          `json:"schema"`
	ConsumerGroup     string          `json:"consumer_group"`
	EventID           string          `json:"event_id"`
	EventType         string          `json:"event_type,omitempty"`
	ProjectID         string          `json:"project_id,omitempty"`
	AggregateKind     string          `json:"aggregate_kind,omitempty"`
	AggregateID       string          `json:"aggregate_id,omitempty"`
	AggregateRevision int64           `json:"aggregate_revision,omitempty"`
	OriginalTopic     string          `json:"original_topic"`
	DLQTopic          string          `json:"dlq_topic"`
	SourcePartition   int32           `json:"source_partition"`
	SourceOffset      int64           `json:"source_offset"`
	PayloadHash       string          `json:"payload_hash"`
	FailureCode       string          `json:"failure_code"`
	FailureMessage    string          `json:"failure_message"`
	Replayable        bool            `json:"replayable"`
	Envelope          json.RawMessage `json:"envelope,omitempty"`
	FailedAt          time.Time       `json:"failed_at"`
}

type InboxRepository interface {
	Acquire(context.Context, InboxDelivery, time.Time, time.Duration, func() string) (InboxClaim, error)
	Complete(context.Context, InboxDelivery, string, time.Time) error
	MarkDeadLetter(context.Context, InboxDelivery, string, DeadLetter, time.Time) error
	RecordRejected(context.Context, DeadLetter, time.Time) error
}

type Processor interface {
	Process(context.Context, eventing.Envelope) error
}

type ConsumerTopics struct {
	BusinessToDLQ map[string]string
}

func (topics ConsumerTopics) DLQFor(topic string) (string, error) {
	dlq := strings.TrimSpace(topics.BusinessToDLQ[topic])
	if dlq == "" || dlq == strings.TrimSpace(topic) {
		return "", fmt.Errorf("business topic %q has no isolated DLQ", topic)
	}
	return dlq, nil
}

type ConsumerConfig struct {
	Group       string
	Now         func() time.Time
	NewID       func() string
	Lease       time.Duration
	MaxAttempts int
}

type Consumer struct {
	repository InboxRepository
	processor  Processor
	broker     PublisherBroker
	topics     ConsumerTopics
	config     ConsumerConfig
}

type HandleResult struct {
	Ack bool
}

func NewConsumer(repository InboxRepository, processor Processor, broker PublisherBroker, topics ConsumerTopics, config ConsumerConfig) *Consumer {
	return &Consumer{repository: repository, processor: processor, broker: broker, topics: topics, config: config}
}

func (consumer *Consumer) Handle(ctx context.Context, message IncomingMessage) (HandleResult, error) {
	if err := consumer.validate(); err != nil {
		return HandleResult{}, err
	}
	now := consumer.config.Now().UTC()
	envelope, err := eventing.DecodeEnvelope(message.Value)
	if err != nil || (message.Key != "" && message.Key != envelope.EventID) {
		if err == nil {
			err = errors.New("Kafka record key does not match event id")
		}
		return consumer.rejectInvalid(ctx, message, now, err)
	}
	delivery := InboxDelivery{Group: consumer.config.Group, Message: message, Envelope: envelope}
	claim, err := consumer.repository.Acquire(ctx, delivery, now, consumer.config.Lease, consumer.config.NewID)
	if err != nil {
		return HandleResult{}, fmt.Errorf("acquire inbox event %s: %w", envelope.EventID, err)
	}
	switch claim.Disposition {
	case InboxDuplicate, InboxStale:
		return HandleResult{Ack: true}, nil
	case InboxBusy:
		return HandleResult{}, fmt.Errorf("inbox event %s is already being processed", envelope.EventID)
	case InboxAcquired:
		if claim.ClaimToken == "" || claim.Attempt < 1 {
			return HandleResult{}, errors.New("inbox repository returned an invalid claim")
		}
	default:
		return HandleResult{}, errors.New("inbox repository returned an unknown disposition")
	}
	processErr := consumer.processor.Process(ctx, envelope)
	if processErr == nil {
		if err = consumer.repository.Complete(ctx, delivery, claim.ClaimToken, now); err != nil {
			return HandleResult{}, fmt.Errorf("complete inbox event %s: %w", envelope.EventID, err)
		}
		return HandleResult{Ack: true}, nil
	}
	if !IsPermanent(processErr) && claim.Attempt < consumer.config.MaxAttempts {
		return HandleResult{}, fmt.Errorf("process event %s: %w", envelope.EventID, processErr)
	}
	return consumer.deadLetter(ctx, delivery, claim.ClaimToken, now, "projection_rejected", processErr)
}

func (consumer *Consumer) deadLetter(
	ctx context.Context,
	delivery InboxDelivery,
	claimToken string,
	now time.Time,
	code string,
	failure error,
) (HandleResult, error) {
	dlqTopic, err := consumer.topics.DLQFor(delivery.Message.Topic)
	if err != nil {
		return HandleResult{}, err
	}
	encodedEnvelope, err := eventing.EncodeEnvelope(delivery.Envelope)
	if err != nil {
		return HandleResult{}, err
	}
	letter := DeadLetter{
		ID: consumer.config.NewID(), Schema: deadLetterSchemaV1, ConsumerGroup: consumer.config.Group,
		EventID: delivery.Envelope.EventID, EventType: delivery.Envelope.EventType,
		ProjectID: delivery.Envelope.ProjectID, AggregateKind: delivery.Envelope.AggregateKind,
		AggregateID: delivery.Envelope.AggregateID, AggregateRevision: delivery.Envelope.AggregateRevision,
		OriginalTopic: delivery.Message.Topic, DLQTopic: dlqTopic, SourcePartition: delivery.Message.Partition,
		SourceOffset: delivery.Message.Offset, PayloadHash: delivery.Envelope.PayloadHash,
		FailureCode: code, FailureMessage: safeError(failure), Replayable: true,
		Envelope: encodedEnvelope, FailedAt: now,
	}
	if err = consumer.publishDeadLetter(ctx, letter); err != nil {
		return HandleResult{}, err
	}
	if err = consumer.repository.MarkDeadLetter(ctx, delivery, claimToken, letter, now); err != nil {
		return HandleResult{}, fmt.Errorf("persist dead letter for event %s: %w", letter.EventID, err)
	}
	return HandleResult{Ack: true}, nil
}

func (consumer *Consumer) rejectInvalid(ctx context.Context, message IncomingMessage, now time.Time, decodeErr error) (HandleResult, error) {
	dlqTopic, err := consumer.topics.DLQFor(message.Topic)
	if err != nil {
		return HandleResult{}, err
	}
	digest := sha256.Sum256(message.Value)
	rawHash := hex.EncodeToString(digest[:])
	letter := DeadLetter{
		ID: consumer.config.NewID(), Schema: deadLetterSchemaV1, ConsumerGroup: consumer.config.Group,
		EventID: "invalid:" + rawHash, OriginalTopic: message.Topic, DLQTopic: dlqTopic,
		SourcePartition: message.Partition, SourceOffset: message.Offset, PayloadHash: rawHash,
		FailureCode: "invalid_envelope", FailureMessage: "event envelope failed strict validation",
		Replayable: false, FailedAt: now,
	}
	if err = consumer.publishDeadLetter(ctx, letter); err != nil {
		return HandleResult{}, errors.Join(decodeErr, err)
	}
	if err = consumer.repository.RecordRejected(ctx, letter, now); err != nil {
		return HandleResult{}, fmt.Errorf("persist rejected Kafka record: %w", err)
	}
	return HandleResult{Ack: true}, nil
}

func (consumer *Consumer) publishDeadLetter(ctx context.Context, letter DeadLetter) error {
	encoded, err := json.Marshal(letter)
	if err != nil {
		return fmt.Errorf("encode dead letter: %w", err)
	}
	if err = consumer.broker.Publish(ctx, Message{
		Topic: letter.DLQTopic, Key: letter.EventID, Value: encoded,
		Headers: map[string]string{"lanverse-schema": deadLetterSchemaV1, "lanverse-event-id": letter.EventID},
	}); err != nil {
		return fmt.Errorf("publish dead letter for event %s: %w", letter.EventID, err)
	}
	return nil
}

func (consumer *Consumer) validate() error {
	if consumer == nil || consumer.repository == nil || consumer.processor == nil || consumer.broker == nil ||
		strings.TrimSpace(consumer.config.Group) == "" || consumer.config.Now == nil || consumer.config.NewID == nil ||
		consumer.config.Lease <= 0 || consumer.config.MaxAttempts < 1 || consumer.config.MaxAttempts > 100 {
		return errors.New("event consumer configuration is invalid")
	}
	return nil
}

type permanentError struct{ cause error }

func (value permanentError) Error() string { return value.cause.Error() }
func (value permanentError) Unwrap() error { return value.cause }

func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentError{cause: err}
}

func IsPermanent(err error) bool {
	var target permanentError
	return errors.As(err, &target)
}
