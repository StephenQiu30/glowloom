package workspace_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	canvasapp "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func TestM1BlankProjectOwnsInitialCanvasAndReceipt(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	orgID := insertWorkspaceOrganization(ctx, t, database)
	actor := insertWorkspaceActor(ctx, t, database, orgID)
	other := insertWorkspaceActor(ctx, t, database, orgID)
	// Legacy rows without a known owner must remain hidden, never reassigned.
	if err := database.Exec(`INSERT INTO workspace.project(id,org_id,name,aspect_ratio,style_type) VALUES(?,?,'Unassigned legacy','16:9','realistic')`, uuid.New(), actor.OrgID).Error; err != nil {
		t.Fatal(err)
	}
	runtimeURL, err := url.Parse(os.Getenv("LV_TEST_WORKSPACE_DB_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	query := runtimeURL.Query()
	query.Set("options", "-c role=lanverse_app")
	runtimeURL.RawQuery = query.Encode()
	conn, err := db.Open(ctx, runtimeURL.String(), noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	database = conn.DB
	var role string
	if err := database.Raw(`SELECT current_user`).Scan(&role).Error; err != nil || role != "lanverse_app" {
		t.Fatal("runtime role", err)
	}
	router := projectHTTPRouter(database, actor)
	key := uuid.NewString()
	body := []byte(`{"name":"M1 blank","aspect_ratio":"1:1","style_key":"film"}`)
	first := projectHTTPRequest(ctx, router, http.MethodPost, "/api/projects", key, body)
	if first.Code != 201 {
		t.Fatalf("create blank: %d %s", first.Code, first.Body.String())
	}
	var receipt struct {
		ID       uuid.UUID `json:"id"`
		CanvasID uuid.UUID `json:"canvas_id"`
		StyleKey string    `json:"style_key"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &receipt); err != nil || receipt.ID == uuid.Nil || receipt.CanvasID == uuid.Nil || receipt.StyleKey != "film" {
		t.Fatal("invalid blank project receipt", err)
	}
	store := pgworkspace.NewStore(database)
	project, err := store.FindProject(ctx, actor, receipt.ID)
	if err != nil || project.AspectRatio != "1:1" {
		t.Fatal("read real project", err)
	}
	snapshot, err := store.ReadProjectSnapshot(ctx, actor, receipt.ID)
	if err != nil || snapshot.Project.OwnerID != actor.ID || snapshot.Project.CanvasID != receipt.CanvasID || snapshot.Project.StyleKey == nil || *snapshot.Project.StyleKey != "film" {
		t.Fatal("detail context", err)
	}
	if _, err := store.ReadProjectSnapshot(ctx, other, receipt.ID); !errors.Is(err, pgworkspace.ErrProjectNotFound) {
		t.Fatal("detail disclosed to other creator", err)
	}
	if err := store.RequireOwnedProject(ctx, other, receipt.ID); !errors.Is(err, pgworkspace.ErrProjectNotFound) {
		t.Fatal("project route guard", err)
	}
	canvas := canvasapp.NewService(pgcanvas.NewStore(database, nil))
	document, err := canvas.Get(ctx, actor, receipt.CanvasID)
	if err != nil || document.ProjectID != receipt.ID || len(document.Nodes) != 0 || len(document.Edges) != 0 || document.Revision != 1 {
		t.Fatal("read initial blank canvas", err)
	}
	if _, err := store.FindProject(ctx, other, receipt.ID); !errors.Is(err, pgworkspace.ErrProjectNotFound) {
		t.Fatal("same-organization project disclosed", err)
	}
	if _, err := canvas.Get(ctx, other, receipt.CanvasID); err == nil {
		t.Fatal("same-organization canvas disclosed")
	}
	page, err := workspaceapp.NewListProjectsQuery(store).Execute(ctx, other, workspaceapp.ListProjectsInput{Limit: 50})
	if err != nil || len(page.Projects) != 0 {
		t.Fatal("another creator sees owner's list", err)
	}
	ownerPage, err := workspaceapp.NewListProjectsQuery(store).Execute(ctx, actor, workspaceapp.ListProjectsInput{Limit: 50})
	if err != nil || len(ownerPage.Projects) != 1 || ownerPage.Projects[0].ID != receipt.ID {
		t.Fatal("legacy project assigned or owner list incomplete", err)
	}
	second := projectHTTPRequest(ctx, router, http.MethodPost, "/api/projects", key, body)
	if second.Code != 201 || second.Body.String() != first.Body.String() {
		t.Fatal("lost receipt retry changed identity")
	}
	conflict := projectHTTPRequest(ctx, router, http.MethodPost, "/api/projects", key, []byte(`{"name":"different","aspect_ratio":"1:1"}`))
	if conflict.Code != 409 {
		t.Fatalf("changed payload reused key: %d", conflict.Code)
	}
	var count int64
	if err := database.Raw(`SELECT count(*) FROM canvas.document WHERE project_id=?`, receipt.ID).Scan(&count).Error; err != nil || count != 1 {
		t.Fatal("duplicate or missing initial canvas", err)
	}
	var audit struct{ StyleKey string }
	if err := database.Raw(`SELECT payload->'data'->'after'->>'style_key' AS style_key FROM infra.outbox WHERE partition_key=? AND topic='lanverse.audit.recorded.v1'`, receipt.ID.String()).Scan(&audit).Error; err != nil || audit.StyleKey != "film" {
		t.Fatal("creation audit omitted selected style", err)
	}
}

func TestM1BlankProjectRatiosAndCrossProjectCanvasConstraint(t *testing.T) {
	ctx, database := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, database, insertWorkspaceOrganization(ctx, t, database))
	router := projectHTTPRouter(database, actor)
	var projects []struct {
		ID       uuid.UUID `json:"id"`
		CanvasID uuid.UUID `json:"canvas_id"`
	}
	for _, ratio := range []string{"9:16", "16:9", "1:1"} {
		response := projectHTTPRequest(ctx, router, http.MethodPost, "/api/projects", uuid.NewString(), []byte(fmt.Sprintf(`{"name":"Ratio %s","aspect_ratio":"%s"}`, ratio, ratio)))
		if response.Code != 201 {
			t.Fatalf("ratio %s: %d %s", ratio, response.Code, response.Body.String())
		}
		var project struct {
			ID       uuid.UUID `json:"id"`
			CanvasID uuid.UUID `json:"canvas_id"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &project); err != nil || project.CanvasID == uuid.Nil {
			t.Fatal("ratio receipt", err)
		}
		projects = append(projects, project)
	}
	foreign := database.Begin()
	if foreign.Error != nil {
		t.Fatal(foreign.Error)
	}
	defer func() { _ = foreign.Rollback().Error }()
	if err := foreign.Exec(`UPDATE workspace.project SET initial_canvas_id=? WHERE id=?`, projects[1].CanvasID, projects[0].ID).Error; err != nil {
		t.Fatal("deferred FK failed before commit", err)
	}
	if err := foreign.Commit().Error; err == nil {
		t.Fatal("initial canvas crossed project boundary")
	}
	var current struct{ CanvasID uuid.UUID }
	if err := database.Raw(`SELECT initial_canvas_id AS canvas_id FROM workspace.project WHERE id=?`, projects[0].ID).Scan(&current).Error; err != nil || current.CanvasID != projects[0].CanvasID {
		t.Fatal("failed FK transaction changed default canvas", err)
	}
	response := projectHTTPRequest(ctx, router, http.MethodPost, "/api/projects", uuid.NewString(), []byte(fmt.Sprintf(`{"name":"Injected owner","aspect_ratio":"1:1","owner_id":"%s"}`, uuid.New())))
	if response.Code != 422 {
		t.Fatalf("owner injection: %d", response.Code)
	}
	retiredKey := uuid.New()
	legacyID := uuid.New()
	if err := database.Exec(`INSERT INTO workspace.project(id,org_id,name,aspect_ratio,style_type) VALUES(?,?,'Unassigned old request','16:9','realistic')`, legacyID, actor.OrgID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`INSERT INTO workspace.project_creation_receipt(actor_id,idem_key,request_hash,project_id,response_body) VALUES(?,?,?,?,?::jsonb)`, actor.ID, retiredKey, strings.Repeat("a", 64), legacyID, fmt.Sprintf(`{"ID":"%s","OrgID":"%s"}`, legacyID, actor.OrgID)).Error; err != nil {
		t.Fatal(err)
	}
	retired := projectHTTPRequest(ctx, router, http.MethodPost, "/api/projects", retiredKey.String(), []byte(`{"name":"Retired request","aspect_ratio":"16:9"}`))
	if retired.Code != 409 || countProjectsNamed(ctx, t, database, actor.OrgID, "Retired request") != 0 {
		t.Fatalf("retired request created duplicate: %d", retired.Code)
	}
}
