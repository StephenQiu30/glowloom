package workspace_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	operationpg "github.com/StephenQiu30/lanverse/backend/internal/operation/adapter/postgres"
	operationapp "github.com/StephenQiu30/lanverse/backend/internal/operation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	workspacehttp "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/http"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

func lifecycleHTTPRouter(database *gorm.DB, actor identityapp.Principal) *gin.Engine {
	router := gin.New()
	router.Use(httpapi.Middleware(workspaceTestOrigin))
	api := router.Group("/api")
	api.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	store := owningLifecycleStore(database)
	workspacehttp.NewHandler(workspaceapp.NewListProjectsQuery(store), workspaceapp.NewCreateProjectCommand(store, time.Now), workspaceapp.NewListStylePresetsQuery(store)).Register(api)
	workspacehttp.NewProjectLifecycleHandler(workspaceapp.NewProjectLifecycle(store, time.Now)).Register(api)
	return router
}

func TestProjectLifecycleHTTPClosedReadPatchAndRecovery(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	id := uuid.MustParse(insertWorkspaceProject(ctx, t, db, actor.OrgID.String(), "16:9", "realistic", nil))
	router := lifecycleHTTPRouter(db, actor)
	path := "/api/projects/" + id.String()
	read := projectHTTPRequest(ctx, router, http.MethodGet, path, "", nil)
	var detail workspacehttp.ProjectDetailResponse
	if read.Code != 200 || json.Unmarshal(read.Body.Bytes(), &detail) != nil || detail.DefaultModels == nil || strings.Contains(read.Body.String(), "org_id") {
		t.Fatalf("detail status=%d body=%s", read.Code, read.Body.String())
	}
	key := uuid.NewString()
	patch := []byte(`{"expected_revision":1,"name":"新名称","description":"私有创作","allow_overseas_models":true}`)
	first := projectHTTPRequest(ctx, router, http.MethodPatch, path, key, patch)
	if first.Code != 200 || json.Unmarshal(first.Body.Bytes(), &detail) != nil || detail.Name != "新名称" || detail.Revision != 2 {
		t.Fatalf("patch status=%d body=%s", first.Code, first.Body.String())
	}
	replay := projectHTTPRequest(ctx, lifecycleHTTPRouter(db, actor), http.MethodPatch, path, key, patch)
	if replay.Code != 200 || !bytes.Equal(first.Body.Bytes(), replay.Body.Bytes()) {
		t.Fatal("HTTP persistent first response changed", replay.Code)
	}
	for _, body := range []string{`{"expected_revision":2,"status":"archived"}`, `{"expected_revision":2,"aspect_ratio":"9:16"}`, `{"expected_revision":null,"name":"invalid"}`, `{"expected_revision":2,"name":"bad\u0000name"}`, `{"expected_revision":2}`} {
		bad := projectHTTPRequest(ctx, router, http.MethodPatch, path, uuid.NewString(), []byte(body))
		if bad.Code != 422 {
			t.Fatalf("unclosed patch status=%d body=%s", bad.Code, bad.Body.String())
		}
	}
	invalidUTF8 := append([]byte(`{"expected_revision":2,"name":"bad`), 0xff)
	invalidUTF8 = append(invalidUTF8, []byte(`"}`)...)
	if bad := projectHTTPRequest(ctx, router, http.MethodPatch, path, uuid.NewString(), invalidUTF8); bad.Code != 422 {
		t.Fatal("invalid UTF-8 silently replaced", bad.Code)
	}
	for _, step := range []struct {
		method, suffix string
		revision       int64
	}{{"POST", "/archive", 2}, {"DELETE", "", 3}, {"POST", "/restore", 4}, {"POST", "/unarchive", 5}} {
		body, _ := json.Marshal(map[string]any{"expected_revision": step.revision})
		response := projectHTTPRequest(ctx, router, step.method, path+step.suffix, uuid.NewString(), body)
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &detail) != nil || detail.Revision != step.revision+1 {
			t.Fatalf("%s%s status=%d body=%s", step.method, step.suffix, response.Code, response.Body.String())
		}
		if step.method == "DELETE" {
			if !detail.IsDelete || detail.Status != "archived" || detail.DeleteTime == nil || detail.PurgeAfter == nil {
				t.Fatal("delete lost persisted recovery state")
			}
			hidden := projectHTTPRequest(ctx, router, "GET", path, "", nil)
			if hidden.Code != 404 {
				t.Fatal("deleted project detail visible")
			}
			recycle := projectHTTPRequest(ctx, router, "GET", "/api/projects?deleted=true", "", nil)
			if recycle.Code != 200 || !strings.Contains(recycle.Body.String(), detail.PurgeAfter.Format(time.RFC3339Nano)) {
				t.Fatal("recycle list lost actual deadline", recycle.Body.String())
			}
		}
	}
	other := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	if response := projectHTTPRequest(ctx, lifecycleHTTPRouter(db, other), "GET", path, "", nil); response.Code != 404 {
		t.Fatal("cross org detail exposed")
	}
}

func TestProjectLifecycleHTTPInFlightConflictAndDeadline(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor, project := lifecycleActorProject(ctx, t, db)
	quote := lifecycleQuote(t, db, actor, project)
	if _, err := operationpg.NewStore(db).ConfirmSingleQuote(ctx, actor, operationapp.ConfirmSingleQuoteInput{ProjectID: project, OperationID: quote.OperationID, RequestID: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	router := lifecycleHTTPRouter(db, actor)
	path := "/api/projects/" + project.String()
	blocked := projectHTTPRequest(ctx, router, "DELETE", path, uuid.NewString(), []byte(`{"expected_revision":1}`))
	if blocked.Code != 409 || !strings.Contains(blocked.Body.String(), "inflight_work") {
		t.Fatalf("HTTP inflight status=%d body=%s", blocked.Code, blocked.Body.String())
	}
	otherActor, otherProject := lifecycleActorProject(ctx, t, db)
	otherRouter := lifecycleHTTPRouter(db, otherActor)
	otherPath := "/api/projects/" + otherProject.String()
	key := uuid.NewString()
	patch := []byte(`{"expected_revision":1,"name":"原请求"}`)
	if first := projectHTTPRequest(ctx, otherRouter, "PATCH", otherPath, key, patch); first.Code != 200 {
		t.Fatal(first.Code, first.Body.String())
	}
	reused := projectHTTPRequest(ctx, otherRouter, "PATCH", otherPath, key, []byte(`{"expected_revision":1,"name":"同键异体"}`))
	if reused.Code != 422 || !strings.Contains(reused.Body.String(), "idempotency_key_reused") {
		t.Fatal("wrong idempotency conflict", reused.Code, reused.Body.String())
	}
	stale := projectHTTPRequest(ctx, otherRouter, "POST", otherPath+"/archive", uuid.NewString(), []byte(`{"expected_revision":1}`))
	if stale.Code != 409 || !strings.Contains(stale.Body.String(), "revision_conflict") {
		t.Fatal("wrong CAS conflict", stale.Code, stale.Body.String())
	}
	lifecycleFixtureSQL(t, db, `UPDATE workspace.project SET is_delete=true,delete_time=clock_timestamp()-interval '31 days',purge_after=clock_timestamp()-interval '1 day',revision=3 WHERE id=?`, otherProject)
	expired := projectHTTPRequest(ctx, otherRouter, "POST", otherPath+"/restore", uuid.NewString(), []byte(`{"expected_revision":3}`))
	if expired.Code != 409 || !strings.Contains(expired.Body.String(), "restore_expired") {
		t.Fatal("wrong recovery deadline", expired.Code, expired.Body.String())
	}
}
