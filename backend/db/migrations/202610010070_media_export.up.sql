CREATE SCHEMA mediatool;
CREATE TABLE mediatool.export_job (
 id uuid PRIMARY KEY,
 project_id uuid NOT NULL REFERENCES workspace.project(id),
 org_id uuid NOT NULL,
 actor_id uuid NOT NULL,
 actor_role text NOT NULL CHECK(actor_role IN ('admin','producer')),
 canvas_id uuid NOT NULL,
 node_id uuid NOT NULL,
 source_revision bigint NOT NULL CHECK(source_revision>0),
 frozen jsonb NOT NULL CHECK(jsonb_typeof(frozen)='object' AND octet_length(frozen::text)<=1048576),
 status text NOT NULL CHECK(status IN ('queued','running','review_required','succeeded','failed','cancel_requested','cancelled')),
 stage text NOT NULL,
 progress integer NOT NULL CHECK(progress BETWEEN 0 AND 100),
 attempt integer NOT NULL CHECK(attempt BETWEEN 1 AND 100),
 revision bigint NOT NULL CHECK(revision>0),
 asset_id uuid,
 sha256 text CHECK(sha256 ~ '^[a-f0-9]{64}$'),
 failure_code text,
 active_worker uuid,
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 CHECK((asset_id IS NULL)=(sha256 IS NULL)),
 CHECK(status NOT IN ('review_required','succeeded') OR asset_id IS NOT NULL),
 CHECK(status <> 'succeeded' OR progress=100)
);
CREATE INDEX export_job_project_source ON mediatool.export_job(project_id,canvas_id,node_id,id DESC);
CREATE TABLE mediatool.export_command (
 actor_id uuid NOT NULL,
 request_id uuid NOT NULL,
 job_id uuid NOT NULL REFERENCES mediatool.export_job(id),
 request_hash text NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),
 response jsonb NOT NULL,
 event_id uuid UNIQUE,
 event_action text CHECK(event_action IN ('start','cancel')),
 event_attempt integer,
 created_at timestamptz NOT NULL,
 PRIMARY KEY(actor_id,request_id),
 CHECK((event_id IS NULL)=(event_action IS NULL)),
 CHECK((event_id IS NULL)=(event_attempt IS NULL))
);

-- Commands and local activities use the same non-owning runtime role. Frozen
-- source identity, source JSON and creation time stay immutable through its ACL.
GRANT USAGE ON SCHEMA mediatool TO lanverse_app;
GRANT SELECT, INSERT, UPDATE (
 status, stage, progress, attempt, revision, asset_id, sha256, failure_code,
 active_worker, actor_id, actor_role, updated_at
) ON TABLE mediatool.export_job TO lanverse_app;
GRANT SELECT, INSERT ON TABLE mediatool.export_command TO lanverse_app;
