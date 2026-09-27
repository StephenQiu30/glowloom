package catalog_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/paramvalidation"
	pgcatalog "github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/pricevalidation"
	catalogapp "github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
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
	withoutRoles := capability
	withoutRoles.ID, withoutRoles.Key, withoutRoles.InputRoles = uuid.New(), "video.no-input."+uuid.NewString(), nil
	if err := store.CreateCapabilityForAdmin(ctx, actor.ID, actor.OrgID, withoutRoles); err != nil {
		t.Fatalf("create capability without input roles: %v", err)
	}
	model := validModelProfile()
	model.ProviderID, model.Capability = provider.ID, capability.Key
	model.Key = "registry-" + model.ID.String()
	if err := store.CreateModelForAdmin(ctx, actor.ID, actor.OrgID, model); err != nil {
		t.Fatal(err)
	}
	validator, err := paramvalidation.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	publish := catalogapp.NewPublishModelVersionCommand(store, validator, time.Now)
	setStatus := catalogapp.NewSetModelStatusCommand(store, time.Now)
	version1 := validPublishModelVersionInput(model.ID)
	version1.ExpectedRevision = 2
	if _, err := publish.Execute(ctx, actor, version1); !errors.Is(err, domain.ErrModelRevisionConflict) {
		t.Fatalf("stale model revision accepted: %v", err)
	}
	version1.ExpectedRevision = 1
	published1, err := publish.Execute(ctx, actor, version1)
	if err != nil {
		t.Fatalf("publish first model version: %v", err)
	}
	version1.ExpectedRevision = 2
	if _, err := publish.Execute(ctx, actor, version1); !errors.Is(err, pgcatalog.ErrModelVersionConflict) {
		t.Fatalf("duplicate model version accepted: %v", err)
	}
	version2 := validPublishModelVersionInput(model.ID)
	version2.ExpectedRevision, version2.VersionNo, version2.ProviderModelID = 2, 2, "seedance-2-updated"
	unsupported := version2
	unsupported.Modes = []string{"audio2video"}
	if _, err := publish.Execute(ctx, actor, unsupported); !errors.Is(err, domain.ErrInvalidModelVersion) {
		t.Fatalf("unsupported capability mode accepted: %v", err)
	}
	published2, err := publish.Execute(ctx, actor, version2)
	if err != nil {
		t.Fatalf("publish second model version: %v", err)
	}
	var firstProviderModelID string
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT provider_model_id FROM catalog.model_profile_version WHERE id = ?::uuid
	`, published1.ID.String()).Scan(&firstProviderModelID).Error; err != nil || firstProviderModelID != version1.ProviderModelID {
		t.Fatalf("historical model version changed to %q: %v", firstProviderModelID, err)
	}
	if _, err := setStatus.Execute(ctx, actor, catalogapp.SetModelStatusInput{
		ModelID: model.ID, Status: domain.ModelActive,
		ExpectedRevision: 3, RequestID: uuid.NewString(),
	}); !errors.Is(err, domain.ErrModelNotPublishable) {
		t.Fatalf("model without price activated: %v", err)
	}
	priceValidator, err := pricevalidation.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	publishPrice := catalogapp.NewPublishPriceRuleCommand(store, priceValidator, time.Now)
	price1 := validPriceVersion(model.ID)
	firstPriceInput := catalogapp.PublishPriceRuleInput{
		ModelID: model.ID, ExpectedRevision: 3, VersionNo: 1,
		Unit: price1.Unit, Rule: price1.Rule, Currency: price1.Currency,
		EffectiveFrom: price1.EffectiveFrom, RequestID: uuid.NewString(),
	}
	publishedPrice1, err := publishPrice.Execute(ctx, actor, firstPriceInput)
	if err != nil {
		t.Fatalf("publish first price: %v", err)
	}
	secondPriceInput := firstPriceInput
	secondPriceInput.ExpectedRevision, secondPriceInput.VersionNo = 4, 2
	secondPriceInput.Currency, secondPriceInput.FXRateToCNY = "USD", "7.123456"
	secondPriceInput.RequestID = uuid.NewString()
	if _, err := publishPrice.Execute(ctx, actor, secondPriceInput); err != nil {
		t.Fatalf("publish second price: %v", err)
	}
	secondPriceInput.ExpectedRevision = 5
	if _, err := publishPrice.Execute(ctx, actor, secondPriceInput); !errors.Is(err, pgcatalog.ErrPriceVersionConflict) {
		t.Fatalf("duplicate price version accepted: %v", err)
	}
	var originalPrice struct {
		Currency   string
		BaseMicros string
	}
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT currency, rule->>'base_micros' AS base_micros
		FROM catalog.price_rule_version WHERE id = ?::uuid
	`, publishedPrice1.ID.String()).Scan(&originalPrice).Error; err != nil || originalPrice.Currency != "CNY" || originalPrice.BaseMicros != "1000" {
		t.Fatalf("historical price changed: %+v, %v", originalPrice, err)
	}
	if _, err := setStatus.Execute(ctx, actor, catalogapp.SetModelStatusInput{
		ModelID: model.ID, Status: domain.ModelActive,
		ExpectedRevision: 5, RequestID: uuid.NewString(),
	}); err != nil {
		t.Fatalf("activate model with version and price: %v", err)
	}
	loaded, err := store.FindModelForAdmin(ctx, actor.ID, actor.OrgID, model.ID)
	if err != nil || loaded.Status != domain.ModelActive || loaded.CurrentVersionID != published2.ID || loaded.Revision != 6 {
		t.Fatalf("loaded model %+v: %v", loaded, err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		UPDATE identity."user" SET role = 'producer' WHERE id = ?::uuid
	`, actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := setStatus.Execute(ctx, actor, catalogapp.SetModelStatusInput{
		ModelID: model.ID, Status: domain.ModelDisabled,
		ExpectedRevision: 6, RequestID: uuid.NewString(),
	}); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked administrator changed model: %v", err)
	}
	if _, err := store.FindModelForAdmin(ctx, actor.ID, actor.OrgID, model.ID); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked administrator read model: %v", err)
	}
}
