package workspace_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	identitydomain "github.com/StephenQiu30/lanverse/backend/internal/identity/domain"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
	"github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

func TestProjectCoverPGCurrentRoleClearAndReplay(t *testing.T) {
	ctx, db, owner := coverTestDB(t)
	actor, project := lifecycleActorProject(ctx, t, owner)
	asset := seedCoverAsset(t, owner, project, "image")
	s := workspaceapp.NewProjectLifecycle(coverStore(db), time.Now)
	set := coverChange(project, 1, &asset)
	if _, err := s.Change(ctx, actor, set); err != nil {
		t.Fatal(err)
	}
	lifecycleFixtureSQL(t, owner, `UPDATE identity."user" SET role='admin' WHERE id=?`, actor.ID)
	if _, err := s.Change(ctx, actor, coverChange(project, 2, nil)); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("stale principal role cleared cover", err)
	}
	if _, err := s.Change(ctx, actor, set); !errors.Is(err, identityapp.ErrForbidden) {
		t.Fatal("stale role replay exposed cover", err)
	}
	actor.Role = identitydomain.RoleAdmin
	if _, err := s.Change(ctx, actor, coverChange(project, 2, nil)); err != nil {
		t.Fatal("fresh role cannot clear", err)
	}
}

func TestProjectCoverPGMediaShareLockAndOuterRollback(t *testing.T) {
	ctx, db, owner := coverTestDB(t)
	actor, project := lifecycleActorProject(ctx, t, owner)
	asset := seedCoverAsset(t, owner, project, "image")
	owned, cancel := context.WithCancel(ctx)
	defer cancel()
	save := db.WithContext(owned).Begin()
	defer rollbackLifecycleTest(t, save)
	withdraw := owner.WithContext(owned).Begin()
	defer rollbackLifecycleTest(t, withdraw)
	in := coverChange(project, 1, &asset)
	if _, err := workspaceapp.NewProjectLifecycle(coverStore(save), time.Now).Change(owned, actor, in); err != nil {
		t.Fatal(err)
	}
	blocker, blocked := projectTransactionPID(t, save), projectTransactionPID(t, withdraw)
	done := make(chan error, 1)
	finished := false
	go func() {
		done <- withdraw.Exec(`UPDATE media.media_asset SET status='processing',moderation_status='pending' WHERE id=?`, asset).Error
	}()
	defer func() {
		if !finished {
			cancel()
			<-done
		}
	}()
	waitProjectLock(owned, t, db, blocker, blocked)
	if err := save.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		finished = true
		if err != nil {
			t.Fatal(err)
		}
	case <-owned.Done():
		t.Fatal(owned.Err())
	}
	if err := withdraw.Commit().Error; err != nil {
		t.Fatal(err)
	}
	read, err := workspaceapp.NewProjectLifecycle(coverStore(db), time.Now).Get(ctx, actor, project)
	if err != nil || read.Project.CoverAssetID != nil || read.Project.Revision != 1 {
		t.Fatal("outer rollback retained cover", read, err)
	}
	var count int64
	if err := db.Raw(`SELECT count(*) FROM workspace.project_change_command WHERE actor_id=? AND idem_key=?`, actor.ID, in.IdempotencyKey.String()).Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("outer rollback retained receipt", err, count)
	}
}

func TestProjectCoverPGOutboxAndReceiptFaultAtomicity(t *testing.T) {
	ctx, db, owner := coverTestDB(t)
	actor, project := lifecycleActorProject(ctx, t, owner)
	asset := seedCoverAsset(t, owner, project, "image")
	for _, table := range []string{"outbox", "project_change_command"} {
		t.Run(table, func(t *testing.T) {
			schema := "infra"
			if table == "project_change_command" {
				schema = "workspace"
			}
			name := "cover_fault_" + uuid.New().String()[:8]
			filter := fmt.Sprintf("NEW.partition_key = '%s'", project.String())
			if table == "project_change_command" {
				filter = fmt.Sprintf("NEW.actor_id = '%s'::uuid", actor.ID.String())
			}
			statement := fmt.Sprintf(`CREATE FUNCTION infra.%s() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF %s THEN RAISE EXCEPTION 'cover fixture fault';END IF;RETURN NEW;END;$$;CREATE TRIGGER %s BEFORE INSERT ON %s.%s FOR EACH ROW EXECUTE FUNCTION infra.%s()`, name, filter, name, schema, table, name)
			lifecycleFixtureSQL(t, owner, statement)
			defer func() {
				lifecycleFixtureSQL(t, owner, fmt.Sprintf(`DROP TRIGGER %s ON %s.%s;DROP FUNCTION infra.%s()`, name, schema, table, name))
			}()
			in := coverChange(project, 1, &asset)
			if _, err := workspaceapp.NewProjectLifecycle(coverStore(db), time.Now).Change(ctx, actor, in); err == nil {
				t.Fatal("fault committed cover")
			}
			var row struct {
				Revision, Events, Receipts int64
				Cover                      *uuid.UUID
			}
			if err := db.Raw(`SELECT p.revision,p.cover_asset_id AS cover,(SELECT count(*) FROM infra.outbox WHERE partition_key=p.id::text) AS events,(SELECT count(*) FROM workspace.project_change_command WHERE actor_id=? AND idem_key=?) AS receipts FROM workspace.project p WHERE id=?`, actor.ID, in.IdempotencyKey.String(), project).Scan(&row).Error; err != nil || row.Revision != 1 || row.Cover != nil || row.Events != 0 || row.Receipts != 0 {
				t.Fatal("partial project cover commit", row, err)
			}
		})
	}
}

func TestProjectCoverPGConcurrentCASAndSameKeySingleMutation(t *testing.T) {
	ctx, db, owner := coverTestDB(t)
	actor, project := lifecycleActorProject(ctx, t, owner)
	asset := seedCoverAsset(t, owner, project, "image")
	in := coverChange(project, 1, &asset)
	s := workspaceapp.NewProjectLifecycle(coverStore(db), time.Now)
	start := make(chan struct{})
	results := make(chan error, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			p, err := s.Change(ctx, actor, in)
			if err == nil && (p.Project.Revision != 2 || p.Project.CoverAssetID == nil || *p.Project.CoverAssetID != asset) {
				err = errors.New("inconsistent repeated request")
			}
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	var counts struct{ Events, Receipts int64 }
	if err := db.Raw(`SELECT(SELECT count(*) FROM infra.outbox WHERE partition_key=?)AS events,(SELECT count(*) FROM workspace.project_change_command WHERE actor_id=? AND idem_key=?)AS receipts`, project.String(), actor.ID, in.IdempotencyKey.String()).Scan(&counts).Error; err != nil || counts.Events != 2 || counts.Receipts != 1 {
		t.Fatal("same key repeated mutation", counts, err)
	}
	start = make(chan struct{})
	results = make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := s.Change(ctx, actor, coverChange(project, 2, nil))
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	success, stale := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, domain.ErrProjectRevisionConflict):
			stale++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || stale != 1 {
		t.Fatal("CAS did not choose one winner", success, stale)
	}
}
