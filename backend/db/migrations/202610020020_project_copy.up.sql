-- Copy targets remain unavailable to ordinary project/canvas/media reads until publication.
ALTER TABLE workspace.project DROP CONSTRAINT project_status_check;
ALTER TABLE workspace.project ADD CONSTRAINT project_status_check CHECK(status IN ('active','archived','copying'));

CREATE TABLE workspace.project_copy_job (
 id uuid PRIMARY KEY,
 org_id uuid NOT NULL REFERENCES workspace.organization(id),
 actor_id uuid NOT NULL REFERENCES identity."user"(id),
 request_id uuid NOT NULL DEFAULT gen_random_uuid(),
 idem_key uuid NOT NULL,
 request_sha256 text NOT NULL CHECK(request_sha256 ~ '^[0-9a-f]{64}$'),
 admission_response jsonb NOT NULL CHECK(jsonb_typeof(admission_response)='object'),
 source_project_id uuid NOT NULL REFERENCES workspace.project(id),
 source_revision integer NOT NULL CHECK(source_revision>0),
 target_project_id uuid NOT NULL UNIQUE REFERENCES workspace.project(id),
 target_name text NOT NULL CHECK(char_length(target_name) BETWEEN 1 AND 50),
 status text NOT NULL CHECK(status IN ('queued','running','failed','cancel_requested','cancelled','succeeded')),
 stage text NOT NULL CHECK(stage IN ('media','canvases','finalizing','cleanup','complete')),
 revision integer NOT NULL DEFAULT 1 CHECK(revision>0),
 attempt integer NOT NULL DEFAULT 0 CHECK(attempt>=0),
 worker_id uuid,
 started_at timestamptz,
 manifest jsonb NOT NULL CHECK(jsonb_typeof(manifest)='object'),
 workspace_snapshot jsonb NOT NULL CHECK(jsonb_typeof(workspace_snapshot)='object'),
 media_receipt jsonb,
 canvas_receipt jsonb,
 failure_code text NOT NULL DEFAULT '',
 retryable boolean NOT NULL DEFAULT false,
 needs_reconciliation boolean NOT NULL DEFAULT false,
 cancellation_requested boolean NOT NULL DEFAULT false,
 reconciliation_requested boolean NOT NULL DEFAULT false,
 execution_unconfirmed boolean NOT NULL DEFAULT false,
 create_time timestamptz NOT NULL DEFAULT now(),
 update_time timestamptz NOT NULL DEFAULT now(),
 UNIQUE(actor_id,idem_key),
 CHECK(source_project_id<>target_project_id),
 CHECK(NOT (retryable AND needs_reconciliation))
);
CREATE INDEX ix_project_copy_source_active ON workspace.project_copy_job(source_project_id,status) WHERE status NOT IN ('succeeded','cancelled');
CREATE INDEX ix_project_copy_org_recent ON workspace.project_copy_job(org_id,create_time DESC,id DESC);

CREATE TABLE workspace.project_copy_command (
 id uuid PRIMARY KEY,
 copy_job_id uuid NOT NULL REFERENCES workspace.project_copy_job(id),
 actor_id uuid NOT NULL REFERENCES identity."user"(id),
 idem_key uuid NOT NULL,
 request_sha256 text NOT NULL CHECK(request_sha256 ~ '^[0-9a-f]{64}$'),
 response_body jsonb NOT NULL CHECK(jsonb_typeof(response_body)='object'),
 create_time timestamptz NOT NULL DEFAULT now(),
 UNIQUE(actor_id,idem_key)
);
GRANT SELECT,INSERT ON workspace.project_copy_command TO lanverse_app;

