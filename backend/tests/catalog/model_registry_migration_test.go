package catalog_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	pgcatalog "github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestModelRegistryTablesEnforceVersionsAndOwnershipOnLocalPostgres(t *testing.T) {
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
	provider := validProvider()
	provider.Key = "registry-" + provider.ID.String()
	if err := pgcatalog.NewStore(conn.DB).CreateProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	capabilityID := uuid.New()
	capabilityKey := "image.generate." + uuid.NewString()
	if err := conn.DB.WithContext(ctx).Exec(`
		INSERT INTO catalog.capability (id, key, output_type, modes, input_roles)
		VALUES (?::uuid, ?, 'image', ARRAY['text2image'], ARRAY['subject'])
	`, capabilityID.String(), capabilityKey).Error; err != nil {
		t.Fatalf("insert capability: %v", err)
	}
	firstID, secondID := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{firstID, secondID} {
		if err := conn.DB.WithContext(ctx).Exec(`
			INSERT INTO catalog.model_profile
			  (id, model_key, provider_id, capability, display_name)
			VALUES (?::uuid, ?, ?::uuid, ?, 'Test Model')
		`, id.String(), "model-"+id.String(), provider.ID.String(), capabilityKey).Error; err != nil {
			t.Fatalf("insert model profile: %v", err)
		}
	}
	versionID := uuid.New()
	if err := conn.DB.WithContext(ctx).Exec(`
		INSERT INTO catalog.model_profile_version
		  (id, model_profile_id, version_no, provider_model_id, modes, limits,
		   param_schema, supports_query, supports_cancel, supports_callback, expected_max_ms)
		VALUES (?::uuid, ?::uuid, 1, 'provider-model', ARRAY['text2image'],
		        '{"max_outputs":1}'::jsonb, '[]'::jsonb, true, false, false, 60000)
	`, versionID.String(), firstID.String()).Error; err != nil {
		t.Fatalf("insert model version: %v", err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		UPDATE catalog.model_profile SET current_version_id = ?::uuid WHERE id = ?::uuid
	`, versionID.String(), secondID.String()).Error; err == nil {
		t.Fatal("another model's version became current")
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		UPDATE catalog.model_profile SET current_version_id = ?::uuid WHERE id = ?::uuid
	`, versionID.String(), firstID.String()).Error; err != nil {
		t.Fatalf("own version rejected: %v", err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		INSERT INTO catalog.model_profile_version
		  (id, model_profile_id, version_no, provider_model_id, modes, limits,
		   param_schema, supports_query, supports_cancel, supports_callback, expected_max_ms)
		VALUES (?::uuid, ?::uuid, 1, 'duplicate-version', ARRAY['text2image'],
		        '{"max_outputs":1}'::jsonb, '[]'::jsonb, true, false, false, 60000)
	`, uuid.NewString(), firstID.String()).Error; err == nil {
		t.Fatal("duplicate model version accepted")
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		INSERT INTO catalog.price_rule_version
		  (id, model_profile_id, version_no, unit, rule, currency, effective_from)
		VALUES (?::uuid, ?::uuid, 1, 'per_image', '{"base_micros":1000}'::jsonb, 'CNY', now())
	`, uuid.NewString(), firstID.String()).Error; err != nil {
		t.Fatalf("insert price version: %v", err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		INSERT INTO catalog.price_rule_version
		  (id, model_profile_id, version_no, unit, rule, currency, effective_from)
		VALUES (?::uuid, ?::uuid, 2, 'per_image', '{"base_micros":1000}'::jsonb, 'USD', now())
	`, uuid.NewString(), firstID.String()).Error; err == nil {
		t.Fatal("foreign currency without versioned exchange rate accepted")
	}
	var count int
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT count(*) FROM catalog.model_profile WHERE provider_id = ?::uuid AND NOT is_delete
	`, provider.ID.String()).Scan(&count).Error; err != nil || count != 2 {
		t.Fatalf("provider model count %d: %v", count, err)
	}
}
