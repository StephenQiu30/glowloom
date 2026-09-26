package audit_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	auditevent "github.com/StephenQiu30/lanverse/backend/internal/audit/adapter/event"
	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	pginbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/adapter/postgres"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestAuditConsumerDeduplicatesAndRollsBack(t *testing.T) {
	dsn := os.Getenv("LV_TEST_AUDIT_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_AUDIT_DB_DSN to a disposable database with inbox and audit migrations applied")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	tx := conn.DB.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatalf("begin fixture transaction: %v", tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })
	parser := auditapp.NewParser(map[string][]string{
		"budget.changed": {"limit_micros"},
	})
	handler := auditevent.NewHandler(pginbox.NewStore(tx), parser)

	orgID := uuid.NewString()
	projectID := uuid.NewString()
	actorID := uuid.NewString()
	when := time.Now().UTC().Truncate(time.Microsecond)
	eventID := uuid.NewString()
	record := auditRecord(t, eventID, orgID, projectID, actorID, when, map[string]any{"limit_micros": 100})
	if err := handler.Handle(ctx, record); err != nil {
		t.Fatalf("handle audit event: %v", err)
	}
	if err := handler.Handle(ctx, record); err != nil {
		t.Fatalf("handle duplicate audit event: %v", err)
	}
	var rows, markers int64
	if err := tx.Raw("SELECT count(*) FROM audit.audit_log WHERE id = ?::uuid", eventID).Scan(&rows).Error; err != nil || rows != 1 {
		t.Fatalf("audit rows = %d, error = %v", rows, err)
	}
	if err := tx.Raw("SELECT count(*) FROM infra.processed_event WHERE consumer = 'audit' AND event_id = ?::uuid", eventID).Scan(&markers).Error; err != nil || markers != 1 {
		t.Fatalf("processed markers = %d, error = %v", markers, err)
	}
	var traceID string
	if err := tx.Raw("SELECT trace_id FROM audit.audit_log WHERE id = ?::uuid", eventID).Scan(&traceID).Error; err != nil || traceID != "11111111111111111111111111111111" {
		t.Fatalf("audit trace ID = %q, error = %v", traceID, err)
	}

	unsafeID := uuid.NewString()
	unsafeRecord := auditRecord(t, unsafeID, orgID, projectID, actorID, when, map[string]any{"secret_key": "do-not-store"})
	if err := handler.Handle(ctx, unsafeRecord); !errors.Is(err, auditapp.ErrInvalidEvent) {
		t.Fatalf("unsafe audit event error = %v", err)
	}
	if err := tx.Raw("SELECT count(*) FROM infra.processed_event WHERE consumer = 'audit' AND event_id = ?::uuid", unsafeID).Scan(&markers).Error; err != nil || markers != 0 {
		t.Fatalf("unsafe event marker = %d, error = %v", markers, err)
	}

	collisionID := uuid.NewString()
	if err := tx.Exec(`
		INSERT INTO audit.audit_log(id, org_id, actor_kind, action, object_type, object_id, create_time)
		VALUES (?::uuid, ?::uuid, 'system', 'budget.changed', 'budget', 'existing', ?)
	`, collisionID, orgID, when).Error; err != nil {
		t.Fatalf("insert collision fixture: %v", err)
	}
	collisionRecord := auditRecord(t, collisionID, orgID, projectID, actorID, when, map[string]any{"limit_micros": 200})
	if err := handler.Handle(ctx, collisionRecord); err == nil {
		t.Fatal("audit insert collision unexpectedly succeeded")
	}
	if err := tx.Raw("SELECT count(*) FROM infra.processed_event WHERE consumer = 'audit' AND event_id = ?::uuid", collisionID).Scan(&markers).Error; err != nil || markers != 0 {
		t.Fatalf("marker after audit insert rollback = %d, error = %v", markers, err)
	}

	orgEventID := uuid.NewString()
	orgRecord := auditRecord(t, orgEventID, orgID, "", "", when, map[string]any{"limit_micros": 300})
	if err := handler.Handle(ctx, orgRecord); err != nil {
		t.Fatalf("handle organization-scoped audit event: %v", err)
	}
	var projectMissing bool
	if err := tx.Raw("SELECT project_id IS NULL FROM audit.audit_log WHERE id = ?::uuid", orgEventID).Scan(&projectMissing).Error; err != nil || !projectMissing {
		t.Fatalf("organization event project missing = %t, error = %v", projectMissing, err)
	}
}

func auditRecord(t *testing.T, eventID, orgID, projectID, actorID string, when time.Time, after map[string]any) inbox.Record {
	t.Helper()
	actor := map[string]any{"kind": "user", "id": actorID}
	key := projectID
	if projectID == "" {
		actor = map[string]any{"kind": "system", "id": nil}
		key = orgID
	}
	body := map[string]any{
		"event_id": eventID, "event_type": "lanverse.audit.recorded.v1",
		"occurred_at": when.Format(time.RFC3339Nano), "org_id": orgID,
		"project_id": projectID, "actor": actor,
		"aggregate": map[string]any{"type": "audit", "id": eventID},
		"trace":     map[string]any{"traceparent": "00-11111111111111111111111111111111-2222222222222222-01"},
		"data": map[string]any{
			"action": "budget.changed", "object": map[string]any{"type": "budget", "id": orgID},
			"before": map[string]any{"limit_micros": 50}, "after": after,
			"request_id": "audit-test-request", "ip": "127.0.0.1",
		},
	}
	value, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode audit event: %v", err)
	}
	return inbox.Record{Topic: "lanverse.audit.recorded.v1", Key: []byte(key), Value: value}
}
