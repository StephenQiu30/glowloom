package microservices_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/identityruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

func TestSessionRevocationWorkerRejectsLateSourceCompletionAfterLeaseLoss(t *testing.T) {
	admin, database := fenceDatabase(t, ""), authorityDatabase(t, "identity")
	now := time.Now().UTC().Truncate(time.Microsecond)
	actor, _ := sessionRevocationActor(t, admin, now)
	input := publicRevocationInput(t, admin, actor, identity.RevokeSessionRequest, actor.SessionID)
	store := pgidentity.NewSessionRevocationStore(database)
	accepted, err := store.PrepareSessionRevocationRequest(t.Context(), actor, input, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	original := writefence.NewCoordinator(pgfence.NewAuthorityStore(database), publicRevocationWriters(t))
	var changed atomic.Bool
	coordinator := sessionRecoveryCoordinator(func(ctx context.Context, authority writefence.Authority, id uuid.UUID, blocked bool) (writefence.Coordination, error) {
		result, err := original.Set(ctx, authority, id, blocked)
		if err != nil {
			return result, err
		}
		if changed.CompareAndSwap(false, true) {
			// The original RPC has completed, but a replacement owns the execution
			// window before the old worker attempts the local completion transaction.
			updated := admin.WithContext(ctx).Exec(`UPDATE infra.task_work SET epoch=epoch+1,owner='replacement-worker',lease_until=now()+interval '1 minute' WHERE service_owner='identity' AND kind=? AND business_id=?`, identity.SessionRevocationRecoveryTask, accepted.ID.String())
			if updated.Error != nil {
				return result, updated.Error
			}
			if updated.RowsAffected != 1 {
				return result, fmt.Errorf("replacement changed %d tasks", updated.RowsAffected)
			}
		}
		return result, nil
	})
	clock := func() time.Time { return now.Add(time.Second) }
	worker, err := identityruntime.NewSessionRevocationRecoveryWorker(t.Context(), database, coordinator, clock, 1)
	if err != nil {
		t.Fatal(err)
	}
	worker.Store = worker.Store.ForBusiness(accepted.ID.String())
	handler := worker.Handlers[identity.SessionRevocationRecoveryTask]
	observed := make(chan error, 1)
	worker.Handlers[identity.SessionRevocationRecoveryTask] = func(ctx context.Context, job taskruntime.Job) (time.Time, error) {
		next, err := handler(ctx, job)
		observed <- err
		return next, err
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()
	defer func() {
		cancel()
		if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
			t.Error(err)
		}
	}()
	select {
	case err := <-observed:
		if !errors.Is(err, taskruntime.ErrLeaseLost) {
			t.Fatalf("late worker published source completion: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("late execution did not finish")
	}
	cancel()
	pending, err := store.LoadSessionRevocationRequestByID(t.Context(), accepted.ID)
	if err != nil || pending.CompletedAt != nil || pending.Revocations[0].CompletedAt != nil {
		t.Fatalf("stale source transaction did not roll back: %+v %v", pending, err)
	}
	var audits int
	if err := admin.Raw(`SELECT count(*) FROM identity.security_event WHERE request_id=?`, accepted.Revocations[0].Control.CoordinationID.String()).Scan(&audits).Error; err != nil || audits != 0 {
		t.Fatalf("stale worker committed audit: %d %v", audits, err)
	}
	if err := admin.Exec(`UPDATE infra.task_work SET lease_until=now()-interval '1 second' WHERE service_owner='identity' AND kind=? AND business_id=?`, identity.SessionRevocationRecoveryTask, accepted.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	restarted, err := identityruntime.NewSessionRevocationRecoveryWorker(t.Context(), database, original, clock, 1)
	if err != nil {
		t.Fatal(err)
	}
	runSessionRevocationWorker(t, restarted, database, accepted.ID)
	completed, err := store.LoadSessionRevocationRequestByID(t.Context(), accepted.ID)
	if err != nil || completed.CompletedAt == nil {
		t.Fatalf("replacement lost original responsibility: %+v %v", completed, err)
	}
}

func TestSessionRevocationRecoveryReconciliationDoesNotReopenExistingTask(t *testing.T) {
	for _, state := range []string{"completed_at", "failed_at"} {
		t.Run(state, func(t *testing.T) {
			admin, database := fenceDatabase(t, ""), authorityDatabase(t, "identity")
			now := time.Now().UTC().Truncate(time.Microsecond)
			actor, _ := sessionRevocationActor(t, admin, now)
			input := publicRevocationInput(t, admin, actor, identity.RevokeSessionRequest, actor.SessionID)
			store := pgidentity.NewSessionRevocationStore(database)
			accepted, err := store.PrepareSessionRevocationRequest(t.Context(), actor, input, func() time.Time { return now })
			if err != nil {
				t.Fatal(err)
			}
			// Only these two test-owned, fixed column names are interpolated.
			if err := admin.Exec(`UPDATE infra.task_work SET `+state+`=? WHERE service_owner='identity' AND kind=? AND business_id=?`, now, identity.SessionRevocationRecoveryTask, accepted.ID.String()).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := store.EnqueuePendingSessionRevocations(t.Context(), 256, now); err != nil {
				t.Fatal(err)
			}
			var rows int
			if err := database.Raw(`SELECT count(*) FROM infra.task_work WHERE kind=? AND business_id=? AND `+state+`=?`, identity.SessionRevocationRecoveryTask, accepted.ID.String(), now).Scan(&rows).Error; err != nil || rows != 1 {
				t.Fatalf("reconciliation reset original %s: count=%d err=%v", state, rows, err)
			}
			foreign := pgidentity.NewSessionRevocationStore(authorityDatabase(t, "media"))
			if _, err := foreign.EnqueuePendingSessionRevocations(t.Context(), 1, now); err == nil {
				t.Fatal("Media reconciled Identity tasks")
			}
			if _, err := foreign.LoadSessionRevocationRequestByID(t.Context(), accepted.ID); err == nil {
				t.Fatal("Media read trusted Identity responsibility")
			}
		})
	}
}

type sessionRecoveryCoordinator func(context.Context, writefence.Authority, uuid.UUID, bool) (writefence.Coordination, error)

func (f sessionRecoveryCoordinator) Set(ctx context.Context, authority writefence.Authority, id uuid.UUID, blocked bool) (writefence.Coordination, error) {
	return f(ctx, authority, id, blocked)
}
