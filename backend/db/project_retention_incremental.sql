-- Apply only to an existing database after review; does not delete business data.
-- psql "$GLOWLOOM_SCHEMA_DB_DSN" -X --single-transaction -v ON_ERROR_STOP=1 -f backend/db/project_retention_incremental.sql
CREATE TABLE IF NOT EXISTS workspace.project_retention_job (
 project_id uuid NOT NULL REFERENCES workspace.project(id),
 org_id uuid NOT NULL,
 owner_scope_id uuid NOT NULL,
 deleted_revision bigint NOT NULL CHECK(deleted_revision>0),
 purge_after timestamptz NOT NULL,
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','completed','skipped')),
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts>=0),
 last_error text,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 completed_at timestamptz,
 PRIMARY KEY(project_id,deleted_revision)
);
CREATE INDEX IF NOT EXISTS project_retention_pending ON workspace.project_retention_job(purge_after,project_id) WHERE status='pending';
CREATE INDEX IF NOT EXISTS project_recycle_expiry ON workspace.project(purge_after,id) WHERE is_delete;
GRANT SELECT,INSERT,UPDATE ON workspace.project_retention_job TO glowloom_app;
-- Only the internal retention transaction uses these; media entities and blobs are retained.
GRANT DELETE ON canvas.edge,canvas.node TO glowloom_app;
GRANT UPDATE ON canvas.save_receipt TO glowloom_app;
GRANT DELETE ON workspace.prompt_draft TO glowloom_app;
GRANT UPDATE ON workspace.prompt_draft_receipt TO glowloom_app;
