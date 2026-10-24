package canvas_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	canvasfixture "github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"

	"github.com/google/uuid"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	pgcanvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func TestToolConfigurationsSurviveDatabaseReloadAndIdempotentReplay(t *testing.T) {
	database := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, database)
	service := canvasfixture.NewService(pgcanvas.NewScopedStore(database, nil, mediawire.NewCanvasProjectAccess))
	doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "完整工具输入恢复"})
	if err != nil {
		t.Fatal(err)
	}
	batch, director, generation := batchNode(nil), directorNode(), generationNode()
	input := canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: doc.Revision, Operations: canvasfixture.Concat(canvasfixture.Nodes([]domain.Node{batch, director, generation}))}
	key := uuid.NewString()
	first, err := service.Edit(t.Context(), actor, doc.ID, key, input)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := service.Edit(t.Context(), actor, doc.ID, key, input)
	if err != nil || !reflect.DeepEqual(first, replayed) {
		t.Fatalf("tool request not replayed exactly: %v", err)
	}
	loaded, err := service.Get(t.Context(), actor, doc.ID)
	slices.SortFunc(first.Nodes, func(a, b domain.Node) int { return strings.Compare(a.ID.String(), b.ID.String()) })
	if err != nil || !reflect.DeepEqual(first, loaded) {
		t.Fatalf("persisted scene inputs changed on reload: %v", err)
	}
	var count int64
	if err := database.Raw(`SELECT count(*) FROM operation.operation WHERE project_id=?`, project).Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("saving inputs created execution: count %d err %v", count, err)
	}
}
