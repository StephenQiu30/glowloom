package prompt_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	cataloghttp "github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/http"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	prompthttp "github.com/StephenQiu30/lanverse/backend/internal/prompt/adapter/http"
	promptpg "github.com/StephenQiu30/lanverse/backend/internal/prompt/adapter/postgres"
	promptapp "github.com/StephenQiu30/lanverse/backend/internal/prompt/application"
	"github.com/StephenQiu30/lanverse/backend/internal/prompt/domain"
	workspacehttp "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/http"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	workspacedomain "github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

type preferenceFixture struct {
	db         *gorm.DB
	actor      identityapp.Principal
	project    uuid.UUID
	model      uuid.UUID
	provider   uuid.UUID
	capability string
	modelKey   string
}

func newPreferenceFixture(t *testing.T) preferenceFixture {
	t.Helper()
	dsn := os.Getenv("LV_TEST_OPERATION_STORE_DB_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL DSN not configured")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Hostname() != "127.0.0.1" || parsed.Path != "/lanverse_receipt" {
		t.Fatal("preferences tests require the task's isolated receipt database")
	}
	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("open isolated preference database")
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Error(err)
		}
	})
	fixture := preferenceFixture{db: database, actor: identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: "producer"}, project: uuid.New(), model: uuid.New(), provider: uuid.New()}
	fixture.capability, fixture.modelKey = "image.prefs."+uuid.NewString(), "prefs."+fixture.model.String()
	version := uuid.New()
	commands := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workspace.organization(id,name) VALUES(?,'Preference fixture')`, []any{fixture.actor.OrgID}},
		{`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password) VALUES(?,?,?,'Preference fixture','producer','synthetic-test-hash',false)`, []any{fixture.actor.ID, fixture.actor.OrgID, "prefs-" + fixture.actor.ID.String()}},
		{`INSERT INTO workspace.project(id,org_id,name,aspect_ratio,style_type) VALUES(?,?,'Preference fixture','16:9','realistic')`, []any{fixture.project, fixture.actor.OrgID}},
		{`INSERT INTO catalog.provider(id,key,name,adapter_key,region,status,concurrency_limit,rate_limit_per_min) VALUES(?,?,'Preference fixture','mock','domestic','active',1,60)`, []any{fixture.provider, "prefs-" + fixture.provider.String()}},
		{`INSERT INTO catalog.capability(id,key,output_type,modes,input_roles) VALUES(?,?,'image',ARRAY['text_to_image'],ARRAY[]::text[])`, []any{uuid.New(), fixture.capability}},
		{`INSERT INTO catalog.model_profile(id,model_key,provider_id,capability,display_name,status) VALUES(?,?,?,?,'Preference fixture','disabled')`, []any{fixture.model, fixture.modelKey, fixture.provider, fixture.capability}},
		{`INSERT INTO catalog.model_profile_version(id,model_profile_id,version_no,provider_model_id,modes,limits,param_schema,supports_query,supports_cancel,supports_callback,expected_max_ms,moderation,queue) VALUES(?,?,1,'fixture-model',ARRAY['text_to_image'],'{"max_outputs":1}','[]',false,false,false,1000,'platform','agent.mock')`, []any{version, fixture.model}},
		{`INSERT INTO catalog.price_rule_version(id,model_profile_id,version_no,unit,rule,currency,effective_from) VALUES(?,?,1,'per_image','{"base_micros":1000000}','CNY',now()-interval '1 day')`, []any{uuid.New(), fixture.model}},
		{`UPDATE catalog.model_profile SET current_version_id=?,status='active' WHERE id=?`, []any{version, fixture.model}},
	}
	for _, command := range commands {
		if err := database.WithContext(t.Context()).Exec(command.sql, command.args...).Error; err != nil {
			t.Fatal("create synthetic preference fixture: ", err)
		}
	}
	return fixture
}

func TestWorkspaceModelDefaultsPersistAndGuard(t *testing.T) {
	f := newPreferenceFixture(t)
	service := workspaceapp.NewModelDefaultsService(workspacepg.NewStore(f.db), time.Now)
	input := workspaceapp.ModelDefaultsChange{ProjectID: f.project, ExpectedRevision: 1, DefaultModels: map[string]string{f.capability: f.modelKey}, IdempotencyKey: uuid.New(), RequestID: uuid.NewString()}
	saved, err := service.Save(t.Context(), f.actor, input)
	if err != nil || saved.Revision != 2 {
		t.Fatalf("save defaults: revision=%d error=%v", saved.Revision, err)
	}
	reloaded, err := service.Read(t.Context(), f.actor, f.project)
	if err != nil || !maps.Equal(reloaded.DefaultModels, input.DefaultModels) {
		t.Fatal("defaults did not survive a real database reread", err)
	}
	replay, err := service.Save(t.Context(), f.actor, input)
	if err != nil || replay.Revision != 2 {
		t.Fatal("same request did not replay", err)
	}
	changed := input
	changed.DefaultModels = map[string]string{}
	if _, err := service.Save(t.Context(), f.actor, changed); !errors.Is(err, workspaceapp.ErrIdempotencyConflict) {
		t.Fatal("different body reused a request key", err)
	}
	changed.IdempotencyKey = uuid.New()
	if _, err := service.Save(t.Context(), f.actor, changed); !errors.Is(err, workspacedomain.ErrProjectRevisionConflict) {
		t.Fatal("stale revision was accepted", err)
	}
	if err := f.db.Exec(`UPDATE catalog.model_profile SET status='disabled' WHERE id=?`, f.model).Error; err != nil {
		t.Fatal(err)
	}
	unavailable := input
	unavailable.ExpectedRevision, unavailable.IdempotencyKey = 2, uuid.New()
	if _, err := service.Save(t.Context(), f.actor, unavailable); !errors.Is(err, workspaceapp.ErrDefaultModelUnavailable) {
		t.Fatal("disabled model accepted", err)
	}
	other := newPreferenceFixture(t)
	if _, err := service.Read(t.Context(), other.actor, f.project); !errors.Is(err, workspaceapp.ErrProjectNotFound) {
		t.Fatal("cross-organization project disclosed", err)
	}
	if err := f.db.Exec(`UPDATE workspace.project SET status='archived' WHERE id=?`, f.project).Error; err != nil {
		t.Fatal(err)
	}
	changed.ExpectedRevision = 2
	if _, err := service.Save(t.Context(), f.actor, changed); !errors.Is(err, workspacedomain.ErrProjectStateConflict) {
		t.Fatal("archived defaults were writable", err)
	}
	var audits int64
	if err := f.db.Raw(`SELECT count(*) FROM infra.outbox WHERE payload->'actor'->>'id'=? AND payload->'data'->>'action'='project.defaults_changed'`, f.actor.ID.String()).Scan(&audits).Error; err != nil || audits != 1 {
		t.Fatal("defaults audit was duplicated or absent", err, audits)
	}
}

func TestPromptPreferencesPersistenceConcurrencyAndIsolation(t *testing.T) {
	f := newPreferenceFixture(t)
	service := promptapp.NewPreferences(promptpg.NewStore(f.db), time.Now)
	definition, _ := domain.DefinitionFor("short_drama_outline")
	input := promptapp.SaveInput{Operation: definition.Operation, Mode: domain.Append, Content: "保持 {{章节数量}} 章节的悬念", BaseTemplateID: definition.TemplateID, ExpectedRevision: 0, IdempotencyKey: uuid.New(), RequestID: uuid.NewString()}
	saved, err := service.Save(t.Context(), f.actor, input)
	if err != nil || saved.Revision != 1 {
		t.Fatal("save prompt preference", err)
	}
	replay, err := service.Save(t.Context(), f.actor, input)
	if err != nil || replay.ID != saved.ID || !replay.UpdateTime.Equal(saved.UpdateTime) {
		t.Fatal("prompt receipt was not stable", err)
	}
	preferences, err := promptapp.NewPreferences(promptpg.NewStore(f.db), time.Now).List(t.Context(), f.actor)
	if err != nil || len(preferences) != 9 {
		t.Fatal("new service did not reread all definitions", err)
	}
	found := false
	for _, preference := range preferences {
		if preference.Definition.Operation == definition.Operation {
			found = preference.Customization != nil && preference.Customization.Content == input.Content
		}
	}
	if !found {
		t.Fatal("personal content missing after reread")
	}
	changed := input
	changed.Content = "不同正文"
	if _, err := service.Save(t.Context(), f.actor, changed); !errors.Is(err, promptapp.ErrIdempotencyConflict) {
		t.Fatal("prompt key reused with different content", err)
	}
	changed.IdempotencyKey = uuid.New()
	if _, err := service.Save(t.Context(), f.actor, changed); !errors.Is(err, promptapp.ErrRevisionConflict) {
		t.Fatal("stale prompt revision accepted", err)
	}
	otherActor := identityapp.Principal{ID: uuid.New(), OrgID: f.actor.OrgID, Role: "producer"}
	if err := f.db.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password) VALUES(?,?,?,'Other preference fixture','producer','synthetic-test-hash',false)`, otherActor.ID, otherActor.OrgID, "prefs-"+otherActor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	otherPreferences, err := service.List(t.Context(), otherActor)
	if err != nil {
		t.Fatal(err)
	}
	for _, preference := range otherPreferences {
		if preference.Customization != nil {
			t.Fatal("another workspace actor's preference disclosed")
		}
	}
	definition, _ = domain.DefinitionFor("skill_draft")
	var group sync.WaitGroup
	errorsSeen := make(chan error, 2)
	for range 2 {
		group.Go(func() {
			_, err := service.Save(t.Context(), f.actor, promptapp.SaveInput{Operation: definition.Operation, Mode: domain.Append, Content: "合成首次并发写", BaseTemplateID: definition.TemplateID, IdempotencyKey: uuid.New(), RequestID: uuid.NewString()})
			errorsSeen <- err
		})
	}
	group.Wait()
	close(errorsSeen)
	successes, conflicts := 0, 0
	for err := range errorsSeen {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, promptapp.ErrRevisionConflict):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("first-write CAS success=%d conflict=%d", successes, conflicts)
	}
	if err := f.db.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, f.actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.List(t.Context(), f.actor); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("disabled actor read private preferences", err)
	}
}

