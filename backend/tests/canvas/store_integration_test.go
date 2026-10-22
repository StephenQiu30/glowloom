package canvas_test

import (
	"errors"
	"os"
	"sync"
	"testing"

	canvasfixture "github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func canvasDB(t *testing.T, key string) *gorm.DB {
	t.Helper()
	dsn := os.Getenv(key)
	if dsn == "" {
		t.Fatal("required isolated PostgreSQL DSN is missing: " + key)
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
	} {
		if err := database.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	a = identityfixture.Session(t, database, a)
	if err := database.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'Canvas test','16:9','realistic')`, p, a.OrgID, a.ID, a.ScopeID).Error; err != nil {
		t.Fatal(err)
	}
	return a, p
}
func TestCanvasPersistenceRevisionReplayRollbackAndRestore(t *testing.T) {
	database := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	service := canvasfixture.NewService(canvasStore(database))
	key := uuid.NewString()
	doc, err := service.Create(t.Context(), actor, project, key, application.CreateInput{Name: "备注"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := canvasfixture.NewService(canvasStore(database)).Create(t.Context(), actor, project, key, application.CreateInput{Name: "备注"})
	if err != nil || again.ID != doc.ID {
		t.Fatalf("persistent create replay: %+v %v", again, err)
	}
	n1, n2, e := uuid.New(), uuid.New(), uuid.New()
	input := application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: canvasfixture.Concat(canvasfixture.Nodes([]domain.Node{{ID: n1, Title: "备注", NodeType: "text", NodeAction: "resource", Config: domain.NodeConfig{Text: ptr("A")}}, {ID: n2, Title: "备注", NodeType: "text", NodeAction: "resource", Config: domain.NodeConfig{Text: ptr("B")}}}), canvasfixture.Edges([]domain.Edge{{ID: e, EdgeType: "annotation", SourceNodeID: n1, TargetNodeID: n2}}))}
	key = uuid.NewString()
	result, err := service.Edit(t.Context(), actor, doc.ID, key, input)
	if err != nil || result.Revision != 2 {
		t.Fatalf("apply: %+v %v", result, err)
	}
	result, err = canvasfixture.NewService(canvasStore(database)).Edit(t.Context(), actor, doc.ID, key, input)
	if err != nil || result.Revision != 2 {
		t.Fatalf("persistent command replay: %+v %v", result, err)
	}
	input.BaseRevision = 2
	if _, err := service.Edit(t.Context(), actor, doc.ID, key, input); !errors.Is(err, application.ErrIdempotencyConflict) {
		t.Fatalf("changed key body: %v", err)
	}
	_, err = service.Edit(t.Context(), actor, doc.ID, uuid.NewString(), application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 2, Operations: canvasfixture.Concat(canvasfixture.Positions([]domain.Move{{ID: n1, X: 80}}), canvasfixture.Deletes([]uuid.UUID{uuid.New()}))})
	if !operationRejected(err, "invalid_operation", "id_conflict") {
		t.Fatalf("bad batch accepted: %v", err)
	}
	current, err := service.Get(t.Context(), actor, doc.ID)
	if err != nil || current.Revision != 2 || current.Nodes[0].X != 0 {
		t.Fatalf("partial write: %+v %v", current, err)
	}
	deleteKey := uuid.NewString()
	if _, err := service.Edit(t.Context(), actor, doc.ID, deleteKey, application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 2, Operations: canvasfixture.Concat(canvasfixture.Deletes([]uuid.UUID{n1}))}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Edit(t.Context(), actor, doc.ID, uuid.NewString(), application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 3, Operations: input.Operations}); !operationRejected(err, "invalid_operation", "id_conflict") {
		t.Fatalf("duplicate existing node allowed: %v", err)
	}
	restore := application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 3, UndoOf: deleteKey}
	result, err = service.Edit(t.Context(), actor, doc.ID, uuid.NewString(), restore)
	if err != nil || len(result.Nodes) != 2 || len(result.Edges) != 1 {
		t.Fatalf("restore: %+v %v", result, err)
	}
	other, otherProject := seedCanvasActor(t, database)
	if _, err := service.Get(t.Context(), other, doc.ID); !errors.Is(err, mediaapp.ErrNotFound) {
		t.Fatalf("crossorg: %v", err)
	}
	otherDoc, err := service.Create(t.Context(), other, otherProject, uuid.NewString(), application.CreateInput{Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Edit(t.Context(), other, otherDoc.ID, uuid.NewString(), application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: canvasfixture.Concat(canvasfixture.Nodes([]domain.Node{{ID: n1, Title: "备注", NodeType: "text", NodeAction: "resource", Config: domain.NodeConfig{Text: ptr("")}}}))})
	if !operationRejected(err, "id_conflict") {
		t.Fatalf("crossdocument UUID restored: %v", err)
	}
	if err := database.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.Edit(t.Context(), actor, doc.ID, key, input); !errors.Is(err, identityapp.ErrSessionInvalid) {
		t.Fatalf("revoked actor replayed: %v", err)
	}
	var changes, audits, logs int64
	database.Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key=? AND topic='glowloom.canvas.document_changed.v1'`, project.String()).Scan(&changes)
	database.Raw(`SELECT count(*) FROM infra.outbox WHERE partition_key=? AND topic='glowloom.audit.recorded.v1'`, project.String()).Scan(&audits)
	database.Raw(`SELECT count(*) FROM canvas.command_log WHERE document_id=?`, doc.ID).Scan(&logs)
	if changes != 4 || audits != 0 || logs != 3 {
		t.Fatalf("events changes=%d audits=%d logs=%d", changes, audits, logs)
	}
}

func TestConcurrentCanvasOperationsJointlyAdvanceRevisions(t *testing.T) {
	database := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	service := canvasfixture.NewService(canvasStore(database))
	doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "Concurrent"})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			_, err := service.Edit(t.Context(), actor, doc.ID, uuid.NewString(), application.CollaborationInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: canvasfixture.Nodes([]domain.Node{resourceNode("text")})})
			results <- err
		})
	}
	wg.Wait()
	close(results)
	accepted := 0
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
		accepted++
	}
	read, err := service.Get(t.Context(), actor, doc.ID)
	if err != nil || accepted != 2 || read.Revision != 3 || len(read.Nodes) != 2 {
		t.Fatal("independent OT contributions lost", read, err)
	}
}

func canvasStore(database *gorm.DB) *pgcanvas.Store {
	return pgcanvas.NewScopedStore(database, func(tx *gorm.DB) application.MediaReader {
		return mediaapp.NewAssetQuery(pgmedia.NewScopedStore(tx, mediawire.NewMediaScopeAccess), nil)
	}, mediawire.NewCanvasProjectAccess)
}

// operationRejected checks the stable terminal business code exposed by OT.
func operationRejected(err error, codes ...string) bool {
	var rejection *domain.OperationError
	if !errors.As(err, &rejection) {
		return false
	}
	for _, code := range codes {
		if rejection.Code == code {
			return true
		}
	}
	return false
}
