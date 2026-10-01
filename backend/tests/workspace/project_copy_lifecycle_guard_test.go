package workspace_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func TestProjectLifecyclePreparationBlocksCopyDuringRecovery(t *testing.T) {
	actor := projectCommandActor(identitydomain.RoleProducer)
	now := time.Now().UTC()
	for _, action := range []string{"unarchive", "restore"} {
		t.Run(action, func(t *testing.T) {
			before := updateProjectFixture(actor)
			var err error
			if action == "unarchive" {
				err = before.Archive(now, false)
			} else {
				err = before.Delete(now, false)
			}
			if err != nil {
				t.Fatal(err)
			}
			input := projectChange(before.ID, before.Revision, action)
			_, events, err := workspaceapp.PrepareProjectChange(actor, input, before, now, true)
			if !errors.Is(err, domain.ErrProjectHasInflightOperations) || len(events) != 0 {
				t.Fatalf("recovery with unfinished copying work: events=%d err=%v", len(events), err)
			}
		})
	}
}

func TestProjectLifecycleStoreRequiresCopyEvidenceBeforeRecovery(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	for _, action := range []string{"unarchive", "restore"} {
		t.Run(action, func(t *testing.T) {
			id := uuid.MustParse(insertWorkspaceProject(ctx, t, db, actor.OrgID.String(), "16:9", "realistic", nil))
			idle := workspaceapp.NewProjectLifecycle(lifecycleStore(db, fixedProjectWork{}), time.Now)
			initial := "archive"
			if action == "restore" {
				initial = "delete"
			}
			saved, err := idle.Change(ctx, actor, projectChange(id, 1, initial))
			if err != nil {
				t.Fatal(err)
			}
			for _, test := range []struct {
				name string
				work fixedProjectWork
				want error
			}{
				{"copy active", fixedProjectWork{blocked: true}, domain.ErrProjectHasInflightOperations},
				{"copy unreadable", fixedProjectWork{err: workspaceapp.ErrProjectDependencyUnavailable}, workspaceapp.ErrProjectDependencyUnavailable},
			} {
				t.Run(test.name, func(t *testing.T) {
					input := projectChange(id, saved.Project.Revision, action)
					service := workspaceapp.NewProjectLifecycle(lifecycleStore(db, test.work), time.Now)
					if _, err := service.Change(ctx, actor, input); !errors.Is(err, test.want) {
						t.Fatalf("recovery without idle copy evidence: %v", err)
					}
					var row struct {
						Revision int64
						Status   string
						IsDelete bool
					}
					if err := db.Raw(`SELECT revision,status,is_delete FROM workspace.project WHERE id=?`, id).Scan(&row).Error; err != nil {
						t.Fatal(err)
					}
					if row.Revision != saved.Project.Revision || row.Status != saved.Project.Status || row.IsDelete != saved.Project.IsDelete {
						t.Fatal("rejected recovery mutated project")
					}
					var receiptCount int64
					if err := db.Raw(`SELECT count(*) FROM infra.idempotency_record WHERE actor_id=? AND idem_key=?`, actor.ID, input.IdempotencyKey.String()).Scan(&receiptCount).Error; err != nil || receiptCount != 0 {
						t.Fatalf("rejected recovery recorded success: %d %v", receiptCount, err)
					}
				})
			}
		})
	}
}
