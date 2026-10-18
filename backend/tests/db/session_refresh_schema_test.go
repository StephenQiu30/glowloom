package db_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// This static review gate never executes DDL against the connected business DB.
func TestSessionV2RepositorySchema(t *testing.T) {
	for _, path := range []string{"../../db/schema.sql", "../../db/migrations/016_identity_session_refresh.sql"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		sql := strings.ToLower(strings.Join(strings.Fields(string(data)), " "))
		for _, clause := range []string{
			"protocol_version smallint not null default 1", "idle_timeout_seconds integer not null default 43200", "refresh_generation bigint not null default 0",
			"create table identity.session_refresh_token", "references identity.user_session(id) on delete restrict",
			"token_digest bytea not null unique", "unique (session_id,generation)",
			"where consumed_at is null and revoked_at is null", "octet_length(token_digest) = 32",
			"(consumed_at is null) = (rotation_request_id is null)", "(replay_until is null) = (receipt_ciphertext is null)", "replay_until <= consumed_at + interval '10 seconds'",
			"grant select,insert,delete on identity.session_refresh_token to lanverse_app",
			"grant update (consumed_at,revoked_at,rotation_request_id,replay_until,receipt_ciphertext) on identity.session_refresh_token to lanverse_app",
		} {
			if !strings.Contains(sql, clause) {
				t.Errorf("%s missing %q", path, clause)
			}
		}
		if regexp.MustCompile(`grant\s+(?:all(?:\s+privileges)?|[^;]*\btruncate\b[^;]*|[^;]*\bupdate\s*(?:,|on)[^;]*)\s+(?:on\s+)?(?:table\s+)?identity\.session_refresh_token\b`).MatchString(sql) {
			t.Errorf("%s grants excessive privileges", path)
		}
	}
}