func TestPromptPreferenceAuditFailureRollsBack(t *testing.T) {
	f := newPreferenceFixture(t)
	name := "prefs_fault_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	statement := fmt.Sprintf(`CREATE FUNCTION infra.%s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
	 IF NEW.payload->'actor'->>'id'='%s' AND NEW.payload->'data'->>'action'='prompt.customization_saved' THEN RAISE EXCEPTION 'synthetic preference audit failure'; END IF; RETURN NEW; END $$;
	 CREATE TRIGGER %s BEFORE INSERT ON infra.outbox FOR EACH ROW EXECUTE FUNCTION infra.%s()`, name, f.actor.ID, name, name)
	if err := f.db.Exec(statement).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.db.Exec(fmt.Sprintf(`DROP TRIGGER %s ON infra.outbox; DROP FUNCTION infra.%s()`, name, name)).Error; err != nil {
			t.Error(err)
		}
	})
	definition, _ := domain.DefinitionFor("skill_draft")
	key := uuid.New()
	_, err := promptapp.NewPreferences(promptpg.NewStore(f.db), time.Now).Save(t.Context(), f.actor, promptapp.SaveInput{Operation: definition.Operation, Mode: domain.Append, Content: "合成不可写审计", BaseTemplateID: definition.TemplateID, IdempotencyKey: key, RequestID: uuid.NewString()})
	if err == nil {
		t.Fatal("fault did not stop preference commit")
	}
	var records int64
	if err := f.db.Raw(`SELECT (SELECT count(*) FROM workspace.prompt_customization WHERE owner_id=?)+(SELECT count(*) FROM infra.idempotency_record WHERE actor_id=? AND idem_key=?)`, f.actor.ID, f.actor.ID, key.String()).Scan(&records).Error; err != nil || records != 0 {
		t.Fatal("preference or receipt escaped transaction rollback", err, records)
	}
}

func preferenceRouter(f preferenceFixture) *gin.Engine {
	router := gin.New()
	router.Use(httpapi.Middleware("http://localhost:3000"))
	router.Use(func(c *gin.Context) { c.Set("principal", f.actor) })
	group := router.Group("/api")
	prompthttp.NewHandler(promptapp.NewPreferences(promptpg.NewStore(f.db), time.Now)).Register(group)
	workspacehttp.NewModelDefaultsHandler(workspaceapp.NewModelDefaultsService(workspacepg.NewStore(f.db), time.Now)).Register(group)
	cataloghttp.NewAdminHandler(cataloghttp.AdminDependencies{}).Register(group)
	return router
}

func TestPreferencePublicHTTPProducerAndSecurity(t *testing.T) {
	f := newPreferenceFixture(t)
	router := preferenceRouter(f)
	call := func(method, path, body, origin, key string) *httptest.ResponseRecorder {
		request := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
		request.Header.Set("Origin", origin)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", key)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	response := call("GET", "/api/settings/prompt-preferences", "", "", "")
	var preferences prompthttp.PreferencesResponse
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &preferences) != nil || len(preferences.Items) != 9 || response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("producer cannot read preference catalog", response.Code)
	}
	definition := preferences.Items[0].Definition
	body := fmt.Sprintf(`{"expected_revision":0,"base_template_id":"%s","mode":"append","content":"合成个人策略"}`, definition.TemplateID)
	path := "/api/settings/prompt-preferences/" + definition.Operation
	if response := call("PUT", path, body, "http://other.invalid", uuid.NewString()); response.Code != 403 {
		t.Fatal("foreign origin accepted", response.Code)
	}
	if response := call("PUT", path, body, "http://localhost:3000", ""); response.Code != 422 {
		t.Fatal("missing request key accepted", response.Code)
	}
	if response := call("PUT", path, body, "http://localhost:3000", uuid.NewString()); response.Code != 200 {
		t.Fatal("producer cannot save own preference", response.Code)
	}
	if response := call("GET", "/api/admin/providers", "", "", ""); response.Code != 403 {
		t.Fatal("preference access elevated producer management", response.Code)
	}
}

func TestPreferenceApplicationRoleAndOutdatedBaseline(t *testing.T) {
	f := newPreferenceFixture(t)
	parsed, err := url.Parse(os.Getenv("LV_TEST_OPERATION_STORE_DB_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	database, err := gorm.Open(postgres.Open(parsed.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("open isolated application-role connection")
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	// The application role deliberately has NOLOGIN. Keep one task-owned connection
	// and switch its effective role to verify the exact table grants without changing it.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	if err := database.Exec(`SET ROLE lanverse_app`).Error; err != nil {
		t.Fatal("switch isolated connection to application role", err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Error(err)
		}
	})
	prompts := promptapp.NewPreferences(promptpg.NewStore(database), time.Now)
	definition, _ := domain.DefinitionFor("storyboard_plan")
	saved, err := prompts.Save(t.Context(), f.actor, promptapp.SaveInput{Operation: definition.Operation, Mode: domain.Rewrite, Content: "合成替换创作模板", BaseTemplateID: definition.TemplateID, IdempotencyKey: uuid.New(), RequestID: uuid.NewString()})
	if err != nil {
		t.Fatal("application role cannot persist its granted preference", err)
	}
	oldBase := uuid.New()
	if err := f.db.Exec(`UPDATE workspace.prompt_customization SET base_template_id=? WHERE id=?`, oldBase, saved.ID).Error; err != nil {
		t.Fatal(err)
	}
	preferences, err := prompts.List(t.Context(), f.actor)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, preference := range preferences {
		if preference.Definition.Operation == definition.Operation {
			found = preference.Outdated && preference.Customization.BaseTemplateID == oldBase && preference.Customization.Content == saved.Content
		}
	}
	if !found {
		t.Fatal("old rewrite was silently rebased or lost")
	}
	defaults := workspaceapp.NewModelDefaultsService(workspacepg.NewStore(database), time.Now)
	if _, err := defaults.Save(t.Context(), f.actor, workspaceapp.ModelDefaultsChange{ProjectID: f.project, ExpectedRevision: 1, DefaultModels: map[string]string{f.capability: f.modelKey}, IdempotencyKey: uuid.New(), RequestID: uuid.NewString()}); err != nil {
		t.Fatal("application role cannot save project defaults through granted tables", err)
	}
}
