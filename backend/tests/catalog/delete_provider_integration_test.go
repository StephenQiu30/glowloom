package catalog_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"testing"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	auditapp "github.com/StephenQiu30/glowloom-server/backend/internal/audit/application"
	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	inboxapp "github.com/StephenQiu30/glowloom-server/backend/internal/infra/inbox/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func TestDeleteProviderProtectsReferencesAndReplays(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("set GLOWLOOM_TEST_CATALOG_DB_DSN to an isolated migrated database")
	}
	ctx := t.Context()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	exec := func(query string, args ...any) {
		t.Helper()
		if err := conn.DB.WithContext(ctx).Exec(query, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	actor := adminPrincipal()
	exec(`INSERT INTO identity."user" (id, org_id, login_name, display_name, role, password_hash, must_change_password, platform_role)
VALUES (?, ?, ?, 'Test Admin', 'admin', 'test-hash', false, 'platform_admin')`, actor.ID, actor.OrgID, "provider-delete-"+actor.ID.String())
	actor = identityfixture.Session(t, conn.DB, actor)
	store := app.NewCatalogStore(conn.DB)
	provider := validProvider()
	provider.Key = "provider-delete-" + provider.ID.String()
	if err := store.CreateProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceCredential(ctx, validCredential(provider.ID)); err != nil {
		t.Fatal(err)
	}
	service := catalogapp.NewProviderDeletion(store)
	preview := func() catalogapp.ProviderDeletionPreview {
		t.Helper()
		result, err := service.Preview(ctx, actor, provider.ID)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	if p := preview(); p.BlockedReason != "active" || p.Token != "" {
		t.Fatalf("active provider preview: %+v", p)
	}
	exec(`UPDATE catalog.provider SET status='disabled', revision=2 WHERE id=?`, provider.ID)
	before := preview()
	modelID := uuid.New()
	exec(`INSERT INTO catalog.model_profile(id, model_key, provider_id, capability, display_name)
VALUES (?, ?, ?, 'text.generate', 'Unpublished model')`, modelID, modelID.String(), provider.ID)
	input := catalogapp.DeleteProviderInput{Token: before.Token, RequestID: uuid.New()}
	if _, err := service.Delete(ctx, actor, provider.ID, input); !errors.Is(err, domain.ErrProviderRevisionConflict) {
		t.Fatalf("stale draft snapshot: %v", err)
	}
	versionID := uuid.New()
	exec(`INSERT INTO catalog.model_profile_version
(id,model_profile_id,version_no,provider_model_id,modes,limits,param_schema,supports_query,supports_cancel,supports_callback,expected_max_ms)
VALUES (?,?,1,'test',ARRAY['text_to_text'],'{}','[]',false,false,false,1000)`, versionID, modelID)
	exec(`UPDATE catalog.model_profile SET current_version_id=? WHERE id=?`, versionID, modelID)
	if p := preview(); p.BlockedReason != "model_reference" || p.Token != "" {
		t.Fatalf("published legacy version preview: %+v", p)
	}
	exec(`UPDATE catalog.model_profile SET candidate_snapshot='{}' WHERE id=?`, modelID)
	ready := preview()
	projectID := uuid.New()
	exec(`INSERT INTO workspace.project(id,org_id,name,aspect_ratio,style_type,default_models)
VALUES (?,?,'Delete provider test','16:9','realistic',jsonb_build_object('text.generate',?::text))`, projectID, actor.OrgID, modelID.String())
	if p := preview(); p.BlockedReason != "model_reference" {
		t.Fatalf("project default preview: %+v", p)
	}
	exec(`UPDATE workspace.project SET default_models='{}' WHERE id=?`, projectID)
	operationID := uuid.New()
	exec(`INSERT INTO operation.operation(id,project_id,capability,mode,input_hash,origin,status,region,model_profile_version_id)
VALUES (?,?,'text.generate','text_to_text','test','canvas','completed','domestic',?)`, operationID, projectID, versionID)
	if p := preview(); p.BlockedReason != "task_history" || p.Token != "" {
		t.Fatalf("historical task preview: %+v", p)
	}
	input = catalogapp.DeleteProviderInput{Token: ready.Token, RequestID: uuid.New()}
	if _, err := service.Delete(ctx, actor, provider.ID, input); !errors.Is(err, domain.ErrProviderRevisionConflict) {
		t.Fatalf("history created after confirmation was not protected: %v", err)
	}
	exec(`DELETE FROM operation.operation WHERE id=?`, operationID)
	router := adminCatalogRouter(t, conn.DB, actor)
	path := "/api/admin/providers/" + provider.ID.String()
	response := adminCatalogRequest(t, router, http.MethodGet, path+"/deletion-preview", "", "")
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &ready) != nil ||
		ready.Token == "" || len(ready.Drafts) != 1 || ready.Drafts[0].ID != modelID {
		t.Fatalf("draft preview: %d %s", response.Code, response.Body)
	}
	input = catalogapp.DeleteProviderInput{Token: ready.Token, RequestID: uuid.New()}
	body, err := json.Marshal(map[string]string{"token": input.Token})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		var result catalogapp.DeletedProvider
		response = adminCatalogRequest(t, router, http.MethodDelete, path, string(body), input.RequestID.String())
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil || result.ID != provider.ID {
			t.Fatalf("delete or replay: %d %s", response.Code, response.Body)
		}
	}
	if _, err := service.Preview(ctx, actor, provider.ID); !errors.Is(err, catalogapp.ErrProviderNotFound) {
		t.Fatalf("deleted provider visible: %v", err)
	}
	var removed bool
	if err := conn.DB.Raw(`SELECT is_delete FROM catalog.model_profile WHERE id=?`, modelID).Scan(&removed).Error; err != nil || !removed {
		t.Fatalf("draft was not removed: %v", err)
	}
	var credentials int
	if err := conn.DB.Raw(`SELECT count(*) FROM catalog.provider_credential WHERE provider_id=? AND status<>'disabled'`, provider.ID).Scan(&credentials).Error; err != nil || credentials != 0 {
		t.Fatalf("credentials remain usable: %d %v", credentials, err)
	}
	receipts := catalogapp.NewConfigReceipts(store)
	if receipt, err := receipts.Read(ctx, actor, "provider.delete", provider.ID, input.RequestID); err != nil || len(receipt.Result) == 0 {
		t.Fatalf("missing receipt: %+v %v", receipt, err)
	}
	const auditTopic = "glowloom.audit.recorded.v1"
	var audits []struct {
		PartitionKey string
		Payload      []byte
	}
	if err := conn.DB.Raw(`SELECT partition_key, payload FROM infra.outbox
WHERE topic=? AND payload->'data'->>'action'='provider.delete'
AND payload->'data'->>'request_id'=?`, auditTopic, input.RequestID.String()).Scan(&audits).Error; err != nil || len(audits) != 1 {
		t.Fatalf("delete and replay wrote %d audits: %v", len(audits), err)
	}
	parsed, err := auditapp.NewRecordedActionParser().Parse(inboxapp.Record{
		Topic: auditTopic,
		Key:   []byte(audits[0].PartitionKey),
		Value: audits[0].Payload,
	})
	if err != nil {
		t.Fatalf("audit relay rejected provider deletion: %v", err)
	}
	if parsed.Action != "provider.delete" || parsed.ObjectID != provider.ID.String() || parsed.RequestID != input.RequestID.String() {
		t.Errorf("audit relay parsed action %q, object %q, request %q", parsed.Action, parsed.ObjectID, parsed.RequestID)
	}
	exec(`UPDATE identity."user" SET platform_role='creator' WHERE id=?`, actor.ID)
	if _, err := receipts.Read(ctx, actor, "provider.delete", provider.ID, input.RequestID); err == nil {
		t.Fatal("administrator with revoked access could read the delete receipt")
	}
}
