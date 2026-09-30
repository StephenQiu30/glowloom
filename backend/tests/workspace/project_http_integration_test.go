package workspace_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	workspacehttp "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/http"
	pgworkspace "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

const workspaceTestOrigin = "http://127.0.0.1:3140"

// The adapter fixture uses a controlled authenticated principal; durable account
// and organization authorization still runs against isolated PostgreSQL.
func projectHTTPRouter(database *gorm.DB, actor identityapp.Principal) *gin.Engine {
	router := gin.New()
	router.Use(httpapi.Middleware(workspaceTestOrigin))
	api := router.Group("/api")
	api.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	store := pgworkspace.NewStore(database)
	workspacehttp.NewHandler(workspaceapp.NewListProjectsQuery(store), workspaceapp.NewCreateProjectCommand(store, time.Now), workspaceapp.NewListStylePresetsQuery(store)).Register(api)
	return router
}

func projectHTTPRequest(ctx context.Context, router http.Handler, method, path, key string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(ctx, method, path, bytes.NewReader(body))
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", workspaceTestOrigin)
		req.Header.Set("Idempotency-Key", key)
		req.Header.Set("X-CSRF-Token", httpapi.CSRFToken("controlled-test-session"))
		req.AddCookie(&http.Cookie{Name: "lv_session", Value: "controlled-test-session"})
		req.AddCookie(&http.Cookie{Name: "lv_csrf", Value: httpapi.CSRFToken("controlled-test-session")})
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func TestProjectHTTPCreatePersistsFirstResponseAcrossConcurrentRequests(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, database, insertWorkspaceOrganization(ctx, t, database))
	router := projectHTTPRouter(database, actor)
	key := uuid.NewString()
	body := []byte(`{"name":"真实项目闭环","description":"不进入审计的描述","aspect_ratio":"16:9","style_type":"stylized","style_subtype":"guofeng_xianxia"}`)
	const workers = 8
	responses := make([]*httptest.ResponseRecorder, workers)
	var wait sync.WaitGroup
	for i := range workers {
		wait.Go(func() {
			responses[i] = projectHTTPRequest(t.Context(), router, http.MethodPost, "/api/projects", key, body)
		})
	}
	wait.Wait()
	var created workspaceapp.CreatedProject
	for i, response := range responses {
		if response.Code != 201 {
			t.Fatalf("create response %d: status=%d body=%s", i, response.Code, response.Body.String())
		}
		if i == 0 {
			if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
				t.Fatal(err)
			}
		} else if !bytes.Equal(response.Body.Bytes(), responses[0].Body.Bytes()) {
			t.Fatal("concurrent replay returned a different first result")
		}
	}
	if created.ID == uuid.Nil || created.OrgID != actor.OrgID || created.Revision != 1 || created.AllowOverseasModels || created.Resolution != "1080p" {
		t.Fatalf("invalid created project: %+v", created)
	}
	// A new store/router stands in for a new API process; later mutable project
	// changes must not replace the original create response snapshot.
	if err := database.Exec(`UPDATE workspace.project SET name='后续名称', revision=revision+1 WHERE id=?`, created.ID).Error; err != nil {
		t.Fatal(err)
	}
	replay := projectHTTPRequest(t.Context(), projectHTTPRouter(database, actor), http.MethodPost, "/api/projects", key, body)
	if replay.Code != 201 || !bytes.Equal(replay.Body.Bytes(), responses[0].Body.Bytes()) {
		t.Fatalf("persistent replay: status=%d body=%s", replay.Code, replay.Body.String())
	}
	var counts struct{ Projects, Budgets, Events, Receipts int64 }
	if err := database.Raw(`SELECT
	 (SELECT count(*) FROM workspace.project WHERE org_id=?) AS projects,
	 (SELECT count(*) FROM billing.budget WHERE project_id=?) AS budgets,
	 (SELECT count(*) FROM infra.outbox WHERE partition_key=?) AS events,
	 (SELECT count(*) FROM infra.idempotency_record WHERE actor_id=? AND idem_key=?) AS receipts`, actor.OrgID, created.ID, created.ID.String(), actor.ID, key).Scan(&counts).Error; err != nil {
		t.Fatal(err)
	}
	if counts.Projects != 1 || counts.Budgets != 1 || counts.Events != 2 || counts.Receipts != 1 {
		t.Fatalf("duplicate creation effects: %+v", counts)
	}
	changed := bytes.Replace(body, []byte("真实项目闭环"), []byte("异体项目"), 1)
	mismatch := projectHTTPRequest(t.Context(), router, http.MethodPost, "/api/projects", key, changed)
	if mismatch.Code != 422 || !bytes.Contains(mismatch.Body.Bytes(), []byte(`"code":"idempotency_key_reused"`)) {
		t.Fatalf("key reuse response: %d %s", mismatch.Code, mismatch.Body.String())
	}
	var audit struct{ Payload []byte }
	if err := database.Raw(`SELECT payload FROM infra.outbox WHERE partition_key=? AND topic='lanverse.audit.recorded.v1'`, created.ID.String()).Scan(&audit).Error; err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(audit.Payload, []byte("真实项目闭环")) || bytes.Contains(audit.Payload, []byte("不进入审计")) {
		t.Fatal("project content leaked into audit")
	}
}

