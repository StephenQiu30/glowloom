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
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	workspacehttp "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/http"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

func folderRouter(db *gorm.DB, actor identityapp.Principal) *gin.Engine {
	router := gin.New()
	router.Use(httpapi.Middleware(workspaceTestOrigin))
	api := router.Group("/api")
	api.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	workspacehttp.NewProjectFolderHandler(workspaceapp.NewProjectFolders(folderStore(db), time.Now)).Register(api)
	workspacehttp.NewHandler(workspaceapp.NewListProjectsQuery(workspacepg.NewStore(db)), nil, nil).Register(api)
	return router
}
func TestProjectFolderHTTPClosedFieldsReplayAndPagination(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor := insertWorkspaceActor(ctx, t, owner, insertWorkspaceOrganization(ctx, t, owner))
	router := folderRouter(db, actor)
	body := []byte(`{"name":"项目目录"}`)
	key := uuid.NewString()
	first := projectHTTPRequest(ctx, router, http.MethodPost, "/api/project-folders", key, body)
	var saved workspacehttp.ProjectFolderChangeResponse
	if first.Code != 200 || json.Unmarshal(first.Body.Bytes(), &saved) != nil || saved.Folder == nil {
		t.Fatalf("create %d %s", first.Code, first.Body.String())
	}
	for _, private := range []string{"org_id", "actor_id", "request_sha256", "object_key", "url"} {
		if strings.Contains(first.Body.String(), private) {
			t.Fatalf("private field %s", private)
		}
	}
	replay := projectHTTPRequest(ctx, folderRouter(db, actor), http.MethodPost, "/api/project-folders", key, body)
	if replay.Code != 200 || !bytes.Equal(first.Body.Bytes(), replay.Body.Bytes()) {
		t.Fatalf("durable response %d %s", replay.Code, replay.Body.String())
	}
	for _, bad := range []string{`{"name":"目录","parent_id":null}`, `{"name":"目录","task_id":"x"}`, `{"name":"目录","cover":{"project_id":"` + uuid.NewString() + `","asset_id":"` + uuid.NewString() + `","url":"https://invalid.example/x"}}`, `{"name":null}`, `{"name":"bad\u0000"}`} {
		response := projectHTTPRequest(ctx, router, http.MethodPost, "/api/project-folders", uuid.NewString(), []byte(bad))
		if response.Code != 422 {
			t.Fatalf("unclosed input %d %s", response.Code, response.Body.String())
		}
	}
	invalid := append([]byte(`{"name":"bad`), 0xff)
	invalid = append(invalid, []byte(`"}`)...)
	if response := projectHTTPRequest(ctx, router, http.MethodPost, "/api/project-folders", uuid.NewString(), invalid); response.Code != 422 {
		t.Fatal("invalid utf8 accepted")
	}
	id := uuid.MustParse(insertWorkspaceProject(ctx, t, owner, actor.OrgID.String(), "16:9", "realistic", nil))
	path := "/api/projects/" + id.String() + "/folder"
	for _, bad := range []string{`{"expected_project_revision":1,"expected_placement_revision":0}`, `{"expected_project_revision":1,"expected_placement_revision":null,"folder_id":null}`, `{"expected_project_revision":1,"expected_placement_revision":0,"folder_id":"00000000-0000-0000-0000-000000000000"}`, `{"expected_project_revision":1,"expected_placement_revision":0,"folder_id":null,"status":"ready"}`} {
		response := projectHTTPRequest(ctx, router, http.MethodPut, path, uuid.NewString(), []byte(bad))
		if response.Code != 422 {
			t.Fatalf("invalid placement %d %s", response.Code, response.Body.String())
		}
	}
	move, _ := json.Marshal(map[string]any{"expected_project_revision": 1, "expected_placement_revision": 0, "folder_id": saved.Folder.ID, "expected_folder_revision": saved.Folder.Revision})
	response := projectHTTPRequest(ctx, router, http.MethodPut, path, uuid.NewString(), move)
	if response.Code != 200 {
		t.Fatalf("move %d %s", response.Code, response.Body.String())
	}
	if response := projectHTTPRequest(ctx, router, http.MethodPut, path, uuid.NewString(), move); response.Code != 409 {
		t.Fatalf("stale move %d %s", response.Code, response.Body.String())
	}
	other := insertWorkspaceActor(ctx, t, owner, actor.OrgID.String())
	if response := projectHTTPRequest(ctx, folderRouter(db, other), http.MethodGet, "/api/projects?folder_id="+saved.Folder.ID.String(), "", nil); response.Code != 404 {
		t.Fatalf("foreign personal folder %d %s", response.Code, response.Body.String())
	}
	createTestFolder(ctx, t, workspaceapp.NewProjectFolders(folderStore(db), time.Now), actor, "第二页")
	list := projectHTTPRequest(ctx, router, http.MethodGet, "/api/project-folders?limit=1", "", nil)
	var page workspacehttp.ProjectFolderListResponse
	if list.Code != 200 || json.Unmarshal(list.Body.Bytes(), &page) != nil || len(page.Items) != 1 || page.NextCursor == nil || page.CurrentActorID != actor.ID || page.CurrentOrgID != actor.OrgID || list.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("list %d %s", list.Code, list.Body.String())
	}
	next := projectHTTPRequest(ctx, folderRouter(db, actor), http.MethodGet, "/api/project-folders?limit=1&cursor="+*page.NextCursor, "", nil)
	var second workspacehttp.ProjectFolderListResponse
	if next.Code != 200 || json.Unmarshal(next.Body.Bytes(), &second) != nil || len(second.Items) != 1 || second.Items[0].ID == page.Items[0].ID {
		t.Fatalf("page %d %s", next.Code, next.Body.String())
	}
	changed := projectHTTPRequest(ctx, router, http.MethodGet, "/api/project-folders?limit=2&cursor="+*page.NextCursor, "", nil)
	if changed.Code != 422 {
		t.Fatal("changed cursor scope accepted")
	}
	if response := projectHTTPRequest(ctx, router, http.MethodGet, "/api/projects?folder_id=root&deleted=true", "", nil); response.Code != 422 {
		t.Fatal("recycle folder filter accepted")
	}
}
