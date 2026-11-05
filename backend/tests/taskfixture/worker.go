// Package taskfixture runs the production PostgreSQL worker in isolated integration tests.
package taskfixture

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

// Start starts the production task worker and joins it during fixture cleanup.
func Start(t *testing.T, db *gorm.DB, handlers map[string]taskruntime.Handler) context.CancelFunc {
	t.Helper()
	return start(t, taskruntime.NewStore(db), handlers)
}
func start(t *testing.T, store *taskruntime.Store, handlers map[string]taskruntime.Handler) context.CancelFunc {
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	worker := taskruntime.Worker{Store: store, Handlers: handlers, Concurrency: 4, Poll: 20 * time.Millisecond, Lease: 600 * time.Millisecond}
	go func() { done <- worker.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("task worker: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("task worker failed to drain")
		}
	})
	return cancel
}

// StartWork targets only the task belonging to this fixture's business record.
func StartWork(t *testing.T, db *gorm.DB, kind string, ref any, handler taskruntime.Handler) string {
	t.Helper()
	raw, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	var id string
	q := db.Raw("SELECT business_id FROM infra.task_work WHERE kind=? AND reference @> ?::jsonb AND completed_at IS NULL ORDER BY next_run_at DESC LIMIT 1", kind, string(raw)).Scan(&id)
	if q.Error != nil || q.RowsAffected != 1 {
		t.Fatalf("missing durable admission %s: %v", kind, q.Error)
	}
	start(t, taskruntime.NewStore(db).ForBusiness(id), map[string]taskruntime.Handler{kind: handler})
	return id
}

// WaitWork waits for the exact admitted fixture task to complete or fail.
func WaitWork(t *testing.T, db *gorm.DB, kind, id string, duration time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), duration)
	defer cancel()
	for {
		var row struct{ Done, Failed bool }
		q := db.WithContext(ctx).Raw("SELECT completed_at IS NOT NULL AS done, failed_at IS NOT NULL AS failed FROM infra.task_work WHERE kind=? AND business_id=?", kind, id).Scan(&row)
		if q.Error != nil || q.RowsAffected != 1 || row.Failed {
			t.Fatalf("execution %s/%s: failed=%t err=%v", kind, id, row.Failed, q.Error)
		}
		if row.Done {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// RunWork starts the fixture's exact task and waits for its completion.
func RunWork(t *testing.T, db *gorm.DB, kind string, ref any, handler taskruntime.Handler) {
	t.Helper()
	id := StartWork(t, db, kind, ref, handler)
	WaitWork(t, db, kind, id, 3*time.Minute)
}
