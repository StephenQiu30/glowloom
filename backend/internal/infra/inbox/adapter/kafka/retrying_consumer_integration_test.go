package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
)

type testRecordHandler func(context.Context, application.Record) error

func (h testRecordHandler) Handle(ctx context.Context, record application.Record) error {
	return h(ctx, record)
}

func TestRetryingConsumerDeadLettersAfterBoundedHandlerRetries(t *testing.T) {
	brokers := os.Getenv("GLOWLOOM_TEST_KAFKA_BROKERS")
	sourceTopic := os.Getenv("GLOWLOOM_TEST_KAFKA_SOURCE_TOPIC")
	deadLetterTopic := os.Getenv("GLOWLOOM_TEST_KAFKA_DEAD_LETTER_TOPIC")
	if brokers == "" || sourceTopic == "" || deadLetterTopic == "" {
		t.Skip("set GLOWLOOM_TEST_KAFKA_BROKERS and isolated source and dead-letter topics")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	var attempts atomic.Int32
	consumer, err := NewRetryingConsumer(brokers, "credit-dlq-test-"+uuid.NewString(), []string{sourceTopic}, testRecordHandler(func(context.Context, application.Record) error {
		attempts.Add(1)
		return errors.New("private handler detail must not enter the dead-letter record")
	}), FailurePolicy{
		MaxAttempts: 3, DeadLetterTopic: deadLetterTopic,
		RetryBase: 10 * time.Millisecond, RetryCap: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("create retrying Kafka consumer: %v", err)
	}
	t.Cleanup(consumer.Close)

	producer, err := kgo.NewClient(kgo.SeedBrokers(splitBrokers(brokers)...))
	if err != nil {
		t.Fatalf("create Kafka producer: %v", err)
	}
	t.Cleanup(producer.Close)
	key, value := []byte("scope-test"), []byte(`{"event_id":"credit-test"}`)
	if err := producer.ProduceSync(ctx, &kgo.Record{Topic: sourceTopic, Key: key, Value: value}).FirstErr(); err != nil {
		t.Fatalf("publish source test record: %v", err)
	}
	processed, err := consumer.RunOnce(ctx)
	if err != nil || !processed {
		t.Fatalf("process failed record into DLQ = (%t, %v)", processed, err)
	}
	if actual := attempts.Load(); actual != 3 {
		t.Fatalf("handler attempts = %d, want 3", actual)
	}

	var deadLetterValue []byte
	deadLetterConsumer, err := NewConsumer(brokers, "credit-dlq-read-test-"+uuid.NewString(), []string{deadLetterTopic}, testRecordHandler(func(_ context.Context, record application.Record) error {
		deadLetterValue = append([]byte(nil), record.Value...)
		return nil
	}))
	if err != nil {
		t.Fatalf("create dead-letter reader: %v", err)
	}
	t.Cleanup(deadLetterConsumer.Close)
	processed, err = deadLetterConsumer.RunOnce(ctx)
	if err != nil || !processed {
		t.Fatalf("read dead-letter record = (%t, %v)", processed, err)
	}
	var envelope deadLetterEnvelope
	if err := json.Unmarshal(deadLetterValue, &envelope); err != nil {
		t.Fatalf("decode dead-letter record: %v", err)
	}
	if envelope.SourceTopic != sourceTopic || envelope.ErrorClass != "handler_failed" || envelope.Attempts != 3 || string(mustDecodeBase64(t, envelope.KeyBase64)) != string(key) || string(mustDecodeBase64(t, envelope.ValueBase64)) != string(value) {
		t.Fatalf("dead-letter envelope = %+v", envelope)
	}
	if strings.Contains(string(deadLetterValue), "private handler detail") {
		t.Fatal("dead-letter record exposed the handler error text")
	}
}

func splitBrokers(value string) []string {
	return strings.Split(value, ",")
}
