package agentbusiness_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	pgmedia "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/postgres"
	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	script "github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	scriptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

type finishGate struct {
	script.SourceStore
	finish func(context.Context, identity.Principal, script.WritePlan) (script.SourceReceipt, error)
}

func (s finishGate) FinishWrite(ctx context.Context, a identity.Principal, p script.WritePlan) (script.SourceReceipt, error) {
	return s.finish(ctx, a, p)
}
func raceSourceCommand(g domain.Grant) script.SourceCommand {
	return script.SourceCommand{ProjectID: g.Scope.ProjectID, Key: uuid.New(), RequestID: uuid.New(), Action: "create", ExpectedRevision: 0, RightsConfirmed: true, Sources: []script.SourceInput{{Kind: "chapter", Title: "Concurrency proof", Status: "draft", Document: scriptdomain.RichDocument{Type: "doc", Content: []scriptdomain.RichDocument{{Type: "paragraph", Content: []scriptdomain.RichDocument{{Type: "text", Text: "Real publication proof"}}}}}}}}
}
func TestSourcePublicationCommitWinsBeforeRestrictedRemoval(t *testing.T) {
	db, _, _, _, g := realCanvasBoundary(t)
	sourceRoute(t, db)
	runtime := restrictedSourceRuntime(t)
	objects := &sourceBytes{data: map[uuid.UUID][]byte{}}
	held := make(chan script.WritePlan, 1)
	release := make(chan struct{})
	var once sync.Once
	releaseCommit := func() { once.Do(func() { close(release) }) }
	t.Cleanup(releaseCommit)
	store := sourceStoreForFixture(runtime)
	gate := finishGate{SourceStore: store, finish: func(ctx context.Context, a identity.Principal, p script.WritePlan) (script.SourceReceipt, error) {
		var receipt script.SourceReceipt
		err := runtime.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			receipt, err = sourceStoreForFixture(tx).FinishWrite(ctx, a, p)
			if err != nil {
				return err
			}
			held <- p
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		return receipt, err
	}}
	service := script.NewSourceService(gate, objects, time.Now)
	done := make(chan error, 1)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	go func() { _, err := service.Write(ctx, g.Scope.Principal(), raceSourceCommand(g)); done <- err }()
	var plan script.WritePlan
	select {
	case plan = <-held:
	case err := <-done:
		t.Fatal("publication failed before held commit", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	location, err := pgmedia.NewStorageRoutes(db).ReadLocation(ctx, plan.Objects[0].LocationID)
	if err != nil {
		t.Fatal(err)
	}
	removal := make(chan struct {
		ready bool
		err   error
	}, 1)
	go func() {
		ready, err := pgmedia.NewStorageRoutes(runtime).BeginPrivateCopyRemoval(ctx, plan.StorageIntentID, location)
		removal <- struct {
			ready bool
			err   error
		}{ready, err}
	}()
	var early *struct {
		ready bool
		err   error
	}
	select {
	case result := <-removal:
		if result.err != nil || result.ready {
			t.Fatal("in-progress publication granted removal", result)
		}
		early = &result
	case <-time.After(50 * time.Millisecond):
	}
	releaseCommit()
	if err := <-done; err != nil {
		t.Fatal("formal publication", err)
	}
	if early == nil {
		result := <-removal
		if result.err != nil || result.ready {
			t.Fatal("committed source acquired deletion permission", result)
		}
	}
	// Under Read Committed the earlier query may safely refuse the old open-work
	// state. A fresh request must also observe the committed immutable reference.
	ready, err := pgmedia.NewStorageRoutes(runtime).BeginPrivateCopyRemoval(ctx, plan.StorageIntentID, location)
	if err != nil || ready {
		t.Fatal("fresh reader ignored committed source", ready, err)
	}
	var n int64
	if err := runtime.Raw(`SELECT count(*) FROM script.script_source WHERE project_id=?`, g.Scope.ProjectID).Scan(&n).Error; err != nil || n != 1 {
		t.Fatal("publication missing", n, err)
	}
	var canUpdate bool
	if err := runtime.Raw(`SELECT has_table_privilege(current_user,'script.script_source','UPDATE')`).Scan(&canUpdate).Error; err != nil || canUpdate {
		t.Fatal("immutable UPDATE was granted", canUpdate, err)
	}
}

type removalGate struct {
	*sourceBytes
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *removalGate) RemoveObject(ctx context.Context, f scriptdomain.ObjectFact) error {
	wait := false
	s.once.Do(func() { close(s.entered); wait = true })
	if wait {
		select {
		case <-s.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return s.sourceBytes.RemoveObject(ctx, f)
}
func TestRestrictedDeletePendingWinsBeforeSourceFinishAndAccept(t *testing.T) {
	db, _, _, _, g := realCanvasBoundary(t)
	sourceRoute(t, db)
	runtime := restrictedSourceRuntime(t)
	objects := &removalGate{sourceBytes: &sourceBytes{data: map[uuid.UUID][]byte{}}, entered: make(chan struct{}), release: make(chan struct{})}
	prepared := make(chan script.WritePlan, 1)
	store := sourceStoreForFixture(runtime)
	gate := finishGate{SourceStore: store, finish: func(ctx context.Context, _ identity.Principal, p script.WritePlan) (script.SourceReceipt, error) {
		prepared <- p
		<-ctx.Done()
		return script.SourceReceipt{}, ctx.Err()
	}}
	service := script.NewSourceService(gate, objects, time.Now)
	recovery := script.NewSourceRecovery(store, service, time.Now)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := service.Write(ctx, g.Scope.Principal(), raceSourceCommand(g)); done <- err }()
	var plan script.WritePlan
	select {
	case plan = <-prepared:
	case err := <-done:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	page, err := recovery.List(t.Context(), g.Scope.Principal(), g.Scope.ProjectID, 0, 10)
	if err != nil || len(page.Items) != 1 {
		t.Fatal(page, err)
	}
	control := script.SourceControlCommand{ProjectID: g.Scope.ProjectID, IntentID: page.Items[0].ID, Key: uuid.New(), RequestID: uuid.New(), Action: "cancel", ExpectedRevision: page.Items[0].Revision}
	if _, err := recovery.Control(t.Context(), g.Scope.Principal(), control); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err == nil {
		t.Fatal("cancelled writer published")
	}
	current, err := recovery.Get(t.Context(), g.Scope.Principal(), g.Scope.ProjectID, control.IntentID)
	if err != nil || current.Status != "pending" {
		t.Fatal(current, err)
	}
	// Administrator-only isolated clock fixture, after the actual source writer
	// has exited. State and deletion permission still come from native owners.
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL session_replication_role='replica'`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE media.storage_work_intent SET closed_at=clock_timestamp()-interval '11 minutes' WHERE id=?`, plan.StorageIntentID).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE media.object_location SET write_until=clock_timestamp()-interval '1 second' WHERE owner_intent_id=? AND write_until IS NOT NULL`, plan.StorageIntentID).Error
	}); err != nil {
		t.Fatal(err)
	}
	control.Action = "reconcile"
	control.Key = uuid.New()
	control.RequestID = uuid.New()
	control.ExpectedRevision = current.Revision
	cleaned := make(chan error, 1)
	go func() { _, err := recovery.Control(t.Context(), g.Scope.Principal(), control); cleaned <- err }()
	select {
	case <-objects.entered:
	case err := <-cleaned:
		t.Fatal("cleanup never obtained actual delete permit", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, err := store.FinishWrite(t.Context(), g.Scope.Principal(), plan); err == nil {
		t.Fatal("delete_pending permitted a new source publication")
	}
	ids := make([]uuid.UUID, len(plan.Objects))
	for i, f := range plan.Objects {
		ids[i] = f.LocationID
	}
	if err := pgmedia.NewStorageRoutes(runtime).AcceptStorageWork(t.Context(), plan.StorageIntentID, ids); !errors.Is(err, media.ErrStorageRouteConflict) {
		t.Fatal("cancelled storage set was newly accepted", err)
	}
	close(objects.release)
	if err := <-cleaned; err != nil {
		t.Fatal(err)
	}
	current, err = recovery.Get(t.Context(), g.Scope.Principal(), g.Scope.ProjectID, control.IntentID)
	if err != nil || current.Status != "cancelled" {
		t.Fatal(current, err)
	}
	var n int64
	if err := runtime.Raw(`SELECT count(*) FROM script.script_source WHERE project_id=?`, g.Scope.ProjectID).Scan(&n).Error; err != nil || n != 0 {
		t.Fatal("source appeared after delete permit", n, err)
	}
}
