package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/app"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

// The owner and application DSNs must point to the same empty, migrated test
// database. Kafka and Redis must be disposable for this test: the relay uses
// the production audit topic and consumer group names.
func TestAuditRestrictedLoginRelayDeduplicates(t *testing.T) {
	ownerDSN := os.Getenv("LV_TEST_AUDIT_OWNER_DB_DSN")
	loginDSN := os.Getenv("LV_TEST_AUDIT_LOGIN_DB_DSN")
	brokers := os.Getenv("LV_TEST_KAFKA_BROKERS")
	redisURL := os.Getenv("LV_TEST_REDIS_URL")
	if ownerDSN == "" && loginDSN == "" {
		t.Skip("set the audit owner and restricted-login DSNs with disposable Kafka and Redis addresses")
	}
	if ownerDSN == "" || loginDSN == "" || brokers == "" || redisURL == "" {
		t.Fatal("all audit owner, restricted-login, Kafka, and Redis test settings are required")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	ownerConn, err := db.Open(ctx, ownerDSN, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open audit fixture database: %v", err)
	}
	t.Cleanup(func() { _ = ownerConn.Close() })
	loginConn, err := db.Open(ctx, loginDSN, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open restricted application database: %v", err)
	}
	t.Cleanup(func() { _ = loginConn.Close() })
	assertRestrictedAuditLogin(ctx, t, loginConn.DB)
	assertEmptyAuditRoleDatabase(ctx, t, ownerConn.DB)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if err := ownerConn.DB.WithContext(cleanupCtx).Exec(
			"TRUNCATE infra.outbox, infra.processed_event, audit.audit_log",
		).Error; err != nil {
			t.Errorf("clear isolated audit-role fixtures: %v", err)
		}
	})

	const topic = "lanverse.audit.recorded.v1"
	producer, err := kgo.NewClient(kgo.SeedBrokers(strings.Split(brokers, ",")...))
	if err != nil {
		t.Fatalf("open audit test Kafka client: %v", err)
	}
	t.Cleanup(producer.Close)
	ensureAuditRoleTopic(ctx, t, producer, topic)

	orgID, actorID, firstID, sentinelID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	firstPayload := auditRolePayload(t, firstID, orgID, actorID)
	insertAuditRoleOutbox(ctx, t, ownerConn.DB, firstID, orgID, topic, firstPayload)

	runCtx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	healthAddr := localHealthAddress(t)
	go func() {
		done <- app.RunRelay(runCtx, config.Config{
			DBDSN: loginDSN, KafkaBrokers: brokers, RedisURL: redisURL,
			RelayHealthAddr: healthAddr,
		}, zap.NewNop())
	}()
	relayExited := false
	t.Cleanup(func() {
		stop()
		if !relayExited {
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("stop restricted audit relay: %v", err)
				}
			case <-time.After(10 * time.Second):
				t.Error("restricted audit relay did not stop after cancellation")
			}
		}
		assertRoleHealthStopped(t, healthAddr)
	})
	assertRoleHealth(ctx, t, healthAddr)
	waitForAuditRoleDelivery(ctx, t, ownerConn.DB, done, &relayExited, firstID)

	// The duplicate and sentinel have the same Kafka key. Seeing the sentinel
	// proves this consumer group passed the duplicate before row counts are read.
	if err := producer.ProduceSync(ctx, &kgo.Record{
		Topic: topic, Key: []byte(orgID.String()), Value: firstPayload,
	}).FirstErr(); err != nil {
		t.Fatalf("publish duplicate audit event: %v", err)
	}
	insertAuditRoleOutbox(ctx, t, ownerConn.DB, sentinelID, orgID, topic,
		auditRolePayload(t, sentinelID, orgID, actorID))
	waitForAuditRoleDelivery(ctx, t, ownerConn.DB, done, &relayExited, sentinelID)
	assertAuditRoleEventCounts(ctx, t, ownerConn.DB, firstID)
	assertAuditRoleEventCounts(ctx, t, ownerConn.DB, sentinelID)

	assertRestrictedAuditMutationDenied(ctx, t, loginConn.DB, firstID)
}

