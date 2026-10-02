ALTER TABLE media.library_item ADD COLUMN purged_at timestamptz;
ALTER TABLE media.library_item ADD CONSTRAINT library_item_purged_check CHECK (
 purged_at IS NULL OR (catalog_state='removed' AND folder_id IS NULL AND trashed_at IS NULL AND
 title='已永久清理' AND tags='[]'::jsonb AND source_label='' AND note='' AND NOT favorite AND
 (plain_text IS NULL OR plain_text='')));
GRANT UPDATE(purged_at) ON media.library_item TO lanverse_app;

CREATE TABLE media.purge_job (
 id uuid PRIMARY KEY,
 org_id uuid NOT NULL,
 actor_id uuid NOT NULL,
 library_id uuid NOT NULL REFERENCES media.library(id),
 scope_kind text NOT NULL CHECK(scope_kind IN ('personal','project')),
 project_id uuid REFERENCES workspace.project(id),
 item_count integer NOT NULL CHECK(item_count BETWEEN 1 AND 200),
 status text NOT NULL CHECK(status IN ('queued','running','needs_reconciliation','succeeded','partial_failed','failed','cancel_requested','cancelled')),
 stage text NOT NULL CHECK(stage IN ('frozen','verifying','removing','completed')),
 attempt integer NOT NULL DEFAULT 1 CHECK(attempt>0),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 cancellation_requested boolean NOT NULL DEFAULT false,
 needs_reconciliation boolean NOT NULL DEFAULT false,
 execution_unconfirmed boolean NOT NULL DEFAULT false,
 execution_id uuid,
 worker_fence uuid,
 lease_until timestamptz,
 process_ended boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(org_id,actor_id) REFERENCES identity."user"(org_id,id),
 CHECK((scope_kind='project')=(project_id IS NOT NULL)),
 CHECK((execution_id IS NULL AND worker_fence IS NULL AND lease_until IS NULL AND process_ended) OR
       (execution_id IS NOT NULL AND worker_fence IS NOT NULL AND lease_until IS NOT NULL))
);
CREATE INDEX purge_actor_jobs ON media.purge_job(org_id,actor_id,created_at DESC,id);
CREATE INDEX purge_project_work ON media.purge_job(project_id,status);

CREATE TABLE media.purge_item (
 job_id uuid NOT NULL REFERENCES media.purge_job(id),
 item_index integer NOT NULL CHECK(item_index BETWEEN 0 AND 199),
 item_id uuid NOT NULL REFERENCES media.library_item(id),
 asset_id uuid REFERENCES media.media_asset(id),
 frozen bytea NOT NULL CHECK(octet_length(frozen) BETWEEN 2 AND 1048576),
 status text NOT NULL CHECK(status IN ('blocked','queued','running','needs_reconciliation','succeeded','cancelled')),
 failure_code text CHECK(failure_code IN ('in_use','source_unavailable','source_changed','object_missing','object_mismatch','object_remove_unknown','object_receipt_unknown','worker_interrupted','cancelled')),
 PRIMARY KEY(job_id,item_index),
 UNIQUE(job_id,item_id)
);
CREATE UNIQUE INDEX purge_reserved_item ON media.purge_item(item_id)
 WHERE status IN ('queued','running','needs_reconciliation','succeeded');

CREATE TABLE media.purge_object (
 job_id uuid NOT NULL,
 item_index integer NOT NULL,
 object_key text NOT NULL,
 rendition_kind text NOT NULL CHECK(rendition_kind IN ('','thumb_256','thumb_640','poster','proxy_720p','waveform')),
 byte_size bigint CHECK(byte_size BETWEEN 1 AND 2147483648),
 sha256 text CHECK(sha256 ~ '^[0-9a-f]{64}$'),
 verified boolean NOT NULL DEFAULT false,
 removal_started boolean NOT NULL DEFAULT false,
 removed boolean NOT NULL DEFAULT false,
 PRIMARY KEY(job_id,item_index,rendition_kind),
 FOREIGN KEY(job_id,item_index) REFERENCES media.purge_item(job_id,item_index),
 CHECK(NOT removal_started OR (verified AND sha256 IS NOT NULL AND byte_size IS NOT NULL)),
 CHECK(NOT removed OR removal_started)
);
CREATE TABLE media.purge_command (
 actor_id uuid NOT NULL,
 org_id uuid NOT NULL,
 idem_key uuid NOT NULL,
 job_id uuid NOT NULL REFERENCES media.purge_job(id),
 action text NOT NULL CHECK(action IN ('create','cancel','reconcile')),
 request_sha256 text NOT NULL CHECK(request_sha256 ~ '^[0-9a-f]{64}$'),
 response_body bytea NOT NULL CHECK(octet_length(response_body) BETWEEN 2 AND 1048576),
 event_id uuid,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(actor_id,idem_key),
 FOREIGN KEY(org_id,actor_id) REFERENCES identity."user"(org_id,id)
);
CREATE FUNCTION media.reject_purge_command_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'media purge command is immutable' USING ERRCODE='42501'; END;
$$;
CREATE TRIGGER purge_command_immutable BEFORE UPDATE OR DELETE ON media.purge_command
 FOR EACH ROW EXECUTE FUNCTION media.reject_purge_command_mutation();

CREATE FUNCTION media.check_purge_scope() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE held media.library%ROWTYPE;
BEGIN
 SELECT * INTO STRICT held FROM media.library WHERE id=NEW.library_id;
 IF held.org_id<>NEW.org_id OR held.kind<>NEW.scope_kind OR held.project_id IS DISTINCT FROM NEW.project_id
  OR (held.kind='personal' AND held.personal_actor_id<>NEW.actor_id) THEN
  RAISE EXCEPTION 'media purge scope mismatch' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER purge_scope BEFORE INSERT ON media.purge_job FOR EACH ROW EXECUTE FUNCTION media.check_purge_scope();

GRANT SELECT,INSERT,UPDATE(status,stage,attempt,revision,cancellation_requested,needs_reconciliation,execution_unconfirmed,execution_id,worker_fence,lease_until,process_ended,updated_at)
 ON media.purge_job TO lanverse_app;
GRANT SELECT,INSERT,UPDATE(status,failure_code) ON media.purge_item TO lanverse_app;
GRANT SELECT,INSERT,UPDATE(byte_size,sha256,verified,removal_started,removed) ON media.purge_object TO lanverse_app;
GRANT SELECT,INSERT ON media.purge_command TO lanverse_app;
