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
	receipt, err := canvas.NewCollaborationService(store).Submit(t.Context(), actor, doc.ID, uuid.NewString(), canvas.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: []domain.Operation{{Type: "create_node", Node: &text}, {Type: "create_node", Node: &prompt}}})
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

// Two authenticated accounts use real sockets against the business router.
func TestCollaborationTeamAccountsObserveChangesAndRevocation(t *testing.T) {
	first, second, team, text := newTeamLiveFixtures(t)
	a, b := first.dial(t), second.dial(t)
	first.join(t, a)
	second.join(t, b)
	key := uuid.NewString()
	input := canvas.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 2, Operations: []domain.Operation{{Type: "text", NodeID: text, Path: []string{"config", "text"}, Text: domain.SequenceEdit{{Insert: []domain.Atom{{Type: "text", Text: "shared "}}}}}}}
	liveSend(t, a, map[string]any{"type": "submit", "op_id": key, "body": input})
	accepted := liveRead(t, a, "ack", 3)
	if accepted.Receipt == nil || accepted.Receipt.Status != "accepted" {
		t.Fatal(accepted)
	}
	observed := liveRead(t, b, "changes", 3)
	if len(observed.Changes) != 1 || observed.Changes[0].OpID != key {
		t.Fatal("other account missed canonical edit")
	}
	if err := first.db.Exec(`UPDATE workspace.team_member SET status='removed' WHERE team_id=? AND user_id=?`, team, second.actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	liveSend(t, b, map[string]any{"type": "sync"})
	denied := liveError(t, b)
	if denied.Status != 404 || denied.Document != nil || denied.Changes != nil {
		t.Fatalf("revoked account received private state: %+v", denied)
	}
}
