package canvas_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	canvaswire "github.com/StephenQiu30/glowloom-server/backend/internal/app/canvas"
	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func TestCanvasAssetCreationAllocatesActualPrimaryAndTemplatesWithoutGeneration(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, fixtureProject := seedCanvasActor(t, db)
	// Remove only this disposable fixture's otherwise-empty seed project:
	// the public asset entry must work before the actor has any projects.
	if err := db.Exec(`DELETE FROM workspace.project WHERE id=? AND owner_id=?`, fixtureProject, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	creator := application.NewAssetCreator(pgcanvas.NewAssetCreationStore(db, canvaswire.AssetCreationAuthority))
	for _, input := range []application.AssetCreationInput{{}, {Kind: "short_drama"}, {TemplateKey: "text-to-image-v1"}, {Kind: "short_drama", TemplateKey: "storyboard-three-shots-v1"}} {
		key := uuid.New()
		created, err := creator.Create(t.Context(), actor, key, input)
		if err != nil || created.ID == uuid.Nil || created.ProjectID == uuid.Nil || created.IsTemplate || created.Revision != 1 || created.ProjectRevision != 1 {
			t.Fatal(created, err)
		}
		doc, err := application.NewService(canvasStore(db)).Get(t.Context(), actor, created.ID)
		if err != nil || doc.ID != created.ID || doc.ProjectID != created.ProjectID || doc.Name != created.Name || application.Snapshot(doc).Kind != created.Kind || doc.Revision != 1 {
			t.Fatal(doc, created, err)
		}
		want := 0
		if input.TemplateKey == "text-to-image-v1" {
			want = 2
		}
		if input.TemplateKey == "storyboard-three-shots-v1" {
			want = 6
		}
		if len(doc.Nodes) != want || len(doc.Edges) != want/2 {
			t.Fatal("template graph not persisted", doc)
		}
		var facts struct {
			CanvasID    uuid.UUID
			LimitMicros int64
			Actor       uuid.UUID
			Scope       uuid.UUID
		}
		if err := db.Raw(`SELECT p.initial_canvas_id AS canvas_id,b.limit_micros,p.owner_id AS actor,p.owner_scope_id AS scope FROM workspace.project p JOIN billing.budget b ON b.project_id=p.id WHERE p.id=?`, created.ProjectID).Scan(&facts).Error; err != nil || facts.CanvasID != created.ID || facts.LimitMicros != 0 || facts.Actor != actor.ID || facts.Scope != actor.ScopeID {
			t.Fatal("workspace creation invariants lost", facts, err)
		}
		var executions int64
		if err := db.Raw(`SELECT count(*) FROM operation.operation WHERE project_id=?`, created.ProjectID).Scan(&executions).Error; err != nil || executions != 0 {
			t.Fatal("template ran generation", executions, err)
		}
		replayed, err := creator.Create(t.Context(), actor, key, input)
		if err != nil || !reflect.DeepEqual(created, replayed) {
			t.Fatal("same creation key duplicated project", created, replayed, err)
		}
		if err := db.Exec(`UPDATE infra.idempotency_record SET expires_at=now()-interval '1 hour' WHERE actor_id=? AND idem_key=?`, actor.ID, key.String()).Error; err != nil {
			t.Fatal(err)
		}
		replayed, err = creator.Create(t.Context(), actor, key, input)
		if err != nil || !reflect.DeepEqual(created, replayed) {
			t.Fatal("creation cache expiry lost durable object", replayed, err)
		}
		changed := input
		changed.Name = "改请求"
		if _, err := creator.Create(t.Context(), actor, key, changed); !errors.Is(err, application.ErrIdempotencyConflict) {
			t.Fatal("expired key accepted different creation", err)
		}
		changedKind := application.AssetCreationInput{Kind: "short_drama", Name: created.Name}
		if created.Kind == "short_drama" {
			changedKind.Kind = "canvas"
		}
		if _, err := creator.Create(t.Context(), actor, key, changedKind); !errors.Is(err, application.ErrIdempotencyConflict) {
			t.Fatal("expired key changed classification", err)
		}
		changedTemplate := input
		changedTemplate.Name = created.Name
		switch {
		case changedTemplate.TemplateKey != "":
			changedTemplate.TemplateKey = ""
		case created.Kind == "short_drama":
			changedTemplate.TemplateKey = "storyboard-three-shots-v1"
		default:
			changedTemplate.TemplateKey = "text-to-image-v1"
		}
		if _, err := creator.Create(t.Context(), actor, key, changedTemplate); !errors.Is(err, application.ErrIdempotencyConflict) {
			t.Fatal("expired key changed template", err)
		}
		if input.TemplateKey != "" {
			runtime := yjsRuntime(t)
			state, err := application.NewYjsService(canvasStore(db), runtime).Initialize(t.Context(), actor, created.ID, 1)
			if err != nil || len(state.State) == 0 || len(state.Document.Nodes) != want {
				t.Fatal("template cannot open current editor", state, err)
			}
		}
	}
}

type failAssetCreation struct {
	application.AssetCreationAccess
	err error
}

func (a failAssetCreation) CreateAssetProject(ctx context.Context, actor identity.Principal, key uuid.UUID, name string) (application.AssetProject, error) {
	project, err := a.AssetCreationAccess.CreateAssetProject(ctx, actor, key, name)
	if err != nil {
		return project, err
	}
	return project, a.err
}
func TestCanvasAssetCreationRollsBackProjectBudgetAuditAndReceipts(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, _ := seedCanvasActor(t, db)
	var before int64
	if err := db.Raw(`SELECT count(*) FROM workspace.project WHERE owner_id=?`, actor.ID).Scan(&before).Error; err != nil {
		t.Fatal(err)
	}
	failed := errors.New("owned fixture post-creation failure")
	creator := application.NewAssetCreator(pgcanvas.NewAssetCreationStore(db, func(tx *gorm.DB) application.AssetCreationAccess {
		return failAssetCreation{AssetCreationAccess: canvaswire.AssetCreationAuthority(tx), err: failed}
	}))
	key := uuid.New()
	if _, err := creator.Create(t.Context(), actor, key, application.AssetCreationInput{TemplateKey: "text-to-image-v1"}); !errors.Is(err, failed) {
		t.Fatal(err)
	}
	var after int64
	if err := db.Raw(`SELECT count(*) FROM workspace.project WHERE owner_id=?`, actor.ID).Scan(&after).Error; err != nil || after != before {
		t.Fatal("failed seed retained project", before, after, err)
	}
	for _, query := range []string{
		`SELECT count(*) FROM workspace.project_creation_receipt WHERE actor_id=?`,
		`SELECT count(*) FROM canvas.asset_creation_receipt WHERE actor_id=?`,
		`SELECT count(*) FROM infra.idempotency_record WHERE actor_id=?`,
		`SELECT count(*) FROM infra.outbox WHERE payload#>>'{actor,id}'=?`,
	} {
		if err := db.Raw(query, actor.ID.String()).Scan(&after).Error; err != nil || after != 0 {
			t.Fatal("failed creation retained audit/receipt", query, after, err)
		}
	}
	if err := db.Raw(`SELECT count(*) FROM billing.budget b LEFT JOIN workspace.project p ON p.id=b.project_id WHERE p.id IS NULL`).Scan(&after).Error; err != nil || after != 0 {
		t.Fatal("orphan budget retained", after, err)
	}
}

func TestCanvasAssetCreationConcurrentRetryAndRevokedSession(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, _ := seedCanvasActor(t, db)
	creator := application.NewAssetCreator(pgcanvas.NewAssetCreationStore(db, canvaswire.AssetCreationAuthority))
	key := uuid.New()
	input := application.AssetCreationInput{Kind: "short_drama", TemplateKey: "storyboard-three-shots-v1"}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	type result struct {
		created application.CreatedAssetCanvas
		err     error
	}
	done := make(chan result, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Go(func() { created, err := creator.Create(ctx, actor, key, input); done <- result{created, err} })
	}
	workers.Wait()
	close(done)
	var first application.CreatedAssetCanvas
	for result := range done {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if first.ID == uuid.Nil {
			first = result.created
		} else if !reflect.DeepEqual(first, result.created) {
			t.Fatal("concurrent retry created a second object", first, result.created)
		}
	}
	if err := db.Exec(`UPDATE identity.user_session SET revoked_at=now() WHERE id=?`, actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := creator.Create(t.Context(), actor, key, input); err == nil {
		t.Fatal("revoked session replayed private creation")
	}
}

func TestCanvasAssetCreationReceiptFailureRollsBackSeedGraph(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, _ := seedCanvasActor(t, db)
	ddl := fmt.Sprintf(`CREATE FUNCTION canvas.asset_creation_receipt_fixture_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.actor_id='%s'::uuid THEN RAISE EXCEPTION 'owned fixture receipt failure'; END IF; RETURN NEW; END $$`, actor.ID.String())
	if err := db.Exec(ddl).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER asset_creation_receipt_fixture_failure BEFORE INSERT ON canvas.asset_creation_receipt FOR EACH ROW EXECUTE FUNCTION canvas.asset_creation_receipt_fixture_failure()`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Exec(`DROP TRIGGER asset_creation_receipt_fixture_failure ON canvas.asset_creation_receipt`).Error; err != nil {
			t.Error(err)
		}
		if err := db.Exec(`DROP FUNCTION canvas.asset_creation_receipt_fixture_failure()`).Error; err != nil {
			t.Error(err)
		}
	})
	creator := application.NewAssetCreator(pgcanvas.NewAssetCreationStore(db, canvaswire.AssetCreationAuthority))
	if _, err := creator.Create(t.Context(), actor, uuid.New(), application.AssetCreationInput{TemplateKey: "text-to-image-v1"}); err == nil {
		t.Fatal("receipt failure claimed successful template creation")
	}
	var count int64
	if err := db.Raw(`SELECT count(*) FROM workspace.project WHERE owner_id=?`, actor.ID).Scan(&count).Error; err != nil || count != 1 {
		t.Fatal("template receipt failure retained new project", count, err)
	}
	for _, query := range []string{`SELECT count(*) FROM canvas.document d JOIN workspace.project p ON p.id=d.project_id WHERE p.owner_id=?`, `SELECT count(*) FROM billing.budget b JOIN workspace.project p ON p.id=b.project_id WHERE p.owner_id=?`, `SELECT count(*) FROM workspace.project_creation_receipt WHERE actor_id=?`, `SELECT count(*) FROM canvas.asset_creation_receipt WHERE actor_id=?`, `SELECT count(*) FROM infra.idempotency_record WHERE actor_id=?`, `SELECT count(*) FROM infra.outbox WHERE payload#>>'{actor,id}'=?`} {
		if err := db.Raw(query, actor.ID.String()).Scan(&count).Error; err != nil || count != 0 {
			t.Fatal("failed graph receipt retained project/graph/budget/audit/receipt", query, count, err)
		}
	}
}
