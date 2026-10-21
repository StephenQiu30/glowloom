package canvas_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/identityruntime"
	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

// Explicit opt-in runtime for manual real-browser acceptance. The manifest is
// private local test data; production credentials/services are never used.
func TestCollaborationBrowserFixture(t *testing.T) {
	manifest := os.Getenv("GLOWLOOM_OT_BROWSER_MANIFEST")
	if manifest == "" {
		t.Skip("manual OT browser fixture not requested")
	}
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	cfg := identityfixture.Configure(t, config.Config{Env: "local", HTTPAddr: "127.0.0.1:18982", PublicOrigin: "http://127.0.0.1:3199"})
	auth := []identity.AuthResult{
		identityfixture.Create(t, db, cfg, "OT Browser A", uuid.NewString()+"Aa!"),
		identityfixture.Create(t, db, cfg, "OT Browser B", uuid.NewString()+"Aa!"),
	}
	team, scope, project := uuid.New(), uuid.New(), uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workspace.team(id,name,created_by) VALUES(?,'OT acceptance',?)`, []any{team, auth[0].User.ID}},
		{`INSERT INTO workspace.creation_scope(id,kind,team_id,physical_scope_id) VALUES(?,'team',?,?)`, []any{scope, team, auth[0].User.OrgID}},
		{`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'team_admin','active',now(),'created'),(?,?,'member','active',now(),'invited')`, []any{team, auth[0].User.ID, team, auth[1].User.ID}},
		{`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'OT acceptance','16:9','realistic')`, []any{project, auth[0].User.OrgID, auth[0].User.ID, scope}},
	} {
		if err := db.Exec(q.sql, q.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	actor := identity.Principal{ID: auth[0].User.ID, OrgID: auth[0].User.OrgID, SessionID: auth[0].Session.ID, CredentialEpoch: auth[0].User.SessionEpoch, PlatformEpoch: auth[0].User.PlatformEpoch, PlatformRole: auth[0].User.PlatformRole, ScopeID: scope}
	store := canvasStore(db)
	doc, err := canvas.NewService(store).Create(t.Context(), actor, project, uuid.NewString(), canvas.CreateInput{Name: "OT acceptance"})
	if err != nil {
		t.Fatal(err)
	}
	text := resourceNode("text")
	text.Title = "协同文本"
	text.X = 100
	text.Y = 100
	text.Config.Text = ptr("AB中文😀")
	prompt := composerNode(t)
	prompt.InputRevision = 0
	prompt.Title = "协同提示词"
	prompt.X = 550
	prompt.Y = 100
	receipt, err := canvas.NewCollaborationService(store).Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvas.CollaborationInput{OperationVersion: 1, BaseRevision: 1, Operations: []domain.Operation{{Type: "create_node", Node: &text}, {Type: "create_node", Node: &prompt}}})
	if err != nil || receipt.Status != "accepted" {
		t.Fatal(receipt, err)
	}
	if err := db.Exec(`UPDATE workspace.project SET initial_canvas_id=? WHERE id=?`, doc.ID, project).Error; err != nil {
		t.Fatal(err)
	}
	life, cancel := context.WithCancel(t.Context())
	defer cancel()
	router, err := app.NewBusinessRouter(zap.NewNop(), nil, noop.NewTracerProvider(), cfg, db, nil, identityruntime.SessionDependencies{}, life)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", cfg.HTTPAddr)
	if err != nil {
		t.Fatal(err)
	}
	server := http.Server{Handler: router, ReadHeaderTimeout: 5 * time.Second}
	defer func() { _ = server.Close() }()
	go func() { _ = server.Serve(listener) }()
	data, err := json.Marshal(map[string]any{"tokens": []string{auth[0].Token, auth[1].Token}, "scope": scope, "project": project, "canvas": doc.ID, "text": text.ID, "prompt": prompt.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("OT browser fixture ready")
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-t.Context().Done():
			return
		case <-tick.C:
			if _, err := os.Stat(os.Getenv("GLOWLOOM_OT_BROWSER_STOP")); err == nil {
				return
			}
		}
	}
}
