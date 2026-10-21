package workspace_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgworkspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
)

type retentionProbe struct {
	calls *int
}

func (p retentionProbe) ClearProjectContent(context.Context, uuid.UUID) error { *p.calls++; return nil }

func TestProjectRetentionQueuesOnlyExpiredBatchesAndKeepsRestoredProjects(t *testing.T) {
	ctx, db := workspaceMigrationDB(t)
	actor := insertWorkspaceActor(ctx, t, db, insertWorkspaceOrganization(ctx, t, db))
	expired := insertWorkspaceProject(ctx, t, db, actor, "16:9", "realistic", nil)
	live := insertWorkspaceProject(ctx, t, db, actor, "16:9", "realistic", nil)
	for _, item := range []struct {
		id  string
		age string
	}{{expired, "31 days"}, {live, "1 day"}} {
		if err := db.Exec(`UPDATE workspace.project SET is_delete=true,revision=2,delete_time=clock_timestamp()-?::interval,purge_after=clock_timestamp()-?::interval+interval '30 days' WHERE id=?`, item.age, item.age, item.id).Error; err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	store := pgworkspace.NewProjectRetentionStore(db, func(_ *gorm.DB) pgworkspace.ProjectRetentionContent { return retentionProbe{calls: &calls} })
	if n, err := store.EnqueueExpired(ctx, 10); err != nil || n < 1 {
		t.Fatalf("enqueue %d %v", n, err)
	}
	if n, err := store.EnqueueExpired(ctx, 10); err != nil || n != 0 {
		t.Fatalf("duplicate %d %v", n, err)
	}
	for {
		done, err := store.ProcessNext(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !done {
			break
		}
	}
	before := calls
	if before < 1 {
		t.Fatal("expired content was never processed")
	}
	// A stale task must never erase a restored or newly deleted version.
	other := insertWorkspaceProject(ctx, t, db, actor, "16:9", "realistic", nil)
	if err := db.Exec(`UPDATE workspace.project SET is_delete=true,revision=2,delete_time=clock_timestamp()-interval '31 days',purge_after=clock_timestamp()-interval '1 day' WHERE id=?`, other).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnqueueExpired(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE workspace.project SET is_delete=false,revision=3,delete_time=NULL,purge_after=NULL WHERE id=?`, other).Error; err != nil {
		t.Fatal(err)
	}
	if done, err := store.ProcessNext(ctx); err != nil || !done || calls != before {
		t.Fatalf("stale task %v %v calls=%d", done, err, calls)
	}
	var state struct {
		IsDelete   bool
		PurgeAfter *time.Time
	}
	if err := db.Raw(`SELECT is_delete,purge_after FROM workspace.project WHERE id=?`, other).Scan(&state).Error; err != nil || state.IsDelete || state.PurgeAfter != nil {
		t.Fatal("restored project changed", err)
	}
}
