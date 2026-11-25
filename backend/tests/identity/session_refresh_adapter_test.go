package identity_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

type refreshSQLState struct {
	queries     []string
	failChild   bool
	consumeRows int64
}
type refreshDriver struct{ state *refreshSQLState }
type refreshConn struct{ state *refreshSQLState }
type refreshSQLTx struct{}

func (d refreshDriver) Open(string) (driver.Conn, error) { return &refreshConn{d.state}, nil }
func (c *refreshConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare not supported")
}
func (c *refreshConn) Close() error              { return nil }
func (c *refreshConn) Begin() (driver.Tx, error) { return refreshSQLTx{}, nil }
func (refreshSQLTx) Commit() error               { return nil }
func (refreshSQLTx) Rollback() error             { return nil }
func (c *refreshConn) ExecContext(_ context.Context, q string, _ []driver.NamedValue) (driver.Result, error) {
	c.state.queries = append(c.state.queries, q)
	if strings.HasPrefix(q, "INSERT INTO identity.session_refresh_token") && c.state.failChild {
		return nil, errors.New("synthetic child failure")
	}
	if strings.HasPrefix(q, "UPDATE identity.session_refresh_token SET consumed_at") {
		return driver.RowsAffected(c.state.consumeRows), nil
	}
	return driver.RowsAffected(1), nil
}
func refreshAdapterStore(t *testing.T, state *refreshSQLState) *pgidentity.Store {
	t.Helper()
	name := "refresh-adapter-" + uuid.NewString()
	sql.Register(name, refreshDriver{state})
	conn, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	gdb, err := gorm.Open(gormpostgres.New(gormpostgres.Config{Conn: conn}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	return pgidentity.NewStore(gdb)
}

func TestSessionV2RepositoryBinding(t *testing.T) {
	now := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	session := domain.AuthSession{ID: uuid.New(), AccountID: uuid.New()}
	old := refreshFixture(session, 0, now)
	next := refreshFixture(session, 1, now)
	for _, tc := range []struct {
		name   string
		change func(*domain.RefreshToken, *domain.RefreshToken, *uuid.UUID, *time.Time, *[]byte)
	}{
		{"wrong session", func(_, n *domain.RefreshToken, _ *uuid.UUID, _ *time.Time, _ *[]byte) { n.SessionID = uuid.New() }},
		{"wrong account", func(_, n *domain.RefreshToken, _ *uuid.UUID, _ *time.Time, _ *[]byte) { n.AccountID = uuid.New() }},
		{"wrong generation", func(_, n *domain.RefreshToken, _ *uuid.UUID, _ *time.Time, _ *[]byte) { n.Generation = 2 }},
		{"empty operation", func(_, _ *domain.RefreshToken, o *uuid.UUID, _ *time.Time, _ *[]byte) { *o = uuid.Nil }},
		{"invalid child digest", func(_, n *domain.RefreshToken, _ *uuid.UUID, _ *time.Time, _ *[]byte) { n.Digest = nil }},
		{"consumed child", func(_, n *domain.RefreshToken, _ *uuid.UUID, _ *time.Time, _ *[]byte) { n.ConsumedAt = &now }},
		{"empty receipt", func(_, _ *domain.RefreshToken, _ *uuid.UUID, _ *time.Time, r *[]byte) { *r = nil }},
		{"overlong replay", func(_, _ *domain.RefreshToken, _ *uuid.UUID, u *time.Time, _ *[]byte) { *u = now.Add(11 * time.Second) }},
		{"already consumed", func(o, _ *domain.RefreshToken, _ *uuid.UUID, _ *time.Time, _ *[]byte) { o.ConsumedAt = &now }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := &refreshSQLState{consumeRows: 1}
			store := refreshAdapterStore(t, state)
			o, n := old, next
			op := uuid.New()
			until := now.Add(10 * time.Second)
			receipt := []byte("opaque")
			tc.change(&o, &n, &op, &until, &receipt)
			err := store.EmailTransaction(t.Context(), func(tx identity.EmailTx) error { return tx.RotateRefresh(o, n, op, until, receipt) })
			if !errors.Is(err, identity.ErrSessionInvalid) {
				t.Fatalf("invalid binding error = %v", err)
			}
			if len(state.queries) != 0 {
				t.Fatal("invalid binding reached SQL mutation")
			}
		})
	}
	for _, tc := range []struct {
		name string
		rows int64
		fail bool
	}{{"single consume", 1, false}, {"lost consume", 0, false}, {"child failure", 1, true}} {
		t.Run(tc.name, func(t *testing.T) {
			state := &refreshSQLState{consumeRows: tc.rows, failChild: tc.fail}
			store := refreshAdapterStore(t, state)
			err := store.EmailTransaction(t.Context(), func(tx identity.EmailTx) error {
				return tx.RotateRefresh(old, next, uuid.New(), now.Add(10*time.Second), []byte("opaque"))
			})
			if tc.rows == 1 && !tc.fail {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatal("failed consumption committed")
			}
			rollback := false
			child := false
			for _, q := range state.queries {
				rollback = rollback || strings.HasPrefix(q, "ROLLBACK TO SAVEPOINT")
				child = child || strings.HasPrefix(q, "INSERT INTO identity.session_refresh_token")
			}
			if !rollback {
				t.Fatal("rotation did not roll back its savepoint")
			}
			if tc.rows == 0 && child {
				t.Fatal("duplicate consume attempted child")
			}
		})
	}
}
