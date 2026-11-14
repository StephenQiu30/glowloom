package media_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
)

func TestUploadProjectCleanupSurvivesRevocationAndArchive(t *testing.T) {
	database := mediaStoreDB(t)
	actor, project := mediaStoreProject(t, database)
	store := mediaTestStore(database)
	if aspect, err := store.AuthorizeUpload(t.Context(), actor, project); err != nil || aspect != "16:9" {
		t.Fatalf("active project lost upload access/aspect: aspect=%q error=%v", aspect, err)
	}
	if err := database.Exec(`UPDATE workspace.project SET status='archived' WHERE id=?`, project).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.AuthorizeUpload(t.Context(), actor, project); !errors.Is(err, mediadomain.ErrMediaStateConflict) {
		t.Fatalf("archived project lost its existing conflict error: %v", err)
	}
	if err := database.Exec(`UPDATE identity."user" SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.AuthorizeUpload(t.Context(), actor, project); err == nil {
		t.Fatal("a revoked actor retained new upload authority")
	}
	if present, err := store.UploadAssetExists(t.Context(), project, uuid.New()); err != nil || present {
		t.Fatalf("known absence after revocation/closure: present=%v error=%v", present, err)
	}
	// A missing project is an unresolved outcome, not proof that an owned object
	// may be removed. Cleanup must remain conservative.
	if _, err := store.UploadAssetExists(t.Context(), uuid.New(), uuid.New()); err == nil {
		t.Fatal("missing project was treated as a resolved rollback")
	}
}

func TestUploadProjectCleanupWaitsOnActualProjectLock(t *testing.T) {
	database := mediaStoreDB(t)
	_, project := mediaStoreProject(t, database)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	locker := database.WithContext(ctx).Begin()
	if locker.Error != nil {
		t.Fatal(locker.Error)
	}
	t.Cleanup(func() { _ = locker.Rollback().Error })
	var lockerPID int
	if err := locker.Raw(`SELECT pg_backend_pid()`).Scan(&lockerPID).Error; err != nil {
		t.Fatal(err)
	}
	if err := pgworkspace.NewProjectContentAccessStore(locker).AwaitUploadProjectCommit(ctx, project); err != nil {
		t.Fatal(err)
	}
	probe := database.WithContext(ctx).Begin()
	if probe.Error != nil {
		t.Fatal(probe.Error)
	}
	var probePID int
	if err := probe.Raw(`SELECT pg_backend_pid()`).Scan(&probePID).Error; err != nil {
		_ = probe.Rollback().Error
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, err := mediaTestStore(probe).UploadAssetExists(ctx, project, uuid.New())
		finished <- err
	}()
	t.Cleanup(func() {
		_ = locker.Rollback().Error
		cancel()
		<-done
		_ = probe.Rollback().Error
	})
	// Different dedicated connections and a principal-free cleanup operation
	// prevent an Identity user lock from masquerading as the project barrier.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var activity struct {
			Blocked bool
			Query   string
		}
		err := database.WithContext(ctx).Raw(`SELECT ? = ANY(pg_blocking_pids(pid)) AS blocked, query FROM pg_stat_activity WHERE pid=?`, lockerPID, probePID).Scan(&activity).Error
		if err != nil {
			t.Fatal(err)
		}
		if activity.Blocked {
			query := strings.ToLower(activity.Query)
			if !strings.Contains(query, "workspace") || !strings.Contains(query, "project") || !strings.Contains(query, "for update") {
				t.Fatalf("cleanup blocked on the wrong resource: %s", activity.Query)
			}
			break
		}
		select {
		case err := <-finished:
			t.Fatalf("cleanup bypassed the pending project commit: %v", err)
		case <-ctx.Done():
			t.Fatal("cleanup did not block on the project's transaction")
		case <-ticker.C:
		}
	}
	if err := locker.Exec(`UPDATE workspace.project SET status='archived' WHERE id=?`, project).Error; err != nil {
		t.Fatal(err)
	}
	if err := locker.Commit().Error; err != nil {
		t.Fatal(err)
	}
	if err := <-finished; err != nil {
		t.Fatalf("cleanup could not resolve the outcome after archive committed: %v", err)
	}
}
