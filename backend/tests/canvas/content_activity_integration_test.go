package canvas_test

import (
	"testing"
	"time"

	canvasfixture "github.com/StephenQiu30/glowloom-server/backend/tests/canvasfixture"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
)

func TestContentActivityTracksPrimaryContentWithoutProjectCAS(t *testing.T) {
	db := canvasDB(t, "GLOWLOOM_TEST_CANVAS_DB_DSN")
	actor, project := seedCanvasActor(t, db)
	service := canvasfixture.NewService(canvasStore(db))
	doc, err := service.Create(t.Context(), actor, project, uuid.NewString(), application.CreateInput{Name: "主画布"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE workspace.project SET initial_canvas_id=? WHERE id=?`, doc.ID, project).Error; err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	if err := db.Exec(`INSERT INTO workspace.project_activity(project_id,modified_at,basis) VALUES(?,?,'created')`, project, old).Error; err != nil {
		t.Fatal(err)
	}
	_, err = service.Edit(t.Context(), actor, doc.ID, uuid.NewString(), canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: 1, Operations: canvasfixture.Concat([]domain.Operation{{Type: "set_viewport"}})})
	if !operationRejected(err, "invalid_operation") {
		t.Fatal("viewport entered shared graph", err)
	}
	var activity struct {
		ModifiedAt time.Time
		Basis      string
		Revision   int64
	}
	read := func() {
		t.Helper()
		if err := db.Raw(`SELECT a.modified_at,a.basis,p.revision FROM workspace.project_activity a JOIN workspace.project p ON p.id=a.project_id WHERE p.id=?`, project).Scan(&activity).Error; err != nil {
			t.Fatal(err)
		}
	}
	read()
	if !activity.ModifiedAt.Equal(old) || activity.Basis != "created" {
		t.Fatal("viewport changed recent content", activity)
	}
	text := "实际保存的中文"
	input := canvasfixture.EditInput{ClientID: canvasfixture.ClientID, OperationVersion: 1, BaseRevision: doc.Revision, Operations: canvasfixture.Concat(canvasfixture.Nodes([]domain.Node{{ID: uuid.New(), Title: "文字", NodeType: "text", NodeAction: "resource", Config: domain.NodeConfig{Text: &text}}}))}
	key := uuid.NewString()
	if _, err := service.Edit(t.Context(), actor, doc.ID, key, input); err != nil {
		t.Fatal(err)
	}
	read()
	if !activity.ModifiedAt.After(old) || activity.Basis != "content" || activity.Revision != 1 {
		t.Fatalf("content activity/CAS: %+v", activity)
	}
	timestamp := activity.ModifiedAt
	if _, err := service.Edit(t.Context(), actor, doc.ID, key, input); err != nil {
		t.Fatal(err)
	}
	read()
	if !timestamp.Equal(activity.ModifiedAt) {
		t.Fatal("replay advanced activity")
	}
}