func ensureAuditRoleTopic(ctx context.Context, t *testing.T, client *kgo.Client, topic string) {
	t.Helper()
	request := kmsg.NewPtrCreateTopicsRequest()
	entry := kmsg.NewCreateTopicsRequestTopic()
	entry.Topic = topic
	entry.NumPartitions = 1
	entry.ReplicationFactor = 1
	request.Topics = append(request.Topics, entry)
	response, err := request.RequestWith(ctx, client)
	if err != nil {
		t.Fatalf("create audit test topic: %v", err)
	}
	if len(response.Topics) != 1 {
		t.Fatalf("create audit test topic returned %d results, want one", len(response.Topics))
	}
	if topicErr := kerr.ErrorForCode(response.Topics[0].ErrorCode); topicErr != nil && !errors.Is(topicErr, kerr.TopicAlreadyExists) {
		t.Fatalf("create audit test topic: %v", topicErr)
	}
}

func assertRestrictedAuditLogin(ctx context.Context, t *testing.T, handle *gorm.DB) {
	t.Helper()
	var identity struct {
		CurrentRole string
		SessionRole string
		CanLogin    bool
		Superuser   bool
		CreateRole  bool
		CreateDB    bool
		AuditOwner  bool
		OutboxOwner bool
		InheritsApp bool
		SchemaUsage bool
		CanInsert   bool
		CanSelect   bool
		CanUpdate   bool
		CanDelete   bool
		CanTruncate bool
	}
	if err := handle.WithContext(ctx).Raw(`
		SELECT current_user AS current_role, session_user AS session_role,
		       r.rolcanlogin AS can_login, r.rolsuper AS superuser,
		       r.rolcreaterole AS create_role, r.rolcreatedb AS create_db,
		       pg_has_role(current_user, a.relowner, 'MEMBER') AS audit_owner,
		       pg_has_role(current_user, o.relowner, 'MEMBER') AS outbox_owner,
		       pg_has_role(current_user, 'lanverse_app', 'USAGE') AS inherits_app,
		       has_schema_privilege('audit', 'USAGE') AS schema_usage,
		       has_table_privilege('audit.audit_log', 'INSERT') AS can_insert,
		       has_table_privilege('audit.audit_log', 'SELECT') AS can_select,
		       has_table_privilege('audit.audit_log', 'UPDATE') AS can_update,
		       has_table_privilege('audit.audit_log', 'DELETE') AS can_delete,
		       has_table_privilege('audit.audit_log', 'TRUNCATE') AS can_truncate
		FROM pg_roles AS r
		JOIN pg_class AS a ON a.oid = 'audit.audit_log'::regclass
		JOIN pg_class AS o ON o.oid = 'infra.outbox'::regclass
		WHERE r.rolname = current_user
	`).Scan(&identity).Error; err != nil {
		t.Fatalf("inspect restricted application login: %v", err)
	}
	if identity.CurrentRole == "" || identity.CurrentRole != identity.SessionRole || !identity.CanLogin ||
		identity.Superuser || identity.CreateRole || identity.CreateDB || identity.AuditOwner || identity.OutboxOwner ||
		!identity.InheritsApp || !identity.SchemaUsage || !identity.CanInsert || !identity.CanSelect ||
		identity.CanUpdate || identity.CanDelete || identity.CanTruncate {
		t.Fatalf("database connection is not a restricted application LOGIN: %+v", identity)
	}
}

func assertEmptyAuditRoleDatabase(ctx context.Context, t *testing.T, handle *gorm.DB) {
	t.Helper()
	for _, table := range []string{"infra.outbox", "infra.processed_event", "audit.audit_log"} {
		var count int64
		if err := handle.WithContext(ctx).Table(table).Count(&count).Error; err != nil {
			t.Fatalf("inspect isolated %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("%s already has %d rows; use an empty disposable database", table, count)
		}
	}
}

func auditRolePayload(t *testing.T, eventID, orgID, actorID uuid.UUID) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"event_id": eventID.String(), "event_type": "lanverse.audit.recorded.v1",
		"occurred_at": time.Now().UTC(), "org_id": orgID.String(),
		"actor":     map[string]any{"kind": "user", "id": actorID.String()},
		"aggregate": map[string]any{"type": "audit", "id": eventID.String()},
		"data": map[string]any{
			"action": "auth.login_succeeded", "object": map[string]any{"type": "user", "id": actorID.String()},
			"request_id": "audit-restricted-login-test",
		},
	})
	if err != nil {
		t.Fatalf("encode audit role event: %v", err)
	}
	return payload
}

