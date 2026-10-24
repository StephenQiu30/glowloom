-- Capability only: preview and explicitly select each historical job via the
-- project-copy-primary operator command. Never infer primary IDs in bulk SQL.
CREATE TABLE IF NOT EXISTS workspace.project_copy_primary_repair (
 job_id uuid PRIMARY KEY REFERENCES workspace.project_copy_job(id),
 org_id uuid NOT NULL REFERENCES workspace.organization(id),
 target_project_id uuid NOT NULL REFERENCES workspace.project(id),
 source_document_id uuid NOT NULL,
 target_document_id uuid NOT NULL,
 review_hash text NOT NULL CHECK(review_hash ~ '^[0-9a-f]{64}$'),
 before_revision bigint NOT NULL CHECK(before_revision > 0),
 after_revision bigint NOT NULL CHECK(after_revision = before_revision + 1),
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 500),
 operator_name text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
REVOKE ALL ON workspace.project_copy_primary_repair FROM PUBLIC, glowloom_app;
