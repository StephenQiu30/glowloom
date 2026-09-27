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

	pgcatalog "github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/postgres"
	catalogapp "github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestListProvidersPagesSafeSummariesAndRealModelCountsOnLocalPostgres(t *testing.T) {
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
	`, actor.ID.String(), actor.OrgID.String(), "provider-list-"+actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	store := pgcatalog.NewStore(conn.DB)
	providers := make([]domain.Provider, 3)
	var latest struct{ CreateTime *time.Time }
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT max(create_time) AS create_time FROM catalog.provider
	`).Scan(&latest).Error; err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC()
	if latest.CreateTime != nil {
		base = latest.CreateTime.Add(3 * time.Hour)
	}
	for index := range providers {
		provider := validProvider()
		provider.Key = "list-" + provider.ID.String()
		if err := store.CreateProvider(ctx, provider); err != nil {
			t.Fatal(err)
		}
		if err := conn.DB.WithContext(ctx).Exec(`
			UPDATE catalog.provider SET create_time = ? WHERE id = ?::uuid
		`, base.Add(-time.Duration(index)*time.Hour), provider.ID.String()).Error; err != nil {
			t.Fatal(err)
		}
		providers[index] = provider
	}
	for _, index := range []int{0, 1} {
		credential := validCredential(providers[index].ID)
		credential.Ciphertext = []byte("sealed-private-list-value")
		if err := store.ReplaceCredential(ctx, credential); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := store.FindProvider(ctx, providers[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.Disable(); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateProvider(ctx, loaded, 1); err != nil {
		t.Fatal(err)
	}
	capabilityKey := "image.generate." + uuid.NewString()
	if err := conn.DB.WithContext(ctx).Exec(`
		INSERT INTO catalog.capability (id, key, output_type, modes, input_roles)
		VALUES (?::uuid, ?, 'image', ARRAY['text2image'], ARRAY['subject'])
	`, uuid.NewString(), capabilityKey).Error; err != nil {
		t.Fatal(err)
	}
	for index, count := range []int{3, 1, 0} {
		for modelIndex := range count {
			modelID := uuid.New()
			if err := conn.DB.WithContext(ctx).Exec(`
				INSERT INTO catalog.model_profile
				  (id, model_key, provider_id, capability, display_name, status, is_delete)
				VALUES (?::uuid, ?, ?::uuid, ?, 'Test Model', ?, ?)
			`, modelID.String(), "list-model-"+modelID.String(), providers[index].ID.String(),
				capabilityKey, map[bool]string{true: "disabled", false: "active"}[modelIndex == 1],
				index == 0 && modelIndex == 2).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	query := catalogapp.NewListProvidersQuery(store)
	first, err := query.Execute(ctx, actor, catalogapp.ListProvidersInput{Limit: 2})
	if err != nil || len(first.Providers) != 2 || first.Next == nil ||
		first.Providers[0].Provider.ID != providers[0].ID || first.Providers[0].ModelCount != 2 ||
		first.Providers[1].Provider.ID != providers[1].ID || first.Providers[1].ModelCount != 1 ||
		first.Providers[1].Provider.Status != domain.ProviderDisabled ||
		first.Providers[1].Credential == nil || first.Providers[1].Credential.Last4 != "A9F2" {
		t.Fatalf("first provider page %+v: %v", first, err)
	}
	second, err := query.Execute(ctx, actor, catalogapp.ListProvidersInput{Limit: 2, After: first.Next})
	if err != nil || len(second.Providers) == 0 ||
		second.Providers[0].Provider.ID != providers[2].ID || second.Providers[0].ModelCount != 0 ||
		second.Providers[0].Credential != nil {
		t.Fatalf("second provider page %+v: %v", second, err)
	}
	encoded, err := json.Marshal(first)
	if err != nil || strings.Contains(string(encoded), "sealed-private-list-value") ||
		strings.Contains(string(encoded), "ciphertext") || strings.Contains(string(encoded), "key_id") {
		t.Fatalf("provider list leaked credential %s: %v", encoded, err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`UPDATE identity."user" SET role = 'producer' WHERE id = ?::uuid`, actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := query.Execute(ctx, actor, catalogapp.ListProvidersInput{}); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked administrator listed providers: %v", err)
	}
}