func insertAuditRoleOutbox(ctx context.Context, t *testing.T, handle *gorm.DB, eventID, orgID uuid.UUID, topic string, payload []byte) {
	t.Helper()
	if err := handle.WithContext(ctx).Exec(`
		INSERT INTO infra.outbox(id, topic, partition_key, payload)
		VALUES (?::uuid, ?, ?, ?::jsonb)
	`, eventID.String(), topic, orgID.String(), string(payload)).Error; err != nil {
		t.Fatalf("insert audit-role Outbox event: %v", err)
	}
}

func waitForAuditRoleDelivery(ctx context.Context, t *testing.T, handle *gorm.DB, done <-chan error, exited *bool, eventID uuid.UUID) {
	t.Helper()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		var published bool
		if err := handle.WithContext(ctx).Raw(
			"SELECT published_at IS NOT NULL FROM infra.outbox WHERE id = ?::uuid", eventID.String(),
		).Scan(&published).Error; err != nil {
			t.Fatalf("read audit-role Outbox delivery: %v", err)
		}
		var auditCount, markerCount int64
		if err := handle.WithContext(ctx).Raw(
			"SELECT count(*) FROM audit.audit_log WHERE id = ?::uuid", eventID.String(),
		).Scan(&auditCount).Error; err != nil {
			t.Fatalf("read audit-role audit row: %v", err)
		}
		if err := handle.WithContext(ctx).Raw(
			"SELECT count(*) FROM infra.processed_event WHERE consumer = 'audit' AND event_id = ?::uuid", eventID.String(),
		).Scan(&markerCount).Error; err != nil {
			t.Fatalf("read audit-role consumer marker: %v", err)
		}
		if published && auditCount == 1 && markerCount == 1 {
			return
		}
		select {
		case err := <-done:
			*exited = true
			t.Fatalf("restricted audit relay exited before event delivery: %v", err)
		case <-ctx.Done():
			t.Fatalf("wait for restricted audit delivery %s: %v (published %t, audit %d, marker %d)",
				eventID, ctx.Err(), published, auditCount, markerCount)
		case <-ticker.C:
		}
	}
}

func assertAuditRoleEventCounts(ctx context.Context, t *testing.T, handle *gorm.DB, eventID uuid.UUID) {
	t.Helper()
	var auditCount, markerCount int64
	if err := handle.WithContext(ctx).Raw(
		"SELECT count(*) FROM audit.audit_log WHERE id = ?::uuid", eventID.String(),
	).Scan(&auditCount).Error; err != nil {
		t.Fatalf("count audit rows: %v", err)
	}
	if err := handle.WithContext(ctx).Raw(
		"SELECT count(*) FROM infra.processed_event WHERE consumer = 'audit' AND event_id = ?::uuid", eventID.String(),
	).Scan(&markerCount).Error; err != nil {
		t.Fatalf("count audit markers: %v", err)
	}
	if auditCount != 1 || markerCount != 1 {
		t.Fatalf("audit event %s has %d rows and %d markers; want one each", eventID, auditCount, markerCount)
	}
}

func assertRestrictedAuditMutationDenied(ctx context.Context, t *testing.T, handle *gorm.DB, eventID uuid.UUID) {
	t.Helper()
	for _, statement := range []string{
		"UPDATE audit.audit_log SET after = '{}'::jsonb WHERE id = ?::uuid",
		"DELETE FROM audit.audit_log WHERE id = ?::uuid",
		"TRUNCATE audit.audit_log",
		"ALTER TABLE audit.audit_log ADD COLUMN restricted_login_probe text",
	} {
		var err error
		if strings.Contains(statement, "?::uuid") {
			err = handle.WithContext(ctx).Exec(statement, eventID.String()).Error
		} else {
			err = handle.WithContext(ctx).Exec(statement).Error
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Fatalf("restricted LOGIN mutation %q returned %v; want SQLSTATE 42501", statement, err)
		}
	}
}
