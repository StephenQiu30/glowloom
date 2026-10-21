package db_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// This static review gate never executes DDL against the connected business DB.
func TestSessionV2RepositorySchema(t *testing.T) {
	data, err := os.ReadFile("../../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(strings.Join(strings.Fields(string(data)), " "))
	for _, clause := range []string{
		"create table identity.session_refresh_token",
		"protocol_version smallint default 1 not null",
		"idle_timeout_seconds integer default 43200 not null",
		"refresh_generation bigint default 0 not null",
		"references identity.user_session(id) on delete restrict",
		"unique (session_id, generation)",
		"unique (token_digest)",
		"where ((consumed_at is null) and (revoked_at is null))",
		"octet_length(token_digest) = 32",
		"(consumed_at is null) = (rotation_request_id is null)",
		"(replay_until is null) = (receipt_ciphertext is null)",
		"replay_until <= (consumed_at + '00:00:10'::interval)",
		"grant select,insert,delete on table identity.session_refresh_token to glowloom_app",
		"grant update(consumed_at) on table identity.session_refresh_token to glowloom_app",
		"grant update(revoked_at) on table identity.session_refresh_token to glowloom_app",
		"grant update(rotation_request_id) on table identity.session_refresh_token to glowloom_app",
		"grant update(replay_until) on table identity.session_refresh_token to glowloom_app",
		"grant update(receipt_ciphertext) on table identity.session_refresh_token to glowloom_app",
	} {
		if !strings.Contains(sql, clause) {
			t.Errorf("schema.sql missing %q", clause)
		}
	}
	if regexp.MustCompile(`grant\s+(?:all(?:\s+privileges)?|[^;]*\btruncate\b[^;]*|[^;]*\bupdate\s*(?:,|on)[^;]*)\s+(?:on\s+)?(?:table\s+)?identity\.session_refresh_token\b`).MatchString(sql) {
		t.Error("schema.sql grants excessive privileges on identity.session_refresh_token")
	}
}
