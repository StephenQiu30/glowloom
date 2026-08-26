package kafka

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	eventingapp "github.com/StephenQiu30/lanverse/backend/internal/eventing/application"
)

const consumerRetryInterval = time.Second

type Config struct {
	Brokers       []string
	ClientID      string
	ConsumerGroup string
	Topics        []string
}

type Handler interface {
	Handle(context.Context, eventingapp.IncomingMessage) (eventingapp.HandleResult, error)
}

type Client struct {
	client     *kgo.Client
	canConsume bool
}

func New(config Config) (*Client, error) {
	brokers, err := cleanValues(config.Brokers)
	if err != nil || strings.TrimSpace(config.ClientID) == "" {
		return nil, errors.New("Kafka client configuration is invalid")
	}
	options := []kgo.Opt{
		kgo.SeedBrokers(brokers...), kgo.ClientID(strings.TrimSpace(config.ClientID)),
		kgo.RequiredAcks(kgo.AllISRAcks()),
	}
	canConsume := strings.TrimSpace(config.ConsumerGroup) != "" || len(config.Topics) > 0
	if canConsume {
		topics, topicErr := cleanValues(config.Topics)
		if topicErr != nil || strings.TrimSpace(config.ConsumerGroup) == "" {
			return nil, errors.New("Kafka consumer group and topics must be configured together")
		}
		options = append(options,
			kgo.ConsumerGroup(strings.TrimSpace(config.ConsumerGroup)), kgo.ConsumeTopics(topics...),
			kgo.DisableAutoCommit(), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		)
	}
	client, err := kgo.NewClient(options...)
	if err != nil {
		return nil, fmt.Errorf("create Kafka client: %w", err)
	}
	return &Client{client: client, canConsume: canConsume}, nil
}

func (client *Client) Publish(ctx context.Context, message eventingapp.Message) error {
	if client == nil || client.client == nil || strings.TrimSpace(message.Topic) == "" || message.Key == "" || len(message.Value) == 0 {
		return errors.New("Kafka message is invalid")
	}
	record := &kgo.Record{Topic: message.Topic, Key: []byte(message.Key), Value: append([]byte(nil), message.Value...)}
	for key, value := range message.Headers {
		if strings.TrimSpace(key) == "" {
			return errors.New("Kafka message header name is empty")
		}
		record.Headers = append(record.Headers, kgo.RecordHeader{Key: key, Value: []byte(value)})
	}
	if err := client.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		return fmt.Errorf("produce Kafka record: %w", err)
	}
	return nil
}

func (client *Client) ConsumeOnce(ctx context.Context, handler Handler) error {
	if err := client.validateConsumer(handler); err != nil {
		return err
	}
	record, err := client.pollOne(ctx)
	if err != nil {
		return err
	}
	return client.handleRecord(ctx, handler, record)
}

func (client *Client) pollOne(ctx context.Context) (*kgo.Record, error) {
	fetches := client.client.PollRecords(ctx, 1)
	if fetchErrs := fetches.Errors(); len(fetchErrs) > 0 {
		return nil, fmt.Errorf("poll Kafka records: %v", fetchErrs)
	}
	records := fetches.Records()
	if len(records) == 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("Kafka poll returned no record")
	}
	return records[0], nil
}

func (client *Client) handleRecord(ctx context.Context, handler Handler, record *kgo.Record) error {
	result, err := handler.Handle(ctx, eventingapp.IncomingMessage{
		Topic: record.Topic, Partition: record.Partition, Offset: record.Offset,
		Key: string(record.Key), Value: append([]byte(nil), record.Value...),
	})
	if err != nil {
		return err
	}
	if !result.Ack {
		return errors.New("Kafka record was not acknowledged")
	}
	if err = client.client.CommitRecords(ctx, record); err != nil {
		return fmt.Errorf("commit Kafka record: %w", err)
	}
	return nil
}

func (client *Client) Run(ctx context.Context, handler Handler, onRetry func(error)) error {
	if err := client.validateConsumer(handler); err != nil {
		return err
	}
	for ctx.Err() == nil {
		record, err := client.pollOne(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			waitForConsumerRetry(ctx, err, onRetry)
			continue
		}
		for ctx.Err() == nil {
			if err = client.handleRecord(ctx, handler, record); err == nil {
				break
			}
			if ctx.Err() != nil {
				return nil
			}
			waitForConsumerRetry(ctx, err, onRetry)
		}
	}
	return nil
}

func (client *Client) validateConsumer(handler Handler) error {
	if client == nil || client.client == nil || !client.canConsume || handler == nil {
		return errors.New("Kafka consumer is not configured")
	}
	return nil
}

func waitForConsumerRetry(ctx context.Context, err error, onRetry func(error)) {
	if onRetry != nil {
		onRetry(err)
	}
	timer := time.NewTimer(consumerRetryInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func (client *Client) Ping(ctx context.Context) error {
	if client == nil || client.client == nil {
		return errors.New("Kafka client is not configured")
	}
	if err := client.client.Ping(ctx); err != nil {
		return fmt.Errorf("ping Kafka broker: %w", err)
	}
	return nil
}

func (client *Client) Close() {
	if client != nil && client.client != nil {
		client.client.Close()
	}
}

func cleanValues(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, errors.New("value list is empty")
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, errors.New("value list contains an empty item")
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}
