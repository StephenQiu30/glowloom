package workspace_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func TestBlankProjectCreatesMatchingMainNameAndActivity(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	input := workspaceapp.CreateProjectInput{Name: "未命名画布", AspectRatio: "16:9", StyleType: "realistic", RequestID: uuid.NewString()}
	key := uuid.New()
	command := workspaceapp.NewCreateProjectCommand(workspaceStore(db), time.Now)
	created, err := command.ExecuteIdempotent(ctx, actor, key, input)
	if err != nil {
		t.Fatal(err)
	}
	var row struct {
		Name, Basis                                string
		Revision                                   int64
		SameTime                                   bool
		LimitMicros, ReservedMicros, SettledMicros int64
	}
	if err := db.Raw(`SELECT d.name,d.revision,a.basis,a.modified_at=p.create_time AS same_time,b.limit_micros,b.reserved_micros,b.settled_micros FROM workspace.project p JOIN canvas.document d ON d.id=p.initial_canvas_id JOIN workspace.project_activity a ON a.project_id=p.id JOIN billing.budget b ON b.project_id=p.id WHERE p.id=?`, created.ID).Scan(&row).Error; err != nil || row.Name != "未命名画布" || row.Revision != 1 || row.Basis != "created" || !row.SameTime || row.LimitMicros != 0 || row.ReservedMicros != 0 || row.SettledMicros != 0 {
		t.Fatalf("blank creation mismatch: %+v err=%v", row, err)
	}
	if err := db.Exec(`UPDATE workspace.project SET name='现在的名称',revision=2 WHERE id=?`, created.ID).Error; err != nil {
		t.Fatal(err)
	}
	replayed, err := command.ExecuteIdempotent(ctx, actor, key, input)
	if err != nil || replayed.ID != created.ID || replayed.CanvasID != created.CanvasID || replayed.Name != "未命名画布" || replayed.Revision != 1 {
		t.Fatalf("original receipt changed: %+v err=%v", replayed, err)
	}
	var count int64
	if err := db.Raw(`SELECT count(*) FROM workspace.project WHERE owner_id=?`, actor.ID).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("duplicate project: %d %v", count, err)
	}
}
