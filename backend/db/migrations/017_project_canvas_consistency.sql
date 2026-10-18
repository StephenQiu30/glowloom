-- Additive project activity model. Run only against an explicitly selected database.
BEGIN;
SET LOCAL lock_timeout = '2s';
SET LOCAL statement_timeout = '30s';
CREATE TABLE IF NOT EXISTS workspace.project_activity (
    project_id uuid PRIMARY KEY REFERENCES workspace.project(id) ON DELETE CASCADE,
    modified_at timestamptz NOT NULL,
    basis text NOT NULL CHECK (basis IN ('created','metadata','content','legacy_metadata'))
);
CREATE INDEX IF NOT EXISTS project_activity_recent_idx
    ON workspace.project_activity (modified_at DESC,project_id DESC);
GRANT SELECT,INSERT,UPDATE ON workspace.project_activity TO lanverse_app;
INSERT INTO workspace.project_activity(project_id,modified_at,basis)
    SELECT id,update_time,'legacy_metadata' FROM workspace.project
    ON CONFLICT(project_id) DO NOTHING;
COMMIT;
