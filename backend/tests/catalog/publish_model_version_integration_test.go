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
	catalogapp "github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func TestPublishModelVersionCommitsAuditAndPreservesHistoryOnLocalPostgres(t *testing.T) {
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
		VALUES (?::uuid, ?::uuid, ?, 'Version Admin', 'admin', 'test-hash', false)
	`, actor.ID.String(), actor.OrgID.String(), "version-admin-"+actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	store := pgcatalog.NewStore(conn.DB)
	provider := validProvider()
	provider.Key = "version-admin-" + provider.ID.String()
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
	registered, err := catalogapp.NewCreateModelCommand(store, time.Now).Execute(ctx, actor, catalogapp.CreateModelInput{
		Key: "ark.seedance-" + uuid.NewString(), ProviderID: provider.ID,
		Capability: capability.Key, DisplayName: "Seedance", RequestID: uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	validator, err := paramvalidation.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	command := catalogapp.NewPublishModelVersionCommand(store, validator, time.Now)
	firstInput := validPublishModelVersionInput(registered.ID)
	firstInput.Limits = json.RawMessage(`{"roles":{"subject":{"max_count":1,"types":["image"]}},"max_outputs":1}`)
	first, err := command.Execute(ctx, actor, firstInput)
	if err != nil || first.ID == uuid.Nil || first.VersionNo != 1 || first.Revision != 2 {
		t.Fatalf("first version %+v: %v", first, err)
	}
	loaded, err := store.FindModelForAdmin(ctx, actor.ID, actor.OrgID, registered.ID)
	if err != nil || loaded.CurrentVersionID != first.ID || loaded.Revision != 2 || loaded.Status != domain.ModelDisabled {
		t.Fatalf("model head after first publication %+v: %v", loaded, err)
	}
	var events []struct {
		Topic        string
		PartitionKey string
		Payload      []byte
	}
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT topic, partition_key, payload FROM infra.outbox
		WHERE partition_key = ? ORDER BY create_time, id
	`, actor.OrgID.String()).Scan(&events).Error; err != nil || len(events) != 2 {
		t.Fatalf("create and publish audit events: %d, %v", len(events), err)
	}
	var publishedAuditCount int
	for _, event := range events {
		audit, parseErr := auditapp.NewRecordedActionParser().Parse(inbox.Record{
			Topic: event.Topic, Key: []byte(event.PartitionKey), Value: event.Payload,
		})
		if parseErr != nil {
			t.Fatalf("persisted audit cannot be parsed: %v", parseErr)
		}
		if audit.Action == "model.version_published" {
			publishedAuditCount++
			if audit.ObjectID != registered.ID.String() || audit.RequestID != firstInput.RequestID {
				t.Fatalf("version audit mismatch: %+v", audit)
			}
		}
	}
	if publishedAuditCount != 1 {
		t.Fatalf("first publication audits: %d", publishedAuditCount)
	}

	secondInput := validPublishModelVersionInput(registered.ID)
	secondInput.VersionNo = 2
	secondInput.ProviderModelID = "seedance-2-updated"
	secondInput.Limits = firstInput.Limits
	if _, err := command.Execute(ctx, actor, secondInput); !errors.Is(err, domain.ErrModelRevisionConflict) {
		t.Fatalf("stale model revision accepted: %v", err)
	}
	secondInput.ExpectedRevision = 2
	secondInput.VersionNo = 3
	if _, err := command.Execute(ctx, actor, secondInput); !errors.Is(err, pgcatalog.ErrModelVersionConflict) {
		t.Fatalf("skipped version number accepted: %v", err)
	}
	secondInput.VersionNo = 2
	secondInput.Modes = []string{"audio2video"}
	if _, err := command.Execute(ctx, actor, secondInput); !errors.Is(err, domain.ErrInvalidModelVersion) {
		t.Fatalf("capability-unsupported mode accepted: %v", err)
	}
	secondInput.Modes = []string{"image2video"}
	secondInput.Limits = json.RawMessage(`{"roles":{"scene":{"max_count":1,"types":["image"]}},"max_outputs":1}`)
	if _, err := command.Execute(ctx, actor, secondInput); !errors.Is(err, domain.ErrInvalidModelVersion) {
		t.Fatalf("capability-unsupported input role accepted: %v", err)
	}
	secondInput.Limits = firstInput.Limits
	second, err := command.Execute(ctx, actor, secondInput)
	if err != nil || second.ID == uuid.Nil || second.ID == first.ID || second.VersionNo != 2 || second.Revision != 3 {
		t.Fatalf("second version %+v: %v", second, err)
	}
	var firstRow struct {
		ProviderModelID string
		VersionNo       int
		MaxOutputs      string
		Description     string
	}
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT provider_model_id, version_no, limits->>'max_outputs' AS max_outputs,
		       param_schema->0->>'description' AS description
		FROM catalog.model_profile_version WHERE id = ?::uuid
	`, first.ID.String()).Scan(&firstRow).Error; err != nil || firstRow.ProviderModelID != firstInput.ProviderModelID || firstRow.VersionNo != 1 ||
		firstRow.MaxOutputs != "1" || firstRow.Description != "private-parameter-description" {
		t.Fatalf("historical version changed: %+v, %v", firstRow, err)
	}
	loaded, err = store.FindModelForAdmin(ctx, actor.ID, actor.OrgID, registered.ID)
	if err != nil || loaded.CurrentVersionID != second.ID || loaded.Revision != 3 {
		t.Fatalf("model head after second publication %+v: %v", loaded, err)
	}

	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE FUNCTION reject_version_audit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		  IF NEW.topic = 'lanverse.audit.recorded.v1' THEN RAISE EXCEPTION 'reject version audit'; END IF;
		  RETURN NEW;
		END $$
	`).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		CREATE TRIGGER reject_version_audit_before_insert BEFORE INSERT ON infra.outbox
		FOR EACH ROW EXECUTE FUNCTION reject_version_audit()
	`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = conn.DB.Exec(`DROP TRIGGER IF EXISTS reject_version_audit_before_insert ON infra.outbox`).Error
		_ = conn.DB.Exec(`DROP FUNCTION IF EXISTS reject_version_audit()`).Error
	})
	thirdInput := validPublishModelVersionInput(registered.ID)
	thirdInput.ExpectedRevision, thirdInput.VersionNo = 3, 3
	thirdInput.Limits = firstInput.Limits
	if _, err := command.Execute(ctx, actor, thirdInput); err == nil {
		t.Fatal("audit insertion failure committed a new version")
	}
	var versionCount int
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT count(*) FROM catalog.model_profile_version WHERE model_profile_id = ?::uuid
	`, registered.ID.String()).Scan(&versionCount).Error; err != nil || versionCount != 2 {
		t.Fatalf("failed audit left %d versions: %v", versionCount, err)
	}
	loaded, err = store.FindModelForAdmin(ctx, actor.ID, actor.OrgID, registered.ID)
	if err != nil || loaded.CurrentVersionID != second.ID || loaded.Revision != 3 {
		t.Fatalf("failed audit changed model head %+v: %v", loaded, err)
	}
	var eventCount int
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT count(*) FROM infra.outbox WHERE partition_key = ?
	`, actor.OrgID.String()).Scan(&eventCount).Error; err != nil || eventCount != 3 {
		t.Fatalf("failed publication left %d audit events: %v", eventCount, err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`DROP TRIGGER reject_version_audit_before_insert ON infra.outbox`).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`DROP FUNCTION reject_version_audit()`).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.DB.WithContext(ctx).Exec(`
		UPDATE identity."user" SET role = 'producer' WHERE id = ?::uuid
	`, actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := command.Execute(ctx, actor, thirdInput); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked administrator published version: %v", err)
	}
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT count(*) FROM catalog.model_profile_version WHERE model_profile_id = ?::uuid
	`, registered.ID.String()).Scan(&versionCount).Error; err != nil || versionCount != 2 {
		t.Fatalf("revoked administrator left %d versions: %v", versionCount, err)
	}
	if err := conn.DB.WithContext(ctx).Raw(`
		SELECT count(*) FROM infra.outbox WHERE partition_key = ?
	`, actor.OrgID.String()).Scan(&eventCount).Error; err != nil || eventCount != 3 {
		t.Fatalf("revoked administrator left %d audit events: %v", eventCount, err)
	}
}
