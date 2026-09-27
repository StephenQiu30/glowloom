package catalog_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/trace/noop"

	pgcatalog "github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestProviderCredentialsWithRealPostgres(t *testing.T) {
	dsn := os.Getenv("LV_TEST_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_CATALOG_DB_DSN to a disposable PostgreSQL database with catalog migration applied")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	store := pgcatalog.NewStore(conn.DB)
	provider := validProvider()
	provider.Key = "minimax-" + provider.ID.String()
	if err := store.CreateProvider(ctx, provider); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	duplicate := provider
	duplicate.ID = uuid.New()
	if err := store.CreateProvider(ctx, duplicate); !errors.Is(err, pgcatalog.ErrProviderKeyExists) {
		t.Fatalf("duplicate provider key: %v", err)
	}
	loaded, err := store.FindProvider(ctx, provider.ID)
	if err != nil || loaded.Key != provider.Key || loaded.Revision != 1 || loaded.Status != domain.ProviderActive {
		t.Fatalf("provider round trip: key %q revision %d status %q error %v", loaded.Key, loaded.Revision, loaded.Status, err)
	}
	loaded.RateLimitPerMin = 120
	loaded.Revision++
	if err := store.UpdateProvider(ctx, loaded, 1); err != nil {
		t.Fatalf("update provider: %v", err)
	}
	if err := store.UpdateProvider(ctx, loaded, 1); !errors.Is(err, pgcatalog.ErrRevisionConflict) {
		t.Fatalf("stale provider update: %v", err)
	}

	first := validCredential(provider.ID)
	if err := store.ReplaceCredential(ctx, first); err != nil {
		t.Fatalf("first credential: %v", err)
	}
	active, err := store.FindActiveCredential(ctx, provider.ID)
	if err != nil || active.ID != first.ID || !bytes.Equal(active.Ciphertext, first.Ciphertext) {
		t.Fatalf("active credential round trip: id %s error %v", active.ID, err)
	}
	second := validCredential(provider.ID)
	second.Label = "replacement"
	second.Last4 = "B8C3"
	if err := store.ReplaceCredential(ctx, second); err != nil {
		t.Fatalf("replace credential: %v", err)
	}
	active, err = store.FindActiveCredential(ctx, provider.ID)
	if err != nil || active.ID != second.ID {
		t.Fatalf("replacement not active: id %s error %v", active.ID, err)
	}
	var oldStatus string
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT status FROM catalog.provider_credential WHERE id = ?::uuid
	`, first.ID.String()).Scan(&oldStatus).Error; err != nil || oldStatus != "disabled" {
		t.Fatalf("old credential retained disabled: status %q error %v", oldStatus, err)
	}
	var pgErr *pgconn.PgError
	if err := conn.DB.WithContext(ctx).Exec(`
		INSERT INTO catalog.provider_credential
		  (id, provider_id, label, ciphertext, key_id, last4)
		VALUES (?::uuid, ?::uuid, 'direct-duplicate', ?::bytea, 'agent-2026', 'C7D4')
	`, uuid.NewString(), provider.ID.String(), []byte("sealed-only")).Error; !errors.As(err, &pgErr) || pgErr.Code != "23505" || pgErr.ConstraintName != "uq_provider_credential_active" {
		t.Fatalf("database accepted second active credential: %v", err)
	}
	broken := validCredential(provider.ID)
	broken.ID = second.ID
	if err := store.ReplaceCredential(ctx, broken); err == nil {
		t.Fatal("duplicate credential id did not roll back replacement")
	}
	active, err = store.FindActiveCredential(ctx, provider.ID)
	if err != nil || active.ID != second.ID {
		t.Fatalf("failed replacement removed active credential: id %s error %v", active.ID, err)
	}
	if err := store.DisableCredential(ctx, provider.ID, second.ID); err != nil {
		t.Fatalf("disable active credential: %v", err)
	}
	if _, err := store.FindActiveCredential(ctx, provider.ID); !errors.Is(err, pgcatalog.ErrCredentialNotFound) {
		t.Fatalf("disabled credential remained active: %v", err)
	}
	if err := store.DisableCredential(ctx, provider.ID, second.ID); !errors.Is(err, pgcatalog.ErrCredentialNotFound) {
		t.Fatalf("repeated credential disable: %v", err)
	}
	if err := store.ReplaceCredential(ctx, validCredential(provider.ID)); err != nil {
		t.Fatalf("credential after disable: %v", err)
	}
	loaded, err = store.FindProvider(ctx, provider.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.Disable(); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateProvider(ctx, loaded, 2); err != nil {
		t.Fatalf("disable provider: %v", err)
	}
	if _, err := store.FindActiveCredential(ctx, provider.ID); !errors.Is(err, pgcatalog.ErrCredentialNotFound) {
		t.Fatalf("disabled provider exposed active credential: %v", err)
	}
	if err := store.ReplaceCredential(ctx, validCredential(provider.ID)); !errors.Is(err, pgcatalog.ErrProviderUnavailable) {
		t.Fatalf("disabled provider accepted credential: %v", err)
	}
}

func TestConcurrentCredentialReplacementKeepsOneActive(t *testing.T) {
	dsn := os.Getenv("LV_TEST_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_CATALOG_DB_DSN to a disposable PostgreSQL database with catalog migration applied")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	store := pgcatalog.NewStore(conn.DB)
	provider := validProvider()
	provider.Key = "parallel-" + provider.ID.String()
	if err := store.CreateProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- store.ReplaceCredential(ctx, validCredential(provider.ID))
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("concurrent replacement: %v", err)
		}
	}
	var activeCount, totalCount int64
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT count(*) FILTER (WHERE status = 'active'), count(*)
		FROM catalog.provider_credential WHERE provider_id = ?::uuid
	`, provider.ID.String()).Row().Scan(&activeCount, &totalCount); err != nil || activeCount != 1 || totalCount != 2 {
		t.Fatalf("concurrent credential counts active=%d total=%d error=%v", activeCount, totalCount, err)
	}
}
