package db_test

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

// Source checks never execute DDL or depend on an existing business database.
func TestSessionRevocationCanonicalSchema(t *testing.T) {
	schema := sessionRevocationSchema(t)
	definitions := regexp.MustCompile(`(?s)CREATE TABLE identity\.session_revocation \((.*?)\n\);`).FindAllStringSubmatch(schema, -1)
	if len(definitions) != 1 {
		t.Fatalf("session_revocation table definitions = %d, want 1", len(definitions))
	}
	definition := normalizeSessionRevocationSQL(definitions[0][1])
	for _, clause := range []string{
		"session_id uuid primary key check (session_id <> '00000000-0000-0000-0000-000000000000'::uuid)",
		"account_id uuid not null check (account_id <> '00000000-0000-0000-0000-000000000000'::uuid)",
		"request_session_id uuid not null check (request_session_id <> '00000000-0000-0000-0000-000000000000'::uuid)",
		"coordination_id uuid not null unique check (coordination_id <> '00000000-0000-0000-0000-000000000000'::uuid)",
		"expected_epoch bigint not null check (expected_epoch >= 0)",
		"requested_at timestamptz not null",
		"completed_at timestamptz check (completed_at >= requested_at)",
	} {
		if !strings.Contains(definition, clause) {
			t.Errorf("session_revocation definition missing %q", clause)
		}
	}
	columns := regexp.MustCompile(`(?m)^\s+[a-z_]+\s+(?:uuid|bigint|timestamptz)\b`).FindAllString(definitions[0][1], -1)
	if len(columns) != 7 {
		t.Errorf("session_revocation columns = %d, want 7", len(columns))
	}
	for _, forbidden := range []string{"references", "foreign key", "service_owner", "default", "cascade"} {
		if strings.Contains(definition, forbidden) {
			t.Errorf("durable source history must not contain %q", forbidden)
		}
	}
	if regexp.MustCompile(`(?i)\balter\s+table\s+(?:only\s+)?identity\.session_revocation\b`).MatchString(schema) {
		t.Error("session_revocation must keep its complete definition in its CREATE TABLE declaration")
	}
}

func TestSessionRevocationCanonicalPrivileges(t *testing.T) {
	schema := sessionRevocationSchema(t)
	sql := normalizeSessionRevocationSQL(schema)
	grants := regexp.MustCompile(`(?i)\bgrant\s+[^;]+?\s+on\s+(?:table\s+)?identity\.session_revocation\b[^;]*;`).FindAllString(sql, -1)
	want := []string{
		"grant select,insert on table identity.session_revocation to glowloom_app,glowloom_identity;",
		"grant update(completed_at) on table identity.session_revocation to glowloom_app,glowloom_identity;",
	}
	if strings.Join(grants, "\n") != strings.Join(want, "\n") {
		t.Errorf("session_revocation grants = %q, want only %q", grants, want)
	}
	if regexp.MustCompile(`(?i)\bgrant\s+[^;]+?\bon\s+all\s+tables\s+in\s+schema\s+identity\b`).MatchString(sql) {
		t.Error("schema-wide Identity grants could bypass session_revocation column ACLs")
	}
	// Existing Identity business capabilities stay intact. The initialization
	// loop copies these legacy privileges only to their owning domain role.
	for _, clause := range []string{
		"case when n.nspname='identity' then 'identity'",
		"else array['glowloom_' || owned_table.domain_owner] end",
		"grant usage on schema identity to glowloom_app;",
		"grant select,insert,update on table identity.\"user\" to glowloom_app;",
		"grant select,insert,update on table identity.platform_settings to glowloom_app;",
		"grant select,insert,update on table identity.user_session to glowloom_app;",
		"grant select,insert,delete on table identity.session_refresh_token to glowloom_app;",
		"grant update(revoked_at) on table identity.session_refresh_token to glowloom_app;",
		"grant update(receipt_ciphertext) on table identity.session_refresh_token to glowloom_app;",
		"grant update(replay_until) on table identity.session_refresh_token to glowloom_app;",
		"grant select,insert on table identity.security_event to glowloom_app;",
		"and not (rolcanlogin or rolsuper or rolcreatedb or rolcreaterole or rolreplication or rolbypassrls)",
		"pg_has_role(runtime_role, 'glowloom_app', 'member')",
	} {
		if !strings.Contains(sql, clause) {
			t.Errorf("Identity source transaction or role isolation contract missing %q", clause)
		}
	}
	// This Identity-only journal is not shared owner infrastructure and needs no
	// service_owner/RLS policy. Grant it only after the generic ACL-copy loop.
	if strings.Index(sql, want[0]) < strings.Index(sql, "for owned_table in") {
		t.Error("session_revocation explicit ACL must follow generic legacy ACL copying")
	}
}

