package identity_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// This test uses the existing connection, rolls back all DDL and fixtures, and
// never creates a database, schema namespace, role, service, or environment.
func TestUserSchemaConstraints(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_EXISTING_DB_DSN")
	if dsn == "" {
		t.Skip("existing connected database not supplied; schema verification not performed")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("connect existing database failed")
	}
	defer func() { _ = conn.Close(context.Background()) }()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, "SET LOCAL lock_timeout = '1s'; SET LOCAL statement_timeout = '10s'"); err != nil {
		t.Fatal(err)
	}
	var before int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM identity."user"`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	var installed bool
	if err := tx.QueryRow(ctx, "SELECT to_regclass('identity.auth_challenge') IS NOT NULL").Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if !installed {
		t.Fatal("database is missing identity.auth_challenge; initialize it from db/schema.sql")
	}
	var after int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM identity."user"`).Scan(&after); err != nil || after != before {
		t.Fatal("expand changed existing account count")
	}
	org, user, peer, team := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO workspace.organization(id,name) VALUES($1,'schema fixture')`, org); err != nil {
		t.Fatal(err)
	}
	for i, id := range []uuid.UUID{user, peer} {
		if _, err := tx.Exec(ctx, `INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,email_key,email_verified_at)
		VALUES($1,$2,$3,'fixture','producer','fixture-hash',$4,now())`, id, org, id.String(), []string{"one@schema.invalid", "two@schema.invalid"}[i]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO workspace.team(id,name,created_by) VALUES($1,'fixture team',$2)`, team, user); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES($1,$2,'team_admin','active',now(),'created')`, team, user); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO workspace.creation_scope(id,kind,owner_user_id,physical_scope_id) VALUES($1,'personal',$2,$3)`, uuid.New(), user, org); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, sql string
		args      []any
	}{
		{"duplicate email", `UPDATE identity."user" SET email_key='one@schema.invalid' WHERE id=$1`, []any{peer}},
		{"unnormalized email", `UPDATE identity."user" SET email_key='Two@schema.invalid' WHERE id=$1`, []any{peer}},
		{"duplicate membership", `INSERT INTO workspace.team_member(team_id,user_id,role,status,joined_at,source) VALUES($1,$2,'member','active',now(),'invited')`, []any{team, user}},
		{"mixed scope", `INSERT INTO workspace.creation_scope(id,kind,owner_user_id,team_id,physical_scope_id) VALUES($1,'personal',$2,$3,$4)`, []any{uuid.New(), peer, team, org}},
		{"second personal scope", `INSERT INTO workspace.creation_scope(id,kind,owner_user_id,physical_scope_id) VALUES($1,'personal',$2,$3)`, []any{uuid.New(), user, org}},
		{"suspended without resume", `UPDATE workspace.team SET status='suspended' WHERE id=$1`, []any{team}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tx.Exec(ctx, "SAVEPOINT rejected_input"); err != nil {
				t.Fatal(err)
			}
			_, err := tx.Exec(ctx, tc.sql, tc.args...)
			if err == nil {
				t.Error("invalid database state accepted")
			}
			if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT rejected_input"); err != nil {
				t.Fatal(err)
			}
		})
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM identity."user" WHERE id IN ($1,$2)`, user, peer).Scan(&after); err != nil || after != 0 {
		t.Fatal("fixture rollback was not confirmed")
	}
}
