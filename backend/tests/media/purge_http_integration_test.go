package media_test

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
	mediahttp "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/http"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

func purgeRouter(db *gorm.DB, actor identityapp.Principal) *gin.Engine {
	router := gin.New()
	router.Use(httpapi.Middleware("http://127.0.0.1:3000"))
	group := router.Group("/api")
	group.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	mediahttp.NewPurgeHandler(pgmedia.NewPurgeStore(db, libraryTestAccess, transferTestGuards, purgeTestReferenceFactory, time.Now)).Register(group)
	return router
}

func TestMediaPurgeHTTPActualExplicitConfirmationClosedScopesAndOriginalRecovery(t *testing.T) {
	db := libraryRuntimeDB(t)
	actor, project, in, _, _ := purgeBinaryFixture(t, db)
	router := purgeRouter(db, actor)
	path := "/api/media/library/purges"
	body, _ := json.Marshal(in)
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	value["permanent_delete_confirmed"] = false
	unconfirmed, _ := json.Marshal(value)
	if response := libraryHTTPRequest(t, router, http.MethodPost, path, unconfirmed, uuid.New()); response.Code != 422 {
		t.Fatal("unconfirmed destructive command accepted", response.Code)
	}
	response := libraryHTTPRequest(t, router, http.MethodPost, path, body, in.Key)
	var job domain.PurgeJob
	if response.Code != 202 || json.Unmarshal(response.Body.Bytes(), &job) != nil || job.CurrentActorID != actor.ID || job.Scope.ProjectID == nil || *job.Scope.ProjectID != project {
		t.Fatal("formal confirmed admission", response.Code, response.Body.String())
	}
	replay := libraryHTTPRequest(t, purgeRouter(db, actor), http.MethodPost, path, body, in.Key)
	if replay.Code != 202 || !bytes.Equal(response.Body.Bytes(), replay.Body.Bytes()) {
		t.Fatal("fresh router broke original durable response", replay.Code, replay.Body.String())
	}
	for _, private := range []string{"object_key", "source_label", "plain_text", "worker_fence", "operation_id"} {
		if strings.Contains(response.Body.String(), private) {
			t.Fatal("private source facts leaked", private)
		}
	}
	get := libraryHTTPRequest(t, router, http.MethodGet, path+"/"+job.ID.String(), nil, uuid.Nil)
	if get.Code != 200 {
		t.Fatal(get.Code, get.Body.String())
	}
	list := libraryHTTPRequest(t, router, http.MethodGet, path+"?scope=project&project_id="+project.String()+"&page=1&page_size=1", nil, uuid.Nil)
	if list.Code != 200 || !strings.Contains(list.Body.String(), job.ID.String()) {
		t.Fatal("durable scoped history", list.Code, list.Body.String())
	}
	for _, query := range []string{"?actor_id=" + uuid.NewString(), "?page=0", "?scope=personal&project_id=" + project.String(), "?scope=personal&scope=project"} {
		if result := libraryHTTPRequest(t, router, http.MethodGet, path+query, nil, uuid.Nil); result.Code != 422 {
			t.Fatal("injected query accepted", query, result.Code)
		}
	}
	for _, field := range []string{"actor_id", "object_key", "activity_task_queue"} {
		if err := json.Unmarshal(body, &value); err != nil {
			t.Fatal(err)
		}
		value[field] = "injected"
		injected, _ := json.Marshal(value)
		if result := libraryHTTPRequest(t, router, http.MethodPost, path, injected, uuid.New()); result.Code != 422 {
			t.Fatal("injected command accepted", field, result.Code)
		}
	}
	other, _ := mediaStoreProject(t, db)
	if result := libraryHTTPRequest(t, purgeRouter(db, other), http.MethodGet, path+"/"+job.ID.String(), nil, uuid.Nil); result.Code != 404 {
		t.Fatal("foreign actor discovered cleanup", result.Code)
	}
	control, _ := json.Marshal(mediahttp.PurgeControlRequest{Revision: job.Revision})
	key := uuid.New()
	cancel := libraryHTTPRequest(t, router, http.MethodPost, path+"/"+job.ID.String()+"/cancel", control, key)
	if cancel.Code != 202 {
		t.Fatal(cancel.Code, cancel.Body.String())
	}
	if repeated := libraryHTTPRequest(t, purgeRouter(db, actor), http.MethodPost, path+"/"+job.ID.String()+"/cancel", control, key); repeated.Code != 202 || !bytes.Equal(cancel.Body.Bytes(), repeated.Body.Bytes()) {
		t.Fatal("permanent HTTP cancel recovery", repeated.Code)
	}
	if _, err := pgmedia.NewPurgeStore(db, libraryTestAccess, transferTestGuards, purgeTestReferenceFactory, time.Now).GetPurge(t.Context(), actor, job.ID); err != nil {
		t.Fatal(err)
	}
}

func TestMediaPurgeHTTPActualUnknownRequiresExplicitReconcileRevision(t *testing.T) {
	db := libraryRuntimeDB(t)
	actor, _, in, _, objects := purgeBinaryFixture(t, db)
	repo := pgmedia.NewPurgeStore(db, libraryTestAccess, transferTestGuards, purgeTestReferenceFactory, time.Now)
	job, err := repo.CreatePurge(t.Context(), actor, in)
	if err != nil {
		t.Fatal(err)
	}
	unknown := &purgeUnknownRemove{PurgeObjects: objects, once: true}
	current, err := mediaapp.NewPurgeWorker(repo, unknown).Execute(t.Context(), mediaapp.PurgeWorkID{JobID: job.ID, Attempt: 1, ExecutionID: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	router := purgeRouter(db, actor)
	path := "/api/media/library/purges/" + job.ID.String() + "/reconcile"
	stale, _ := json.Marshal(mediahttp.PurgeControlRequest{Revision: job.Revision})
	if response := libraryHTTPRequest(t, router, http.MethodPost, path, stale, uuid.New()); response.Code != 409 {
		t.Fatal("stale observed revision accepted", response.Code)
	}
	body, _ := json.Marshal(mediahttp.PurgeControlRequest{Revision: current.Revision})
	key := uuid.New()
	response := libraryHTTPRequest(t, router, http.MethodPost, path, body, key)
	if response.Code != 202 {
		t.Fatal(response.Code, response.Body.String())
	}
	replay := libraryHTTPRequest(t, purgeRouter(db, actor), http.MethodPost, path, body, key)
	if replay.Code != 202 || !bytes.Equal(response.Body.Bytes(), replay.Body.Bytes()) {
		t.Fatal("new router changed original reconciliation receipt", replay.Code)
	}
}
