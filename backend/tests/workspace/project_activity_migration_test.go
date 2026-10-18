package workspace_test

import (
	"os"
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
	var grants struct { Read, Write, Remove bool }
	if err := db.Raw(`SELECT has_table_privilege('lanverse_app','workspace.project_activity','SELECT') AS read,has_table_privilege('lanverse_app','workspace.project_activity','INSERT,UPDATE') AS write,has_table_privilege('lanverse_app','workspace.project_activity','DELETE') AS remove`).Scan(&grants).Error; err != nil || !grants.Read || !grants.Write || grants.Remove { t.Fatalf("activity role permissions: %+v %v",grants,err) }
}

func TestProjectActivityMigrationPreservesLegacyProjects(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	id := insertWorkspaceProject(ctx, t, db, actor, "16:9", "realistic", nil)
	sql, err := os.ReadFile("../../db/migrations/017_project_canvas_consistency.sql")
	if err != nil {
		t.Fatalf("activity migration missing: %v", err)
	}
	for range 2 {
		if err := db.Exec(string(sql)).Error; err != nil {
			t.Fatalf("repeat migration: %v", err)
		}
	}
	var row struct { Basis string; SameTime bool; Revision int64 }
	if err := db.Raw(`SELECT a.basis,a.modified_at=p.update_time AS same_time,p.revision FROM workspace.project_activity a JOIN workspace.project p ON p.id=a.project_id WHERE p.id=?`, id).Scan(&row).Error; err != nil || row.Basis != "legacy_metadata" || !row.SameTime || row.Revision != 1 {
		t.Fatalf("legacy project changed or backfill absent: %+v err=%v", row, err)
	}
	if err := db.Exec(`UPDATE workspace.project_activity SET basis='content',modified_at=clock_timestamp() WHERE project_id=?`, id).Error; err != nil { t.Fatal(err) }
	if err := db.Exec(string(sql)).Error; err != nil { t.Fatal(err) }
	var basis string
	if err := db.Raw(`SELECT basis FROM workspace.project_activity WHERE project_id=?`, id).Scan(&basis).Error; err != nil || basis != "content" { t.Fatalf("migration overwrote new activity: %q %v", basis, err) }
}
