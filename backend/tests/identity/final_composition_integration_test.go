package identity_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func TestFinalUserRouterSeparatesIdentityAndContentScopes(t *testing.T) {
	tx := existingIdentityTransaction(t)
	cfg := identityfixture.Configure(t, config.Config{Env: "local", HTTPAddr: "127.0.0.1:8080", PublicOrigin: "http://localhost:3200"})
	login := identityfixture.Create(t, tx, cfg, "Final router", "long final password")
	if err := tx.Exec(`UPDATE identity."user" SET must_change_password=true WHERE id=?`, login.User.ID).Error; err != nil {
		t.Fatal(err)
	}
	router, err := app.NewBusinessRouter(zap.NewNop(), nil, noop.NewTracerProvider(), cfg, tx, nil)
	if err != nil {
		t.Fatal(err)
	}
	scope := identityfixture.Scope(t, tx, login.User.ID)
	call := func(path, scope string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: "lanverse_session", Value: login.Token})
		if scope != "" {
			req.Header.Set("X-Creation-Scope", scope)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	for _, path := range []string{"/api/me", "/api/me/preferences", "/api/me/spaces", "/api/me/sessions", "/api/settings/prompt-preferences"} {
		if rec := call(path, ""); rec.Code != 200 {
			t.Errorf("identity-only %s requires creation scope: %d %s", path, rec.Code, rec.Body.String())
		}
	}
	for _, path := range []string{"/api/projects", "/api/projects/" + uuid.NewString() + "/canvases", "/api/projects/" + uuid.NewString()} {
		if rec := call(path, ""); rec.Code != 422 || !strings.Contains(rec.Body.String(), "invalid_creation_scope") {
			t.Errorf("content %s omitted explicit scope boundary: %d", path, rec.Code)
		}
	}
	if rec := call("/api/projects", scope.String()); rec.Code != 200 {
		t.Fatalf("personal scope failed: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call("/api/platform/users", ""); rec.Code != 403 {
		t.Fatalf("creator accessed platform directory: %d", rec.Code)
	}
	for _, path := range []string{"/api/accounts/register", "/api/accounts/availability", "/api/admin/users"} {
		if rec := call(path, ""); rec.Code != 404 {
			t.Errorf("legacy path survives: %s %d", path, rec.Code)
		}
	}
	var me map[string]json.RawMessage
	if err := json.Unmarshal(call("/api/me", "").Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	var revision string
	if err := json.Unmarshal(me["revision"], &revision); err != nil || revision == "" {
		t.Fatal("profile revision is not a JSON string")
	}
	if err := tx.Exec(`UPDATE identity.user_session SET revoked_at=now() WHERE id=?`, login.Session.ID).Error; err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/me", "/api/me/spaces", "/api/projects", "/api/platform/users"} {
		if rec := call(path, scope.String()); rec.Code != 401 {
			t.Errorf("revoked session reached %s: %d", path, rec.Code)
		}
	}
}

func TestMinimumUserRuntimeDependencyIncrement(t *testing.T) {
	verifyProjectConsumerRollback(t)
	tx := existingIdentityTransaction(t)
	var before struct{ Assets, Renditions, Copies, Transfers int64 }
	counts := `SELECT (SELECT count(*) FROM media.media_asset) AS assets,(SELECT count(*) FROM media.rendition) AS renditions,(SELECT count(*) FROM media.project_copy_object) AS copies,(SELECT count(*) FROM media.transfer_object) AS transfers`
	if err := tx.Raw(counts).Scan(&before).Error; err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../db/migrations/012_user_runtime_dependencies.sql")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := tx.Exec(string(migration)).Error; err != nil {
			t.Fatal("minimum additive migration or replay failed", err)
		}
	}
	var after struct{ Assets, Renditions, Copies, Transfers int64 }
	if err := tx.Raw(counts).Scan(&after).Error; err != nil || before != after {
		t.Fatal("minimum increment changed existing content rows", err)
	}
	var columns int64
	if err := tx.Raw(`SELECT count(*) FROM information_schema.columns WHERE table_schema='media' AND ((table_name IN ('media_asset','rendition') AND column_name='object_location_id') OR (table_name IN ('project_copy_object','transfer_object') AND column_name IN ('source_location_id','target_location_id'))) AND data_type='uuid' AND is_nullable='YES'`).Scan(&columns).Error; err != nil || columns != 6 {
		t.Fatal("runtime dependency columns incomplete", err)
	}
	var tables int64
	if err := tx.Raw(`SELECT count(*) FROM information_schema.tables WHERE table_schema='media' AND table_name IN ('package_job','package_command','package_object')`).Scan(&tables).Error; err != nil || tables != 3 {
		t.Fatal("package dependency tables incomplete", err)
	}
	var constraints int64
	if err := tx.Raw(`SELECT count(*) FROM pg_constraint WHERE conrelid IN ('media.package_job'::regclass,'media.package_command'::regclass,'media.package_object'::regclass) AND contype='f'`).Scan(&constraints).Error; err != nil || constraints != 8 {
		t.Fatal("package identity/resource foreign keys incomplete", constraints, err)
	}
}

// The installed runtime lacks these exact dependencies until the reviewed increment
// is applied. Tests exercise the target schema only inside their outer rollback.
func installUserRuntimeDependencies(t *testing.T, tx *gorm.DB) {
	t.Helper()
	migration, err := os.ReadFile("../../db/migrations/012_user_runtime_dependencies.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(string(migration)).Error; err != nil {
		t.Fatal(err)
	}
}
