package workspace_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	workspacewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/workspace"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/workspace/domain"
)

type lockedRetentionContent struct {
	run func(context.Context, uuid.UUID) error
}

func (c lockedRetentionContent) ClearProjectContent(ctx context.Context, id uuid.UUID) error {
	return c.run(ctx, id)
}

func seedLockedRetention(t *testing.T, db *gorm.DB, id uuid.UUID) {
	t.Helper()
	lifecycleFixtureSQL(t, db, `UPDATE workspace.project SET is_delete=true,revision=2,delete_time='2000-01-01',purge_after='2000-01-31' WHERE id=?`, id)
	lifecycleFixtureSQL(t, db, `INSERT INTO workspace.project_retention_job(project_id,org_id,owner_scope_id,deleted_revision,purge_after) SELECT id,org_id,owner_scope_id,revision,purge_after FROM workspace.project WHERE id=?`, id)
}

func TestProjectRetentionWaitsForProjectLockAndSkipsNewVersion(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	_, id := lifecycleActorProject(ctx, t, db)
	seedLockedRetention(t, db, id)
	restore := db.WithContext(ctx).Begin()
	if restore.Error != nil {
		t.Fatal(restore.Error)
	}
	defer restore.Rollback()
	blocker := projectTransactionPID(t, restore)
	// Model an already-authorized restoration/newer deletion committing first.
	// The public restore endpoint remains subject to its separate deadline tests.
	lifecycleFixtureSQL(t, restore, `UPDATE workspace.project SET revision=3,is_delete=false,delete_time=NULL,purge_after=NULL WHERE id=?`, id)
	pidc := make(chan int, 1)
	done := make(chan error, 1)
	calls := 0
	go func() {
		done <- db.WithContext(ctx).Connection(func(conn *gorm.DB) error {
			var pid int
			if err := conn.Raw(`SELECT pg_backend_pid()`).Scan(&pid).Error; err != nil {
				return err
			}
			pidc <- pid
			store := pgworkspace.NewProjectRetentionStore(conn, func(*gorm.DB) pgworkspace.ProjectRetentionContent {
				return lockedRetentionContent{run: func(context.Context, uuid.UUID) error { calls++; return nil }}
			})
			worked, err := store.ProcessNext(ctx)
			if err == nil && !worked {
				return errors.New("job was not processed")
			}
			return err
		})
	}()
	select {
	case pid := <-pidc:
		waitProjectLock(ctx, t, db, blocker, pid)
	case err := <-done:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not start")
	}
	if err := restore.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not finish")
	}
	var state string
	if err := db.Raw(`SELECT status FROM workspace.project_retention_job WHERE project_id=? AND deleted_revision=2`, id).Scan(&state).Error; err != nil || state != "skipped" || calls != 0 {
		t.Fatalf("state=%s calls=%d err=%v", state, calls, err)
	}
}

func TestProjectRetentionLockPreventsRestoreDuringContentCleanup(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor, id := lifecycleActorProject(ctx, t, db)
	seedLockedRetention(t, db, id)
	held := make(chan int, 1)
	release := make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { close(release) }) }
	defer finish()
	workDone := make(chan error, 1)
	go func() {
		workDone <- db.WithContext(ctx).Connection(func(conn *gorm.DB) error {
			store := pgworkspace.NewProjectRetentionStore(conn, func(tx *gorm.DB) pgworkspace.ProjectRetentionContent {
				return lockedRetentionContent{run: func(ctx context.Context, target uuid.UUID) error {
					if target != id {
						return errors.New("unexpected project")
					}
					var pid int
					if err := tx.Raw(`SELECT pg_backend_pid()`).Scan(&pid).Error; err != nil {
						return err
					}
					held <- pid
					select {
					case <-release:
					case <-ctx.Done():
						return ctx.Err()
					}
					return tx.Exec(`UPDATE workspace.project SET description='retention lock verified' WHERE id=?`, id).Error
				}}
			})
			_, err := store.ProcessNext(ctx)
			return err
		})
	}()
	var workerPID int
	select {
	case workerPID = <-held:
	case err := <-workDone:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("cleanup did not start")
	}
	restorePID := make(chan int, 1)
	restoreDone := make(chan error, 1)
	go func() {
		restoreDone <- db.WithContext(ctx).Connection(func(conn *gorm.DB) error {
			var pid int
			if err := conn.Raw(`SELECT pg_backend_pid()`).Scan(&pid).Error; err != nil {
				return err
			}
			restorePID <- pid
			_, err := workspaceapp.NewProjectLifecycle(workspacewire.ProvideWorkspaceLifecycleStore(conn), time.Now).Change(ctx, actor, projectChange(id, 2, "restore"))
			return err
		})
	}()
	select {
	case pid := <-restorePID:
		waitProjectLock(ctx, t, db, workerPID, pid)
	case err := <-restoreDone:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("restore did not start")
	}
	finish()
	select {
	case err := <-workDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cleanup did not finish")
	}
	select {
	case err := <-restoreDone:
		if !errors.Is(err, domain.ErrProjectRestoreExpired) {
			t.Fatalf("expired restore: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("restore did not finish")
	}
	var completed bool
	if err := db.Raw(`SELECT EXISTS(SELECT 1 FROM workspace.project_retention_job j JOIN workspace.project p ON p.id=j.project_id WHERE j.project_id=? AND j.status='completed' AND p.is_delete AND p.revision=2)`, id).Scan(&completed).Error; err != nil || !completed {
		t.Fatal("cleanup changed restore eligibility", err)
	}
}
