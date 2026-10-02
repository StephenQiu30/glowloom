package media_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	mediahttp "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/http"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

func transferRouter(db *gorm.DB, actor identityapp.Principal) *gin.Engine {
	router := gin.New()
	router.Use(httpapi.Middleware("http://127.0.0.1:3000"))
	group := router.Group("/api")
	group.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	mediahttp.NewTransferHandler(pgmedia.NewTransferStore(db, libraryTestAccess, transferTestGuards, time.Now)).Register(group)
	return router
}

func TestMediaTransferHTTPRealStrictScopesPermanentReplayAndCurrentQueries(t *testing.T) {
	f := newTransferRecoveryFixture(t, 1)
	router := transferRouter(f.db, f.actor)
	body, err := json.Marshal(f.input)
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/media/library/transfers"
	create := libraryHTTPRequest(t, router, http.MethodPost, path, body, f.input.Key)
	if create.Code != 202 {
		t.Fatal(create.Code, create.Body.String())
	}
	replay := libraryHTTPRequest(t, transferRouter(f.db, f.actor), http.MethodPost, path, body, f.input.Key)
	if replay.Code != 202 || !bytes.Equal(create.Body.Bytes(), replay.Body.Bytes()) {
		t.Fatal("fresh router changed permanent admission", replay.Code, replay.Body.String())
	}
	get := libraryHTTPRequest(t, router, http.MethodGet, path+"/"+f.job.ID.String(), nil, uuid.Nil)
	var current domain.TransferJob
	if get.Code != 200 || json.Unmarshal(get.Body.Bytes(), &current) != nil || current.CurrentActorID != f.actor.ID || current.CurrentOrgID != f.actor.OrgID {
		t.Fatal("current authorized job", get.Code, get.Body.String())
	}
	page := libraryHTTPRequest(t, router, http.MethodGet, path+"?scope=personal&page=1&page_size=1", nil, uuid.Nil)
	var list mediahttp.TransferPage
	if page.Code != 200 || json.Unmarshal(page.Body.Bytes(), &list) != nil || len(list.Items) != 1 || list.PageSize != 1 || list.CurrentActorID != f.actor.ID {
		t.Fatal("current scoped page", page.Code, page.Body.String())
	}
	for _, query := range []string{"?actor_id=" + uuid.NewString(), "?page=0", "?scope=personal&project_id=" + uuid.NewString(), "?scope=personal&scope=project"} {
		if response := libraryHTTPRequest(t, router, http.MethodGet, path+query, nil, uuid.Nil); response.Code != 422 {
			t.Fatal("invalid query accepted", query, response.Code)
		}
	}
	for _, private := range []string{"actor_id", "operation_id", "object_key", "activity_task_queue"} {
		var value map[string]any
		if err := json.Unmarshal(body, &value); err != nil {
			t.Fatal(err)
		}
		value[private] = "injected"
		injected, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if response := libraryHTTPRequest(t, router, http.MethodPost, path, injected, uuid.New()); response.Code != 422 {
			t.Fatal("private command field accepted", private, response.Code)
		}
	}
	foreign, _ := mediaStoreProject(t, f.db)
	if response := libraryHTTPRequest(t, transferRouter(f.db, foreign), http.MethodGet, path+"/"+f.job.ID.String(), nil, uuid.Nil); response.Code != 404 {
		t.Fatal("foreign creator read private job", response.Code)
	}
	control, err := json.Marshal(mediahttp.TransferControlRequest{Revision: f.job.Revision})
	if err != nil {
		t.Fatal(err)
	}
	key := uuid.New()
	cancelled := libraryHTTPRequest(t, router, http.MethodPost, path+"/"+f.job.ID.String()+"/cancel", control, key)
	if cancelled.Code != 202 {
		t.Fatal(cancelled.Code, cancelled.Body.String())
	}
	again := libraryHTTPRequest(t, transferRouter(f.db, f.actor), http.MethodPost, path+"/"+f.job.ID.String()+"/cancel", control, key)
	if again.Code != 202 || !bytes.Equal(cancelled.Body.Bytes(), again.Body.Bytes()) {
		t.Fatal("permanent cancel replay", again.Code, again.Body.String())
	}
	for _, action := range []string{"retry", "reconcile"} {
		if response := libraryHTTPRequest(t, router, http.MethodPost, path+"/"+f.job.ID.String()+"/"+action, control, uuid.New()); response.Code != 409 {
			t.Fatal("cancelled job restarted", action, response.Code)
		}
	}
	changed := f.input
	changed.ExpectedSourceRevision++
	changedBody, err := json.Marshal(changed)
	if err != nil {
		t.Fatal(err)
	}
	if response := libraryHTTPRequest(t, router, http.MethodPost, path, changedBody, f.input.Key); response.Code != 409 {
		t.Fatal("permanent admission key accepted changed body", response.Code)
	}
	for _, response := range [][]byte{create.Body.Bytes(), get.Body.Bytes(), page.Body.Bytes(), cancelled.Body.Bytes()} {
		for _, hidden := range []string{"object_key", "source_object_key", "file_name", "frozen", "worker_fence"} {
			if bytes.Contains(response, []byte(`"`+hidden+`":`)) {
				t.Fatal("safe response exposed private transfer facts", hidden)
			}
		}
	}
}
