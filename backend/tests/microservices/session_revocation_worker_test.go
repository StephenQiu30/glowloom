package microservices_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/app/identityruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

func TestSessionRevocationRecoveryTaskAdmissionIsAtomic(t *testing.T) {
	admin, database := fenceDatabase(t, ""), authorityDatabase(t, "identity")
	now := time.Now().UTC().Truncate(time.Microsecond)
	actor, _ := sessionRevocationActor(t, admin, now)
	input := publicRevocationInput(t, admin, actor, identity.RevokeSessionRequest, actor.SessionID)
	synthetic := errors.New("synthetic task admission failure")
	var intercepted atomic.Bool
	if err := database.Callback().Create().Before("gorm:create").Register("reject_recovery_admission", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "infra.task_work" {
			intercepted.Store(true)
			if err := tx.AddError(synthetic); !errors.Is(err, synthetic) {
				t.Errorf("injected error lost its identity: %v", err)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	store := pgidentity.NewSessionRevocationStore(database)
	_, err := store.PrepareSessionRevocationRequest(t.Context(), actor, input, func() time.Time { return now })
	if !errors.Is(err, synthetic) || !intercepted.Load() {
		t.Fatalf("request did not atomically enqueue: intercepted=%t err=%v", intercepted.Load(), err)
	}
	for _, query := range []string{
		`SELECT count(*) FROM identity.session_revocation_request WHERE account_id=?`,
		`SELECT count(*) FROM identity.session_revocation WHERE account_id=?`,
		`SELECT count(*) FROM infra.authority_head WHERE authority_id=?`,
		`SELECT count(*) FROM identity.user_session WHERE account_id=? AND revoked_at IS NOT NULL`,
		`SELECT count(*) FROM identity.session_refresh_token WHERE session_id IN (SELECT id FROM identity.user_session WHERE account_id=?) AND revoked_at IS NOT NULL`,
	} {
		var count int64
		argument := actor.ID
		if query == `SELECT count(*) FROM infra.authority_head WHERE authority_id=?` {
			argument = actor.SessionID
		}
		if err := admin.Raw(query, argument).Scan(&count).Error; err != nil || count != 0 {
			t.Fatalf("partial admission survived rollback: count=%d err=%v query=%s", count, err, query)
		}
	}
}

func TestSessionRevocationWorkerCompletesWithoutBrowserCredentials(t *testing.T) {
	admin, database := fenceDatabase(t, ""), authorityDatabase(t, "identity")
	now := time.Now().UTC().Truncate(time.Microsecond)
	actor, _ := sessionRevocationActor(t, admin, now)
	input := publicRevocationInput(t, admin, actor, identity.RevokeSessionRequest, actor.SessionID)
	store := pgidentity.NewSessionRevocationStore(database)
	accepted, err := store.PrepareSessionRevocationRequest(t.Context(), actor, input, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	var before struct {
		Count     int
		Reference string
	}
	if err := database.Raw(`SELECT count(*) AS count, min(reference::text) AS reference FROM infra.task_work WHERE kind=? AND business_id=?`, identity.SessionRevocationRecoveryTask, accepted.ID.String()).Scan(&before).Error; err != nil || before.Count != 1 || before.Reference != "null" {
		t.Fatalf("original task missing or contains credential material: %+v %v", before, err)
	}
	if err := admin.Exec(`DELETE FROM identity.session_refresh_token WHERE session_id=?`, actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := admin.Exec(`DELETE FROM identity.user_session WHERE id=?`, actor.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	// Simulate an accepted request from before atomic wakeups were introduced.
	if err := admin.Exec(`DELETE FROM infra.task_work WHERE service_owner='identity' AND kind=? AND business_id=?`, identity.SessionRevocationRecoveryTask, accepted.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	coordinator := writefence.NewCoordinator(pgfence.NewAuthorityStore(database), publicRevocationWriters(t))
	clock := func() time.Time { return now.Add(time.Second) }
	worker, err := identityruntime.NewSessionRevocationRecoveryWorker(t.Context(), database, coordinator, clock, 1)
	if err != nil {
		t.Fatal(err)
	}
	runSessionRevocationWorker(t, worker, database, accepted.ID)
	resolved, err := store.LoadSessionRevocationRequestByID(t.Context(), accepted.ID)
	if err != nil || resolved.CompletedAt == nil || resolved.ID != accepted.ID {
		t.Fatalf("worker lost original request: %+v %v", resolved, err)
	}
	for _, writer := range []string{"creation", "media", "execution"} {
		var rows int64
		if err := authorityDatabase(t, writer).Raw(`SELECT count(*) FROM infra.task_work WHERE kind=? AND business_id=?`, identity.SessionRevocationRecoveryTask, accepted.ID.String()).Scan(&rows).Error; err != nil || rows != 0 {
			t.Fatalf("%s observed Identity recovery task: count=%d err=%v", writer, rows, err)
		}
	}
	recovery := identity.NewSessionRevocationRecovery(store, identity.NewSessionRevocations(store, coordinator, clock), clock)
	if err := recovery.Resume(t.Context(), accepted.ID); err != nil {
		t.Fatal(err)
	}
	var audits int
	if err := admin.Raw(`SELECT count(*) FROM identity.security_event WHERE request_id=?`, accepted.Revocations[0].Control.CoordinationID.String()).Scan(&audits).Error; err != nil || audits != 1 {
		t.Fatalf("replayed original task duplicated success: count=%d err=%v", audits, err)
	}
	if job, err := taskruntime.NewStore(database).ForBusiness(accepted.ID.String()).Claim(t.Context(), []string{identity.SessionRevocationRecoveryTask}, uuid.NewString(), time.Second); err != nil || job != nil {
		t.Fatalf("completed task was reopened: %+v %v", job, err)
	}
}

func runSessionRevocationWorker(t *testing.T, worker *taskruntime.Worker, database *gorm.DB, id uuid.UUID) {
	t.Helper()
	worker.Store = worker.Store.ForBusiness(id.String())
	worker.Poll = 5 * time.Millisecond
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()
	defer func() {
		cancel()
		if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
			t.Error(err)
		}
	}()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var complete bool
		if err := database.Raw(`SELECT completed_at IS NOT NULL FROM infra.task_work WHERE kind=? AND business_id=?`, identity.SessionRevocationRecoveryTask, id.String()).Scan(&complete).Error; err != nil {
			t.Fatal(err)
		}
		if complete {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("worker did not finish original responsibility")
		case <-ticker.C:
		}
	}
}
