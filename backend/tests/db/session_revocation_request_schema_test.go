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

func TestPublicSessionRevocationRequestCanonicalSchema(t *testing.T) {
	schema := sessionRevocationSchema(t)
	for _, tc := range []struct {
		table   string
		columns int
		clauses []string
	}{
		{"session_revocation_request", 8, []string{
			"unique (account_id,request_session_id,idem_key)",
			"action text not null check (action in ('revoke_session','revoke_others'))",
			"completed_at timestamptz check (completed_at >= requested_at)",
			"check ((action='revoke_session' and target_session_id is not null) or (action='revoke_others' and target_session_id is null))",
		}},
		{"session_revocation_request_target", 2, []string{
			"request_id uuid not null references identity.session_revocation_request(id)",
			"session_id uuid not null references identity.session_revocation(session_id)",
			"primary key (request_id,session_id)",
		}},
		{"session_revocation_request_proof", 4, []string{
			"request_id uuid not null references identity.session_revocation_request(id)",
			"kind text not null check (kind in ('legacy','access','refresh'))",
			"token_digest bytea not null check (octet_length(token_digest)=32)",
			"csrf_hash bytea not null check (octet_length(csrf_hash)=32)",
			"primary key (request_id,kind)",
		}},
	} {
		t.Run(tc.table, func(t *testing.T) {
			definitions := regexp.MustCompile(`(?s)CREATE TABLE identity\.`+tc.table+` \((.*?)\n\);`).FindAllStringSubmatch(schema, -1)
			if len(definitions) != 1 {
				t.Fatalf("table definitions = %d, want 1", len(definitions))
			}
			definition := normalizeSessionRevocationSQL(definitions[0][1])
			for _, clause := range tc.clauses {
				if !strings.Contains(definition, clause) {
					t.Errorf("missing %q", clause)
				}
			}
			columns := regexp.MustCompile(`(?m)^    [a-z_]+ (?:uuid|text|bytea|timestamptz)\b`).FindAllString(definitions[0][1], -1)
			if len(columns) != tc.columns {
				t.Errorf("columns = %d, want %d", len(columns), tc.columns)
			}
			for _, forbidden := range []string{"cascade", "references identity.user_session", "references identity.\"user\"", "service_owner", "token_raw", "csrf_token"} {
				if strings.Contains(definition, forbidden) {
					t.Errorf("durable request contains %q", forbidden)
				}
			}
			sql := normalizeSessionRevocationSQL(schema)
			grants := regexp.MustCompile(`\bgrant\s+[^;]+?\s+on\s+(?:table\s+)?identity\.`+tc.table+`\b[^;]*;`).FindAllString(sql, -1)
			want := []string{"grant select,insert on table identity." + tc.table + " to glowloom_app,glowloom_identity;"}
			if tc.table == "session_revocation_request" {
				want = append(want, "grant update(completed_at) on table identity."+tc.table+" to glowloom_app,glowloom_identity;")
			}
			if strings.Join(grants, "\n") != strings.Join(want, "\n") {
				t.Errorf("grants = %q, want %q", grants, want)
			}
			if strings.Index(sql, want[0]) < strings.Index(sql, "for owned_table in") {
				t.Error("explicit request ACL precedes legacy privilege-copy loop")
			}
		})
	}
}

func TestPublicSessionRevocationRequestRuntimePrivileges(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("set GLOWLOOM_TEST_DB_DSN to a disposable PostgreSQL database initialized from schema.sql")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	connection, err := db.Open(ctx, dsn, noop.NewTracerProvider())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	pool, err := connection.DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"session_revocation_request", "session_revocation_request_target", "session_revocation_request_proof"} {
		for _, role := range []string{"glowloom_app", "glowloom_identity", "glowloom_creation", "glowloom_media", "glowloom_execution"} {
			owner := role == "glowloom_app" || role == "glowloom_identity"
			for _, privilege := range []string{"SELECT", "INSERT", "UPDATE", "DELETE", "TRUNCATE", "REFERENCES", "TRIGGER", "MAINTAIN"} {
				var got bool
				if err := pool.QueryRowContext(ctx, `SELECT has_table_privilege($1,$2,$3)`, role, "identity."+table, privilege).Scan(&got); err != nil {
					t.Fatal(err)
				}
				want := owner && (privilege == "SELECT" || privilege == "INSERT")
				if got != want {
					t.Errorf("%s %s %s = %t, want %t", role, privilege, table, got, want)
				}
			}
			rows, err := pool.QueryContext(ctx, `SELECT attname,has_column_privilege($1,attrelid,attnum,'UPDATE') FROM pg_attribute WHERE attrelid=$2::regclass AND attnum>0 AND NOT attisdropped`, role, "identity."+table)
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				var column string
				var got bool
				if err := rows.Scan(&column, &got); err != nil {
					_ = rows.Close()
					t.Fatal(err)
				}
				want := owner && table == "session_revocation_request" && column == "completed_at"
				if got != want {
					t.Errorf("%s UPDATE %s.%s = %t, want %t", role, table, column, got, want)
				}
			}
			if err := rows.Err(); err != nil {
				_ = rows.Close()
				t.Fatal(err)
			}
			if err := rows.Close(); err != nil {
				t.Fatal(err)
			}
		}
		var rls, runtimeOwner bool
		if err := pool.QueryRowContext(ctx, `SELECT relrowsecurity,pg_get_userbyid(relowner) IN ('glowloom_app','glowloom_identity','glowloom_creation','glowloom_media','glowloom_execution') FROM pg_class WHERE oid=$1::regclass`, "identity."+table).Scan(&rls, &runtimeOwner); err != nil {
			t.Fatal(err)
		}
		if rls || runtimeOwner {
			t.Errorf("%s RLS/runtime-owned = %t/%t", table, rls, runtimeOwner)
		}
	}
}