// This optional catalog-only check reuses the existing disposable-DB fixture.
// It does not initialize schemas, create roles, or write application records.
func TestSessionRevocationRuntimePrivileges(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("set GLOWLOOM_TEST_DB_DSN to a disposable PostgreSQL database initialized from schema.sql")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	conn, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	pool, err := conn.DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	columns := []string{"session_id", "account_id", "request_session_id", "coordination_id", "expected_epoch", "requested_at", "completed_at"}
	for _, role := range []string{"glowloom_identity", "glowloom_app", "glowloom_creation", "glowloom_media", "glowloom_execution"} {
		allowed := role == "glowloom_identity" || role == "glowloom_app"
		for _, privilege := range []string{"SELECT", "INSERT", "UPDATE", "DELETE", "TRUNCATE", "REFERENCES", "TRIGGER", "MAINTAIN"} {
			var got bool
			if err := pool.QueryRowContext(ctx, `SELECT has_table_privilege($1,'identity.session_revocation',$2)`, role, privilege).Scan(&got); err != nil {
				t.Fatal(err)
			}
			want := allowed && (privilege == "SELECT" || privilege == "INSERT")
			if got != want {
				t.Errorf("%s %s session_revocation = %t, want %t", role, privilege, got, want)
			}
		}
		for _, column := range columns {
			var got bool
			if err := pool.QueryRowContext(ctx, `SELECT has_column_privilege($1,'identity.session_revocation',$2,'UPDATE')`, role, column).Scan(&got); err != nil {
				t.Fatal(err)
			}
			want := allowed && column == "completed_at"
			if got != want {
				t.Errorf("%s UPDATE session_revocation.%s = %t, want %t", role, column, got, want)
			}
		}
	}
	for _, table := range []string{"user", "platform_settings", "user_session", "session_refresh_token", "security_event"} {
		for _, role := range []string{"glowloom_identity", "glowloom_creation", "glowloom_media", "glowloom_execution"} {
			var got bool
			if err := pool.QueryRowContext(ctx, `SELECT has_table_privilege($1,$2,'SELECT')`, role, `identity."`+table+`"`).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != (role == "glowloom_identity") {
				t.Errorf("%s SELECT identity.%s = %t", role, table, got)
			}
		}
	}
	for _, probe := range []struct{ table, column string }{
		{"user", "id"}, {"platform_settings", "singleton"}, {"user_session", "revoked_at"},
		{"session_refresh_token", "revoked_at"}, {"session_refresh_token", "receipt_ciphertext"}, {"session_refresh_token", "replay_until"},
	} {
		var allowed bool
		if err := pool.QueryRowContext(ctx, `SELECT has_column_privilege('glowloom_identity',$1,$2,'UPDATE')`, `identity."`+probe.table+`"`, probe.column).Scan(&allowed); err != nil || !allowed {
			t.Errorf("Identity source transaction requires UPDATE identity.%s.%s: allowed=%t, error=%v", probe.table, probe.column, allowed, err)
		}
	}
	var usage, auditInsert, rls, runtimeOwner bool
	if err := pool.QueryRowContext(ctx, `SELECT
		has_schema_privilege('glowloom_identity','identity','USAGE'),
		has_table_privilege('glowloom_identity','identity.security_event','INSERT'),
		relrowsecurity,
		pg_get_userbyid(relowner) IN ('glowloom_app','glowloom_identity','glowloom_creation','glowloom_media','glowloom_execution')
		FROM pg_class WHERE oid='identity.session_revocation'::regclass`).Scan(&usage, &auditInsert, &rls, &runtimeOwner); err != nil {
		t.Fatal(err)
	}
	if !usage || !auditInsert || rls || runtimeOwner {
		t.Errorf("Identity usage/audit insert/journal RLS/runtime-owned = %t/%t/%t/%t, want true/true/false/false", usage, auditInsert, rls, runtimeOwner)
	}
}

func sessionRevocationSchema(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func normalizeSessionRevocationSQL(statement string) string {
	return strings.ToLower(strings.Join(strings.Fields(statement), " "))
}
