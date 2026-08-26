package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	eventing "github.com/StephenQiu30/lanverse/backend/internal/eventing/domain"
)

type Message struct {
	Topic   string
	Key     string
	Value   []byte
	Headers map[string]string
}

type ClaimedOutbox struct {
	Event      eventing.OutboxEvent
	ClaimToken string
}

type OutboxRepository interface {
	ClaimPending(context.Context, time.Time, time.Duration, int, func() string) ([]ClaimedOutbox, error)
	MarkPublished(context.Context, string, string, time.Time) error
	ReleaseClaim(context.Context, string, string, string) error
}

type PublisherBroker interface {
	Publish(context.Context, Message) error
}

type TopicRouter interface {
	TopicFor(string) (string, error)
}

type StaticTopics map[string]string

func (topics StaticTopics) TopicFor(eventType string) (string, error) {
	topic := strings.TrimSpace(topics[eventType])
	if topic == "" {
		return "", fmt.Errorf("event type %q has no business topic", eventType)
	}
	return topic, nil
}

type PublisherConfig struct {
	Now       func() time.Time
	NewID     func() string
	Lease     time.Duration
	BatchSize int
}

type Publisher struct {
	repository OutboxRepository
	broker     PublisherBroker
	topics     TopicRouter
	config     PublisherConfig
}

func NewPublisher(repository OutboxRepository, broker PublisherBroker, topics TopicRouter, config PublisherConfig) *Publisher {
	return &Publisher{repository: repository, broker: broker, topics: topics, config: config}
}

func (publisher *Publisher) PublishOnce(ctx context.Context) (int, error) {
	if publisher == nil || publisher.repository == nil || publisher.broker == nil || publisher.topics == nil ||
		publisher.config.Now == nil || publisher.config.NewID == nil || publisher.config.Lease <= 0 ||
		publisher.config.BatchSize < 1 || publisher.config.BatchSize > 200 {
		return 0, errors.New("event publisher configuration is invalid")
	}
	now := publisher.config.Now().UTC()
	claimed, err := publisher.repository.ClaimPending(
		ctx, now, publisher.config.Lease, publisher.config.BatchSize, publisher.config.NewID,
	)
	if err != nil {
		return 0, fmt.Errorf("claim outbox events: %w", err)
	}
	published := 0
	for _, claim := range claimed {
		if err = publisher.publishClaim(ctx, now, claim); err != nil {
			return published, err
		}
		published++
	}
	return published, nil
}

func (publisher *Publisher) publishClaim(ctx context.Context, now time.Time, claim ClaimedOutbox) error {
	topic, err := publisher.topics.TopicFor(claim.Event.EventType)
	if err != nil {
		return publisher.release(ctx, claim, err)
	}
	envelope, err := eventing.NewEnvelope(claim.Event, eventing.TraceContext{RequestID: claim.Event.SourceReceiptID})
	if err != nil {
		return publisher.release(ctx, claim, fmt.Errorf("build event envelope: %w", err))
	}
	encoded, err := eventing.EncodeEnvelope(envelope)
	if err != nil {
		return publisher.release(ctx, claim, err)
	}
	message := Message{
		Topic: topic, Key: envelope.EventID, Value: encoded,
		Headers: map[string]string{
			"lanverse-schema":     envelope.Schema,
			"lanverse-event-id":   envelope.EventID,
			"lanverse-event-type": envelope.EventType,
		},
	}
	if err = publisher.broker.Publish(ctx, message); err != nil {
		return publisher.release(ctx, claim, fmt.Errorf("publish event %s: %w", envelope.EventID, err))
	}
	if err = publisher.repository.MarkPublished(ctx, envelope.EventID, claim.ClaimToken, now); err != nil {
		return fmt.Errorf("mark event %s published: %w", envelope.EventID, err)
	}
	return nil
}

func (publisher *Publisher) release(ctx context.Context, claim ClaimedOutbox, publishErr error) error {
	releaseErr := publisher.repository.ReleaseClaim(ctx, claim.Event.ID, claim.ClaimToken, safeError(publishErr))
	if releaseErr != nil {
		return errors.Join(publishErr, fmt.Errorf("release event %s claim: %w", claim.Event.ID, releaseErr))
	}
	return publishErr
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 500 {
		message = message[:500]
	}
	return message
}
