package identity_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	pgidentity "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/postgres"
	redisidentity "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/redis"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/redisconn"
)

func TestAuthenticateRejectsDisabledAccountWithRealPostgresAndRedis(t *testing.T) {
	dsn := os.Getenv("LV_TEST_IDENTITY_DB_DSN")
	redisURL := os.Getenv("LV_TEST_IDENTITY_REDIS_URL")
	if dsn == "" || redisURL == "" {
		t.Skip("set LV_TEST_IDENTITY_DB_DSN and LV_TEST_IDENTITY_REDIS_URL to disposable services")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	dbConn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatalf("open identity database: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })
	redisConn, err := redisconn.Open(redisURL)
	if err != nil {
		t.Fatalf("open identity Redis: %v", err)
	}
	t.Cleanup(func() { _ = redisConn.Close() })
	accounts := pgidentity.NewStore(dbConn.DB)
	sessions, err := redisidentity.NewSessionStore(redisConn.Client, time.Hour, 24*time.Hour, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	authenticator := identityapp.NewAuthenticator(accounts, sessions)
	orgID, userID := uuid.New(), uuid.New()
	hash, err := domain.HashPassword("initialPassword123", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := accounts.Create(ctx, domain.User{
		ID: userID, OrgID: orgID, LoginName: userID.String(), DisplayName: "Alice",
		Role: domain.RoleProducer, PasswordHash: hash,
	}); err != nil {
		t.Fatalf("create test account: %v", err)
	}
	token, _, err := sessions.Create(ctx, orgID, userID, 1)
	if err != nil {
		t.Fatalf("create test session: %v", err)
	}
	t.Cleanup(func() { _ = sessions.Destroy(context.Background(), token) })
	if principal, err := authenticator.Authenticate(ctx, token); err != nil || principal.ID != userID {
		t.Fatalf("initial authentication = %+v, %v", principal, err)
	}
	wrongOrgToken, _, err := sessions.Create(ctx, uuid.New(), userID, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sessions.Destroy(context.Background(), wrongOrgToken) })
	if _, err := authenticator.Authenticate(ctx, wrongOrgToken); !errors.Is(err, identityapp.ErrUnauthenticated) {
		t.Fatalf("cross-organization session = %v", err)
	}
	if err := accounts.Disable(ctx, orgID, userID, 1); err != nil {
		t.Fatalf("disable test account: %v", err)
	}
	if _, err := authenticator.Authenticate(ctx, token); !errors.Is(err, identityapp.ErrUnauthenticated) {
		t.Fatalf("disabled account session = %v", err)
	}
	if _, err := sessions.Load(ctx, token); err != nil {
		t.Fatalf("session disappeared before TTL; test must prove account epoch check: %v", err)
	}
}
