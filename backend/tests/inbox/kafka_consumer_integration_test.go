package inbox_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"

	kafkainbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/kafka"
	"github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
)

type recordHandler func(context.Context, application.Record) error

func (f recordHandler) Handle(ctx context.Context, record application.Record) error {
	return f(ctx, record)
}

func TestKafkaConsumerCommitsOnlyAfterHandling(t *testing.T) {
	brokers := os.Getenv("LV_TEST_KAFKA_BROKERS")
	topic := os.Getenv("LV_TEST_INBOX_KAFKA_TOPIC")
	if brokers == "" || topic == "" {
		t.Skip("set LV_TEST_KAFKA_BROKERS and a disposable LV_TEST_INBOX_KAFKA_TOPIC")
	}
	producer, err := kgo.NewClient(kgo.SeedBrokers(strings.Split(brokers, ",")...))
	if err != nil {
		t.Fatalf("open Kafka producer: %v", err)
	}
	t.Cleanup(producer.Close)
	group := "lanverse-test-inbox-" + uuid.NewString()
	value := []byte(uuid.NewString())
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if err := producer.ProduceSync(ctx, &kgo.Record{Topic: topic, Key: []byte("project"), Value: value}).FirstErr(); err != nil {
		t.Fatalf("produce: %v", err)
	}

	fail := errors.New("effect unavailable")
	failed, err := kafkainbox.NewConsumer(brokers, group, []string{topic}, recordHandler(func(_ context.Context, record application.Record) error {
		if string(record.Value) != string(value) {
			t.Errorf("consumed value = %q", record.Value)
		}
		return fail
	}))
	if err != nil {
		t.Fatalf("new failing consumer: %v", err)
	}
	if applied, err := failed.RunOnce(ctx); applied || !errors.Is(err, fail) {
		t.Fatalf("failed record = (%t, %v)", applied, err)
	}
	failed.Close()

	calls := 0
	retry, err := kafkainbox.NewConsumer(brokers, group, []string{topic}, recordHandler(func(_ context.Context, record application.Record) error {
		calls++
		if record.Topic != topic || string(record.Key) != "project" || string(record.Value) != string(value) {
			t.Errorf("retried record = %+v", record)
		}
		return nil
	}))
	if err != nil {
		t.Fatalf("new retry consumer: %v", err)
	}
	if applied, err := retry.RunOnce(ctx); !applied || err != nil {
		t.Fatalf("retried record = (%t, %v)", applied, err)
	}
	retry.Close()
	if calls != 1 {
		t.Fatalf("retry handler calls = %d, want 1", calls)
	}

	committed, err := kafkainbox.NewConsumer(brokers, group, []string{topic}, recordHandler(func(context.Context, application.Record) error {
		t.Error("committed Kafka record replayed")
		return nil
	}))
	if err != nil {
		t.Fatalf("new verification consumer: %v", err)
	}
	defer committed.Close()
	verifyCtx, verifyCancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer verifyCancel()
	if applied, err := committed.RunOnce(verifyCtx); applied || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("after commit = (%t, %v), want timeout without record", applied, err)
	}
}
