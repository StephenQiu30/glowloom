package audit_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/trace/noop"

	auditevent "github.com/StephenQiu30/lanverse/backend/internal/audit/adapter/event"
	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	kafkainbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/kafka"
	pginbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/postgres"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

type auditHandlerFunc func(context.Context, inbox.Record) error

func (f auditHandlerFunc) Handle(ctx context.Context, record inbox.Record) error {
	return f(ctx, record)
}

func TestAuditKafkaRetriesThenCommitsAfterDatabaseWrite(t *testing.T) {
	dsn := os.Getenv("LV_TEST_AUDIT_DB_DSN")
	brokers := os.Getenv("LV_TEST_AUDIT_KAFKA_BROKERS")
	if dsn == "" || brokers == "" {
		t.Skip("set LV_TEST_AUDIT_DB_DSN and LV_TEST_AUDIT_KAFKA_BROKERS with a disposable audit topic")
	}
	const topic = "lanverse.audit.recorded.v1"
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	producer, err := kgo.NewClient(kgo.SeedBrokers(strings.Split(brokers, ",")...))
	if err != nil {
		t.Fatalf("open Kafka producer: %v", err)
	}
	t.Cleanup(producer.Close)

	eventID := uuid.NewString()
	orgID := uuid.NewString()
	projectID := uuid.NewString()
	record := auditRecord(t, eventID, orgID, projectID, uuid.NewString(), time.Now().UTC(), map[string]any{"limit_micros": 100})
	if err := producer.ProduceSync(ctx, &kgo.Record{Topic: topic, Key: record.Key, Value: record.Value}).FirstErr(); err != nil {
		t.Fatalf("produce audit event: %v", err)
	}
	group := "lanverse-test-audit-" + uuid.NewString()
	wantErr := errors.New("audit database temporarily unavailable")
	failed, err := kafkainbox.NewConsumer(brokers, group, []string{topic}, auditHandlerFunc(func(context.Context, inbox.Record) error {
		return wantErr
	}))
	if err != nil {
		t.Fatalf("create failing audit consumer: %v", err)
	}
	if applied, err := failed.RunOnce(ctx); applied || !errors.Is(err, wantErr) {
		t.Fatalf("failed audit consume = (%t, %v)", applied, err)
	}
	failed.Close()

	parser := auditapp.NewParser(map[string][]string{"budget.changed": {"limit_micros"}})
	handler := auditevent.NewHandler(pginbox.NewStore(conn.DB), parser)
	retry, err := kafkainbox.NewConsumer(brokers, group, []string{topic}, handler)
	if err != nil {
		t.Fatalf("create retry audit consumer: %v", err)
	}
	if applied, err := retry.RunOnce(ctx); !applied || err != nil {
		t.Fatalf("retry audit consume = (%t, %v)", applied, err)
	}
	retry.Close()
	var auditRows, markers int64
	if err := conn.DB.WithContext(ctx).Raw("SELECT count(*) FROM audit.audit_log WHERE id = ?::uuid", eventID).Scan(&auditRows).Error; err != nil || auditRows != 1 {
		t.Fatalf("audit rows after Kafka retry = %d, error = %v", auditRows, err)
	}
	if err := conn.DB.WithContext(ctx).Raw("SELECT count(*) FROM infra.processed_event WHERE consumer = 'audit' AND event_id = ?::uuid", eventID).Scan(&markers).Error; err != nil || markers != 1 {
		t.Fatalf("markers after Kafka retry = %d, error = %v", markers, err)
	}

	committed, err := kafkainbox.NewConsumer(brokers, group, []string{topic}, auditHandlerFunc(func(context.Context, inbox.Record) error {
		t.Error("committed audit record replayed")
		return nil
	}))
	if err != nil {
		t.Fatalf("create committed audit consumer: %v", err)
	}
	defer committed.Close()
	verifyCtx, verifyCancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer verifyCancel()
	if applied, err := committed.RunOnce(verifyCtx); applied || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("after audit offset commit = (%t, %v), want timeout", applied, err)
	}
}