CREATE TABLE canvas.project_copy_snapshot (
 id uuid PRIMARY KEY,
 job_id uuid NOT NULL UNIQUE REFERENCES workspace.project_copy_job(id) DEFERRABLE INITIALLY DEFERRED,
 org_id uuid NOT NULL REFERENCES workspace.organization(id),
 source_project_id uuid NOT NULL REFERENCES workspace.project(id),
 target_project_id uuid NOT NULL REFERENCES workspace.project(id),
 manifest_sha256 text NOT NULL CHECK(manifest_sha256 ~ '^[0-9a-f]{64}$'),
 document_count integer NOT NULL CHECK(document_count BETWEEN 0 AND 256),
 asset_mapping jsonb NOT NULL CHECK(jsonb_typeof(asset_mapping)='object'),
 documents jsonb NOT NULL CHECK(jsonb_typeof(documents)='array' AND octet_length(documents::text)<=33554432),
 create_time timestamptz NOT NULL DEFAULT now(),
 CHECK(source_project_id<>target_project_id)
);
CREATE TABLE canvas.project_copy_receipt (
 snapshot_id uuid PRIMARY KEY REFERENCES canvas.project_copy_snapshot(id),
 content_sha256 text NOT NULL CHECK(content_sha256 ~ '^[0-9a-f]{64}$'),
 document_count integer NOT NULL CHECK(document_count>=0),
 cleared_operation_bindings integer NOT NULL CHECK(cleared_operation_bindings>=0),
 create_time timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE media.project_copy_snapshot (
 id uuid PRIMARY KEY,
 job_id uuid NOT NULL UNIQUE REFERENCES workspace.project_copy_job(id) DEFERRABLE INITIALLY DEFERRED,
 org_id uuid NOT NULL REFERENCES workspace.organization(id),
 source_project_id uuid NOT NULL REFERENCES workspace.project(id),
 target_project_id uuid NOT NULL REFERENCES workspace.project(id),
 manifest_sha256 text NOT NULL CHECK(manifest_sha256 ~ '^[0-9a-f]{64}$'),
 asset_count integer NOT NULL CHECK(asset_count>=0),
 rendition_count integer NOT NULL CHECK(rendition_count>=0),
 content jsonb NOT NULL CHECK(jsonb_typeof(content)='object' AND octet_length(content::text)<=33554432),
 asset_mapping jsonb NOT NULL CHECK(jsonb_typeof(asset_mapping)='object'),
 create_time timestamptz NOT NULL DEFAULT now(),
 CHECK(source_project_id<>target_project_id)
);
CREATE TABLE media.project_copy_object (
 snapshot_id uuid NOT NULL REFERENCES media.project_copy_snapshot(id),
 target_asset_id uuid NOT NULL,
 rendition_kind text NOT NULL DEFAULT '',
 source_object_key text NOT NULL,
 target_object_key text NOT NULL UNIQUE,
 byte_size bigint CHECK(byte_size>0),
 content_type text NOT NULL,
 source_verified boolean NOT NULL DEFAULT false,
 write_started boolean NOT NULL DEFAULT false,
 sha256 text CHECK(sha256 ~ '^[0-9a-f]{64}$'),
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','verified','removed')),
 create_time timestamptz NOT NULL DEFAULT now(),
 update_time timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(snapshot_id,target_asset_id,rendition_kind),
 CHECK(source_object_key<>target_object_key)
);
CREATE TABLE media.project_copy_receipt (
 snapshot_id uuid PRIMARY KEY REFERENCES media.project_copy_snapshot(id),
 content_sha256 text NOT NULL CHECK(content_sha256 ~ '^[0-9a-f]{64}$'),
 asset_count integer NOT NULL CHECK(asset_count>=0),
 rendition_count integer NOT NULL CHECK(rendition_count>=0),
 create_time timestamptz NOT NULL DEFAULT now()
);
-- Frozen owner snapshots and completed receipts never accept runtime updates/deletes.
GRANT SELECT,INSERT ON canvas.project_copy_snapshot,canvas.project_copy_receipt,media.project_copy_snapshot,media.project_copy_receipt TO lanverse_app;
GRANT SELECT,INSERT ON workspace.project_copy_job,media.project_copy_object TO lanverse_app;
GRANT UPDATE(status,stage,revision,attempt,worker_id,started_at,media_receipt,canvas_receipt,failure_code,retryable,needs_reconciliation,cancellation_requested,reconciliation_requested,execution_unconfirmed,update_time) ON workspace.project_copy_job TO lanverse_app;
GRANT UPDATE(sha256,byte_size,source_verified,write_started,status,update_time) ON media.project_copy_object TO lanverse_app;

-- Copy owners insert private preset snapshots and only soft-delete owned target renditions.
GRANT INSERT ON workspace.style_preset TO lanverse_app;
GRANT UPDATE(is_delete,update_time) ON media.rendition TO lanverse_app;
