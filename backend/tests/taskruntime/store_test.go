package taskruntime_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	runtime "github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

func TestClaimRollbackDedupAndFence(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_TASK_DB_DSN")
	if dsn == "" {
		t.Skip("requires isolated task DB with task_runtime.sql installed")
	}
	db, err := gorm.Open(postgres.Open(dsn))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Error("close task database:", err)
		}
	})
	store := runtime.NewStore(db)
	ctx := context.Background()
	kind := "test." + uuid.NewString()
	now := time.Now()
	rollback := errors.New("rollback")
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := runtime.Enqueue(ctx, tx, kind, "business", now); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	claim := func(owner string) (*runtime.Job, error) { return store.Claim(ctx, []string{kind}, owner, time.Second) }
	if got, err := claim("a"); err != nil || got != nil {
		t.Fatalf("rolled back claim: %v %v", got, err)
	}
	for range 2 {
		if err := runtime.Enqueue(ctx, db, kind, "business", now); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	jobs := make(chan *runtime.Job, 8)
	for range 8 {
		wg.Go(func() {
			job, err := claim(uuid.NewString())
			if err != nil {
				t.Error(err)
			}
			if job != nil {
				jobs <- job
			}
		})
	}
	wg.Wait()
	close(jobs)
	if len(jobs) != 1 {
		t.Fatalf("got %d owners", len(jobs))
	}
	first := <-jobs
	if err := db.Exec("UPDATE infra.task_work SET lease_until=clock_timestamp()-interval '1 second' WHERE kind=?", kind).Error; err != nil {
		t.Fatal(err)
	}
	second, err := claim("next")
	if err != nil || second == nil || second.Epoch != first.Epoch+1 {
		t.Fatalf("recovery: %v %v", second, err)
	}
	if err := store.Complete(ctx, *first); !errors.Is(err, runtime.ErrLeaseLost) {
		t.Fatalf("old completion: %v", err)
	}
	if err := store.Renew(ctx, *first, time.Second); !errors.Is(err, runtime.ErrLeaseLost) {
		t.Fatalf("old renewal: %v", err)
	}
	if err := store.Reschedule(ctx, *second, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, err := claim("early"); err != nil || got != nil {
		t.Fatalf("early claim: %v %v", got, err)
	}
}

func TestWorkerStopsAndExpiredWindowCanBeRecovered(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_TASK_DB_DSN")
	if dsn == "" {
		t.Skip("requires isolated task DB")
	}
	db, err := gorm.Open(postgres.Open(dsn))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Error("close task database:", err)
		}
	})
	store := runtime.NewStore(db)
	kind := "test." + uuid.NewString()
	if err := runtime.Enqueue(context.Background(), db, kind, "work", time.Now()); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := runtime.Worker{Store: store, Handlers: map[string]runtime.Handler{kind: func(ctx context.Context, _ runtime.Job) (time.Time, error) {
		close(started)
		<-ctx.Done()
		return time.Time{}, ctx.Err()
	}}, Concurrency: 2, Lease: 500 * time.Millisecond, Poll: 10 * time.Millisecond}
	stopped := make(chan error, 1)
	go func() { stopped <- w.Run(ctx) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("worker failed to start")
	}
	cancel()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("worker failed to drain")
	}
	if err := db.Exec("UPDATE infra.task_work SET lease_until=clock_timestamp()-interval '1 second' WHERE kind=?", kind).Error; err != nil {
		t.Fatal(err)
	}
	next, err := store.Claim(context.Background(), []string{kind}, "recovery", time.Second)
	if err != nil || next == nil || next.Epoch != 2 {
		t.Fatalf("recovery after stop: %v %v", next, err)
	}
	if err := store.Complete(context.Background(), *next); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Enqueue(context.Background(), db, kind, "work", time.Now()); err != nil {
		t.Fatal(err)
	}
	next, err = store.Claim(context.Background(), []string{kind}, "other", time.Second)
	if err != nil || next != nil {
		t.Fatalf("replayed completed work: %v %v", next, err)
	}
}

func TestLeaseRejectsBusinessMutationAndStopWaitsForHandler(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_TASK_DB_DSN")
	if dsn == "" {
		t.Skip("requires isolated PG")
	}
	db, err := gorm.Open(postgres.Open(dsn))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Error("close task database:", err)
		}
	})
	for _, stop := range []bool{false, true} {
		t.Run(fmt.Sprint("stop_", stop), func(t *testing.T) {
			kind := "test." + uuid.NewString()
			ref := map[string]string{"job_id": uuid.NewString()}
			if err := runtime.EnqueueReference(t.Context(), db, kind, "one", time.Now(), ref); err != nil {
				t.Fatal(err)
			}
			if err := runtime.Enqueue(t.Context(), db, kind+".proof", "result", time.Now()); err != nil {
				t.Fatal(err)
			}
			entered := make(chan struct{})
			release := make(chan struct{})
			finished := make(chan error, 1)
			worker := runtime.Worker{Store: runtime.NewStore(db), Lease: 210 * time.Millisecond, Poll: 5 * time.Millisecond, Concurrency: 1,
				Handlers: map[string]runtime.Handler{kind: func(ctx context.Context, _ runtime.Job) (time.Time, error) {
					close(entered)
					if stop {
						<-ctx.Done()
						if !errors.Is(context.Cause(ctx), runtime.ErrStopRequested) {
							t.Errorf("cancel cause: %v", context.Cause(ctx))
						}
						finished <- nil
						return time.Time{}, nil
					}
					<-release
					err := runtime.Transaction(ctx, db, func(tx *gorm.DB) error {
						return tx.Exec("UPDATE infra.task_work SET completed_at=now() WHERE kind=?", kind+".proof").Error
					})
					finished <- err
					return time.Time{}, err
				}}}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- worker.Run(ctx) }()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("handler did not start")
			}
			if stop {
				if err := runtime.RequestStop(t.Context(), db, kind, ref); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := db.Exec("UPDATE infra.task_work SET owner='other',epoch=epoch+1,lease_until=now()+interval '1 minute' WHERE kind=?", kind).Error; err != nil {
					t.Fatal(err)
				}
				close(release)
			}
			select {
			case err := <-finished:
				if !stop && !errors.Is(err, runtime.ErrLeaseLost) {
					t.Fatalf("stale progress committed: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("handler failed to return")
			}
			cancel()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			var changed bool
			if err := db.Raw("SELECT completed_at IS NOT NULL FROM infra.task_work WHERE kind=?", kind+".proof").Scan(&changed).Error; err != nil || changed {
				t.Fatalf("business transaction escaped fence: %t %v", changed, err)
			}
		})
	}
}
