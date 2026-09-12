package gormdb_test

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	"github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Reuses only the owned, already committed storage fixture. No new database or
// provider/target setup is required to prove progress and snapshot consistency.
func assertReferenceExecutionProgressStorage(t *testing.T, ctx context.Context, database *gorm.DB, actor application.Actor, execution domain.ReferenceExecution) {
	t.Helper()
	member := model.Membership{ID: uuid.New(), WorkspaceID: uuid.MustParse(execution.WorkspaceID), UserID: uuid.MustParse(actor.UserID), Role: "viewer", Status: "active", JoinedAt: time.Now().UTC()}
	if err := database.Omit(clause.Associations).Create(&member).Error; err != nil {
		t.Fatal(err)
	}
	store := generationgorm.New(database)
	query := application.NewReferenceExecutionQuery(store)
	read := func() domain.ReferenceJobProgress {
		t.Helper()
		value, err := query.Get(ctx, actor, execution.ProjectID, execution.ID)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := read()
	if before.Total != 9 || before.Succeeded != 1 || before.Pending != 6 || before.Terminal {
		t.Fatalf("incomplete persisted progress: %+v", before)
	}
	if again := read(); !reflect.DeepEqual(before, again) {
		t.Fatal("read changed persisted identity")
	}
	for _, fault := range []string{"foreign_actor", "revoked_token", "foreign_project", "missing_execution", "outer_transaction"} {
		gotActor, project, id := actor, execution.ProjectID, execution.ID
		reader := query
		switch fault {
		case "foreign_actor":
			gotActor.UserID = uuid.NewString()
		case "revoked_token":
			gotActor.TokenVersion++
		case "foreign_project":
			project = uuid.NewString()
		case "missing_execution":
			id = uuid.NewString()
		case "outer_transaction":
			tx := database.Begin()
			if tx.Error != nil {
				t.Fatal(tx.Error)
			}
			defer tx.Rollback()
			reader = application.NewReferenceExecutionQuery(generationgorm.New(tx))
		}
		got, err := reader.Get(ctx, gotActor, project, id)
		if err == nil || !reflect.DeepEqual(got, domain.ReferenceJobProgress{}) {
			t.Fatalf("accepted %s", fault)
		}
	}
	// Commit a different connection's transition after the query's execution read.
	// This response must retain the original snapshot; the next must observe it.
	var key string
	for _, call := range before.Calls {
		if call.Status == domain.ProviderCallPending {
			key = call.CallKey
			break
		}
	}
	if key == "" {
		t.Fatal("no pending fixture Call")
	}
	pending, err := domain.NewReferenceCallState(key)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	claimed, _, err := domain.ClaimReferenceCall(pending, domain.ReferenceCallDispatch{SubmissionToken: uuid.NewString(), DispatchedBy: actor.UserID, MembershipTokenVersion: actor.TokenVersion, DispatchedAt: now, DeadlineAt: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	callback := "test_reference_progress_snapshot_" + uuid.NewString()
	var fired atomic.Bool
	var concurrentErr error
	if err := database.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Statement.Schema.Name != "GenerationReferenceExecution" || !fired.CompareAndSwap(false, true) {
			return
		}
		concurrentErr = store.WithinReferenceCallDispatch(ctx, func(repo application.ReferenceCallDispatchRepository) error {
			return repo.UpdateReferenceCallState(ctx, execution.WorkspaceID, execution.ProjectID, execution.ID, pending, claimed)
		})
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Callback().Query().Remove(callback) })
	during := read()
	if !fired.Load() || concurrentErr != nil || !reflect.DeepEqual(before, during) {
		t.Fatalf("mixed snapshots: fired=%v err=%v", fired.Load(), concurrentErr)
	}
	after := read()
	if after.Pending != before.Pending-1 || after.Dispatching != before.Dispatching+1 || after.ContentHash == before.ContentHash {
		t.Fatal("next snapshot missed committed transition")
	}
	// Exact owned-row corruption is restored even when an assertion fails.
	var row model.GenerationReferenceProviderCall
	if err := database.Where("call_key = ?", key).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	update := func(value string) error {
		return database.Model(&model.GenerationReferenceProviderCall{}).Where("call_key = ?", key).UpdateColumn("state_hash", value).Error
	}
	t.Cleanup(func() {
		if err := update(row.StateHash); err != nil {
			t.Error(err)
		}
	})
	if err := update(before.JobHash); err != nil {
		t.Fatal(err)
	}
	got, err := query.Get(ctx, actor, execution.ProjectID, execution.ID)
	if err == nil || !reflect.DeepEqual(got, domain.ReferenceJobProgress{}) {
		t.Fatal("corrupt persisted state returned progress")
	}
	if err := update(row.StateHash); err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&member).Update("status", "removed").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := query.Get(ctx, actor, execution.ProjectID, execution.ID); err == nil {
		t.Fatal("removed membership retained access")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := query.Get(cancelled, actor, execution.ProjectID, execution.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
}