func TestProjectHTTPCreateRechecksScopeAndRollsBackReceiptFailure(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, database, insertWorkspaceOrganization(ctx, t, database))
	router := projectHTTPRouter(database, actor)
	body := []byte(`{"name":"回执原子事务","aspect_ratio":"9:16","style_type":"realistic"}`)
	key := uuid.NewString()
	identifier := strings.ReplaceAll(uuid.NewString(), "-", "")
	function := "reject_creation_receipt_" + identifier
	trigger := "reject_creation_receipt_before_" + identifier
	if err := database.Exec(fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
	 BEGIN IF NEW.actor_id='%s'::uuid AND NEW.idem_key='%s' THEN RAISE EXCEPTION 'owned receipt failure'; END IF; RETURN NEW; END $$`, function, actor.ID, key)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Exec("DROP FUNCTION IF EXISTS " + function + "()").Error })
	if err := database.Exec(fmt.Sprintf(`CREATE TRIGGER %s BEFORE INSERT ON infra.idempotency_record FOR EACH ROW EXECUTE FUNCTION %s()`, trigger, function)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Exec("DROP TRIGGER IF EXISTS " + trigger + " ON infra.idempotency_record").Error })
	failed := projectHTTPRequest(t.Context(), router, http.MethodPost, "/api/projects", key, body)
	if failed.Code != 503 {
		t.Fatalf("receipt failure: %d %s", failed.Code, failed.Body.String())
	}
	var counts struct{ Projects, Budgets, Events, Receipts int64 }
	if err := database.Raw(`SELECT
	 (SELECT count(*) FROM workspace.project WHERE org_id=?) AS projects,
	 (SELECT count(*) FROM billing.budget b JOIN workspace.project p ON p.id=b.project_id WHERE p.org_id=?) AS budgets,
	 (SELECT count(*) FROM infra.outbox WHERE payload->>'org_id'=?) AS events,
	 (SELECT count(*) FROM infra.idempotency_record WHERE actor_id=?) AS receipts`, actor.OrgID, actor.OrgID, actor.OrgID.String(), actor.ID).Scan(&counts).Error; err != nil {
		t.Fatal(err)
	}
	if counts.Projects != 0 || counts.Budgets != 0 || counts.Events != 0 || counts.Receipts != 0 {
		t.Fatalf("receipt failure leaked atomic effects: %+v", counts)
	}
	if err := database.Exec("DROP TRIGGER " + trigger + " ON infra.idempotency_record").Error; err != nil {
		t.Fatal(err)
	}
	created := projectHTTPRequest(t.Context(), router, http.MethodPost, "/api/projects", key, body)
	if created.Code != 201 {
		t.Fatalf("retry after rollback: %d %s", created.Code, created.Body.String())
	}
	for _, test := range []struct {
		name, statement, restore string
		id                       uuid.UUID
	}{
		{"disabled actor", `UPDATE identity."user" SET status='disabled' WHERE id=?`, `UPDATE identity."user" SET status='active' WHERE id=?`, actor.ID},
		{"must change password", `UPDATE identity."user" SET must_change_password=true WHERE id=?`, `UPDATE identity."user" SET must_change_password=false WHERE id=?`, actor.ID},
		{"disabled organization", `UPDATE workspace.organization SET status='disabled' WHERE id=?`, `UPDATE workspace.organization SET status='active' WHERE id=?`, actor.OrgID},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := database.Exec(test.statement, test.id).Error; err != nil {
				t.Fatal(err)
			}
			response := projectHTTPRequest(t.Context(), router, http.MethodPost, "/api/projects", key, body)
			if response.Code != 403 {
				t.Fatalf("revoked caller replay: %d %s", response.Code, response.Body.String())
			}
			if err := database.Exec(test.restore, test.id).Error; err != nil {
				t.Fatal(err)
			}
		})
	}
	var project workspaceapp.CreatedProject
	if err := json.Unmarshal(created.Body.Bytes(), &project); err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`UPDATE workspace.project SET is_delete=true,delete_time=now(),purge_after=now()+interval '30 days' WHERE id=?`, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	hidden := projectHTTPRequest(t.Context(), router, http.MethodPost, "/api/projects", key, body)
	if hidden.Code != 404 {
		t.Fatalf("deleted project replay disclosed original object: %d %s", hidden.Code, hidden.Body.String())
	}
}

func TestProjectHTTPListDeletedAndBindsCursorToCallerAndFilters(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(ctx, t, database)
	actor := insertWorkspaceActor(ctx, t, database, orgID)
	for range 3 {
		insertWorkspaceProject(ctx, t, database, orgID, "9:16", "realistic", nil)
	}
	deletedID := insertWorkspaceProject(ctx, t, database, orgID, "9:16", "realistic", nil)
	if err := database.Exec(`UPDATE workspace.project SET is_delete=true,delete_time=now(),purge_after=now()+interval '30 days' WHERE id=?`, deletedID).Error; err != nil {
		t.Fatal(err)
	}
	router := projectHTTPRouter(database, actor)
	deleted := projectHTTPRequest(t.Context(), router, http.MethodGet, "/api/projects?deleted=true", "", nil)
	var recycled workspacehttp.ListResponse
	if err := json.Unmarshal(deleted.Body.Bytes(), &recycled); err != nil || deleted.Code != 200 || len(recycled.Items) != 1 || !recycled.Items[0].IsDelete || recycled.Items[0].ID.String() != deletedID {
		t.Fatalf("recycled list: status=%d page=%+v err=%v", deleted.Code, recycled, err)
	}
	first := projectHTTPRequest(t.Context(), router, http.MethodGet, "/api/projects?limit=1&q=%20project-%20", "", nil)
	var page workspacehttp.ListResponse
	if err := json.Unmarshal(first.Body.Bytes(), &page); err != nil || first.Code != 200 || len(page.Items) != 1 || page.NextCursor == nil || page.Items[0].IsDelete {
		t.Fatalf("first page: %d %+v %v", first.Code, page, err)
	}
	cursor := url.QueryEscape(*page.NextCursor)
	next := projectHTTPRequest(t.Context(), router, http.MethodGet, "/api/projects?limit=1&q=project-&cursor="+cursor, "", nil)
	var second workspacehttp.ListResponse
	if err := json.Unmarshal(next.Body.Bytes(), &second); err != nil || next.Code != 200 || len(second.Items) != 1 || second.Items[0].ID == page.Items[0].ID {
		t.Fatalf("canonical filters next: %d %+v %v", next.Code, second, err)
	}
	for _, path := range []string{
		"/api/projects?limit=2&q=project-&cursor=",
		"/api/projects?limit=1&q=changed&cursor=",
		"/api/projects?limit=1&q=project-&status=archived&cursor=",
		"/api/projects?limit=1&q=project-&deleted=true&cursor=",
	} {
		response := projectHTTPRequest(t.Context(), router, http.MethodGet, path+cursor, "", nil)
		if response.Code != 422 {
			t.Fatalf("mixed cursor filters: %d %s", response.Code, response.Body.String())
		}
	}
	for _, other := range []identityapp.Principal{
		insertWorkspaceActor(ctx, t, database, orgID),
		insertWorkspaceActor(ctx, t, database, insertWorkspaceOrganization(ctx, t, database)),
	} {
		response := projectHTTPRequest(t.Context(), projectHTTPRouter(database, other), http.MethodGet, "/api/projects?limit=1&q=project-&cursor="+cursor, "", nil)
		if response.Code != 422 {
			t.Fatalf("mixed caller cursor: %d %s", response.Code, response.Body.String())
		}
	}
}

func TestProjectHTTPListsOnlySafeOrganizationStylePresets(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(ctx, t, database)
	actor := insertWorkspaceActor(ctx, t, database, orgID)
	presetID := insertStylePreset(ctx, t, database, orgID, "", false)
	insertStylePreset(ctx, t, database, orgID, "", true)
	projectID := insertWorkspaceProject(ctx, t, database, orgID, "16:9", "stylized", "guofeng_xianxia")
	insertStylePreset(ctx, t, database, orgID, projectID, false)
	insertStylePreset(ctx, t, database, insertWorkspaceOrganization(ctx, t, database), "", false)
	if err := database.Exec(`UPDATE workspace.style_preset SET prompt_fragment='SECRET_PROMPT', negative_prompt='PRIVATE_NEGATIVE' WHERE id=?`, presetID).Error; err != nil {
		t.Fatal(err)
	}
	response := projectHTTPRequest(t.Context(), projectHTTPRouter(database, actor), http.MethodGet, "/api/style-presets?style_type=stylized&style_subtype=guofeng_xianxia", "", nil)
	if response.Code != 200 {
		t.Fatalf("preset list: %d %s", response.Code, response.Body.String())
	}
	var page struct {
		Items      []struct{ ID uuid.UUID }
		NextCursor *string `json:"next_cursor"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil || len(page.Items) != 1 || page.Items[0].ID != presetID || page.NextCursor != nil {
		t.Fatalf("preset scope: %+v err=%v", page, err)
	}
	for _, forbidden := range []string{"prompt_fragment", "negative_prompt", "reference_asset_ids", "SECRET_PROMPT", "PRIVATE_NEGATIVE", "org_id", "project_id"} {
		if bytes.Contains(response.Body.Bytes(), []byte(forbidden)) {
			t.Fatalf("preset response exposed %s", forbidden)
		}
	}
}

