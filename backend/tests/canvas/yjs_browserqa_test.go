//go:build browserqa

package canvas_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/identityruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/yjsruntime"
	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/objectstorage"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

// TestYjsBrowserFixture runs only in an explicitly isolated QA process. Its login
// helper is not compiled into the application binary or ordinary test targets.
func TestYjsBrowserFixture(t *testing.T) {
	output := os.Getenv("GLOWLOOM_BROWSER_QA_OUTPUT")
	if output == "" {
		t.Fatal("explicit fixture output directory required")
	}
	database := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	origin := os.Getenv("GLOWLOOM_BROWSER_QA_ORIGIN")
	if origin == "" {
		origin = "http://[::1]:1126"
	}
	cfg := identityfixture.Configure(t, config.Config{Env: "local", HTTPAddr: "127.0.0.1:8080", PublicOrigin: origin, CanvasRuntimeURL: os.Getenv("GLOWLOOM_CANVAS_RUNTIME_URL"), CanvasRuntimeToken: os.Getenv("GLOWLOOM_COLLAB_RUNTIME_TOKEN")})
	if cfg.CanvasRuntimeURL == "" || len(cfg.CanvasRuntimeToken) < 32 {
		t.Fatal("private QA runtime required")
	}
	accounts := []identity.AuthResult{identityfixture.Create(t, database, cfg, "协作验收甲", uuid.NewString()+"Aa!"), identityfixture.Create(t, database, cfg, "协作验收乙", uuid.NewString()+"Aa!")}
	team, scope, project := uuid.New(), uuid.New(), uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workspace.team(id,name,created_by) VALUES(?,'Yjs 三页面验收',?)`, []any{team, accounts[0].User.ID}},
		{`INSERT INTO workspace.creation_scope(id,kind,team_id,physical_scope_id) VALUES(?,'team',?,?)`, []any{scope, team, accounts[0].User.OrgID}},
		{`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'team_admin','active',now(),'created'),(?,?,'member','active',now(),'invited')`, []any{team, accounts[0].User.ID, team, accounts[1].User.ID}},
		{`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'多人协作验收','16:9','realistic')`, []any{project, accounts[0].User.OrgID, accounts[0].User.ID, scope}},
	} {
		if err := database.Exec(q.sql, q.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	actor := identity.Principal{ID: accounts[0].User.ID, OrgID: accounts[0].User.OrgID, SessionID: accounts[0].Session.ID, CredentialEpoch: accounts[0].User.SessionEpoch, PlatformEpoch: accounts[0].User.PlatformEpoch, PlatformRole: accounts[0].User.PlatformRole, ScopeID: scope}
	store := canvasStore(database)
	document, err := canvas.NewService(store).Create(t.Context(), actor, project, uuid.NewString(), canvas.CreateInput{Name: "多人协作验收"})
	if err != nil {
		t.Fatal(err)
	}
	text := resourceNode("text")
	text.Title = "同段富文本"
	text.Config.Text = ptr("共同编辑：中文与emoji 🌏")
	text.X = 140
	text.Y = 140
	text.Width = ptr(480.0)
	text.Height = ptr(280.0)
	sourceA, sourceB := resourceNode("text"), resourceNode("text")
	sourceA.Title = "参考甲"
	sourceA.Config.Text = ptr("甲参考内容")
	sourceA.X = 720
	sourceA.Y = 100
	sourceA.Width = ptr(240.0)
	sourceA.Height = ptr(150.0)
	sourceB.Title = "参考乙"
	sourceB.Config.Text = ptr("乙参考内容")
	sourceB.X = 720
	sourceB.Y = 380
	sourceB.Width = ptr(240.0)
	sourceB.Height = ptr(150.0)
	if os.Getenv("GLOWLOOM_BROWSER_QA_ADMIN_COPY") == "true" {
		text.Config.Editor = &domain.EditorConfig{SchemaVersion: 1, Parameters: &domain.EditorParameters{Mode: "MANUAL"}}
	}
	_, err = canvasfixture.NewLegacySeeder(database, store).Submit(t.Context(), actor, document.ID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: []domain.Operation{{Type: "create_node", Node: &text}, {Type: "create_node", Node: &sourceA}, {Type: "create_node", Node: &sourceB}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`UPDATE workspace.project SET initial_canvas_id=? WHERE id=?`, document.ID, project).Error; err != nil {
		t.Fatal(err)
	}
	if os.Getenv("GLOWLOOM_BROWSER_QA_LEGACY") == "true" {
		live := canvas.NewYjsService(store, yjsruntime.New(cfg.CanvasRuntimeURL, cfg.CanvasRuntimeToken))
		state, err := live.Initialize(t.Context(), actor, document.ID, 2)
		if err != nil {
			t.Fatal(err)
		}
		payload, _ := json.Marshal(map[string]any{"state": state.State, "legacy": true})
		command := exec.CommandContext(t.Context(), "node", "../../../canvas-collab/tests/legacy-fixture.mjs")
		command.Stdin = bytes.NewReader(payload)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("legacy fixture: %s %v", output, err)
		}
		var legacy struct {
			State     []byte
			GraphHash string
		}
		if err = json.Unmarshal(output, &legacy); err != nil {
			t.Fatal(err)
		}
		if err = database.Exec(`UPDATE canvas.collab_document SET state=?,graph_hash=? WHERE document_id=?`, legacy.State, legacy.GraphHash, document.ID).Error; err != nil {
			t.Fatal(err)
		}
		if err = database.Exec(`UPDATE canvas.collab_version SET state=?,graph_hash=? WHERE document_id=?`, legacy.State, legacy.GraphHash, document.ID).Error; err != nil {
			t.Fatal(err)
		}
	}
	life, cancel := context.WithCancel(t.Context())
	defer cancel()
	if os.Getenv("GLOWLOOM_BROWSER_QA_ADMIN_COPY") == "true" {
		cfg.TemporalAddr = os.Getenv("GLOWLOOM_TEST_TEMPORAL_ADDR")
		cfg.TemporalNamespace = os.Getenv("GLOWLOOM_TEST_TEMPORAL_NAMESPACE")
		if err := database.Exec(`UPDATE identity."user" SET platform_role='platform_admin' WHERE id=?`, accounts[0].User.ID).Error; err != nil {
			t.Fatal(err)
		}
		startAdminCopyBrowserWorkers(t, life, database, cfg, project)
	}
	var storage *objectstorage.Client
	if os.Getenv("GLOWLOOM_BROWSER_QA_MEDIA") == "true" {
		storage, err = objectstorage.Open(os.Getenv("GLOWLOOM_OBJECT_STORAGE_ENDPOINT"), os.Getenv("GLOWLOOM_OBJECT_STORAGE_BUCKET"), os.Getenv("GLOWLOOM_OBJECT_STORAGE_ACCESS_KEY"), os.Getenv("GLOWLOOM_OBJECT_STORAGE_SECRET_KEY"), "us-east-1")
		if err != nil {
			t.Fatal("isolated QA object storage configuration unavailable")
		}
		if err := storage.Ping(t.Context()); err != nil {
			t.Fatal("isolated QA object storage unavailable")
		}
	}
	router, err := app.NewBusinessRouter(zap.NewNop(), nil, noop.NewTracerProvider(), cfg, database, storage, identityruntime.SessionDependencies{}, life)
	if err != nil {
		t.Fatal(err)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/__qa/login/") {
			index := 0
			if strings.HasSuffix(r.URL.Path, "/b") {
				index = 1
			}
			http.SetCookie(w, &http.Cookie{Name: "glowloom_session", Value: accounts[index].Token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
			http.Redirect(w, r, origin+"/projects", http.StatusSeeOther)
			return
		}
		router.ServeHTTP(w, r)
	})
	address := os.Getenv("GLOWLOOM_BROWSER_QA_ADDR")
	if address == "" {
		address = "127.0.0.1:1125"
	}
	listener, err := (&net.ListenConfig{}).Listen(life, "tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	_ = server.Listener.Close()
	server.Listener = listener
	server.Start()
	defer server.Close()
	manifest, _ := json.MarshalIndent(map[string]any{"canvasID": document.ID, "projectID": project, "scopeID": scope, "teamID": team, "nodeID": text.ID, "actors": []uuid.UUID{accounts[0].User.ID, accounts[1].User.ID}, "url": origin + "/canvas?project_id=" + project.String() + "&canvas_id=" + document.ID.String()}, "", "  ")
	if err := os.WriteFile(filepath.Join(output, "browser-fixture.json"), manifest, 0600); err != nil {
		t.Fatal(err)
	}
	timer := time.NewTimer(45 * time.Minute)
	defer timer.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-t.Context().Done():
			return
		case <-timer.C:
			return
		case <-ticker.C:
			if _, err := os.Stat(filepath.Join(output, "stop-browser-fixture")); err == nil {
				return
			}
		}
	}
}
