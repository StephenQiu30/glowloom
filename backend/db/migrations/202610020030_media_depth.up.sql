CREATE TABLE mediatool.depth_job (
 id uuid PRIMARY KEY,
 project_id uuid NOT NULL REFERENCES workspace.project(id),
 org_id uuid NOT NULL,
 actor_id uuid NOT NULL,
 actor_role text NOT NULL CHECK(actor_role IN ('admin','producer')),
 canvas_id uuid NOT NULL,
 node_id uuid NOT NULL,
 source_revision bigint NOT NULL CHECK(source_revision>0),
 source_asset_id uuid NOT NULL,
 source_asset_revision bigint NOT NULL CHECK(source_asset_revision>0),
 source_sha256 text NOT NULL CHECK(source_sha256 ~ '^[a-f0-9]{64}$'),
 profile_id text NOT NULL CHECK(profile_id='vda-small-relative-v1'),
 frozen jsonb NOT NULL CHECK(jsonb_typeof(frozen)='object' AND octet_length(frozen::text)<=1048576),
 frozen_sha256 text NOT NULL CHECK(frozen_sha256 ~ '^[a-f0-9]{64}$'),
 status text NOT NULL CHECK(status IN ('queued','running','review_required','succeeded','failed','cancel_requested','cancelled')),
 stage text NOT NULL CHECK(length(stage) BETWEEN 1 AND 32),
 attempt integer NOT NULL CHECK(attempt BETWEEN 1 AND 100),
 revision bigint NOT NULL CHECK(revision>0),
 process_state text NOT NULL CHECK(process_state IN ('none','started','ended','unknown')),
 active_worker uuid,
 failure_code text,
 retryable boolean NOT NULL DEFAULT false,
 needs_reconciliation boolean NOT NULL DEFAULT false,
 execution_unconfirmed boolean NOT NULL DEFAULT false,
 cancellation_requested boolean NOT NULL DEFAULT false,
 reconciliation_requested boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 CHECK(NOT(retryable AND (needs_reconciliation OR execution_unconfirmed))),
 CHECK(NOT execution_unconfirmed OR (needs_reconciliation AND active_worker IS NOT NULL)),
 CHECK(status NOT IN ('review_required','succeeded','cancelled') OR (active_worker IS NULL AND NOT execution_unconfirmed AND process_state IN ('none','ended')))
);
CREATE INDEX depth_job_project_source ON mediatool.depth_job(project_id,canvas_id,node_id,id DESC);
CREATE TABLE mediatool.depth_result_intent (
 job_id uuid NOT NULL REFERENCES mediatool.depth_job(id),
 attempt integer NOT NULL CHECK(attempt BETWEEN 1 AND 100),
 asset_id uuid NOT NULL UNIQUE,
 output_sha256 text NOT NULL CHECK(output_sha256 ~ '^[a-f0-9]{64}$'),
 artifact_sha256 text NOT NULL CHECK(artifact_sha256 ~ '^[a-f0-9]{64}$'),
 artifact jsonb NOT NULL CHECK(jsonb_typeof(artifact)='object' AND octet_length(artifact::text)<=1048576),
 created_at timestamptz NOT NULL,
 PRIMARY KEY(job_id,attempt)
);
CREATE TABLE mediatool.depth_object (
 job_id uuid NOT NULL,
 attempt integer NOT NULL,
 kind text NOT NULL CHECK(kind IN ('original','poster','proxy_720p')),
 object_key text NOT NULL UNIQUE,
 sha256 text NOT NULL CHECK(sha256 ~ '^[a-f0-9]{64}$'),
 byte_size bigint NOT NULL CHECK(byte_size BETWEEN 1 AND 524288000),
 mime_type text NOT NULL,
 write_started boolean NOT NULL DEFAULT false,
 delete_started boolean NOT NULL DEFAULT false,
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','verified','removed')),
 updated_at timestamptz NOT NULL,
 PRIMARY KEY(job_id,attempt,kind),
 FOREIGN KEY(job_id,attempt) REFERENCES mediatool.depth_result_intent(job_id,attempt)
);
CREATE TABLE mediatool.depth_command (
 actor_id uuid NOT NULL,
 request_id uuid NOT NULL,
 job_id uuid NOT NULL REFERENCES mediatool.depth_job(id),
 request_hash text NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),
 response jsonb NOT NULL,
 event_id uuid UNIQUE,
 event_action text CHECK(event_action IN ('start','cancel','reconcile')),
 event_attempt integer,
 created_at timestamptz NOT NULL,
 PRIMARY KEY(actor_id,request_id),
 CHECK((event_id IS NULL)=(event_action IS NULL)),
 CHECK((event_id IS NULL)=(event_attempt IS NULL))
);
GRANT SELECT,INSERT ON mediatool.depth_job,mediatool.depth_result_intent,mediatool.depth_object,mediatool.depth_command TO lanverse_app;
GRANT UPDATE(status,stage,attempt,revision,process_state,active_worker,failure_code,retryable,needs_reconciliation,execution_unconfirmed,cancellation_requested,reconciliation_requested,updated_at) ON mediatool.depth_job TO lanverse_app;
GRANT UPDATE(write_started,delete_started,status,updated_at) ON mediatool.depth_object TO lanverse_app;