func TestProjectHTTPStylePresetPaginationAndCreationAuthorization(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(ctx, t, database)
	actor := insertWorkspaceActor(ctx, t, database, orgID)
	router := projectHTTPRouter(database, actor)
	empty := projectHTTPRequest(t.Context(), router, http.MethodGet, "/api/style-presets", "", nil)
	if empty.Code != 200 || empty.Body.String() != `{"items":[],"next_cursor":null}` {
		t.Fatalf("empty presets fabricated defaults: %d %s", empty.Code, empty.Body.String())
	}
	var presetIDs []uuid.UUID
	for range 3 {
		presetIDs = append(presetIDs, insertStylePreset(ctx, t, database, orgID, "", false))
	}
	first := projectHTTPRequest(t.Context(), router, http.MethodGet, "/api/style-presets?style_type=stylized&limit=1", "", nil)
	var page workspacehttp.StylePresetListResponse
	if err := json.Unmarshal(first.Body.Bytes(), &page); err != nil || first.Code != 200 || len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatalf("first preset page: %d %+v %v", first.Code, page, err)
	}
	seen := map[uuid.UUID]bool{page.Items[0].ID: true}
	for page.NextCursor != nil {
		response := projectHTTPRequest(t.Context(), router, http.MethodGet, "/api/style-presets?style_type=stylized&limit=1&cursor="+url.QueryEscape(*page.NextCursor), "", nil)
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil || response.Code != 200 || len(page.Items) != 1 || seen[page.Items[0].ID] {
			t.Fatalf("next preset page: %d %+v %v", response.Code, page, err)
		}
		seen[page.Items[0].ID] = true
	}
	if len(seen) != 3 {
		t.Fatalf("preset pagination skipped records: %d", len(seen))
	}
	var initial workspacehttp.StylePresetListResponse
	if err := json.Unmarshal(first.Body.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}
	mixed := projectHTTPRequest(t.Context(), router, http.MethodGet, "/api/style-presets?style_type=realistic&limit=1&cursor="+url.QueryEscape(*initial.NextCursor), "", nil)
	if mixed.Code != 422 {
		t.Fatalf("mixed preset cursor: %d %s", mixed.Code, mixed.Body.String())
	}
	projectID := insertWorkspaceProject(ctx, t, database, orgID, "16:9", "stylized", "guofeng_xianxia")
	for _, test := range []struct {
		name   string
		id     uuid.UUID
		status int
	}{
		{"other organization", insertStylePreset(ctx, t, database, insertWorkspaceOrganization(ctx, t, database), "", false), 404},
		{"project scoped", insertStylePreset(ctx, t, database, orgID, projectID, false), 404},
		{"deleted", insertStylePreset(ctx, t, database, orgID, "", true), 404},
		{"specification mismatch", presetIDs[0], 422},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"name":"预设授权","aspect_ratio":"9:16","style_type":"realistic","style_preset_id":"%s"}`, test.id))
			response := projectHTTPRequest(t.Context(), router, http.MethodPost, "/api/projects", uuid.NewString(), body)
			if response.Code != test.status {
				t.Fatalf("unusable preset: %d %s", response.Code, response.Body.String())
			}
		})
	}
	if count := countProjectsNamed(ctx, t, database, actor.OrgID, "预设授权"); count != 0 {
		t.Fatalf("unusable preset left %d projects", count)
	}
}

func TestProjectHTTPWriteSecurityAndBoundedRequest(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, database, insertWorkspaceOrganization(ctx, t, database))
	router := projectHTTPRouter(database, actor)
	for _, test := range []struct {
		name, body string
		status     int
	}{
		{"unknown field", `{"name":"安全入口","aspect_ratio":"9:16","style_type":"realistic","org_id":"` + uuid.NewString() + `"}`, 422},
		{"invalid subtype", `{"name":"安全入口","aspect_ratio":"9:16","style_type":"stylized","style_subtype":"watercolor"}`, 422},
		{"invalid name", `{"name":"` + strings.Repeat("剧", 51) + `","aspect_ratio":"9:16","style_type":"realistic"}`, 422},
		{"oversized body", `{"name":"安全入口","aspect_ratio":"9:16","style_type":"realistic","description":"` + strings.Repeat("x", 1<<20) + `"}`, 413},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := projectHTTPRequest(t.Context(), router, http.MethodPost, "/api/projects", uuid.NewString(), []byte(test.body))
			if response.Code != test.status {
				t.Fatalf("invalid write: %d %s", response.Code, response.Body.String())
			}
		})
	}
	for _, test := range []struct {
		name, origin, csrf, key, session string
		status                           int
	}{
		{"Origin", "http://external.example", httpapi.CSRFToken("controlled-test-session"), uuid.NewString(), "controlled-test-session", 403},
		{"CSRF", workspaceTestOrigin, "invalid", uuid.NewString(), "controlled-test-session", 403},
		{"key", workspaceTestOrigin, httpapi.CSRFToken("controlled-test-session"), "invalid", "controlled-test-session", 422},
		{"session", workspaceTestOrigin, "", uuid.NewString(), "", 401},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/projects", strings.NewReader(`{"name":"安全入口","aspect_ratio":"9:16","style_type":"realistic"}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Origin", test.origin)
			request.Header.Set("X-CSRF-Token", test.csrf)
			request.Header.Set("Idempotency-Key", test.key)
			request.AddCookie(&http.Cookie{Name: "lv_session", Value: test.session})
			request.AddCookie(&http.Cookie{Name: "lv_csrf", Value: httpapi.CSRFToken("controlled-test-session")})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.status || !strings.HasPrefix(response.Header().Get("Content-Type"), "application/problem+json") {
				t.Fatalf("write security: %d %s", response.Code, response.Body.String())
			}
		})
	}
	if count := countProjectsNamed(ctx, t, database, actor.OrgID, "安全入口"); count != 0 {
		t.Fatalf("invalid writes left %d projects", count)
	}
}

