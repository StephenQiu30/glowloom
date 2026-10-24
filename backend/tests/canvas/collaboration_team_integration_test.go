package canvas_test

import (
	"context"
	"net/http/httptest"

	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"

	"testing"

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

func newTeamLiveFixtures(t *testing.T, configure ...func(*config.Config)) (liveFixture, liveFixture, uuid.UUID, uuid.UUID) {
	t.Helper()
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	cfg := identityfixture.Configure(t, config.Config{Env: "local", HTTPAddr: "127.0.0.1:18982", PublicOrigin: liveOrigin})
	for _, apply := range configure {
		apply(&cfg)
	}
	auth := []identity.AuthResult{
		identityfixture.Create(t, db, cfg, "Yjs Browser A", uuid.NewString()+"Aa!"),
		identityfixture.Create(t, db, cfg, "Yjs Browser B", uuid.NewString()+"Aa!"),
	}
	team, scope, project := uuid.New(), uuid.New(), uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workspace.team(id,name,created_by) VALUES(?,'Yjs acceptance',?)`, []any{team, auth[0].User.ID}},
		{`INSERT INTO workspace.creation_scope(id,kind,team_id,physical_scope_id) VALUES(?,'team',?,?)`, []any{scope, team, auth[0].User.OrgID}},
		{`INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES(?,?,'team_admin','active',now(),'created'),(?,?,'member','active',now(),'invited')`, []any{team, auth[0].User.ID, team, auth[1].User.ID}},
		{`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'Yjs acceptance','16:9','realistic')`, []any{project, auth[0].User.OrgID, auth[0].User.ID, scope}},
	} {
		if err := db.Exec(q.sql, q.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	actor := identity.Principal{ID: auth[0].User.ID, OrgID: auth[0].User.OrgID, SessionID: auth[0].Session.ID, CredentialEpoch: auth[0].User.SessionEpoch, PlatformEpoch: auth[0].User.PlatformEpoch, PlatformRole: auth[0].User.PlatformRole, ScopeID: scope}
	store := canvasStore(db)
	doc, err := canvas.NewService(store).Create(t.Context(), actor, project, uuid.NewString(), canvas.CreateInput{Name: "Yjs acceptance"})
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
	receipt, err := canvasfixture.NewLegacySeeder(db, store).Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: []domain.Operation{{Type: "create_node", Node: &text}, {Type: "create_node", Node: &prompt}}})
	if err != nil || receipt.Status != "accepted" {
		t.Fatal(receipt, err)
	}
	if err := db.Exec(`UPDATE workspace.project SET initial_canvas_id=? WHERE id=?`, doc.ID, project).Error; err != nil {
		t.Fatal(err)
	}
	life, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	router, err := app.NewBusinessRouter(zap.NewNop(), nil, noop.NewTracerProvider(), cfg, db, nil, identityruntime.SessionDependencies{}, life)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	service := canvas.NewService(store)
	first := liveFixture{db: db, actor: actor, auth: auth[0], doc: doc, server: server, service: service}
	secondActor := identity.Principal{ID: auth[1].User.ID, OrgID: actor.OrgID, SessionID: auth[1].Session.ID, CredentialEpoch: auth[1].User.SessionEpoch, PlatformEpoch: auth[1].User.PlatformEpoch, PlatformRole: auth[1].User.PlatformRole, ScopeID: scope}
	second := liveFixture{db: db, actor: secondActor, auth: auth[1], doc: doc, server: server, service: service}
	return first, second, team, text.ID
}
