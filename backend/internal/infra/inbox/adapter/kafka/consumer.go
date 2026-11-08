// Package kafka consumes one event at a time and commits only after handling.
package kafka

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
)

// ErrInvalidConsumer means the group, topics, brokers, or handler is missing.
var ErrInvalidConsumer = errors.New("invalid Kafka consumer configuration")

// Consumer owns its Kafka client and a single record handler.
type Consumer struct {
	client        *kgo.Client
	handler       application.Handler
	failurePolicy *FailurePolicy
}

// FailurePolicy retries handler failures before durably parking the source
// record in a Kafka dead-letter topic. Offsets are committed only after either
// the handler succeeds or the dead-letter publish succeeds.
type FailurePolicy struct {
	MaxAttempts     int
	DeadLetterTopic string
	RetryBase       time.Duration
	RetryCap        time.Duration
}

// NewConsumer joins a manual-commit consumer group for the supplied topics.
func NewConsumer(brokers, group string, topics []string, handler application.Handler) (*Consumer, error) {
	return newConsumer(brokers, group, topics, handler, nil)
}

// NewRetryingConsumer constructs a consumer with bounded retries and a durable
// dead-letter topic for records whose handler keeps failing.
func NewRetryingConsumer(brokers, group string, topics []string, handler application.Handler, policy FailurePolicy) (*Consumer, error) {
	if policy.MaxAttempts < 1 || policy.DeadLetterTopic == "" || policy.RetryBase <= 0 || policy.RetryCap < policy.RetryBase {
		return nil, ErrInvalidConsumer
	}
	return newConsumer(brokers, group, topics, handler, &policy)
}

func newConsumer(brokers, group string, topics []string, handler application.Handler, policy *FailurePolicy) (*Consumer, error) {
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
	return &Consumer{client: client, handler: handler, failurePolicy: policy}, nil
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
	if err := c.handleRecord(ctx, record); err != nil {
		return false, fmt.Errorf("handle Kafka record %s[%d]@%d: %w", record.Topic, record.Partition, record.Offset, err)
	}
	if err := c.client.CommitRecords(ctx, record); err != nil {
		return false, fmt.Errorf("commit Kafka record %s[%d]@%d: %w", record.Topic, record.Partition, record.Offset, err)
	}
	return true, nil
}

func (c *Consumer) handleRecord(ctx context.Context, record *kgo.Record) error {
	if c.failurePolicy == nil {
		return c.handler.Handle(ctx, application.Record{Topic: record.Topic, Key: record.Key, Value: record.Value})
	}
	var handlerErr error
	for attempt := 1; attempt <= c.failurePolicy.MaxAttempts; attempt++ {
		handlerErr = c.handler.Handle(ctx, application.Record{Topic: record.Topic, Key: record.Key, Value: record.Value})
		if handlerErr == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt == c.failurePolicy.MaxAttempts {
			break
		}
		if err := waitRetry(ctx, retryDelay(*c.failurePolicy, attempt)); err != nil {
			return err
		}
	}
	deadLetter, err := encodeDeadLetter(record, c.failurePolicy.MaxAttempts, classifyHandlerFailure(handlerErr))
	if err != nil {
		return fmt.Errorf("encode Kafka dead-letter record: %w", err)
	}
	payload, err := deadLetter.Payload()
	if err != nil {
		return err
	}
	result := c.client.ProduceSync(ctx, &kgo.Record{
		Topic:   c.failurePolicy.DeadLetterTopic,
		Key:     []byte(deadLetter.FailureID),
		Value:   payload,
		Headers: []kgo.RecordHeader{{Key: "error_class", Value: []byte(deadLetter.ErrorClass)}},
	})
	if err := result.FirstErr(); err != nil {
		return fmt.Errorf("publish Kafka dead-letter record: %w", err)
	}
	return nil
}

type deadLetterEnvelope struct {
	FailureID       string    `json:"failure_id"`
	SourceTopic     string    `json:"source_topic"`
	SourcePartition int32     `json:"source_partition"`
	SourceOffset    int64     `json:"source_offset"`
	Attempts        int       `json:"attempts"`
	ErrorClass      string    `json:"error_class"`
	FailedAt        time.Time `json:"failed_at"`
	KeyBase64       string    `json:"key_base64"`
	ValueBase64     string    `json:"value_base64"`
}

func encodeDeadLetter(record *kgo.Record, attempts int, errorClass string) (deadLetterEnvelope, error) {
	if record == nil || record.Topic == "" || record.Partition < 0 || record.Offset < 0 || attempts < 1 || errorClass == "" {
		return deadLetterEnvelope{}, ErrInvalidConsumer
	}
	failureID := fmt.Sprintf("%s:%d:%d", record.Topic, record.Partition, record.Offset)
	envelope := deadLetterEnvelope{
		FailureID: failureID, SourceTopic: record.Topic, SourcePartition: record.Partition,
		SourceOffset: record.Offset, Attempts: attempts, ErrorClass: errorClass,
		FailedAt: time.Now().UTC(), KeyBase64: base64.RawStdEncoding.EncodeToString(record.Key),
		ValueBase64: base64.RawStdEncoding.EncodeToString(record.Value),
	}
	return envelope, nil
}

func (envelope deadLetterEnvelope) Payload() ([]byte, error) {
	value, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshal Kafka dead-letter envelope: %w", err)
	}
	return value, nil
}

func classifyHandlerFailure(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "handler_timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "handler_canceled"
	}
	return "handler_failed"
}

func retryDelay(policy FailurePolicy, attempt int) time.Duration {
	delay := policy.RetryBase
	for retry := 1; retry < attempt && delay < policy.RetryCap; retry++ {
		delay *= 2
	}
	if delay > policy.RetryCap {
		return policy.RetryCap
	}
	return delay
}

func waitRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
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