func TestProjectHTTPCreationReceiptUsesExistingTwentyFourHourWindow(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, database, insertWorkspaceOrganization(ctx, t, database))
	router := projectHTTPRouter(database, actor)
	key := uuid.NewString()
	body := []byte(`{"name":"24小时回执窗口","aspect_ratio":"9:16","style_type":"realistic"}`)
	first := projectHTTPRequest(t.Context(), router, http.MethodPost, "/api/projects", key, body)
	if first.Code != 201 {
		t.Fatalf("first create: %d %s", first.Code, first.Body.String())
	}
	var lifetime struct{ WithinWindow bool }
	if err := database.Raw(`SELECT expires_at>now()+interval '23 hours' AND expires_at<=now()+interval '24 hours' AS within_window FROM infra.idempotency_record WHERE actor_id=? AND idem_key=?`, actor.ID, key).Scan(&lifetime).Error; err != nil || !lifetime.WithinWindow {
		t.Fatalf("receipt window: %+v %v", lifetime, err)
	}
	if err := database.Exec(`UPDATE infra.idempotency_record SET expires_at=now()-interval '1 second' WHERE actor_id=? AND idem_key=?`, actor.ID, key).Error; err != nil {
		t.Fatal(err)
	}
	second := projectHTTPRequest(t.Context(), projectHTTPRouter(database, actor), http.MethodPost, "/api/projects", key, body)
	if second.Code != 201 || bytes.Equal(second.Body.Bytes(), first.Body.Bytes()) {
		t.Fatalf("expired receipt silently extended policy: %d %s", second.Code, second.Body.String())
	}
	if count := countProjectsNamed(ctx, t, database, actor.OrgID, "24小时回执窗口"); count != 2 {
		t.Fatalf("expired window effects: %d", count)
	}
}

