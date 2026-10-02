package media_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	copyobjects "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/objectstorage"
	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	pgworkspace "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	workspacedomain "github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func transferLifecycleFixture(t *testing.T, sourceProject bool) (*gorm.DB, identityapp.Principal, uuid.UUID, *pgmedia.TransferStore, domain.TransferJob) {
	t.Helper()
	db := libraryRuntimeDB(t)
	actor, project := mediaStoreProject(t, db)
	personal := domain.LibraryScope{Kind: domain.LibraryPersonal}
	projectScope := domain.LibraryScope{Kind: domain.LibraryProject, ProjectID: &project}
	in := mediaapp.TransferInput{Source: personal, Target: projectScope, ExpectedSourceRevision: 1, ExpectedProjectRevision: 1, Key: uuid.New()}
	if sourceProject {
		in.Source, in.Target, in.ExpectedProjectRevision = projectScope, personal, 2
	}
	text := "真实目录文字转移"
	created, err := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now).ApplyLibraryCommand(t.Context(), actor, mediaapp.LibraryCommand{Scope: in.Source, Key: uuid.New(), Action: "create_text", Metadata: &mediaapp.LibraryMetadata{PlainText: &text, Title: "生命周期门禁合成文字", Category: "other", Tags: []string{}}})
	if err != nil {
		t.Fatal(err)
	}
	in.Items = []mediaapp.LibraryItemRevision{{ID: created.Items[0].ID, Revision: 1}}
	repo := pgmedia.NewTransferStore(db, libraryTestAccess, transferTestGuards, time.Now)
	job, err := repo.CreateTransfer(t.Context(), actor, in)
	if err != nil {
		t.Fatal(err)
	}
	return db, actor, project, repo, job
}

func transferLifecycleService(db *gorm.DB) *workspaceapp.ProjectLifecycle {
	store := pgworkspace.NewStoreWithProjectWorkGuard(db, func(tx *gorm.DB) workspaceapp.ProjectWorkGuard {
		return pgmedia.NewTransferStore(tx, libraryTestAccess, transferTestGuards, time.Now)
	})
	return workspaceapp.NewProjectLifecycle(store, time.Now)
}

func transferLifecycleChange(project uuid.UUID, revision int64, action string) workspaceapp.ProjectChangeInput {
	return workspaceapp.ProjectChangeInput{Action: action, IdempotencyKey: uuid.New(), Patch: workspaceapp.UpdateProjectInput{ProjectID: project, ExpectedRevision: revision, RequestID: uuid.NewString()}}
}

func TestMediaTransferWorkspaceLifecycleActualSourceAndTargetBlockUnsettledWork(t *testing.T) {
	for _, sourceProject := range []bool{false, true} {
		name := "target project"
		if sourceProject {
			name = "source project"
		}
		t.Run(name, func(t *testing.T) {
			for _, state := range []string{"queued", "running", "needs_reconciliation", "cancel_unended", "terminal_unconfirmed"} {
				t.Run(state, func(t *testing.T) {
					db, actor, project, repo, job := transferLifecycleFixture(t, sourceProject)
					if state != "queued" {
						work := transferExecution(job)
						lease, err := repo.ClaimTransfer(t.Context(), work)
						if err != nil {
							t.Fatal(err)
						}
						switch state {
						case "needs_reconciliation":
							if err := repo.FailTransferItem(t.Context(), lease, 0, "object_write_unknown", true); err != nil {
								t.Fatal(err)
							}
							if err := repo.EndTransferPhysical(t.Context(), lease); err != nil {
								t.Fatal(err)
							}
							if _, err := repo.FinishTransfer(t.Context(), lease); err != nil {
								t.Fatal(err)
							}
						case "cancel_unended":
							current, err := repo.GetTransfer(t.Context(), actor, job.ID)
							if err != nil {
								t.Fatal(err)
							}
							if _, err := repo.ControlTransfer(t.Context(), actor, job.ID, uuid.New(), current.Revision, "cancel"); err != nil {
								t.Fatal(err)
							}
						case "terminal_unconfirmed":
							// An adversarial terminal label is not cessation evidence.
							// Working columns are changed only in this synthetic fixture.
							if err := db.Exec(`UPDATE media.transfer_job SET status='cancelled',stage='completed',execution_unconfirmed=true WHERE id=?`, job.ID).Error; err != nil {
								t.Fatal(err)
							}
						}
					}
					service := transferLifecycleService(db)
					before, err := service.Get(t.Context(), actor, project)
					if err != nil {
						t.Fatal(err)
					}
					for _, action := range []string{"archive", "delete"} {
						input := transferLifecycleChange(project, before.Project.Revision, action)
						if _, err := service.Change(t.Context(), actor, input); !errors.Is(err, workspacedomain.ErrProjectHasInflightOperations) {
							t.Fatal("actual owning transfer was omitted from project lifecycle guard", action, state, err)
						}
						var receipts int64
						if err := db.Raw(`SELECT count(*) FROM workspace.project_change_command WHERE actor_id=? AND idem_key=?`, actor.ID, input.IdempotencyKey).Scan(&receipts).Error; err != nil || receipts != 0 {
							t.Fatal("blocked lifecycle recorded a successful permanent command", receipts, err)
						}
					}
					after, err := service.Get(t.Context(), actor, project)
					if err != nil || after.Project.Status != "active" || after.Project.IsDelete || after.Project.Revision != before.Project.Revision {
						t.Fatal("blocked lifecycle mutated the owning project", after, err)
					}
				})
			}
		})
	}
}

func TestMediaTransferWorkspaceLifecycleReleasesOnlyRealCompletedOrUnstartedCancelledWork(t *testing.T) {
	for _, sourceProject := range []bool{false, true} {
		for _, cancel := range []bool{false, true} {
			name := "target completed"
			if sourceProject {
				name = "source completed"
			}
			if cancel {
				name += " unstarted cancel"
			}
			t.Run(name, func(t *testing.T) {
				db, actor, project, repo, job := transferLifecycleFixture(t, sourceProject)
				if cancel {
					result, err := repo.ControlTransfer(t.Context(), actor, job.ID, uuid.New(), job.Revision, "cancel")
					if err != nil || result.Status != "cancelled" {
						t.Fatal("unstarted cancellation did not settle", result, err)
					}
				} else {
					result, err := mediaapp.NewTransferWorker(repo, copyobjects.NewProjectCopyObjects(glbTestObjects(t)), t.TempDir()).Execute(t.Context(), transferExecution(job))
					if err != nil || result.Status != "succeeded" {
						t.Fatal("actual metadata transfer did not complete", result, err)
					}
				}
				service := transferLifecycleService(db)
				before, err := service.Get(t.Context(), actor, project)
				if err != nil {
					t.Fatal(err)
				}
				archived, err := service.Change(t.Context(), actor, transferLifecycleChange(project, before.Project.Revision, "archive"))
				if err != nil || archived.Project.Status != "archived" {
					t.Fatal("settled transfer failed to release archive guard", archived, err)
				}
				deleted, err := service.Change(t.Context(), actor, transferLifecycleChange(project, archived.Project.Revision, "delete"))
				if err != nil || !deleted.Project.IsDelete {
					t.Fatal("settled transfer failed to release recycle guard", deleted, err)
				}
			})
		}
	}
}
