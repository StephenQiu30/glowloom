package postgres

import (
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Exercise the same user-row SHARE lock retained by project authorization while
// an independent reader is active. A command may serialize its own receipts,
// but must not upgrade that identity lock after acquiring a project lock.
func TestMediatoolCommandLockPreservesAuthorizedReadersAndCommandExclusion(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_SHOT_PIPELINE_DB_DSN")
	if dsn == "" {
		t.Skip("isolated shot PostgreSQL required")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(parsed.Path, "_test") || parsed.Hostname() != "127.0.0.1" {
		t.Fatal("isolated loopback *_test database required")
	}
	db, err := gorm.Open(gormpostgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	org, actor := uuid.New(), uuid.New()
	if err = db.Exec(`INSERT INTO workspace.organization(id,name) VALUES(?,'command lock regression')`, org).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Exec(`DELETE FROM identity."user" WHERE id=?`, actor).Error; err != nil {
			t.Error(err)
		}
		if err := db.Exec(`DELETE FROM workspace.organization WHERE id=?`, org).Error; err != nil {
			t.Error(err)
		}
		if err := connection.Close(); err != nil {
			t.Error(err)
		}
	})
	if err = db.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,must_change_password) VALUES(?,?,?,'lock fixture','producer','synthetic',false)`, actor, org, actor.String()).Error; err != nil {
		t.Fatal(err)
	}
	reader := db.Begin()
	if reader.Error != nil {
		t.Fatal(reader.Error)
	}
	defer rollbackCommandTest(t, reader)
	if err = reader.Exec(`SELECT id FROM identity."user" WHERE id=? FOR SHARE`, actor).Error; err != nil {
		t.Fatal(err)
	}
	writer := db.Begin()
	if writer.Error != nil {
		t.Fatal(writer.Error)
	}
	defer rollbackCommandTest(t, writer)
	if err = writer.Exec(`SET LOCAL lock_timeout='2s'`).Error; err != nil {
		t.Fatal(err)
	}
	if err = writer.Exec(`SELECT id FROM identity."user" WHERE id=? FOR SHARE`, actor).Error; err != nil {
		t.Fatal(err)
	}
	if err = lockMediatoolPrincipal(writer, actor); err != nil {
		t.Fatalf("command upgraded identity SHARE lock and blocked an authorized reader: %v", err)
	}
	competing := db.Begin()
	if competing.Error != nil {
		t.Fatal(competing.Error)
	}
	if err = competing.Exec(`SET LOCAL lock_timeout='150ms'`).Error; err != nil {
		t.Fatal(err)
	}
	err = lockMediatoolPrincipal(competing, actor)
	rollbackCommandTest(t, competing)
	var lockError *pgconn.PgError
	if !errors.As(err, &lockError) || lockError.Code != "55P03" {
		t.Fatalf("same-principal command must wait for the original transaction: %v", err)
	}
	rollbackCommandTest(t, writer)
	next := db.Begin()
	if next.Error != nil {
		t.Fatal(next.Error)
	}
	defer rollbackCommandTest(t, next)
	if err = lockMediatoolPrincipal(next, actor); err != nil {
		t.Fatalf("original command did not release its transaction lock: %v", err)
	}
}

func rollbackCommandTest(t *testing.T, tx *gorm.DB) {
	t.Helper()
	if err := tx.Rollback().Error; err != nil && !errors.Is(err, sql.ErrTxDone) {
		t.Error(err)
	}
}
