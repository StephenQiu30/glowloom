package canvas_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	pgcanvas "github.com/StephenQiu30/lanverse/backend/internal/canvas/adapter/postgres"
	"github.com/StephenQiu30/lanverse/backend/internal/canvas/application"
	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
)

func TestToolConfigurationsSurviveDatabaseReloadAndIdempotentReplay(t *testing.T) {
	database := canvasDB(t, "LV_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	service := application.NewService(pgcanvas.NewStore(database, nil))
	doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "完整工具输入恢复"})
	if err != nil {
		t.Fatal(err)
	}
	batch, director, generation := batchNode(nil), directorNode(), generationNode()
	input := application.CommandsInput{ExpectedRevision: doc.Revision, Commands: []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{batch, director, generation}}}}
	key := uuid.NewString()
	first, err := service.Execute(t.Context(), actor, doc.ID, key, input)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := service.Execute(t.Context(), actor, doc.ID, key, input)
	if err != nil || !reflect.DeepEqual(first, replayed) {
		t.Fatalf("tool request not replayed exactly: %v", err)
	}
	loaded, err := service.Get(t.Context(), actor, doc.ID)
	slices.SortFunc(first.Nodes, func(a, b domain.Node) int { return strings.Compare(a.ID.String(), b.ID.String()) })
	if err != nil || !reflect.DeepEqual(first.Document, loaded) {
		t.Fatalf("persisted scene inputs changed on reload: %v", err)
	}
	var count int64
	if err := database.Raw(`SELECT count(*) FROM operation.operation WHERE project_id=?`, project).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("saving inputs created execution: count %d err %v", count, err)
	}
}
