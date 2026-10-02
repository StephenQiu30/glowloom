package workspace_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

func TestProjectFolderPGWriteFaultsCannotCommitPartialRecycle(t *testing.T) {
	ctx, db, owner := folderTestDB(t)
	actor := insertWorkspaceActor(ctx, t, owner, insertWorkspaceOrganization(ctx, t, owner))
	project := uuid.MustParse(insertWorkspaceProject(ctx, t, owner, actor.OrgID.String(), "16:9", "realistic", nil))
	service := workspaceapp.NewProjectFolders(folderStore(db), time.Now)
	f := createTestFolder(ctx, t, service, actor, "原子提交故障")
	f = *moveTestProject(ctx, t, service, actor, project, f, 0).Folder
	for _, table := range []string{"infra.outbox", "workspace.project_folder_command"} {
		t.Run(table, func(t *testing.T) {
			in := folderCommand("recycle")
			in.FolderID = f.ID
			in.ExpectedRevision = f.Revision
			rollback := errors.New("fixture rollback")
			err := owner.Transaction(func(tx *gorm.DB) error {
				if err := tx.Exec(`CREATE FUNCTION pg_temp.reject_folder_write()RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'synthetic folder write fault';END$$`).Error; err != nil {
					return err
				}
				if err := tx.Exec("CREATE TRIGGER folder_test_fault BEFORE INSERT ON " + table + " FOR EACH ROW EXECUTE FUNCTION pg_temp.reject_folder_write()").Error; err != nil {
					return err
				}
				if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
					return err
				}
				if _, err := workspaceapp.NewProjectFolders(folderStore(tx), time.Now).Change(ctx, actor, in); err == nil {
					t.Error("fault reported success")
				}
				return rollback
			})
			if !errors.Is(err, rollback) {
				t.Fatal(err)
			}
			var counts struct{ Deleted, Receipts, Members int64 }
			if err := db.Raw(`SELECT(SELECT count(*) FROM workspace.project WHERE id=? AND is_delete)AS deleted,(SELECT count(*) FROM workspace.project_folder_command WHERE actor_id=? AND idem_key=?)AS receipts,(SELECT count(*) FROM workspace.project_folder_placement WHERE actor_id=? AND folder_id=?)AS members`, project, actor.ID, in.IdempotencyKey, actor.ID, f.ID).Scan(&counts).Error; err != nil || counts.Deleted != 0 || counts.Receipts != 0 || counts.Members != 1 {
				t.Fatalf("fault leaked %+v %v", counts, err)
			}
		})
	}
	in := folderCommand("recycle")
	in.FolderID = f.ID
	in.ExpectedRevision = f.Revision
	missing := workspaceapp.NewProjectFolders(workspacepg.NewFolderStore(db, nil, nil), time.Now)
	if _, err := missing.Change(ctx, actor, in); !errors.Is(err, workspaceapp.ErrProjectDependencyUnavailable) {
		t.Fatalf("missing evidence admitted %v", err)
	}
	degraded := workspaceapp.NewProjectFolders(workspacepg.NewFolderStore(db, func(*gorm.DB) workspaceapp.ProjectWorkGuard {
		return fixedProjectWork{err: workspaceapp.ErrProjectDependencyUnavailable}
	}, nil), time.Now)
	if _, err := degraded.Change(ctx, actor, in); !errors.Is(err, workspaceapp.ErrProjectDependencyUnavailable) {
		t.Fatalf("degraded evidence admitted %v", err)
	}
}

func TestProjectFolderPGSchemaEnforcesNarrowGrants(t *testing.T) {
	ctx, _, owner := folderTestDB(t)
	actor := insertWorkspaceActor(ctx, t, owner, insertWorkspaceOrganization(ctx, t, owner))
	rollback := errors.New("DDL verification rollback")
	err := owner.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		var grants struct{ Insert, Rewrite, Remove bool }
		if err := tx.Raw(`SELECT has_table_privilege(current_user,'workspace.project_folder_command','INSERT')AS insert,has_table_privilege(current_user,'workspace.project_folder_command','UPDATE')AS rewrite,has_table_privilege(current_user,'workspace.project_folder_command','DELETE')AS remove`).Scan(&grants).Error; err != nil {
			return err
		}
		if !grants.Insert || grants.Rewrite || grants.Remove {
			t.Fatalf("receipt grants %+v", grants)
		}
		in := folderCommand("create")
		name := "Schema 权限验证"
		in.Name = &name
		if _, err := workspaceapp.NewProjectFolders(folderStore(tx), time.Now).Change(ctx, actor, in); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
}
