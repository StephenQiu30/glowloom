package outbox_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	kafkaoutbox "github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/adapter/kafka"
	"github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/kafkaconn"
)

func TestKafkaPublisherPreservesEventContract(t *testing.T) {
	brokers := os.Getenv("LV_TEST_KAFKA_BROKERS")
	topic := os.Getenv("LV_TEST_OUTBOX_KAFKA_TOPIC")
	if brokers == "" || topic == "" {
		t.Skip("set LV_TEST_KAFKA_BROKERS and a disposable LV_TEST_OUTBOX_KAFKA_TOPIC")
	}
	conn, err := kafkaconn.Open(brokers)
	if err != nil {
		t.Fatalf("open Kafka client: %v", err)
	}
	t.Cleanup(conn.Close)
	reader, err := kgo.NewClient(
		kgo.SeedBrokers(strings.Split(brokers, ",")...),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		t.Fatalf("open Kafka reader: %v", err)
	}
	t.Cleanup(reader.Close)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	event := application.Event{
		ID:           "test-event",
		Topic:        topic,
		PartitionKey: "test-project",
		Payload:      []byte(`{"event_id":"test-event"}`),
		Headers:      map[string]string{"traceparent": "test-trace"},
	}
	if err := kafkaoutbox.NewPublisher(conn.Client).Publish(ctx, event); err != nil {
		t.Fatalf("publish event: %v", err)
	}

	var got *kgo.Record
	for got == nil && ctx.Err() == nil {
		fetches := reader.PollFetches(ctx)
		fetches.EachRecord(func(record *kgo.Record) {
			if bytes.Equal(record.Value, event.Payload) {
				got = record
			}
		})
		for _, fetchErr := range fetches.Errors() {
			if ctx.Err() == nil {
				t.Fatalf("consume Kafka event: %v", fetchErr.Err)
			}
		}
	}
	if got == nil {
		t.Fatal("published event was not consumed")
	}
	if got.Topic != topic || string(got.Key) != event.PartitionKey {
		t.Fatalf("Kafka route = (%q, %q), want (%q, %q)", got.Topic, got.Key, topic, event.PartitionKey)
	}
	if len(got.Headers) != 1 || got.Headers[0].Key != "traceparent" || string(got.Headers[0].Value) != "test-trace" {
		t.Fatalf("Kafka headers = %+v", got.Headers)
	}
}
