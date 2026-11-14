package taskruntime_test

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	runtime "github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

func TestEnqueueUsesDatabaseServiceOwner(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=unused dbname=unused"}), &gorm.Config{
		DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var statement string
	if err := db.Callback().Create().After("gorm:create").Register("test:capture_insert", func(tx *gorm.DB) {
		statement = tx.Statement.SQL.String()
	}); err != nil {
		t.Fatal(err)
	}
	for _, reference := range []any{nil, map[string]string{"job_id": "business"}} {
		if err := runtime.EnqueueReference(t.Context(), db, "test.kind", "business", time.Now(), reference); err != nil {
			t.Fatal(err)
		}
		const target = `ON CONFLICT ("service_owner","kind","business_id") DO NOTHING`
		if !strings.Contains(statement, target) {
			t.Errorf("enqueue conflict target must include database owner: %s", statement)
		}
		insert, _, ok := strings.Cut(statement, " VALUES ")
		if !ok || strings.Contains(insert, "service_owner") {
			t.Errorf("enqueue must omit service_owner from inserted columns: %s", statement)
		}
	}
}

// Keep raw SQL writers aligned with the database-owned technical namespace;
// business table conflict keys and business idempotency identities stay intact.
func TestTechnicalSQLUsesDatabaseServiceOwner(t *testing.T) {
	targets := map[string]string{
		"infra.task_work":                "service_owner,kind,business_id",
		"infra.processed_event":          "service_owner,consumer,event_id",
		"infra.idempotency_record":       "service_owner,actor_id,idem_key",
		"infra.outbox":                   "service_owner,id,create_time",
		"infra.outbox_dead_letter":       "service_owner,event_id,event_create_time",
		"infra.outbox_dead_letter_audit": "service_owner,id",
		"audit.audit_log":                "service_owner,id,create_time",
	}
	insertPattern := regexp.MustCompile(`(?is)\binsert\s+into\s+((?:infra|audit)\.[a-z_]+)\s*\(([^)]*)\)`)
	conflictPattern := regexp.MustCompile(`(?is)\bon\s+conflict\s*\(([^)]*)\)`)
	ownerPattern := regexp.MustCompile(`(?i)\bservice_owner\b`)
	spaces := strings.NewReplacer(" ", "", "\n", "", "\t", "", "\r", "")
	seen := make(map[string]int)
	conflicts := make(map[string]int)
	files := token.NewFileSet()
	err := filepath.WalkDir(filepath.Join("..", ".."), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case "tests", "node_modules", ".git":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(files, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			lower := strings.ToLower(value)
			ownerSetting := ownerPattern.MatchString(lower) || strings.Contains(lower, "runtime_owner")
			setsConfig := strings.Contains(lower, "set_config") || strings.Contains(lower, "set local")
			if strings.Contains(lower, "column:service_owner") || setsConfig && ownerSetting {
				t.Errorf("%s: service owner must not be supplied by an application model or GUC", files.Position(literal.Pos()))
			}
			insert := insertPattern.FindStringSubmatch(lower)
			if len(insert) == 0 {
				return true
			}
			expected, technical := targets[insert[1]]
			if !technical {
				return true
			}
			seen[insert[1]]++
			if ownerPattern.MatchString(insert[2]) {
				t.Errorf("%s: %s INSERT must let the database default service_owner", files.Position(literal.Pos()), insert[1])
			}
			if !strings.Contains(lower, "on conflict") {
				return true
			}
			conflicts[insert[1]]++
			conflict := conflictPattern.FindStringSubmatch(lower)
			if len(conflict) != 2 || spaces.Replace(conflict[1]) != expected {
				t.Errorf("%s: %s conflict target must be (%s)", files.Position(literal.Pos()), insert[1], expected)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"infra.processed_event", "infra.idempotency_record", "infra.outbox", "infra.outbox_dead_letter", "infra.outbox_dead_letter_audit", "audit.audit_log"} {
		if seen[table] == 0 {
			t.Errorf("no INSERT inspected for %s", table)
		}
	}
	for table, minimum := range map[string]int{"infra.processed_event": 1, "infra.outbox_dead_letter": 1, "infra.idempotency_record": 5} {
		if conflicts[table] < minimum {
			t.Errorf("inspected %d %s conflict targets, want at least %d", conflicts[table], table, minimum)
		}
	}
}

func TestClaimRollbackDedupAndFence(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_TASK_DB_DSN")
	if dsn == "" {
		t.Skip("requires isolated task DB with db/schema.sql installed")
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
