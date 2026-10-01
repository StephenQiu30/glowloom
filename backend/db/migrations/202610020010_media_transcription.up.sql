CREATE TABLE mediatool.transcription_job (
 id uuid PRIMARY KEY,
 project_id uuid NOT NULL REFERENCES workspace.project(id),
 org_id uuid NOT NULL,
 actor_id uuid NOT NULL,
 actor_role text NOT NULL CHECK(actor_role IN ('admin','producer')),
 canvas_id uuid NOT NULL,
 node_id uuid NOT NULL,
 source_revision bigint NOT NULL CHECK(source_revision>0),
 language text NOT NULL CHECK(length(language) BETWEEN 2 AND 4),
 frozen jsonb NOT NULL CHECK(jsonb_typeof(frozen)='object' AND octet_length(frozen::text)<=1048576),
 status text NOT NULL CHECK(status IN ('queued','running','succeeded','failed','cancel_requested','cancelled')),
 stage text NOT NULL,
 progress integer NOT NULL CHECK(progress BETWEEN 0 AND 100),
 attempt integer NOT NULL CHECK(attempt BETWEEN 1 AND 100),
 revision bigint NOT NULL CHECK(revision>0),
 inference_state text NOT NULL CHECK(inference_state IN ('none','submitted','terminal','unknown')),
 result jsonb CHECK(result IS NULL OR (jsonb_typeof(result)='object' AND octet_length(result::text)<=8388608)),
 result_sha256 text CHECK(result_sha256 ~ '^[a-f0-9]{64}$'),
 failure_code text,
 active_worker uuid,
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 CHECK((result IS NULL)=(result_sha256 IS NULL)),
 CHECK(status <> 'succeeded' OR (progress=100 AND result IS NOT NULL AND inference_state='terminal')),
 CHECK(status <> 'cancelled' OR inference_state IN ('none','terminal'))
);
CREATE INDEX transcription_job_project_source ON mediatool.transcription_job(project_id,canvas_id,node_id,id DESC);
CREATE TABLE mediatool.transcription_command (
 actor_id uuid NOT NULL,
 request_id uuid NOT NULL,
 job_id uuid NOT NULL REFERENCES mediatool.transcription_job(id),
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
GRANT SELECT, INSERT, UPDATE (
 status, stage, progress, attempt, revision, inference_state, result,
 result_sha256, failure_code, active_worker, actor_id, actor_role, updated_at
) ON TABLE mediatool.transcription_job TO lanverse_app;
GRANT SELECT, INSERT ON TABLE mediatool.transcription_command TO lanverse_app;
