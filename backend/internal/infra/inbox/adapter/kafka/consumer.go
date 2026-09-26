// Package kafka consumes one event at a time and commits only after handling.
package kafka

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
)

// ErrInvalidConsumer means the group, topics, brokers, or handler is missing.
var ErrInvalidConsumer = errors.New("invalid Kafka consumer configuration")

// Consumer owns its Kafka client and a single record handler.
type Consumer struct {
	client  *kgo.Client
	handler application.Handler
}

// NewConsumer joins a manual-commit consumer group for the supplied topics.
func NewConsumer(brokers, group string, topics []string, handler application.Handler) (*Consumer, error) {
	if strings.TrimSpace(brokers) == "" || strings.TrimSpace(group) == "" || len(topics) == 0 || handler == nil {
		return nil, ErrInvalidConsumer
	}
	for _, topic := range topics {
		if strings.TrimSpace(topic) == "" {
			return nil, ErrInvalidConsumer
		}
	}
	client, err := kgo.NewClient(
		kgo.SeedBrokers(strings.Split(brokers, ",")...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topics...),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.DisableAutoCommit(),
		kgo.BlockRebalanceOnPoll(),
	)
	if err != nil {
		return nil, fmt.Errorf("create Kafka consumer: %w", err)
	}
	return &Consumer{client: client, handler: handler}, nil
}

// RunOnce handles and commits at most one record. Errors leave the offset uncommitted.
func (c *Consumer) RunOnce(ctx context.Context) (bool, error) {
	fetches := c.client.PollRecords(ctx, 1)
	defer c.client.AllowRebalance()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	for _, fetchErr := range fetches.Errors() {
		return false, fmt.Errorf("poll Kafka topic %s: %w", fetchErr.Topic, fetchErr.Err)
	}
	records := fetches.Records()
	if len(records) == 0 {
		return false, nil
	}
	record := records[0]
	if err := c.handler.Handle(ctx, application.Record{Topic: record.Topic, Key: record.Key, Value: record.Value}); err != nil {
		return false, fmt.Errorf("handle Kafka record %s[%d]@%d: %w", record.Topic, record.Partition, record.Offset, err)
	}
	if err := c.client.CommitRecords(ctx, record); err != nil {
		return false, fmt.Errorf("commit Kafka record %s[%d]@%d: %w", record.Topic, record.Partition, record.Offset, err)
	}
	return true, nil
}

// Run consumes until cancellation or the first failed record.
func (c *Consumer) Run(ctx context.Context) error {
	for {
		if _, err := c.RunOnce(ctx); err != nil {
			if errors.Is(err, context.Canceled) && ctx.Err() != nil {
				return nil
			}
			return err
		}
	}
}

// Close releases Kafka connections and group membership.
func (c *Consumer) Close() {
	c.client.Close()
}
