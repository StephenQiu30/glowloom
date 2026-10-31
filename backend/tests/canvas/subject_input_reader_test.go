package canvas_test

import (
	"errors"
	"testing"

	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
	"github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestSubjectInputReaderRechecksCurrentSourceAndRelation(t *testing.T) {
	database := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	service := canvasfixture.NewService(canvasStore(database))
	doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "主体来源读回"})
	if err != nil {
		t.Fatal(err)
	}
	fixture := boundSubjectFixture(t)
	asset := seedCanvasMedia(t, database, project, "image")
	fixture.Nodes[0].RefID = &asset
	fixture.Nodes[1].Config.Subject.Local.Primary.AssetID = asset
	saved, err := service.Edit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: doc.Revision, Operations: canvasfixture.Concat(canvasfixture.Nodes(fixture.Nodes), canvasfixture.Edges(fixture.Edges))})
	if err != nil {
		t.Fatal(err)
	}
	var target domain.Node
	for _, node := range saved.Nodes {
		if node.ID == fixture.Nodes[1].ID {
			target = node
		}
	}
	read := func(projectID uuid.UUID) error {
		return database.Transaction(func(tx *gorm.DB) error {
			actual, readErr := pgcanvas.ReadSubjectInput(t.Context(), tx, projectID, target.ID, target.InputRevision)
			if readErr == nil && (actual.Config.Subject.Local.Primary.SourceNodeID == nil || *actual.Config.Subject.Local.Primary.SourceNodeID != fixture.Nodes[0].ID) {
				t.Fatal("source identity not preserved")
			}
			return readErr
		})
	}
	if err := read(project); err != nil {
		t.Fatal(err)
	}
	if err := read(uuid.New()); !errors.Is(err, prompt.ErrReferenceUnavailable) {
		t.Fatal("foreign project read", err)
	}
	// Fault injection bypasses normal admission to prove the generation read fails closed.
	// The writer setting is scoped to this fixture transaction, never a global bypass.
	fault := func(query string, args ...any) {
		t.Helper()
		err := database.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(`SELECT set_config('glowloom.yjs_document_id',?,true)`, doc.ID.String()).Error; err != nil {
				return err
			}
			return tx.Exec(query, args...).Error
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	replacement := seedCanvasMedia(t, database, project, "image")
	fault(`UPDATE canvas.node SET ref_id=? WHERE id=?`, replacement, fixture.Nodes[0].ID)
	if err := read(project); !errors.Is(err, prompt.ErrReferenceUnavailable) {
		t.Fatal("stale asset accepted", err)
	}
	fault(`UPDATE canvas.node SET ref_id=? WHERE id=?`, asset, fixture.Nodes[0].ID)
	fault(`UPDATE canvas.edge SET is_delete=true WHERE id=?`, fixture.Edges[0].ID)
	if err := read(project); !errors.Is(err, prompt.ErrReferenceUnavailable) {
		t.Fatal("detached relation accepted", err)
	}
}
