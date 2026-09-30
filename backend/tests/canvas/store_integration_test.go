package canvas_test

import (
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	pgcanvas "github.com/StephenQiu30/lanverse/backend/internal/canvas/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/db"
)

func canvasDB(t *testing.T, key string) *gorm.DB {
	t.Helper()
	dsn := os.Getenv(key)
	if dsn == "" {
		t.Skip("set isolated " + key)
	}
	conn, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn.DB
}
func seedCanvasActor(t *testing.T, database *gorm.DB) (identityapp.Principal, uuid.UUID) {
	t.Helper()
	a := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), Role: identitydomain.RoleProducer}
	p := uuid.New()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workspace.organization(id,name) VALUES(?, 'Canvas test')`, []any{a.OrgID}},
		{`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password) VALUES(?,?,?,'Canvas test','producer','synthetic',false)`, []any{a.ID, a.OrgID, "canvas-" + a.ID.String()}},
		{`INSERT INTO workspace.project(id,org_id,name,aspect_ratio,style_type) VALUES(?,?,'Canvas test','16:9','realistic')`, []any{p, a.OrgID}},
	} {
		if err := database.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	return a, p
}
func TestCanvasPersistenceRevisionReplayRollbackAndRestore(t *testing.T) {
	database := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	service := application.NewService(pgcanvas.NewStore(database))
	key := uuid.NewString()
	doc, err := service.Create(t.Context(), actor, project, key, application.CreateInput{Name: "备注"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := application.NewService(pgcanvas.NewStore(database)).Create(t.Context(), actor, project, key, application.CreateInput{Name: "备注"})
	if err != nil || again.ID != doc.ID {
		t.Fatalf("persistent create replay: %+v %v", again, err)
	}
	n1, n2, e := uuid.New(), uuid.New(), uuid.New()
	input := application.CommandsInput{ExpectedRevision: 1, Commands: []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{{ID: n1, NodeType: "text", NodeAction: "resource", Config: domain.TextConfig{Text: "A"}}, {ID: n2, NodeType: "text", NodeAction: "resource", Config: domain.TextConfig{Text: "B"}}}}, {Type: "Connect", Edge: &domain.Edge{ID: e, EdgeType: "annotation", SourceNodeID: n1, TargetNodeID: n2}}}}
	key = uuid.NewString()
	result, err := service.Execute(t.Context(), actor, doc.ID, key, input)
	if err != nil || result.Revision != 2 {
		t.Fatalf("apply: %+v %v", result, err)
	}
	result, err = application.NewService(pgcanvas.NewStore(database)).Execute(t.Context(), actor, doc.ID, key, input)
	if err != nil || result.Revision != 2 {
		t.Fatalf("persistent command replay: %+v %v", result, err)
	}
	input.ExpectedRevision = 2
	if _, err := service.Execute(t.Context(), actor, doc.ID, key, input); !errors.Is(err, application.ErrIdempotencyConflict) {
		t.Fatalf("changed key body: %v", err)
	}
	_, err = service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 2, Commands: []domain.Command{{Type: "MoveNodes", Moves: []domain.Move{{ID: n1, X: 80}}}, {Type: "DeleteNodes", IDs: []uuid.UUID{uuid.New()}}}})
	if !errors.Is(err, domain.ErrInvalidCommand) {
		t.Fatalf("bad batch accepted: %v", err)
	}
	current, err := service.Get(t.Context(), actor, doc.ID)
	if err != nil || current.Revision != 2 || current.Nodes[0].X != 0 {
		t.Fatalf("partial write: %+v %v", current, err)
	}
	if _, err := service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 2, Commands: []domain.Command{{Type: "DeleteNodes", IDs: []uuid.UUID{n1}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 3, Commands: input.Commands}); !errors.Is(err, domain.ErrInvalidCommand) {
		t.Fatalf("duplicate existing node allowed: %v", err)
	}
	restore := application.CommandsInput{ExpectedRevision: 3, Commands: []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{{ID: n1, NodeType: "text", NodeAction: "resource", Config: domain.TextConfig{Text: "A"}}}}, {Type: "Connect", Edge: &domain.Edge{ID: e, EdgeType: "annotation", SourceNodeID: n1, TargetNodeID: n2}}}}
	result, err = service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), restore)
	if err != nil || len(result.Nodes) != 2 || len(result.Edges) != 1 {
		t.Fatalf("restore: %+v %v", result, err)
	}
	other, otherProject := seedCanvasActor(t, database)
	if _, err := service.Get(t.Context(), other, doc.ID); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("crossorg: %v", err)
	}
	otherDoc, err := service.Create(t.Context(), other, otherProject, uuid.NewString(), application.CreateInput{Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Execute(t.Context(), other, otherDoc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 1, Commands: []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{{ID: n1, NodeType: "text", NodeAction: "resource"}}}}})
	if !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("crossdocument UUID restored: %v", err)
	}
	if err := database.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.Execute(t.Context(), actor, doc.ID, key, input); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatalf("revoked actor replayed: %v", err)
	}
	var changes, audits, logs int64
	database.Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key=? AND topic='lanverse.canvas.document_changed.v1'`, project.String()).Scan(&changes)
	database.Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key=? AND topic='lanverse.audit.recorded.v1'`, project.String()).Scan(&audits)
	database.Raw(`SELECT count(*) FROM canvas.command_log WHERE document_id=?`, doc.ID).Scan(&logs)
	if changes != 4 || audits != 0 || logs != 3 {
		t.Fatalf("events changes=%d audits=%d logs=%d", changes, audits, logs)
	}
}

func TestConcurrentCanvasCommandsAdvanceOneRevision(t *testing.T) {
	database := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	service := application.NewService(pgcanvas.NewStore(database))
	doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "Concurrent"})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			_, err := service.Execute(t.Context(), actor, doc.ID, uuid.NewString(), application.CommandsInput{ExpectedRevision: 1, Commands: []domain.Command{{Type: "SetViewport", Viewport: &domain.Viewport{Zoom: 1}}}})
			results <- err
		})
	}
	wg.Wait()
	close(results)
	ok, conflict := 0, 0
	for err := range results {
		var revision *application.RevisionConflict
		if err == nil {
			ok++
		} else if errors.As(err, &revision) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", ok, conflict)
	}
}
