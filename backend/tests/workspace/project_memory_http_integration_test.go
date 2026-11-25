package workspace_test

import (
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	agent "github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	agentwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/agent"
	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/creation/canvas/domain"
	workspacehttp "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/adapter/http"
	workspacepg "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/adapter/postgres"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/creation/workspace/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func memoryRouter(db *gorm.DB, actor identity.Principal) *gin.Engine {
	r := gin.New()
	api := r.Group("/api", func(c *gin.Context) { c.Set("principal", actor); c.Next() })
	workspacehttp.RegisterProjectMemory(api, workspace.NewProjectMemories(owningLifecycleStore(db), time.Now))
	return r
}

func TestProjectMemoryHTTPPublishesCurrentRevisionIntoAgentSnapshot(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor, project := lifecycleActorProject(ctx, t, db)
	router := memoryRouter(db, actor)
	path := "/api/projects/" + project.String() + "/memory"
	key := uuid.NewString()

	write := func(id, data string, status int) workspace.ProjectMemory {
		t.Helper()
		resp := projectHTTPRequest(ctx, router, http.MethodPut, path+"/声音", id, []byte(data))
		if resp.Code != status {
			t.Fatalf("memory publish status=%d want=%d body=%s", resp.Code, status, resp.Body)
		}
		var revision workspace.ProjectMemory
		if status == 200 && json.Unmarshal(resp.Body.Bytes(), &revision) != nil {
			t.Fatalf("invalid receipt: %s", resp.Body)
		}
		return revision
	}
	view := func() workspace.ProjectMemoryView {
		t.Helper()
		resp := projectHTTPRequest(ctx, router, http.MethodGet, path, "", nil)
		var result workspace.ProjectMemoryView
		if resp.Code != 200 || json.Unmarshal(resp.Body.Bytes(), &result) != nil {
			t.Fatalf("invalid list: %d %s", resp.Code, resp.Body)
		}
		return result
	}
	snapshot := func() map[string]any {
		t.Helper()
		value, err := agentwire.NewBoundary(db, nil).Capabilities.Snapshot(ctx, agent.Scope{ActorID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, ProjectID: project})
		if err != nil {
			t.Fatal(err)
		}
		return value.(map[string]any)
	}
	if empty := view(); empty.Items == nil || len(empty.Items) != 0 || !empty.CanEdit {
		t.Fatalf("unexpected empty project: %+v", empty)
	}
	emptyVersion := snapshot()["project_memory_version"]
	canvasID := uuid.New()
	lifecycleFixtureSQL(t, db, "INSERT INTO canvas.document(id,project_id,name) VALUES(?,?,'开场')", canvasID, project)
	agentSnapshot := agentwire.NewBoundary(db, nil).Capabilities.Snapshot
	scoped := agent.Scope{ActorID: actor.ID, OrgID: actor.OrgID, ScopeID: actor.ScopeID, ProjectID: project, CanvasID: &canvasID}
	value, err := agentSnapshot(ctx, scoped)
	if err != nil || value.(map[string]any)["active_canvas"].(canvas.Document).ID != canvasID {
		t.Fatalf("selected canvas missing: %v", err)
	}
	unrelated := uuid.New()
	scoped.CanvasID = &unrelated
	if _, err := agentSnapshot(ctx, scoped); err == nil {
		t.Fatal("snapshot accepted an unrelated canvas")
	}
	body := `{"expected_revision":0,"statement":"全片保留同期声"}`
	first := write(key, body, 200)
	if replay := write(key, body, 200); replay != first {
		t.Fatalf("receipt changed: %+v vs %+v", replay, first)
	}
	write(key, `{"expected_revision":0,"statement":"不同内容"}`, 409)
	write(uuid.NewString(), `{"statement":"缺少明确修订"}`, 422)
	write(uuid.NewString(), `{"expected_revision":1,"statement":"加旁白","private_chat":"anything"}`, 422)

	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for _, value := range []string{"只在开场用同期声", "全片无旁白"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			payload, _ := json.Marshal(map[string]any{"expected_revision": 1, "statement": value})
			codes <- projectHTTPRequest(ctx, router, http.MethodPut, path+"/声音", uuid.NewString(), payload).Code
		}()
	}
	wg.Wait()
	close(codes)
	counts := map[int]int{}
	for code := range codes {
		counts[code]++
	}
	if counts[200] != 1 || counts[409] != 1 {
		t.Fatalf("concurrent revisions: %v", counts)
	}

	current := snapshot()
	items := current["project_memory"].([]workspace.ProjectMemory)
	if len(items) != 1 || items[0].Revision != 2 || current["project_memory_version"] == emptyVersion || current["project_memory_version"] != workspacepg.ProjectMemoryVersion(items) {
		t.Fatalf("snapshot did not read the current decision: %v", items)
	}

	otherActor, _ := lifecycleActorProject(ctx, t, db)
	if reply := projectHTTPRequest(ctx, memoryRouter(db, otherActor), http.MethodGet, path, "", nil); reply.Code != 404 {
		t.Fatalf("foreign project disclosure: %d %s", reply.Code, reply.Body)
	}
	write(uuid.NewString(), `{"expected_revision":2,"withdraw":true}`, 200)
	if len(view().Items) != 0 || snapshot()["project_memory_version"] != emptyVersion {
		t.Fatal("withdrawn decision still in active context")
	}
	resp := projectHTTPRequest(ctx, router, http.MethodGet, path+"/声音", "", nil)
	var history []workspace.ProjectMemory
	if resp.Code != 200 || json.Unmarshal(resp.Body.Bytes(), &history) != nil || len(history) != 3 || !history[0].Withdrawn || history[2].ID != first.ID {
		t.Fatalf("invalid immutable history: %d %s", resp.Code, resp.Body)
	}

	archive := projectHTTPRequest(ctx, lifecycleHTTPRouter(db, actor), http.MethodPost, "/api/projects/"+project.String()+"/archive", uuid.NewString(), []byte(`{"expected_revision":1}`))
	if archive.Code != 200 || view().CanEdit {
		t.Fatalf("project archival/edit authority: %d %s", archive.Code, archive.Body)
	}
	write(uuid.NewString(), `{"expected_revision":3,"statement":"已归档"}`, 409)
}