func TestProjectHTTPCreationRechecksReceiptExpiryAfterKeyLockWait(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, database, insertWorkspaceOrganization(ctx, t, database))
	router := projectHTTPRouter(database, actor)
	key := uuid.NewString()
	body := []byte(`{"name":"跨锁到期","aspect_ratio":"9:16","style_type":"realistic"}`)
	first := projectHTTPRequest(t.Context(), router, http.MethodPost, "/api/projects", key, body)
	if first.Code != 201 {
		t.Fatalf("first create: %d %s", first.Code, first.Body.String())
	}
	if err := database.Exec(`UPDATE infra.idempotency_record SET expires_at=clock_timestamp()+interval '1 second' WHERE actor_id=? AND idem_key=?`, actor.ID, key).Error; err != nil {
		t.Fatal(err)
	}
	lock := database.Begin()
	if lock.Error != nil {
		t.Fatal(lock.Error)
	}
	defer func() { _ = lock.Rollback().Error }()
	var locked int
	if err := lock.Raw(`SELECT 1 FROM pg_advisory_xact_lock(69360,hashtext(?))`, actor.ID.String()+":"+key).Scan(&locked).Error; err != nil {
		t.Fatal(err)
	}
	requestCtx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	response := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		response <- projectHTTPRequest(requestCtx, router, http.MethodPost, "/api/projects", key, body)
	}()
	// Observe only this task-owned database's wait metadata. The controlled
	// advisory lock guarantees the request transaction began before expiry.
	for {
		var state struct{ Ready bool }
		err := database.WithContext(requestCtx).Raw(`SELECT clock_timestamp()>expires_at
		 AND EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event='advisory') AS ready
		 FROM infra.idempotency_record WHERE actor_id=? AND idem_key=?`, actor.ID, key).Scan(&state).Error
		if err != nil {
			t.Fatal(err)
		}
		if state.Ready {
			break
		}
		select {
		case <-requestCtx.Done():
			t.Fatal("request did not wait across owned receipt expiry")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := lock.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-response:
		if result.Code != 201 || bytes.Equal(result.Body.Bytes(), first.Body.Bytes()) {
			t.Fatalf("expired receipt replayed after lock wait: %d %s", result.Code, result.Body.String())
		}
	case <-requestCtx.Done():
		t.Fatal("creation did not resume after releasing owned key lock")
	}
	var lifetime struct{ FullWindow bool }
	if err := database.Raw(`SELECT expires_at>=clock_timestamp()+interval '23 hours 59 minutes 59 seconds' AS full_window FROM infra.idempotency_record WHERE actor_id=? AND idem_key=?`, actor.ID, key).Scan(&lifetime).Error; err != nil || !lifetime.FullWindow {
		t.Fatalf("lock wait shortened new receipt lifetime: %+v %v", lifetime, err)
	}
}

