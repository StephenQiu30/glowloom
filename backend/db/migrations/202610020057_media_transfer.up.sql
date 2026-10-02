CREATE TABLE media.transfer_job (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL,
  actor_id uuid NOT NULL,
  source_library_id uuid NOT NULL REFERENCES media.library(id),
  target_library_id uuid NOT NULL REFERENCES media.library(id),
  source_kind text NOT NULL CHECK(source_kind IN ('personal','project')),
  target_kind text NOT NULL CHECK(target_kind IN ('personal','project')),
  source_project_id uuid REFERENCES workspace.project(id),
  target_project_id uuid REFERENCES workspace.project(id),
  target_folder_id uuid,
  target_folder_revision integer NOT NULL CHECK(target_folder_revision>=0),
  source_revision integer NOT NULL CHECK(source_revision>=0),
  target_revision integer NOT NULL CHECK(target_revision>=0),
  project_revision integer NOT NULL CHECK(project_revision>0),
  manifest_sha256 text NOT NULL CHECK(manifest_sha256 ~ '^[0-9a-f]{64}$'),
  item_count integer NOT NULL CHECK(item_count BETWEEN 1 AND 200),
  status text NOT NULL CHECK(status IN ('queued','running','needs_reconciliation','succeeded','partial_failed','failed','cancel_requested','cancelled')),
  stage text NOT NULL CHECK(stage IN ('frozen','copying','registering','cleanup','completed')),
  attempt integer NOT NULL DEFAULT 1 CHECK(attempt>0),
  revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
  needs_reconciliation boolean NOT NULL DEFAULT false,
  cancellation_requested boolean NOT NULL DEFAULT false,
  execution_unconfirmed boolean NOT NULL DEFAULT false,
  execution_id uuid,
  worker_fence uuid,
  lease_until timestamptz,
  process_ended boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY(org_id,actor_id) REFERENCES identity."user"(org_id,id),
  CHECK(source_library_id<>target_library_id AND source_kind<>target_kind),
  CHECK((source_kind='project')=(source_project_id IS NOT NULL)),
  CHECK((target_kind='project')=(target_project_id IS NOT NULL)),
  CHECK((target_folder_id IS NULL AND target_folder_revision=0) OR (target_folder_id IS NOT NULL AND target_folder_revision>0)),
  CHECK((execution_id IS NULL AND worker_fence IS NULL AND lease_until IS NULL AND process_ended) OR
        (execution_id IS NOT NULL AND worker_fence IS NOT NULL AND lease_until IS NOT NULL))
);
CREATE INDEX transfer_actor_jobs ON media.transfer_job(org_id,actor_id,created_at DESC,id);
CREATE INDEX transfer_source_work ON media.transfer_job(source_project_id,status);
CREATE INDEX transfer_target_work ON media.transfer_job(target_project_id,status);

