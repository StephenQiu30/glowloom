-- Reviewed additive migration; never applied automatically by the API.
BEGIN;
CREATE TABLE identity.directory_export (
 id uuid PRIMARY KEY,
 actor_id uuid NOT NULL REFERENCES identity."user"(id) ON DELETE RESTRICT,
 team_id uuid REFERENCES workspace.team(id) ON DELETE RESTRICT,
 scope_kind text NOT NULL CHECK(scope_kind IN ('platform','team')),
 scope_id uuid NOT NULL,
 request_key uuid NOT NULL,
 actor_session_id uuid NOT NULL,
 credential_epoch bigint NOT NULL,
 platform_epoch bigint NOT NULL,
 team_revision bigint NOT NULL DEFAULT 0,
 filter_snapshot jsonb NOT NULL,
 snapshot_payload bytea,
 snapshot_key_id text NOT NULL,
 as_of timestamptz NOT NULL,
 status text NOT NULL CHECK(status IN ('pending','running','completed','failed','expired','revoked')),
 row_count integer NOT NULL CHECK(row_count BETWEEN 0 AND 10000),
 private_object_id uuid,
 expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 error_category text NOT NULL DEFAULT '',
 CHECK((scope_kind='platform' AND team_id IS NULL AND scope_id='00000000-0000-0000-0000-000000000000') OR (scope_kind='team' AND team_id IS NOT NULL AND scope_id=team_id)),
 UNIQUE(actor_id,scope_id,request_key)
);
CREATE INDEX directory_export_actor_idx ON identity.directory_export(actor_id,created_at DESC,id);
CREATE INDEX directory_export_work_idx ON identity.directory_export(status,expires_at,created_at,id);
COMMIT;