func TestProjectMemoryTeamMemberReceivesPublishedDecisionsUntilRevoked(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	org := insertWorkspaceOrganization(ctx, t, db)
	editor := workspaceTeamActor(t, db, insertWorkspaceActor(ctx, t, db, org))
	member := insertWorkspaceActor(ctx, t, db, org)
	var team uuid.UUID
	if err := db.Raw("SELECT team_id FROM workspace.creation_scope WHERE id=?", editor.ScopeID).Row().Scan(&team); err != nil {
		t.Fatal(err)
	}
	lifecycleFixtureSQL(t, db, `INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source)
		VALUES(?,?,'member','active',now(),'invited')`, team, member.ID)
	member.ScopeID = editor.ScopeID
	project := uuid.MustParse(insertWorkspaceProject(ctx, t, db, editor, "16:9", "realistic", nil))
	path := "/api/projects/" + project.String() + "/memory"
	editorHTTP, memberHTTP := memoryRouter(db, editor), memoryRouter(db, member)
	publish := projectHTTPRequest(ctx, editorHTTP, http.MethodPut, path+"/direction", uuid.NewString(), []byte(`{"expected_revision":0,"statement":"乘客视角"}`))
	if publish.Code != 200 {
		t.Fatalf("publish status=%d body=%s", publish.Code, publish.Body)
	}
	collaboratorScope := agent.Scope{ActorID: member.ID, OrgID: member.OrgID, ScopeID: member.ScopeID, ProjectID: project}
	agentSnapshot := agentwire.NewBoundary(db, nil).Capabilities.Snapshot
	value, err := agentSnapshot(ctx, collaboratorScope)
	if err != nil {
		t.Fatal(err)
	}
	items := value.(map[string]any)["project_memory"].([]workspace.ProjectMemory)
	if len(items) != 1 || items[0].Statement != "乘客视角" || items[0].ConfirmedBy != editor.ID {
		t.Fatalf("collaborator sees incorrect shared scope: %+v", items)
	}
	lifecycleFixtureSQL(t, db, "UPDATE workspace.team SET status='archived' WHERE id=?", team)
	read := projectHTTPRequest(ctx, memberHTTP, http.MethodGet, path, "", nil)
	var view workspace.ProjectMemoryView
	if read.Code != 200 || json.Unmarshal(read.Body.Bytes(), &view) != nil || view.CanEdit || len(view.Items) != 1 {
		t.Fatalf("archived team read: %d %s", read.Code, read.Body)
	}
	if response := projectHTTPRequest(ctx, memberHTTP, http.MethodPut, path+"/direction", uuid.NewString(), []byte(`{"expected_revision":1,"statement":"拒绝写入"}`)); response.Code != 404 {
		t.Fatalf("read-only team wrote or exposed scope: %d %s", response.Code, response.Body)
	}
	lifecycleFixtureSQL(t, db, "UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?", team, member.ID)
	if _, err := agentSnapshot(ctx, collaboratorScope); err == nil {
		t.Fatal("removed member can still read model snapshot")
	}
	if response := projectHTTPRequest(ctx, memberHTTP, http.MethodGet, path+"/direction", "", nil); response.Code != 404 {
		t.Fatalf("removed member can still read history: %d %s", response.Code, response.Body)
	}
}