func TestProjectHTTPStylePresetCursorRoundTripsEscapedNameAtFieldLimit(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(ctx, t, database)
	actor := insertWorkspaceActor(ctx, t, database, orgID)
	for range 2 {
		presetID := insertStylePreset(ctx, t, database, orgID, "", false)
		if err := database.Exec(`UPDATE workspace.style_preset SET name=? WHERE id=?`, strings.Repeat(`"`, 4096), presetID).Error; err != nil {
			t.Fatal(err)
		}
	}
	router := projectHTTPRouter(database, actor)
	first := projectHTTPRequest(t.Context(), router, http.MethodGet, "/api/style-presets?limit=1", "", nil)
	var page workspacehttp.StylePresetListResponse
	if err := json.Unmarshal(first.Body.Bytes(), &page); err != nil || first.Code != 200 || len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatalf("escaped preset page: %d items=%d %v", first.Code, len(page.Items), err)
	}
	second := projectHTTPRequest(t.Context(), router, http.MethodGet, "/api/style-presets?limit=1&cursor="+url.QueryEscape(*page.NextCursor), "", nil)
	var next workspacehttp.StylePresetListResponse
	if err := json.Unmarshal(second.Body.Bytes(), &next); err != nil || second.Code != 200 || len(next.Items) != 1 || next.Items[0].ID == page.Items[0].ID || next.NextCursor != nil {
		t.Fatalf("server-issued escaped cursor rejected: %d items=%d %v", second.Code, len(next.Items), err)
	}
}

