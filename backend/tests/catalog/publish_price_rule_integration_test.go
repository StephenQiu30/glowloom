package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/paramvalidation"
	pgcatalog "github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/pricevalidation"
	catalogapp "github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestPublishPriceRuleCommitsHistoryAndAuditOnLocalPostgres(t *testing.T) {
	dsn := os.Getenv("LV_TEST_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_CATALOG_DB_DSN to an isolated migrated database")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	actor := adminPrincipal()
	if err := conn.DB.WithContext(ctx).Exec(`
		INSERT INTO identity."user" (id, org_id, login_name, display_name, role, password_hash, must_change_password)
		VALUES (?::uuid, ?::uuid, ?, 'Price Admin', 'admin', 'test-hash', false)
	`, actor.ID.String(), actor.OrgID.String(), "price-admin-"+actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	store := pgcatalog.NewStore(conn.DB)
	provider := validProvider()
	provider.Key = "price-admin-" + provider.ID.String()
	if err := store.CreateProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	capability := domain.Capability{
		ID: uuid.New(), Key: "video.generate." + uuid.NewString(), OutputType: domain.OutputVideo,
		Modes: []string{"image2video"}, InputRoles: []string{"subject"},
	}
	if err := store.CreateCapabilityForAdmin(ctx, actor.ID, actor.OrgID, capability); err != nil {
		t.Fatal(err)
	}
	registered, err := catalogapp.NewCreateModelCommand(store, time.Now).Execute(ctx, actor, catalogapp.CreateModelInput{
		Key: "price-model-" + uuid.NewString(), ProviderID: provider.ID,
		Capability: capability.Key, DisplayName: "Price Model", RequestID: uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	priceValidator, err := pricevalidation.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	publishPrice := catalogapp.NewPublishPriceRuleCommand(store, priceValidator, time.Now)
	first := catalogapp.PublishPriceRuleInput{
		ModelID: registered.ID, ExpectedRevision: 1, VersionNo: 1,
		Unit: domain.PricePerSecond, Rule: json.RawMessage(`{"base_micros":1000}`),
		Currency: "CNY", EffectiveFrom: time.Now().Add(-time.Minute), RequestID: uuid.NewString(),
	}
	if _, err := publishPrice.Execute(ctx, actor, first); !errors.Is(err, domain.ErrModelNotPublishable) {
		t.Fatalf("price without model version published: %v", err)
	}
	modelValidator, err := paramvalidation.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	versionInput := validPublishModelVersionInput(registered.ID)
	versionInput.Limits = json.RawMessage(`{"roles":{"subject":{"max_count":1,"types":["image"]}},"resolutions":["1080p"],"max_outputs":1}`)
	if _, err := catalogapp.NewPublishModelVersionCommand(store, modelValidator, time.Now).Execute(ctx, actor, versionInput); err != nil {
		t.Fatal(err)
	}
	first.ExpectedRevision = 2
	first.Rule = json.RawMessage(`{"base_micros":1000,"by_resolution":{"1080p":1.5},"by_mode":{"image2video":1.2}}`)
	bad := first
	bad.Rule = json.RawMessage(`{"base_micros":1000,"by_mode":{"unsupported":2}}`)
	if _, err := publishPrice.Execute(ctx, actor, bad); !errors.Is(err, domain.ErrInvalidPriceRule) {
		t.Fatalf("unsupported mode price published: %v", err)
	}
	bad.Rule = json.RawMessage(`{"base_micros":1000,"by_resolution":{"720p":2}}`)
	if _, err := publishPrice.Execute(ctx, actor, bad); !errors.Is(err, domain.ErrInvalidPriceRule) {
		t.Fatalf("unsupported resolution price published: %v", err)
	}
	bad.Rule = json.RawMessage(`{"by_mode":{"image2video":1.2}}`)
	if _, err := publishPrice.Execute(ctx, actor, bad); !errors.Is(err, pricevalidation.ErrInvalidConfiguration) {
		t.Fatalf("incomplete price published: %v", err)
	}
	var count int
	if err := conn.DB.WithContext(ctx).Raw(`SELECT count(*) FROM catalog.price_rule_version WHERE model_profile_id = ?::uuid`, registered.ID.String()).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("invalid prices left %d rows: %v", count, err)
	}
	if err := conn.DB.WithContext(ctx).Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key = ?`, actor.OrgID.String()).Scan(&count).Error; err != nil || count != 2 {
		t.Fatalf("invalid prices left %d audits: %v", count, err)
	}
	first.EffectiveFrom = time.Now().Add(24 * time.Hour)
	published1, err := publishPrice.Execute(ctx, actor, first)
	if err != nil || published1.VersionNo != 1 || published1.Revision != 3 || published1.ID == uuid.Nil {
		t.Fatalf("first price %+v: %v", published1, err)
	}
	var storedEffectiveFrom time.Time
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT effective_from FROM catalog.price_rule_version WHERE id = ?::uuid
	`, published1.ID.String()).Scan(&storedEffectiveFrom).Error; err != nil ||
		!storedEffectiveFrom.Equal(first.EffectiveFrom.Round(time.Microsecond)) {
		t.Fatalf("stored effective time %s: %v", storedEffectiveFrom, err)
	}
	if _, err := catalogapp.NewSetModelStatusCommand(store, time.Now).Execute(ctx, actor, catalogapp.SetModelStatusInput{
		ModelID: registered.ID, Status: domain.ModelActive,
		ExpectedRevision: 3, RequestID: uuid.NewString(),
	}); !errors.Is(err, domain.ErrModelNotPublishable) {
		t.Fatalf("future-only price activated model: %v", err)
	}
	effective := first
	effective.ExpectedRevision, effective.VersionNo = 3, 2
	effective.Currency, effective.FXRateToCNY = "USD", "7.123456"
	effective.EffectiveFrom = time.Now().Add(-time.Minute)
	effective.RequestID = uuid.NewString()
	stale := effective
	stale.ExpectedRevision = 2
	if _, err := publishPrice.Execute(ctx, actor, stale); !errors.Is(err, domain.ErrModelRevisionConflict) {
		t.Fatalf("stale price revision accepted: %v", err)
	}
	direct := validPriceVersion(registered.ID)
	direct.VersionNo, direct.CreateBy = 2, actor.ID
	if _, err := store.PublishPriceRuleWithAudit(ctx, actor.ID, actor.OrgID, direct, 3, identityapp.OutboxEvent{}); !errors.Is(err, catalogapp.ErrInvalidPublishPriceRule) {
		t.Fatalf("price without audit accepted: %v", err)
	}
	if _, err := publishPrice.Execute(ctx, actor, effective); err != nil {
		t.Fatalf("effective price publication: %v", err)
	}
	var original struct {
		Currency   string
		BaseMicros string
		FXRate     *string
	}
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT currency, rule->>'base_micros' AS base_micros, fx_rate_to_cny::text AS fx_rate
		FROM catalog.price_rule_version WHERE id = ?::uuid
	`, published1.ID.String()).Scan(&original).Error; err != nil || original.Currency != "CNY" || original.BaseMicros != "1000" {
		t.Fatalf("historical price changed: %+v, %v", original, err)
	}
	var events []struct {
		Topic        string
		PartitionKey string
		Payload      []byte
	}
	if err := conn.DB.WithContext(ctx).Raw(`SELECT topic, partition_key, payload FROM infra.outbox WHERE partition_key = ?`, actor.OrgID.String()).Scan(&events).Error; err != nil {
		t.Fatal(err)
	}
	priceAudits := 0
	for _, event := range events {
		audit, parseErr := auditapp.NewRecordedActionParser().Parse(inbox.Record{
			Topic: event.Topic, Key: []byte(event.PartitionKey), Value: event.Payload,
		})
		if parseErr != nil {
			t.Fatalf("persisted audit cannot be parsed: %v", parseErr)
		}
		if audit.Action == "price.published" {
			priceAudits++
			if audit.ObjectType != "model_profile" || audit.ObjectID != registered.ID.String() {
				t.Fatalf("price audit object %+v", audit)
			}
		}
	}
	if priceAudits != 2 {
		t.Fatalf("expected two price audits, got %d", priceAudits)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE FUNCTION reject_price_audit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		  IF NEW.payload->'data'->>'action' = 'price.published' THEN
		    RAISE EXCEPTION 'reject price audit';
		  END IF;
		  RETURN NEW;
		END $$
	`).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`CREATE TRIGGER reject_price_audit_before_insert BEFORE INSERT ON infra.outbox FOR EACH ROW EXECUTE FUNCTION reject_price_audit()`).Error; err != nil {
		t.Fatal(err)
	}
	third := first
	third.ExpectedRevision, third.VersionNo, third.RequestID = 4, 3, uuid.NewString()
	if _, err := publishPrice.Execute(ctx, actor, third); err == nil {
		t.Fatal("audit failure committed a price")
	}
	if err := conn.DB.WithContext(ctx).Raw(`SELECT count(*) FROM catalog.price_rule_version WHERE model_profile_id = ?::uuid`, registered.ID.String()).Scan(&count).Error; err != nil || count != 2 {
		t.Fatalf("failed audit left %d prices: %v", count, err)
	}
	loaded, err := store.FindModelForAdmin(ctx, actor.ID, actor.OrgID, registered.ID)
	if err != nil || loaded.Revision != 4 {
		t.Fatalf("failed audit advanced model revision: %+v, %v", loaded, err)
	}
	if err := conn.DB.WithContext(ctx).Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key = ?`, actor.OrgID.String()).Scan(&count).Error; err != nil || count != 4 {
		t.Fatalf("failed audit left %d events: %v", count, err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`DROP TRIGGER reject_price_audit_before_insert ON infra.outbox`).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`DROP FUNCTION reject_price_audit()`).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`UPDATE identity."user" SET role = 'producer' WHERE id = ?::uuid`, actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := publishPrice.Execute(ctx, actor, third); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked administrator published price: %v", err)
	}
}
