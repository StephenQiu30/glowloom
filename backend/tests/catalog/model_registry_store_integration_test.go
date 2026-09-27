package catalog_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	pgcatalog "github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestModelRegistryAppendOnlyWritesAndAdminRevocationOnLocalPostgres(t *testing.T) {
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
		VALUES (?::uuid, ?::uuid, ?, 'Registry Admin', 'admin', 'test-hash', false)
	`, actor.ID.String(), actor.OrgID.String(), "registry-"+actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	store := pgcatalog.NewStore(conn.DB)
	provider := validProvider()
	provider.Key = "registry-" + provider.ID.String()
	if err := store.CreateProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	capability := domain.Capability{
		ID: uuid.New(), Key: "video.generate." + uuid.NewString(), OutputType: domain.OutputVideo,
		Modes: []string{"image2video", "text2video"}, InputRoles: []string{"subject"},
	}
	if err := store.CreateCapabilityForAdmin(ctx, actor.ID, actor.OrgID, capability); err != nil {
		t.Fatal(err)
	}
	model := validModelProfile()
	model.ProviderID, model.Capability = provider.ID, capability.Key
	model.Key = "registry-" + model.ID.String()
	if err := store.CreateModelForAdmin(ctx, actor.ID, actor.OrgID, model); err != nil {
		t.Fatal(err)
	}
	version1 := validModelVersion(model.ID)
	version1.CreateBy = actor.ID
	if err := store.AppendModelVersionForAdmin(ctx, actor.ID, actor.OrgID, version1, 2); !errors.Is(err, domain.ErrModelRevisionConflict) {
		t.Fatalf("stale model revision accepted: %v", err)
	}
	if err := store.AppendModelVersionForAdmin(ctx, actor.ID, actor.OrgID, version1, 1); err != nil {
		t.Fatalf("publish first model version: %v", err)
	}
	if err := store.AppendModelVersionForAdmin(ctx, actor.ID, actor.OrgID, version1, 2); !errors.Is(err, pgcatalog.ErrModelVersionConflict) {
		t.Fatalf("duplicate model version accepted: %v", err)
	}
	version2 := validModelVersion(model.ID)
	version2.VersionNo, version2.ProviderModelID, version2.CreateBy = 2, "seedance-2-updated", actor.ID
	if err := store.AppendModelVersionForAdmin(ctx, actor.ID, actor.OrgID, version2, 2); err != nil {
		t.Fatalf("publish second model version: %v", err)
	}
	var firstProviderModelID string
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT provider_model_id FROM catalog.model_profile_version WHERE id = ?::uuid
	`, version1.ID.String()).Scan(&firstProviderModelID).Error; err != nil || firstProviderModelID != version1.ProviderModelID {
		t.Fatalf("historical model version changed to %q: %v", firstProviderModelID, err)
	}
	price1 := validPriceVersion(model.ID)
	price1.CreateBy = actor.ID
	if err := store.AppendPriceRuleForAdmin(ctx, actor.ID, actor.OrgID, price1, 3); err != nil {
		t.Fatalf("publish first price: %v", err)
	}
	price2 := validPriceVersion(model.ID)
	price2.VersionNo, price2.Currency, price2.FXRateToCNY, price2.CreateBy = 2, "USD", "7.123456", actor.ID
	if err := store.AppendPriceRuleForAdmin(ctx, actor.ID, actor.OrgID, price2, 4); err != nil {
		t.Fatalf("publish second price: %v", err)
	}
	if err := store.AppendPriceRuleForAdmin(ctx, actor.ID, actor.OrgID, price2, 5); !errors.Is(err, pgcatalog.ErrPriceVersionConflict) {
		t.Fatalf("duplicate price version accepted: %v", err)
	}
	if err := store.SetModelStatusForAdmin(ctx, actor.ID, actor.OrgID, model.ID, domain.ModelActive, 5, time.Now()); err != nil {
		t.Fatalf("activate model with version and price: %v", err)
	}
	loaded, err := store.FindModelForAdmin(ctx, actor.ID, actor.OrgID, model.ID)
	if err != nil || loaded.Status != domain.ModelActive || loaded.CurrentVersionID != version2.ID || loaded.Revision != 6 {
		t.Fatalf("loaded model %+v: %v", loaded, err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		UPDATE identity."user" SET role = 'producer' WHERE id = ?::uuid
	`, actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.SetModelStatusForAdmin(ctx, actor.ID, actor.OrgID, model.ID, domain.ModelDisabled, 6, time.Now()); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked administrator changed model: %v", err)
	}
	if _, err := store.FindModelForAdmin(ctx, actor.ID, actor.OrgID, model.ID); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked administrator read model: %v", err)
	}
}