func TestProjectHTTPCreationReceiptReplaysInAnotherProcess(t *testing.T) {
	if os.Getenv("LV_TEST_PROJECT_REPLAY_CHILD") == "1" {
		_, database := workspaceMigrationDB(t)
		actorID, err := uuid.Parse(os.Getenv("LV_TEST_PROJECT_ACTOR"))
		if err != nil {
			t.Fatal(err)
		}
		orgID, err := uuid.Parse(os.Getenv("LV_TEST_PROJECT_ORG"))
		if err != nil {
			t.Fatal(err)
		}
		actor := identityapp.Principal{ID: actorID, OrgID: orgID, Role: identitydomain.RoleProducer}
		response := projectHTTPRequest(t.Context(), projectHTTPRouter(database, actor), http.MethodPost, "/api/projects", os.Getenv("LV_TEST_PROJECT_KEY"), []byte(`{"name":"跨进程回执","aspect_ratio":"16:9","style_type":"realistic"}`))
		if response.Code != 201 {
			t.Fatalf("child-process replay: %d %s", response.Code, response.Body.String())
		}
		if err := os.WriteFile(os.Getenv("LV_TEST_PROJECT_RESPONSE"), response.Body.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	ctx, database := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, database, insertWorkspaceOrganization(ctx, t, database))
	key := uuid.NewString()
	first := projectHTTPRequest(t.Context(), projectHTTPRouter(database, actor), http.MethodPost, "/api/projects", key, []byte(`{"name":"跨进程回执","aspect_ratio":"16:9","style_type":"realistic"}`))
	if first.Code != 201 {
		t.Fatalf("parent-process create: %d %s", first.Code, first.Body.String())
	}
	responsePath := filepath.Join(t.TempDir(), "owned-response.json")
	childCtx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(childCtx, os.Args[0], "-test.run=^TestProjectHTTPCreationReceiptReplaysInAnotherProcess$", "-test.count=1")
	// Explicitly pass only synthetic fixture identifiers and the isolated peer
	// DSN. The child does not inherit unrelated local credentials or configuration.
	command.Env = []string{
		"LV_TEST_WORKSPACE_DB_DSN=" + os.Getenv("LV_TEST_WORKSPACE_DB_DSN"),
		"LV_TEST_PROJECT_REPLAY_CHILD=1", "LV_TEST_PROJECT_ACTOR=" + actor.ID.String(),
		"LV_TEST_PROJECT_ORG=" + actor.OrgID.String(), "LV_TEST_PROJECT_KEY=" + key,
		"LV_TEST_PROJECT_RESPONSE=" + responsePath,
	}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("owned child process: %v %s", err, output)
	}
	replayed, err := os.ReadFile(responsePath)
	if err != nil || !bytes.Equal(replayed, first.Body.Bytes()) {
		t.Fatalf("different persisted response across processes: %v", err)
	}
	if count := countProjectsNamed(ctx, t, database, actor.OrgID, "跨进程回执"); count != 1 {
		t.Fatalf("cross-process replay left %d projects", count)
	}
}
