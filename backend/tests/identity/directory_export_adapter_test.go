package identity_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func TestDirectoryExportCanonicalSchemaAndGrantParity(t *testing.T) {
	migration, err := os.ReadFile("../../db/migrations/011_identity_directory_export.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile("../../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(migration), "CREATE TABLE identity.directory_export")
	end := strings.LastIndex(string(migration), "COMMIT;")
	if start < 0 || end < start {
		t.Fatal("missing export DDL")
	}
	if !strings.Contains(string(schema), string(migration[start:end])) {
		t.Error("canonical schema does not contain exact export DDL/index/grant block")
	}
	grant := "GRANT SELECT,INSERT,UPDATE ON identity.directory_export TO lanverse_app;"
	if !strings.Contains(string(migration), grant) || !strings.Contains(string(schema), grant) {
		t.Error("application role lacks explicit least-privilege export grant")
	}
}

// cleanupDriver is a SQL protocol double: it does not create any database.
// It records whether the actual GORM adapter calls object I/O inside a SQL tx.
type cleanupDriver struct{ state *cleanupSQLState }
type cleanupSQLState struct {
	mu            sync.Mutex
	active        bool
	lastIsolation driver.IsolationLevel
	finalizeSQL   string
	now           time.Time
	job, object   uuid.UUID
	finalizeRows  int64
}
type cleanupConn struct{ state *cleanupSQLState }
type cleanupSQLTx struct{ state *cleanupSQLState }
type cleanupRows struct {
	values []driver.Value
	done   bool
}

func (d cleanupDriver) Open(string) (driver.Conn, error) { return &cleanupConn{state: d.state}, nil }
func (c *cleanupConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare unsupported by test double")
}
func (c *cleanupConn) Close() error { return nil }
func (c *cleanupConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}
func (c *cleanupConn) BeginTx(_ context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	c.state.active = true
	c.state.lastIsolation = opts.Isolation
	return &cleanupSQLTx{state: c.state}, nil
}
func (t *cleanupSQLTx) Commit() error {
	t.state.mu.Lock()
	defer t.state.mu.Unlock()
	t.state.active = false
	return nil
}
func (t *cleanupSQLTx) Rollback() error { return t.Commit() }
func (c *cleanupConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "private_object_id=NULL") {
		c.state.mu.Lock()
		defer c.state.mu.Unlock()
		c.state.finalizeSQL = query
		return driver.RowsAffected(c.state.finalizeRows), nil
	}
	return driver.RowsAffected(1), nil
}
func (c *cleanupConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if !strings.Contains(query, "identity.directory_export") {
		return nil, errors.New("unexpected test query")
	}
	s := c.state
	return &cleanupRows{values: []driver.Value{s.job.String(), uuid.NewString(), uuid.Nil.String(), uuid.NewString(), nil, uuid.NewString(), int64(1), int64(1), int64(0), []byte(`{}`), "completed", nil, "test", int64(1), s.now.Add(-time.Hour), s.now.Add(-time.Hour), s.now.Add(-time.Hour), s.now.Add(-time.Minute), s.object.String(), ""}}, nil
}
func (r *cleanupRows) Columns() []string {
	return []string{"id", "actor_id", "scope_id", "request_key", "team_id", "actor_session_id", "credential_epoch", "platform_epoch", "team_revision", "filter_snapshot", "status", "snapshot_payload", "snapshot_key_id", "row_count", "as_of", "created_at", "updated_at", "expires_at", "private_object_id", "error_category"}
}
func (r *cleanupRows) Close() error { return nil }
func (r *cleanupRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	copy(dest, r.values)
	r.done = true
	return nil
}
func cleanupTestStore(t *testing.T) (*pgidentity.DirectoryExportStore, *cleanupSQLState) {
	t.Helper()
	state := &cleanupSQLState{now: time.Now().UTC().Truncate(time.Microsecond), job: uuid.New(), object: uuid.New(), finalizeRows: 1}
	name := "directory-cleanup-" + uuid.NewString()
	sql.Register(name, cleanupDriver{state: state})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	gdb, err := gorm.Open(gormpostgres.New(gormpostgres.Config{Conn: db}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	return pgidentity.NewDirectoryExportStore(gdb), state
}
func TestDirectoryExportCleanupObjectIOOutsideSQLTransaction(t *testing.T) {
	store, state := cleanupTestStore(t)
	count, err := store.Cleanup(t.Context(), 1, state.now, func(ctx context.Context, job application.DirectoryExportJob) error {
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 30*time.Second {
			t.Error("cleanup object I/O has no bounded deadline")
		}
		state.mu.Lock()
		active := state.active
		state.mu.Unlock()
		if active {
			t.Error("object Remove called while SQL transaction remains open")
		}
		if job.ObjectID == nil || *job.ObjectID != state.object {
			t.Error("cleanup lost deterministic object ownership")
		}
		return nil
	})
	if err != nil || count != 1 {
		t.Fatal(count, err)
	}
}

func TestDirectoryExportCleanupCancellationAndFinalCAS(t *testing.T) {
	t.Run("cancelled delete retains claim", func(t *testing.T) {
		store, state := cleanupTestStore(t)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		count, err := store.Cleanup(ctx, 1, state.now, func(cleanupCtx context.Context, _ application.DirectoryExportJob) error {
			if _, ok := cleanupCtx.Deadline(); !ok {
				t.Fatal("object deletion did not receive deadline")
			}
			state.mu.Lock()
			active := state.active
			state.mu.Unlock()
			if active {
				t.Fatal("blocking object delete retains transaction")
			}
			cancel()
			<-cleanupCtx.Done()
			return cleanupCtx.Err()
		})
		if !errors.Is(err, context.Canceled) || count != 0 || state.finalizeSQL != "" {
			t.Fatal("uncertain deletion cleared its retry reference", count, err)
		}
	})
	t.Run("newer claim is not cleared", func(t *testing.T) {
		store, state := cleanupTestStore(t)
		state.finalizeRows = 0
		count, err := store.Cleanup(t.Context(), 1, state.now, func(context.Context, application.DirectoryExportJob) error { return nil })
		if err != nil || count != 0 {
			t.Fatal("stale cleanup reported completion", count, err)
		}
		for _, guard := range []string{"status='expired'", "updated_at=", "private_object_id IS NOT DISTINCT FROM", "expires_at<="} {
			if !strings.Contains(state.finalizeSQL, guard) {
				t.Fatalf("cleanup lacks %s CAS", guard)
			}
		}
	})
}
