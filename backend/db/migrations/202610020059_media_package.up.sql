CREATE TABLE media.package_job (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL,
  actor_id uuid NOT NULL,
  library_id uuid NOT NULL REFERENCES media.library(id),
  library_kind text NOT NULL CHECK(library_kind IN ('personal','project')),
  project_id uuid REFERENCES workspace.project(id),
  idem_key uuid NOT NULL,
  request_sha256 text NOT NULL CHECK(request_sha256 ~ '^[0-9a-f]{64}$'),
  archive_sha256 text NOT NULL CHECK(archive_sha256 ~ '^[0-9a-f]{64}$'),
  archive_bytes bigint NOT NULL CHECK(archive_bytes BETWEEN 1 AND 524288000),
  frozen bytea NOT NULL CHECK(octet_length(frozen) BETWEEN 2 AND 33554432),
  frozen_sha256 text NOT NULL CHECK(frozen_sha256 ~ '^[0-9a-f]{64}$'),
  status text NOT NULL CHECK(status IN ('needs_reconciliation','succeeded','cancelled')),
  revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
  library_revision integer NOT NULL CHECK(library_revision>=0),
  project_revision integer NOT NULL CHECK(project_revision>=0),
  result bytea CHECK(result IS NULL OR octet_length(result) BETWEEN 2 AND 1048576),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE(actor_id,idem_key),
  FOREIGN KEY(org_id,actor_id) REFERENCES identity."user"(org_id,id),
  CHECK((library_kind='project')=(project_id IS NOT NULL)),
  CHECK((status='succeeded')=(result IS NOT NULL))
);
CREATE INDEX package_actor_jobs ON media.package_job(org_id,actor_id,created_at DESC,id);
CREATE INDEX package_project_work ON media.package_job(project_id,status);
CREATE UNIQUE INDEX package_library_active ON media.package_job(library_id) WHERE status='needs_reconciliation';
CREATE TABLE media.package_command (
  actor_id uuid NOT NULL,
  org_id uuid NOT NULL,
  idem_key uuid NOT NULL,
  job_id uuid NOT NULL REFERENCES media.package_job(id),
  action text NOT NULL CHECK(action IN ('import','reconcile','cancel')),
  request_sha256 text NOT NULL CHECK(request_sha256 ~ '^[0-9a-f]{64}$'),
  response bytea NOT NULL CHECK(octet_length(response) BETWEEN 2 AND 1048576),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(actor_id,idem_key),
  FOREIGN KEY(org_id,actor_id) REFERENCES identity."user"(org_id,id)
);
CREATE FUNCTION media.reject_package_command_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'media package command is immutable' USING ERRCODE='42501'; END $$;
CREATE TRIGGER package_command_immutable BEFORE UPDATE OR DELETE ON media.package_command FOR EACH ROW EXECUTE FUNCTION media.reject_package_command_mutation();
CREATE TABLE media.package_object (
  job_id uuid NOT NULL REFERENCES media.package_job(id),
  object_index integer NOT NULL CHECK(object_index BETWEEN 0 AND 14999),
  object_key text NOT NULL UNIQUE,
  byte_size bigint NOT NULL CHECK(byte_size BETWEEN 1 AND 2147483648),
  content_type text NOT NULL,
  sha256 text NOT NULL CHECK(sha256 ~ '^[0-9a-f]{64}$'),
  status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','verified','removed')),
  PRIMARY KEY(job_id,object_index)
);
CREATE FUNCTION media.reject_package_frozen_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.status IN ('succeeded','cancelled') AND NEW IS DISTINCT FROM OLD THEN
    RAISE EXCEPTION 'terminal media package result is immutable' USING ERRCODE='42501';
  END IF;
  IF ROW(NEW.id,NEW.org_id,NEW.actor_id,NEW.library_id,NEW.library_kind,NEW.project_id,NEW.idem_key,NEW.request_sha256,NEW.archive_sha256,NEW.archive_bytes,NEW.frozen,NEW.frozen_sha256,NEW.created_at)
    IS DISTINCT FROM ROW(OLD.id,OLD.org_id,OLD.actor_id,OLD.library_id,OLD.library_kind,OLD.project_id,OLD.idem_key,OLD.request_sha256,OLD.archive_sha256,OLD.archive_bytes,OLD.frozen,OLD.frozen_sha256,OLD.created_at) THEN
    RAISE EXCEPTION 'media package frozen facts are immutable' USING ERRCODE='42501';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER package_frozen_immutable BEFORE UPDATE ON media.package_job FOR EACH ROW EXECUTE FUNCTION media.reject_package_frozen_mutation();
CREATE FUNCTION media.check_package_scope() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE owned media.library%ROWTYPE;
BEGIN
  SELECT * INTO STRICT owned FROM media.library WHERE id=NEW.library_id;
  IF owned.org_id<>NEW.org_id OR owned.kind<>NEW.library_kind OR owned.project_id IS DISTINCT FROM NEW.project_id OR
    (owned.kind='personal' AND owned.personal_actor_id<>NEW.actor_id) THEN
    RAISE EXCEPTION 'media package scope mismatch' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER package_scope BEFORE INSERT ON media.package_job FOR EACH ROW EXECUTE FUNCTION media.check_package_scope();
GRANT SELECT,INSERT,UPDATE(status,revision,library_revision,project_revision,result,updated_at) ON media.package_job TO lanverse_app;
GRANT SELECT,INSERT,UPDATE(status) ON media.package_object TO lanverse_app;
GRANT SELECT,INSERT ON media.package_command TO lanverse_app;
