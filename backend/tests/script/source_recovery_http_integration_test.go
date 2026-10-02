package script_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	scripthttp "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/http"
	objectsadapter "github.com/StephenQiu30/lanverse/backend/internal/script/adapter/objects"
	app "github.com/StephenQiu30/lanverse/backend/internal/script/application"
)

func recoveryRouter(service *app.SourceRecovery, actor identityapp.Principal) *gin.Engine {
	router := gin.New()
	router.Use(httpapi.Middleware("http://127.0.0.1:3000"))
	g := router.Group("/api")
	g.Use(func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	scripthttp.NewSourceRecoveryHandler(service).Register(g)
	return router
}
func TestScriptSourceRecoveryHTTPCurrentScopeAndPermanent202IsNotCompletion(t *testing.T) {
	db, owner := scriptTestDB(t)
	creator, pid := scriptActorProject(t, owner)
	admin := scriptAdmin(t, owner, creator.OrgID)
	privateObjects := objectsadapter.NewStorage(scriptStorage(t))
	store := scriptStore(db)
	sources := app.NewSourceService(store, &lostPrivatePut{PrivateObjects: privateObjects}, time.Now)
	recovery := app.NewSourceRecovery(store, sources, time.Now)
	_, input := sourceCommand()
	input.ProjectID = pid
	if _, err := sources.Write(t.Context(), creator, input); err == nil {
		t.Fatal("fault did not retain intent")
	}
	plan := pendingScriptPlan(t, owner, pid)
	cleanScriptPlan(t, privateObjects, plan)
	router := recoveryRouter(recovery, admin)
	path := "/api/projects/" + pid.String() + "/script-source-writes"
	page := scriptHTTP(t, router, "GET", path, uuid.Nil, nil)
	if page.Code != 200 || strings.Contains(page.Body.String(), "projects/") || strings.Contains(page.Body.String(), "plan") || strings.Contains(page.Body.String(), "object_key") {
		t.Fatal("unsafe recovery DTO", page.Code, page.Body.String())
	}
	var selected app.SourceWritePage
	if err := json.Unmarshal(page.Body.Bytes(), &selected); err != nil || len(selected.Items) != 1 || selected.CurrentActorID != admin.ID || selected.CurrentOrgID != admin.OrgID {
		t.Fatal("authoritative current scope", err)
	}
	body := scripthttp.SourceControlRequest{ExpectedRevision: &selected.Items[0].Revision}
	key := uuid.New()
	cancelPath := path + "/" + selected.Items[0].ID.String() + "/cancel"
	accepted := scriptHTTP(t, router, "POST", cancelPath, key, body)
	if accepted.Code != 202 {
		t.Fatal("explicit acceptance", accepted.Code, accepted.Body.String())
	}
	current := scriptHTTP(t, router, "GET", path+"/"+selected.Items[0].ID.String(), uuid.Nil, nil)
	var intent app.SourceWriteIntent
	if err := json.Unmarshal(current.Body.Bytes(), &intent); err != nil || current.Code != 200 || intent.Status != "cancelled" || intent.CanControl {
		t.Fatal("actual public cleanup state", current.Code, current.Body.String(), err)
	}
	replay := scriptHTTP(t, router, "POST", cancelPath, key, body)
	if replay.Code != 202 || replay.Body.String() != accepted.Body.String() {
		t.Fatal("202 original response changed", replay.Code, replay.Body.String())
	}
	foreign, foreignPID := scriptActorProject(t, owner)
	denied := scriptHTTP(t, recoveryRouter(recovery, foreign), "GET", path, uuid.Nil, nil)
	if denied.Code != 404 {
		t.Fatal("cross-org list", foreignPID, denied.Code)
	}
}
