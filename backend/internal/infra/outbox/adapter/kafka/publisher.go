// Package kafka publishes committed Outbox events using their project key.
package kafka

import (
	"context"
	"fmt"
	"slices"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/application"
)

// Publisher sends one event and waits for the broker acknowledgement.
type Publisher struct {
	client *kgo.Client
}

// NewPublisher constructs a publisher using a caller-owned Kafka client.
func NewPublisher(client *kgo.Client) *Publisher {
	return &Publisher{client: client}
}

// Publish preserves the event topic, project partition key, payload, and headers.
func (p *Publisher) Publish(ctx context.Context, event application.Event) error {
	keys := make([]string, 0, len(event.Headers))
	for key := range event.Headers {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	headers := make([]kgo.RecordHeader, 0, len(keys))
	for _, key := range keys {
		headers = append(headers, kgo.RecordHeader{Key: key, Value: []byte(event.Headers[key])})
	}
	record := &kgo.Record{
		Topic:   event.Topic,
		Key:     []byte(event.PartitionKey),
		Value:   []byte(event.Payload),
		Headers: headers,
	}
	if err := p.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		return fmt.Errorf("publish outbox event %s: %w", event.ID, err)
	}
	return nil
}

var _ application.Publisher = (*Publisher)(nil)
