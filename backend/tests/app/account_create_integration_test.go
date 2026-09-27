package app_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/StephenQiu30/lanverse/backend/internal/app"
	pgidentity "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/postgres"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/config"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestAccountCreationAuditReachesPostgresThroughRelay(t *testing.T) {
	dsn := os.Getenv("LV_TEST_RELAY_DB_DSN")
	brokers := os.Getenv("LV_TEST_RELAY_KAFKA_BROKERS")
	redisURL := os.Getenv("LV_TEST_RELAY_REDIS_URL")
	if dsn == "" || brokers == "" || redisURL == "" {
		t.Skip("set disposable LV_TEST_RELAY_DB_DSN, LV_TEST_RELAY_KAFKA_BROKERS, and LV_TEST_RELAY_REDIS_URL")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open disposable database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	store := pgidentity.NewStore(conn.DB)
	orgID, actorID := uuid.New(), uuid.New()
	hash, err := domain.HashPassword("adminPassword123", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, domain.User{
		ID: actorID, OrgID: orgID, LoginName: "admin", DisplayName: "Admin",
		Role: domain.RoleAdmin, PasswordHash: hash,
	}); err != nil {
		t.Fatalf("create administrator: %v", err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		UPDATE identity."user" SET must_change_password = false WHERE id = ?::uuid
	`, actorID.String()).Error; err != nil {
		t.Fatalf("clear administrator first-login flag: %v", err)
	}
	created, err := identityapp.NewCreateUserCommand(store, time.Now).Execute(ctx,
		identityapp.Principal{ID: actorID, OrgID: orgID, Role: domain.RoleAdmin},
		identityapp.CreateUserInput{
			LoginName: "alice", DisplayName: "Alice", Role: domain.RoleProducer,
			InitialPassword: "initialPassword123", RequestID: "account-relay-integration",
		})
	if err != nil {
		t.Fatalf("create account command: %v", err)
	}
	runCtx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	healthAddr := localHealthAddress(t)
	go func() {
		done <- app.RunRelay(runCtx, config.Config{
			DBDSN: dsn, KafkaBrokers: brokers, RedisURL: redisURL,
			RelayHealthAddr: healthAddr,
		}, zap.NewNop())
	}()
	t.Cleanup(func() {
		stop()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("stop relay: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("relay did not stop")
		}
		assertRoleHealthStopped(t, healthAddr)
	})
	assertRoleHealth(ctx, t, healthAddr)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		var auditRows, markers, published int64
		if err := conn.DB.WithContext(ctx).Raw(`
			SELECT count(*) FROM audit.audit_log
			WHERE org_id = ?::uuid AND object_id = ? AND action = 'user.created'
		`, orgID.String(), created.ID.String()).Scan(&auditRows).Error; err != nil {
			t.Fatalf("read durable audit: %v", err)
		}
		if err := conn.DB.WithContext(ctx).Raw(`
			SELECT count(*) FROM infra.processed_event
			WHERE consumer = 'audit' AND event_id IN (
			  SELECT id FROM infra.outbox WHERE topic = 'lanverse.audit.recorded.v1'
			)
		`).Scan(&markers).Error; err != nil {
			t.Fatalf("read audit marker: %v", err)
		}
		if err := conn.DB.WithContext(ctx).Raw(`
			SELECT count(*) FROM infra.outbox WHERE published_at IS NOT NULL
		`).Scan(&published).Error; err != nil {
			t.Fatalf("read published events: %v", err)
		}
		if auditRows == 1 && markers == 1 && published == 2 {
			return
		}
		select {
		case err := <-done:
			t.Fatalf("relay exited before audit completion: %v", err)
		case <-ctx.Done():
			t.Fatalf("wait for account audit: %v (audit %d, markers %d, published %d)", ctx.Err(), auditRows, markers, published)
		case <-ticker.C:
		}
	}
}
