package outbox_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	pgoutbox "github.com/StephenQiu30/glowloom-server/backend/internal/infra/outbox/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

func TestOutboxDeadLetterReplayIsAuditedAndUpdatesLatestFailure(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_OUTBOX_DB_DSN")
	if dsn == "" {
		t.Skip("set GLOWLOOM_TEST_OUTBOX_DB_DSN to an isolated PostgreSQL database with the schema applied")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	connection, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open Outbox database: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })

	eventID, operatorID := uuid.New(), uuid.New()
	createdAt := time.Now().UTC().Truncate(time.Microsecond)
	topic := "glowloom.credit.dead_letter_test.v1"
	payload, err := json.Marshal(map[string]string{"event_id": eventID.String(), "event_type": topic})
	if err != nil {
		t.Fatalf("encode test event: %v", err)
	}
	if err := connection.DB.WithContext(ctx).Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload,create_time)
		VALUES (?::uuid,?,?,?::jsonb,?)`, eventID, topic, "credit-test", string(payload), createdAt).Error; err != nil {
		t.Fatalf("insert Outbox event: %v", err)
	}
	store := pgoutbox.NewStore(connection.DB)
	event, claimed, err := store.ClaimNext(ctx, createdAt.Add(time.Second), 30*time.Second)
	if err != nil || !claimed || event.ID != eventID.String() {
		t.Fatalf("claim test event = (%+v, %t, %v)", event, claimed, err)
	}
	dead, err := store.Retry(ctx, event, "broker_publish_failed", 1, createdAt.Add(2*time.Second), createdAt.Add(2*time.Second))
	if err != nil || !dead {
		t.Fatalf("dead-letter first failure = (%t, %v)", dead, err)
	}
	if err := store.ReplayDeadLetter(ctx, eventID, createdAt, operatorID, "broker access restored", createdAt.Add(3*time.Second)); err != nil {
		t.Fatalf("replay dead-letter: %v", err)
	}
	event, claimed, err = store.ClaimNext(ctx, createdAt.Add(4*time.Second), 30*time.Second)
	if err != nil || !claimed || event.ID != eventID.String() {
		t.Fatalf("claim replayed event = (%+v, %t, %v)", event, claimed, err)
	}
	dead, err = store.Retry(ctx, event, "broker_timeout", 1, createdAt.Add(5*time.Second), createdAt.Add(5*time.Second))
	if err != nil || !dead {
		t.Fatalf("dead-letter replayed failure = (%t, %v)", dead, err)
	}
	var latest struct {
		Attempts   int
		ErrorClass string
		ReplayedAt *time.Time
	}
	if err := connection.DB.WithContext(ctx).Raw(`SELECT attempt_count AS attempts,error_class,replayed_at
		FROM infra.outbox_dead_letter WHERE event_id=?::uuid AND event_create_time=?`, eventID, createdAt).Scan(&latest).Error; err != nil {
		t.Fatalf("read latest dead-letter state: %v", err)
	}
	if latest.Attempts != 1 || latest.ErrorClass != "broker_timeout" || latest.ReplayedAt != nil {
		t.Fatalf("latest dead-letter facts = %+v", latest)
	}
	var auditCount int64
	if err := connection.DB.WithContext(ctx).Raw(`SELECT count(*) FROM infra.outbox_dead_letter_audit
		WHERE event_id=?::uuid AND event_create_time=? AND operator_id=?::uuid`, eventID, createdAt, operatorID).Scan(&auditCount).Error; err != nil || auditCount != 1 {
		t.Fatalf("dead-letter replay audits = %d, error = %v", auditCount, err)
	}
}
