package outbox_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/trace/noop"

	kafkaoutbox "github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/adapter/kafka"
	pgoutbox "github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/infra/outbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/kafkaconn"
)

func TestOutboxRelayWithHostPostgresAndKafka(t *testing.T) {
	dsn := os.Getenv("LV_TEST_OUTBOX_DB_DSN")
	brokers := os.Getenv("LV_TEST_KAFKA_BROKERS")
	topic := os.Getenv("LV_TEST_OUTBOX_KAFKA_TOPIC")
	if dsn == "" || brokers == "" || topic == "" {
		t.Skip("set LV_TEST_OUTBOX_DB_DSN, LV_TEST_KAFKA_BROKERS, and a disposable LV_TEST_OUTBOX_KAFKA_TOPIC")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	dbConn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })
	kafkaConn, err := kafkaconn.Open(brokers)
	if err != nil {
		t.Fatalf("open Kafka: %v", err)
	}
	t.Cleanup(kafkaConn.Close)
	reader, err := kgo.NewClient(
		kgo.SeedBrokers(strings.Split(brokers, ",")...),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		t.Fatalf("open Kafka reader: %v", err)
	}
	t.Cleanup(reader.Close)

	id, projectID := uuid.NewString(), uuid.NewString()
	payload := []byte(`{"event_id":"` + id + `","event_type":"` + topic + `","project_id":"` + projectID + `"}`)
	if err := dbConn.DB.WithContext(ctx).Exec(
		"INSERT INTO infra.outbox(id, topic, partition_key, payload, headers) VALUES (?::uuid, ?, ?, ?::jsonb, ?::jsonb)",
		id, topic, projectID, string(payload), `{"traceparent":"test-trace"}`,
	).Error; err != nil {
		t.Fatalf("insert outbox event: %v", err)
	}
	t.Cleanup(func() {
		if err := dbConn.DB.Exec("DELETE FROM infra.outbox WHERE id = ?::uuid", id).Error; err != nil {
			t.Errorf("remove outbox event: %v", err)
		}
	})

	relay := application.NewRelay(pgoutbox.NewStore(dbConn.DB), kafkaoutbox.NewPublisher(kafkaConn.Client))
	if sent, err := relay.RunOnce(ctx); !sent || err != nil {
		t.Fatalf("relay event = (%t, %v)", sent, err)
	}
	var got *kgo.Record
	for got == nil && ctx.Err() == nil {
		fetches := reader.PollFetches(ctx)
		fetches.EachRecord(func(record *kgo.Record) {
			var envelope struct {
				EventID string `json:"event_id"`
			}
			if json.Unmarshal(record.Value, &envelope) == nil && envelope.EventID == id {
				got = record
			}
		})
		for _, fetchErr := range fetches.Errors() {
			if ctx.Err() == nil {
				t.Fatalf("consume relayed event: %v", fetchErr.Err)
			}
		}
	}
	if got == nil || string(got.Key) != projectID {
		t.Fatalf("relayed Kafka record = %+v, want project key %s", got, projectID)
	}
	var envelope struct {
		EventID   string `json:"event_id"`
		EventType string `json:"event_type"`
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(got.Value, &envelope); err != nil ||
		envelope.EventID != id || envelope.EventType != topic || envelope.ProjectID != projectID {
		t.Fatalf("relayed event envelope = %+v, error = %v", envelope, err)
	}
	var pending bool
	if err := dbConn.DB.WithContext(ctx).Raw(
		"SELECT published_at IS NULL FROM infra.outbox WHERE id = ?::uuid", id,
	).Scan(&pending).Error; err != nil || pending {
		t.Fatalf("relayed event pending = %t, error = %v", pending, err)
	}
}
