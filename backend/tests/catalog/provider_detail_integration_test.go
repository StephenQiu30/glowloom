package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/credentialschema"
	pgcatalog "github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/postgres"
	catalogapp "github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestProviderDetailReadsSafeMetadataWithCurrentAdminOnLocalPostgres(t *testing.T) {
	dsn := os.Getenv("LV_TEST_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_CATALOG_DB_DSN to an isolated migrated database")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	actor := adminPrincipal()
	if err := conn.DB.WithContext(ctx).Exec(`
		INSERT INTO identity."user" (id, org_id, login_name, display_name, role, password_hash, must_change_password)
		VALUES (?::uuid, ?::uuid, ?, 'Test Admin', 'admin', 'test-hash', false)
	`, actor.ID.String(), actor.OrgID.String(), "provider-detail-"+actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	store := pgcatalog.NewStore(conn.DB)
	provider := validProvider()
	provider.Key = "provider-detail-" + provider.ID.String()
	if err := store.CreateProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	query := catalogapp.NewProviderDetailQuery(store, credentialschema.NewRegistry())
	detail, err := query.Execute(ctx, actor, provider.ID)
	if err != nil || detail.Provider.ID != provider.ID || detail.Credential != nil {
		t.Fatalf("provider without credential %+v: %v", detail, err)
	}
	credential := validCredential(provider.ID)
	credential.Ciphertext = []byte("private-sealed-test-value")
	if err := store.ReplaceCredential(ctx, credential); err != nil {
		t.Fatal(err)
	}
	detail, err = query.Execute(ctx, actor, provider.ID)
	if err != nil || detail.Credential == nil || detail.Credential.ID != credential.ID ||
		detail.Credential.Last4 != credential.Last4 {
		t.Fatalf("provider with credential %+v: %v", detail, err)
	}
	encoded, err := json.Marshal(detail)
	if err != nil || strings.Contains(string(encoded), "private-sealed-test-value") ||
		strings.Contains(string(encoded), "Ciphertext") || strings.Contains(string(encoded), "key_id") {
		t.Fatalf("credential leaked in detail %s: %v", encoded, err)
	}
	loaded, err := store.FindProvider(ctx, provider.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.Disable(); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateProvider(ctx, loaded, 1); err != nil {
		t.Fatal(err)
	}
	detail, err = query.Execute(ctx, actor, provider.ID)
	if err != nil || detail.Provider.Status != domain.ProviderDisabled ||
		detail.Credential == nil || detail.Credential.ID != credential.ID {
		t.Fatalf("disabled provider detail %+v: %v", detail, err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`UPDATE identity."user" SET role = 'producer' WHERE id = ?::uuid`, actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := query.Execute(ctx, actor, provider.ID); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked administrator read provider detail: %v", err)
	}
	other := actor
	other.OrgID = uuid.New()
	if _, err := query.Execute(ctx, other, provider.ID); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("other organization read provider detail: %v", err)
	}
}
