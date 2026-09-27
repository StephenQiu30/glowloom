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

func TestSetModelStatusCommitsAuditAndGuardsActivationOnLocalPostgres(t *testing.T) {
	dsn := os.Getenv("LV_TEST_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("set LV_TEST_CATALOG_DB_DSN to an isolated migrated database")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	actor := adminPrincipal()
	if err := conn.DB.WithContext(ctx).Exec(`
		INSERT INTO identity."user" (id, org_id, login_name, display_name, role, password_hash, must_change_password)
		VALUES (?::uuid, ?::uuid, ?, 'Status Admin', 'admin', 'test-hash', false)
	`, actor.ID.String(), actor.OrgID.String(), "status-admin-"+actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	store := pgcatalog.NewStore(conn.DB)
	provider := validProvider()
	provider.Key = "status-provider-" + provider.ID.String()
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
	created, err := catalogapp.NewCreateModelCommand(store, time.Now).Execute(ctx, actor, catalogapp.CreateModelInput{
		Key: "status-model-" + uuid.NewString(), ProviderID: provider.ID,
		Capability: capability.Key, DisplayName: "Status Model", RequestID: uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	command := catalogapp.NewSetModelStatusCommand(store, time.Now)
	enable := catalogapp.SetModelStatusInput{
		ModelID: created.ID, Status: domain.ModelActive,
		ExpectedRevision: 1, RequestID: uuid.NewString(),
	}
	if _, err := command.Execute(ctx, actor, enable); !errors.Is(err, domain.ErrModelNotPublishable) {
		t.Fatalf("model without version activated: %v", err)
	}
	modelValidator, err := paramvalidation.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	versionInput := validPublishModelVersionInput(created.ID)
	versionInput.Limits = json.RawMessage(`{"roles":{"subject":{"max_count":1,"types":["image"]}},"max_outputs":1}`)
	if _, err := catalogapp.NewPublishModelVersionCommand(store, modelValidator, time.Now).Execute(ctx, actor, versionInput); err != nil {
		t.Fatal(err)
	}
	enable.ExpectedRevision = 2
	if _, err := command.Execute(ctx, actor, enable); !errors.Is(err, domain.ErrModelNotPublishable) {
		t.Fatalf("model without price activated: %v", err)
	}
	priceValidator, err := pricevalidation.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	publishPrice := catalogapp.NewPublishPriceRuleCommand(store, priceValidator, time.Now)
	futurePrice := catalogapp.PublishPriceRuleInput{
		ModelID: created.ID, ExpectedRevision: 2, VersionNo: 1,
		Unit: domain.PricePerSecond, Rule: json.RawMessage(`{"base_micros":1000}`),
		Currency: "CNY", EffectiveFrom: time.Now().Add(24 * time.Hour), RequestID: uuid.NewString(),
	}
	if _, err := publishPrice.Execute(ctx, actor, futurePrice); err != nil {
		t.Fatal(err)
	}
	enable.ExpectedRevision = 3
	if _, err := command.Execute(ctx, actor, enable); !errors.Is(err, domain.ErrModelNotPublishable) {
		t.Fatalf("future-only price activated model: %v", err)
	}
	effectivePrice := futurePrice
	effectivePrice.ExpectedRevision, effectivePrice.VersionNo = 3, 2
	effectivePrice.EffectiveFrom = time.Now().Add(-time.Minute)
	effectivePrice.RequestID = uuid.NewString()
	if _, err := publishPrice.Execute(ctx, actor, effectivePrice); err != nil {
		t.Fatal(err)
	}
	if _, err := command.Execute(ctx, actor, enable); !errors.Is(err, domain.ErrModelRevisionConflict) {
		t.Fatalf("stale status revision accepted: %v", err)
	}
	enable.ExpectedRevision = 4
	active, err := command.Execute(ctx, actor, enable)
	if err != nil || active.ID != created.ID || active.Status != domain.ModelActive || active.Revision != 5 {
		t.Fatalf("enabled model %+v: %v", active, err)
	}
	if _, err := command.Execute(ctx, actor, catalogapp.SetModelStatusInput{
		ModelID: created.ID, Status: domain.ModelActive,
		ExpectedRevision: 5, RequestID: uuid.NewString(),
	}); !errors.Is(err, domain.ErrInvalidModelTransition) {
		t.Fatalf("repeated enable accepted: %v", err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE FUNCTION reject_status_audit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		  IF NEW.payload->'data'->>'action' = 'model.disabled' THEN
		    RAISE EXCEPTION 'reject status audit';
		  END IF;
		  RETURN NEW;
		END $$
	`).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE TRIGGER reject_status_audit_before_insert BEFORE INSERT ON infra.outbox
		FOR EACH ROW EXECUTE FUNCTION reject_status_audit()
	`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = conn.DB.Exec(`DROP TRIGGER IF EXISTS reject_status_audit_before_insert ON infra.outbox`).Error
		_ = conn.DB.Exec(`DROP FUNCTION IF EXISTS reject_status_audit()`).Error
	})
	disable := catalogapp.SetModelStatusInput{
		ModelID: created.ID, Status: domain.ModelDisabled,
		ExpectedRevision: 5, RequestID: uuid.NewString(),
	}
	if _, err := command.Execute(ctx, actor, disable); err == nil {
		t.Fatal("audit failure committed model disable")
	}
	loaded, err := store.FindModelForAdmin(ctx, actor.ID, actor.OrgID, created.ID)
	if err != nil || loaded.Status != domain.ModelActive || loaded.Revision != 5 {
		t.Fatalf("audit failure changed model %+v: %v", loaded, err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`DROP TRIGGER reject_status_audit_before_insert ON infra.outbox`).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`DROP FUNCTION reject_status_audit()`).Error; err != nil {
		t.Fatal(err)
	}
	disabled, err := command.Execute(ctx, actor, disable)
	if err != nil || disabled.ID != created.ID || disabled.Status != domain.ModelDisabled || disabled.Revision != 6 {
		t.Fatalf("disabled model %+v: %v", disabled, err)
	}
	var events []struct {
		Topic        string
		PartitionKey string
		Payload      []byte
	}
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT topic, partition_key, payload FROM infra.outbox WHERE partition_key = ?
	`, actor.OrgID.String()).Scan(&events).Error; err != nil || len(events) != 6 {
		t.Fatalf("model outbox events %d: %v", len(events), err)
	}
	statusAudits := map[string]int{}
	for _, event := range events {
		audit, parseErr := auditapp.NewRecordedActionParser().Parse(inbox.Record{
			Topic: event.Topic, Key: []byte(event.PartitionKey), Value: event.Payload,
		})
		if parseErr != nil {
			t.Fatalf("unparseable status audit: %v", parseErr)
		}
		if audit.Action == "model.enabled" || audit.Action == "model.disabled" {
			statusAudits[audit.Action]++
			if audit.ObjectID != created.ID.String() {
				t.Fatalf("status audit object %+v", audit)
			}
		}
	}
	if statusAudits["model.enabled"] != 1 || statusAudits["model.disabled"] != 1 {
		t.Fatalf("status audits: %+v", statusAudits)
	}
	var modelVersionCount, priceVersionCount int
	if err := conn.DB.WithContext(ctx).Raw(`SELECT count(*) FROM catalog.model_profile_version WHERE model_profile_id = ?::uuid`, created.ID.String()).Scan(&modelVersionCount).Error; err != nil || modelVersionCount != 1 {
		t.Fatalf("model history changed: %d, %v", modelVersionCount, err)
	}
	if err := conn.DB.WithContext(ctx).Raw(`SELECT count(*) FROM catalog.price_rule_version WHERE model_profile_id = ?::uuid`, created.ID.String()).Scan(&priceVersionCount).Error; err != nil || priceVersionCount != 2 {
		t.Fatalf("price history changed: %d, %v", priceVersionCount, err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`UPDATE identity."user" SET role = 'producer' WHERE id = ?::uuid`, actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	enable.ExpectedRevision = 6
	if _, err := command.Execute(ctx, actor, enable); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked administrator enabled model: %v", err)
	}
	var finalStatus struct {
		Status   string
		Revision int64
	}
	if err := conn.DB.WithContext(ctx).Raw(`SELECT status, revision FROM catalog.model_profile WHERE id = ?::uuid`, created.ID.String()).Scan(&finalStatus).Error; err != nil ||
		finalStatus.Status != string(domain.ModelDisabled) || finalStatus.Revision != 6 {
		t.Fatalf("revoked administrator changed model %+v: %v", finalStatus, err)
	}
}