CREATE TABLE media.transfer_item (
  job_id uuid NOT NULL REFERENCES media.transfer_job(id),
  item_index integer NOT NULL CHECK(item_index BETWEEN 0 AND 199),
  source_item_id uuid NOT NULL,
  target_item_id uuid NOT NULL,
  source_asset_id uuid REFERENCES media.media_asset(id),
  target_asset_id uuid,
  frozen bytea NOT NULL CHECK(octet_length(frozen) BETWEEN 2 AND 1048576),
  status text NOT NULL CHECK(status IN ('queued','running','succeeded','failed','needs_reconciliation','cancelled')),
  failure_code text CHECK(failure_code IN ('source_changed','source_unavailable','target_folder_changed','object_missing','object_mismatch','object_write_unknown','object_receipt_unknown','object_cleanup_unknown','registration_failed','worker_interrupted','cancelled')),
  PRIMARY KEY(job_id,item_index),
  UNIQUE(job_id,source_item_id),
  UNIQUE(target_item_id),
  CHECK((source_asset_id IS NULL)=(target_asset_id IS NULL)),
  CHECK(target_asset_id IS NULL OR target_asset_id=target_item_id)
);
CREATE TABLE media.transfer_object (
  job_id uuid NOT NULL,
  item_index integer NOT NULL,
  rendition_kind text NOT NULL DEFAULT '' CHECK(rendition_kind IN ('','thumb_256','thumb_640','poster','proxy_720p','waveform')),
  source_object_key text NOT NULL,
  target_object_key text NOT NULL UNIQUE,
  byte_size bigint CHECK(byte_size BETWEEN 1 AND 2147483648),
  content_type text NOT NULL,
  sha256 text CHECK(sha256 ~ '^[0-9a-f]{64}$'),
  source_verified boolean NOT NULL DEFAULT false,
  write_started boolean NOT NULL DEFAULT false,
  status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','verified','removed')),
  PRIMARY KEY(job_id,item_index,rendition_kind),
  FOREIGN KEY(job_id,item_index) REFERENCES media.transfer_item(job_id,item_index)
);
CREATE TABLE media.transfer_command (
  actor_id uuid NOT NULL,
  org_id uuid NOT NULL,
  idem_key uuid NOT NULL,
  job_id uuid NOT NULL REFERENCES media.transfer_job(id),
  action text NOT NULL CHECK(action IN ('create','cancel','retry','reconcile')),
  request_sha256 text NOT NULL CHECK(request_sha256 ~ '^[0-9a-f]{64}$'),
  response_body bytea NOT NULL CHECK(octet_length(response_body) BETWEEN 2 AND 1048576),
  event_id uuid,
  event_action text CHECK(event_action IN ('start','cancel','reconcile')),
  event_attempt integer CHECK(event_attempt>0),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(actor_id,idem_key),
  FOREIGN KEY(org_id,actor_id) REFERENCES identity."user"(org_id,id),
  CHECK((event_id IS NULL AND event_action IS NULL AND event_attempt IS NULL) OR
    (event_id IS NOT NULL AND event_action IS NOT NULL AND event_attempt IS NOT NULL))
);
CREATE FUNCTION media.reject_transfer_command_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'media transfer command is immutable' USING ERRCODE='42501'; END;
$$;
CREATE TRIGGER transfer_command_immutable BEFORE UPDATE OR DELETE ON media.transfer_command
  FOR EACH ROW EXECUTE FUNCTION media.reject_transfer_command_mutation();

CREATE FUNCTION media.check_transfer_scope() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE source media.library%ROWTYPE; target media.library%ROWTYPE;
BEGIN
  SELECT * INTO STRICT source FROM media.library WHERE id=NEW.source_library_id;
  SELECT * INTO STRICT target FROM media.library WHERE id=NEW.target_library_id;
  IF source.org_id<>NEW.org_id OR target.org_id<>NEW.org_id OR source.kind<>NEW.source_kind OR target.kind<>NEW.target_kind
    OR source.project_id IS DISTINCT FROM NEW.source_project_id OR target.project_id IS DISTINCT FROM NEW.target_project_id
    OR (source.kind='personal' AND source.personal_actor_id<>NEW.actor_id)
    OR (target.kind='personal' AND target.personal_actor_id<>NEW.actor_id) THEN
    RAISE EXCEPTION 'media transfer scope mismatch' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER transfer_scope BEFORE INSERT ON media.transfer_job FOR EACH ROW EXECUTE FUNCTION media.check_transfer_scope();

GRANT SELECT,INSERT,UPDATE(status,stage,attempt,revision,needs_reconciliation,cancellation_requested,execution_unconfirmed,execution_id,worker_fence,lease_until,process_ended,updated_at)
  ON media.transfer_job TO lanverse_app;
GRANT SELECT,INSERT,UPDATE(status,failure_code) ON media.transfer_item TO lanverse_app;
GRANT SELECT,INSERT,UPDATE(byte_size,sha256,source_verified,write_started,status) ON media.transfer_object TO lanverse_app;
GRANT SELECT,INSERT ON media.transfer_command TO lanverse_app;
