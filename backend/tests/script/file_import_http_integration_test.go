package script_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/script/adapter/extract"
	scripthttp "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/http"
	app "github.com/StephenQiu30/lanverse/backend/internal/script/application"
)

func TestScriptFileImportHTTPActualAdmissionRefreshPartialRetryAndControls(t *testing.T) {
	db, owner := scriptTestDB(t)
	actor, pid := scriptActorProject(t, owner)
	storage := scriptStorage(t)
	first := scriptDocument(t, db, storage, actor, pid, "第一份.txt", []byte("先失败"))
	second := scriptDocument(t, db, storage, actor, pid, "第二份.txt", []byte("原件二"))
	service := app.NewImportService(scriptImportStore(db, storage), time.Now)
	router := scriptRouter(app.NewSourceService(scriptStore(db), &sourceObjects{data: make(map[string][]byte)}, time.Now), actor)
	g := router.Group("/api")
	g.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	scripthttp.NewImportHandler(service).Register(g)
	path := "/api/projects/" + pid.String() + "/script-file-imports"
	rev := int64(0)
	key := uuid.New()
	response := scriptHTTP(t, router, "POST", path, key, scripthttp.FileImportRequest{ExpectedRevision: &rev, AssetIDs: []uuid.UUID{first, second}, RightsConfirmed: true})
	var accepted app.ImportJob
	if err := json.Unmarshal(response.Body.Bytes(), &accepted); err != nil || response.Code != 202 {
		t.Fatal(response.Code, err)
	}
	pageResponse := scriptHTTP(t, router, "GET", path, uuid.Nil, nil)
	var page app.ImportPage
	if err := json.Unmarshal(pageResponse.Body.Bytes(), &page); err != nil || pageResponse.Code != 200 || page.CurrentActorID != actor.ID || page.CurrentOrgID != actor.OrgID || len(page.Items) != 1 {
		t.Fatal("safe refresh", pageResponse.Code, page, err)
	}
	for _, forbidden := range []string{"publication_key", "object_key", "actor_role", "source_sha", "receipt", "plan", "document"} {
		if strings.Contains(pageResponse.Body.String(), forbidden) {
			t.Fatal("private import fact exposed", forbidden)
		}
	}
	if err := scriptImportWorker(db, storage, &failImportFileOnce{original: extract.NewExtractor()}).Execute(t.Context(), importDelivery(t, owner, actor, key)); err != nil {
		t.Fatal(err)
	}
	detail := scriptHTTP(t, router, "GET", path+"/"+accepted.ID.String(), uuid.Nil, nil)
	var partial app.ImportJob
	if err := json.Unmarshal(detail.Body.Bytes(), &partial); err != nil || partial.Status != "partial" || partial.Files[0].FailureCode == "" {
		t.Fatal("partial exposed as finished", partial, err)
	}
	retryKey := uuid.New()
	control := scripthttp.FileImportControlRequest{ExpectedRevision: &partial.Revision}
	retry := scriptHTTP(t, router, "POST", path+"/"+accepted.ID.String()+"/retry", retryKey, control)
	if retry.Code != 202 {
		t.Fatal("retry route", retry.Code, retry.Body.String())
	}
	if err := scriptImportWorker(db, storage, extract.NewExtractor()).Execute(t.Context(), importDelivery(t, owner, actor, retryKey)); err != nil {
		t.Fatal(err)
	}
	stale := scriptHTTP(t, router, "POST", path+"/"+accepted.ID.String()+"/cancel", uuid.New(), control)
	if stale.Code != 409 {
		t.Fatal("stale job CAS", stale.Code)
	}
	reconcile := scriptHTTP(t, router, "POST", path+"/"+accepted.ID.String()+"/reconcile", uuid.New(), control)
	if reconcile.Code != 409 {
		t.Fatal("invented reconciliation", reconcile.Code)
	}
	foreign, foreignProject := scriptActorProject(t, owner)
	foreignRouter := scriptRouter(app.NewSourceService(scriptStore(db), &sourceObjects{data: make(map[string][]byte)}, time.Now), foreign)
	fg := foreignRouter.Group("/api")
	fg.Use(func(c *gin.Context) { c.Set("principal", foreign); c.Next() })
	scripthttp.NewImportHandler(service).Register(fg)
	if hidden := scriptHTTP(t, foreignRouter, "GET", path+"/"+accepted.ID.String(), uuid.Nil, nil); hidden.Code != 404 {
		t.Fatal("foreign batch exposed", foreignProject, hidden.Code)
	}
}
