package workspace_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"

	mo "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	wh "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/http"
	wf "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/workflow"
	wa "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	wd "github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func copyHTTPRequest(t *testing.T, r *gin.Engine, method, path, body string, key uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Idempotency-Key", key.String())
	req.Header.Set("X-Request-Id", uuid.NewString())
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)
	return response
}
func decodeCopyHTTP(t *testing.T, response *httptest.ResponseRecorder, expected int) wh.ProjectCopyResponse {
	t.Helper()
	if response.Code != expected {
		t.Fatalf("copy HTTP status=%d expected=%d body=%s", response.Code, expected, response.Body.String())
	}
	var job wh.ProjectCopyResponse
	if json.Unmarshal(response.Body.Bytes(), &job) != nil {
		t.Fatal("invalid copy response")
	}
	for _, field := range []string{"object_key", "worker_id", "request_sha256", "workspace_snapshot", "prompt_fragment", "manifest", "RequestID", "ActorID"} {
		if bytes.Contains(response.Body.Bytes(), []byte(field)) {
			t.Fatal("private copy field exposed", field)
		}
	}
	return job
}
func TestProjectCopyRealHTTPWorkflowAndRefreshRecovery(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	objects := projectCopyObjects(t)
	actor, source, _, _ := seedProjectCopyContent(ctx, t, db, objects, false)
	store := projectCopyStore(db)
	current := actor
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpapi.Middleware("http://localhost:3000"))
	g := router.Group("/api")
	g.Use(func(c *gin.Context) { c.Set("principal", current); c.Next() })
	wh.NewProjectCopyHandler(wa.NewProjectCopyService(store, time.Now)).Register(g)
	path := "/api/projects/" + source.String() + "/copies"
	key := uuid.New()
	body := `{"expected_revision":1,"target_name":"正式完整副本"}`
	invalid := copyHTTPRequest(t, router, "POST", path, `{"expected_revision":1,"target_name":"副本","source_url":"https://example.invalid"}`, uuid.New())
	if invalid.Code != 422 {
		t.Fatal("undeclared source accepted", invalid.Code)
	}
	first := decodeCopyHTTP(t, copyHTTPRequest(t, router, "POST", path, body, key), 202)
	if first.Status != "queued" || first.SourceRevision != 1 || first.Assets != 1 || first.Documents != 2 || first.CompletedAssets != 0 {
		t.Fatal("202 claimed completed data", first)
	}
	replay := decodeCopyHTTP(t, copyHTTPRequest(t, router, "POST", path, body, key), 202)
	if replay.ID != first.ID || replay.TargetProjectID != first.TargetProjectID {
		t.Fatal("durable public admission identity")
	}
	different := copyHTTPRequest(t, router, "POST", path, `{"expected_revision":1,"target_name":"异体副本"}`, key)
	if different.Code != 422 {
		t.Fatal("same-key different request admitted", different.Code)
	}
	list := copyHTTPRequest(t, router, "GET", path+"?limit=1", "", uuid.New())
	if list.Code != 200 {
		t.Fatal("refresh list", list.Code, list.Body.String())
	}
	var page wh.ProjectCopyListResponse
	if json.Unmarshal(list.Body.Bytes(), &page) != nil || len(page.Copies) != 1 || page.Copies[0].ID != first.ID || page.CurrentActorID != actor.ID || page.CurrentOrgID != actor.OrgID {
		t.Fatal("scoped recovery page", page)
	}
	get := "/api/project-copies/" + first.ID.String()
	decodeCopyHTTP(t, copyHTTPRequest(t, router, "GET", get, "", uuid.New()), 200)
	stale := copyHTTPRequest(t, router, "POST", get+"/cancel", `{"expected_revision":99}`, uuid.New())
	if stale.Code != 409 {
		t.Fatal("stale cancel accepted", stale.Code)
	}
	actual := actualCopyWorker(t, db, mo.NewProjectCopyObjects(objects))
	activities := wf.NewProjectCopyActivities(actual, store)
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(wf.ProjectCopyWorkflow)
	env.RegisterActivityWithOptions(activities.ExecuteProjectCopy, activity.RegisterOptions{Name: wf.ExecuteCopyActivity})
	env.RegisterActivityWithOptions(activities.InterruptProjectCopy, activity.RegisterOptions{Name: wf.InterruptCopyActivity})
	env.ExecuteWorkflow(wf.ProjectCopyWorkflow, wa.ProjectCopyWorkID{OrgID: actor.OrgID, JobID: first.ID})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatal("real copy SDK workflow", err)
	}
	var result wd.ProjectCopyJob
	if err := env.GetWorkflowResult(&result); err != nil || result.Status != "succeeded" {
		t.Fatal("real copy workflow completion", err)
	}
	fresh := decodeCopyHTTP(t, copyHTTPRequest(t, router, "GET", get, "", uuid.New()), 200)
	if fresh.Status != "succeeded" || fresh.CompletedAssets != fresh.Assets || fresh.CompletedDocuments != fresh.Documents || fresh.CompletedRenditions != fresh.Renditions {
		t.Fatal("actual refreshed receipts", fresh)
	}
	original := decodeCopyHTTP(t, copyHTTPRequest(t, router, "POST", path, body, key), 202)
	if original.Status != "queued" || original.ID != first.ID {
		t.Fatal("original public receipt overwritten after completion")
	}
	second := decodeCopyHTTP(t, copyHTTPRequest(t, router, "POST", path, `{"expected_revision":1,"target_name":"第二份独立完整副本"}`, uuid.New()), 202)
	list = copyHTTPRequest(t, router, "GET", path+"?limit=1", "", uuid.New())
	if list.Code != 200 || json.Unmarshal(list.Body.Bytes(), &page) != nil || len(page.Copies) != 1 || page.Copies[0].ID != second.ID || page.NextCursor == nil {
		t.Fatal("recent immutable copy cursor", list.Code, page)
	}
	older := copyHTTPRequest(t, router, "GET", path+"?limit=1&cursor="+url.QueryEscape(*page.NextCursor), "", uuid.New())
	if older.Code != 200 || json.Unmarshal(older.Body.Bytes(), &page) != nil || len(page.Copies) != 1 || page.Copies[0].ID != first.ID || page.NextCursor != nil || page.CurrentActorID != actor.ID || page.CurrentOrgID != actor.OrgID {
		t.Fatal("persistent cursor lost or duplicated copy jobs", older.Code, page)
	}
	other, _ := lifecycleActorProject(ctx, t, db)
	current = other
	if response := copyHTTPRequest(t, router, "GET", get, "", uuid.New()); response.Code != 404 {
		t.Fatal("cross-org job leaked", response.Code)
	}
	if response := copyHTTPRequest(t, router, "GET", path, "", uuid.New()); response.Code != 404 {
		t.Fatal("cross-org source jobs leaked", response.Code)
	}
	current = actor
	if response := copyHTTPRequest(t, router, "GET", path+"?limit=101", "", uuid.New()); response.Code != 422 {
		t.Fatal("unbounded recovery page admitted", response.Code)
	}
}
