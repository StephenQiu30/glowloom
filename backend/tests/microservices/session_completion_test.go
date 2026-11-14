package microservices_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

func TestIdentitySessionCompletionAndReplayKeepOnePolicyLockOrder(t *testing.T) {
	admin := fenceDatabase(t, "")
	database := authorityDatabase(t, "identity")
	now := time.Now().UTC().Truncate(time.Microsecond)
	actor, target := sessionRevocationActor(t, admin, now)
	store := pgidentity.NewSessionRevocationStore(database)
	ticket, err := store.PrepareSessionRevocation(t.Context(), actor, target, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	repository := pgfence.NewAuthorityStore(database)
	for _, writer := range writefence.Writers() {
		if err := pgfence.New(authorityDatabase(t, string(writer))).Install(t.Context(), ticket.Control); err != nil {
			t.Fatal(err)
		}
		if _, err := repository.Acknowledge(t.Context(), ticket.Control, writer); err != nil {
			t.Fatal(err)
		}
	}
	// Hold only the authority row. Completion must acquire its earlier locks and
	// wait here, making the replay/foreign-key lock ordering deterministic.
	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
	held := admin.WithContext(ctx).Begin()
	if held.Error != nil {
		cancel()
		t.Fatal(held.Error)
	}
	var workers sync.WaitGroup
	released := false
	defer func() {
		if !released {
			_ = held.Rollback().Error
		}
		cancel()
		workers.Wait()
	}()
	var heldPID int
	if err := held.Raw(`SELECT pg_backend_pid()`).Scan(&heldPID).Error; err != nil {
		t.Fatal(err)
	}
	if err := held.Exec(`SELECT authority_id FROM infra.authority_head WHERE service_owner='identity' AND issuer='identity' AND authority_kind='session' AND authority_id=? FOR UPDATE`, target).Error; err != nil {
		t.Fatal(err)
	}
	completed, replayed := make(chan error, 1), make(chan error, 1)
	workers.Go(func() {
		completed <- store.CompleteSessionRevocation(ctx, ticket, now.Add(time.Second))
	})
	completionPID := waitSessionBlockedBackend(ctx, t, admin, heldPID, "%authority_head%")
	workers.Go(func() {
		_, err := store.PrepareSessionRevocation(ctx, actor, target, func() time.Time { return now })
		replayed <- err
	})
	waitSessionBlockedBackend(ctx, t, admin, completionPID, "%")
	if err := held.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	released = true
	if err := <-completed; err != nil {
		t.Error("completion and valid replay did not serialize:", err)
	}
	if err := <-replayed; err != nil {
		t.Error("valid replay and completion did not serialize:", err)
	}
	var events int
	if err := admin.Raw(`SELECT count(*) FROM identity.security_event WHERE request_id=? AND action='session.revoked'`, ticket.Control.CoordinationID.String()).Scan(&events).Error; err != nil || events != 1 {
		t.Fatalf("concurrent completion audit was not exactly once: %d %v", events, err)
	}
}

func TestIdentitySessionCompletionCancellationRetainsPendingResponsibility(t *testing.T) {
	admin := fenceDatabase(t, "")
	database := authorityDatabase(t, "identity")
	now := time.Now().UTC().Truncate(time.Microsecond)
	actor, target := sessionRevocationActor(t, admin, now)
	store := pgidentity.NewSessionRevocationStore(database)
	ticket, err := store.PrepareSessionRevocation(t.Context(), actor, target, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	repository := pgfence.NewAuthorityStore(database)
	for _, writer := range writefence.Writers() {
		if err := pgfence.New(authorityDatabase(t, string(writer))).Install(t.Context(), ticket.Control); err != nil {
			t.Fatal(err)
		}
		if _, err := repository.Acknowledge(t.Context(), ticket.Control, writer); err != nil {
			t.Fatal(err)
		}
	}
	held := admin.WithContext(t.Context()).Begin()
	if held.Error != nil {
		t.Fatal(held.Error)
	}
	defer func() { _ = held.Rollback().Error }()
	var heldPID int
	if err := held.Raw(`SELECT pg_backend_pid()`).Scan(&heldPID).Error; err != nil {
		t.Fatal(err)
	}
	if err := held.Exec(`SELECT id FROM identity."user" WHERE id=? FOR UPDATE`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	done := make(chan error, 1)
	var worker sync.WaitGroup
	defer func() { cancel(); worker.Wait() }()
	worker.Go(func() { done <- store.CompleteSessionRevocation(ctx, ticket, now) })
	waitSessionBlockedBackend(ctx, t, admin, heldPID, "%")
	cancel()
	if err := <-done; err == nil {
		t.Fatal("canceled completion reported success")
	}
	pending, err := store.LoadSessionRevocation(t.Context(), target)
	if err != nil || pending.CompletedAt != nil || pending.Control != ticket.Control {
		t.Fatalf("cancellation lost the original source responsibility: %+v %v", pending, err)
	}
	var events int
	if err := admin.Raw(`SELECT count(*) FROM identity.security_event WHERE request_id=? AND action='session.revoked'`, ticket.Control.CoordinationID.String()).Scan(&events).Error; err != nil || events != 0 {
		t.Fatalf("canceled completion emitted success audit: %d %v", events, err)
	}
	if err := held.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteSessionRevocation(t.Context(), pending, now); err != nil {
		t.Fatal("pending source decision could not resume:", err)
	}
}

func waitSessionBlockedBackend(ctx context.Context, t *testing.T, admin *gorm.DB, blocker int, pattern string) int {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var pid int
		err := admin.WithContext(ctx).Raw(`SELECT coalesce(min(pid),0) FROM pg_stat_activity
			WHERE datname=current_database() AND usename='owner_b_identity_test'
			AND query LIKE ? AND ?=ANY(pg_blocking_pids(pid))`, pattern, blocker).Scan(&pid).Error
		if err != nil {
			t.Fatal(err)
		}
		if pid != 0 {
			return pid
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("expected PostgreSQL blocking edge was not observed")
		}
	}
}
