package workspace_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	workspacehttp "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/http"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

func placementRouter(db *gorm.DB, actor identityapp.Principal) *gin.Engine {
	router := folderRouter(db, actor)
	api := router.Group("/api")
	api.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	workspacehttp.NewProjectCopyHandler(workspaceapp.NewProjectCopyService(projectCopyStore(db), time.Now)).Register(api)
	workspacehttp.NewProjectLifecycleHandler(workspaceapp.NewProjectLifecycle(workspacepg.NewStoreWithProjectWorkGuard(db, folderWork), time.Now)).Register(api)
	return router
}
func TestProjectCopyPlacementHTTPClosedCASAndRetiredRestore(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor, source := lifecycleActorProject(ctx, t, owner)
	router := placementRouter(db, actor)
	path := "/api/projects/" + source.String() + "/copies"
	for _, bad := range []string{`null`, `{}`, `{"expected_placement_revision":0,"expected_folder_revision":0}`, `{"expected_placement_revision":0,"expected_folder_revision":0,"folder_id":"00000000-0000-0000-0000-000000000000"}`, `{"expected_placement_revision":0,"expected_folder_revision":0,"folder_id":null,"url":"https://invalid.example/x"}`} {
		body := `{"expected_revision":1,"target_name":"副本","placement":` + bad + `}`
		r := projectHTTPRequest(ctx, router, http.MethodPost, path, uuid.NewString(), []byte(body))
		if r.Code != 422 {
			t.Fatalf("unclosed placement %d %s", r.Code, r.Body.String())
		}
	}
	key := uuid.NewString()
	body := []byte(`{"expected_revision":1,"target_name":"正式root副本","placement":{"expected_placement_revision":0,"expected_folder_revision":0,"folder_id":null}}`)
	first := projectHTTPRequest(ctx, router, http.MethodPost, path, key, body)
	var job workspacehttp.ProjectCopyResponse
	if first.Code != 202 || json.Unmarshal(first.Body.Bytes(), &job) != nil {
		t.Fatalf("CAS admission %d %s", first.Code, first.Body.String())
	}
	replay := projectHTTPRequest(ctx, placementRouter(db, actor), http.MethodPost, path, key, body)
	if replay.Code != 202 || replay.Body.String() != first.Body.String() {
		t.Fatalf("fresh immutable receipt %d %s", replay.Code, replay.Body.String())
	}
	if _, err := projectCopyStore(db).Change(ctx, actor, job.ID, "cancel", job.Revision, uuid.New(), uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	worker := uuid.New()
	store := projectCopyStore(db)
	if _, err := store.Claim(ctx, actor, job.ID, worker, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	cancelled, err := store.FinishCancelled(ctx, actor, job.ID, worker)
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.FinishCancelled(ctx, actor, job.ID, worker)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(cancelled)
	b, _ := json.Marshal(again)
	if string(a) != string(b) {
		t.Fatal("duplicate final cleanup changed terminal history")
	}
	restore := projectHTTPRequest(ctx, router, http.MethodPost, "/api/projects/"+job.TargetProjectID.String()+"/restore", uuid.NewString(), []byte(`{"expected_revision":2}`))
	if restore.Code != 409 {
		t.Fatalf("cleaned unpublished target restored %d %s", restore.Code, restore.Body.String())
	}
	read := projectHTTPRequest(ctx, router, http.MethodGet, "/api/projects?deleted=true", "", nil)
	var projects workspacehttp.ListResponse
	if read.Code != 200 || json.Unmarshal(read.Body.Bytes(), &projects) != nil || len(projects.Items) != 0 {
		t.Fatalf("copy target exposed as ordinary recycle item %d %s", read.Code, read.Body.String())
	}
}
