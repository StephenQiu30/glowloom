package workspace_test

import (
	"testing"

	"github.com/google/uuid"
)

func TestProjectActivityRejectsOrphansAndInvalidBasis(t *testing.T) {
	_, db := workspaceMigrationDB(t)
	var exists bool
	if err := db.Raw(`SELECT to_regclass('workspace.project_activity') IS NOT NULL`).Scan(&exists).Error; err != nil || !exists {
		t.Fatalf("project activity read model is missing: exists=%v err=%v", exists, err)
	}
	if err := db.Exec(`INSERT INTO workspace.project_activity(project_id,modified_at,basis) VALUES(?,now(),'content')`, uuid.New()).Error; err == nil {
		t.Fatal("orphan project activity accepted")
	}
	ctx, _ := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	id := insertWorkspaceProject(ctx, t, db, actor, "16:9", "realistic", nil)
	if err := db.Exec(`INSERT INTO workspace.project_activity(project_id,modified_at,basis) VALUES(?,now(),'viewed')`, id).Error; err == nil {
		t.Fatal("non-content basis accepted")
	}
	var grants struct{ Read, Write, Remove bool }
	if err := db.Raw(`SELECT has_table_privilege('lanverse_app','workspace.project_activity','SELECT') AS read,has_table_privilege('lanverse_app','workspace.project_activity','INSERT,UPDATE') AS write,has_table_privilege('lanverse_app','workspace.project_activity','DELETE') AS remove`).Scan(&grants).Error; err != nil || !grants.Read || !grants.Write || grants.Remove {
		t.Fatalf("activity role permissions: %+v %v", grants, err)
	}
}
