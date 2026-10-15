-- Minimal dependencies of current user-system content readers.
-- No storage profile, provider route, object binding, or existing row is changed.
-- Apply only after installed 005/006/007/009 are verified; see final integration handoff.
SET LOCAL lock_timeout = '2s';
SET LOCAL statement_timeout = '30s';

ALTER TABLE media.media_asset ADD COLUMN IF NOT EXISTS object_location_id uuid;
ALTER TABLE media.rendition ADD COLUMN IF NOT EXISTS object_location_id uuid;
ALTER TABLE media.project_copy_object ADD COLUMN IF NOT EXISTS source_location_id uuid;
ALTER TABLE media.project_copy_object ADD COLUMN IF NOT EXISTS target_location_id uuid;
ALTER TABLE media.transfer_object ADD COLUMN IF NOT EXISTS source_location_id uuid;
ALTER TABLE media.transfer_object ADD COLUMN IF NOT EXISTS target_location_id uuid;


CREATE OR REPLACE FUNCTION media.check_package_scope() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE owned media.library%ROWTYPE;
BEGIN
  SELECT * INTO STRICT owned FROM media.library WHERE id=NEW.library_id;
  IF owned.org_id<>NEW.org_id OR owned.kind<>NEW.library_kind OR owned.project_id IS DISTINCT FROM NEW.project_id OR
    (owned.kind='personal' AND owned.personal_actor_id<>NEW.actor_id) THEN
    RAISE EXCEPTION 'media package scope mismatch' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION media.reject_package_command_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN RAISE EXCEPTION 'media package command is immutable' USING ERRCODE='42501'; END $$;

CREATE OR REPLACE FUNCTION media.reject_package_frozen_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
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

CREATE TABLE IF NOT EXISTS media.package_job (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    library_id uuid NOT NULL,
    library_kind text NOT NULL,
    project_id uuid,
    idem_key uuid NOT NULL,
    request_sha256 text NOT NULL,
    archive_sha256 text NOT NULL,
    archive_bytes bigint NOT NULL,
    frozen bytea NOT NULL,
    frozen_sha256 text NOT NULL,
    status text NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    library_revision integer NOT NULL,
    project_revision integer NOT NULL,
    result bytea,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT package_job_archive_bytes_check CHECK (((archive_bytes >= 1) AND (archive_bytes <= 524288000))),
    CONSTRAINT package_job_archive_sha256_check CHECK ((archive_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT package_job_check CHECK (((library_kind = 'project'::text) = (project_id IS NOT NULL))),
    CONSTRAINT package_job_check1 CHECK (((status = 'succeeded'::text) = (result IS NOT NULL))),
    CONSTRAINT package_job_frozen_check CHECK (((octet_length(frozen) >= 2) AND (octet_length(frozen) <= 33554432))),
    CONSTRAINT package_job_frozen_sha256_check CHECK ((frozen_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT package_job_library_kind_check CHECK ((library_kind = ANY (ARRAY['personal'::text, 'project'::text]))),
    CONSTRAINT package_job_library_revision_check CHECK ((library_revision >= 0)),
    CONSTRAINT package_job_project_revision_check CHECK ((project_revision >= 0)),
    CONSTRAINT package_job_request_sha256_check CHECK ((request_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT package_job_result_check CHECK (((result IS NULL) OR ((octet_length(result) >= 2) AND (octet_length(result) <= 1048576)))),
    CONSTRAINT package_job_revision_check CHECK ((revision > 0)),
    CONSTRAINT package_job_status_check CHECK ((status = ANY (ARRAY['needs_reconciliation'::text, 'succeeded'::text, 'cancelled'::text])))
);

CREATE TABLE IF NOT EXISTS media.package_object (
    job_id uuid NOT NULL,
    object_index integer NOT NULL,
    object_key text NOT NULL,
    byte_size bigint NOT NULL,
    content_type text NOT NULL,
    sha256 text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    CONSTRAINT package_object_byte_size_check CHECK (((byte_size >= 1) AND (byte_size <= '2147483648'::bigint))),
    CONSTRAINT package_object_object_index_check CHECK (((object_index >= 0) AND (object_index <= 14999))),
    CONSTRAINT package_object_sha256_check CHECK ((sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT package_object_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'verified'::text, 'removed'::text])))
);

CREATE TABLE IF NOT EXISTS media.package_command (
    actor_id uuid NOT NULL,
    org_id uuid NOT NULL,
    idem_key uuid NOT NULL,
    job_id uuid NOT NULL,
    action text NOT NULL,
    request_sha256 text NOT NULL,
    response bytea NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT package_command_action_check CHECK ((action = ANY (ARRAY['import'::text, 'reconcile'::text, 'cancel'::text]))),
    CONSTRAINT package_command_request_sha256_check CHECK ((request_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT package_command_response_check CHECK (((octet_length(response) >= 2) AND (octet_length(response) <= 1048576)))
);

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='media.package_command'::regclass AND conname='package_command_pkey') THEN
ALTER TABLE ONLY media.package_command
    ADD CONSTRAINT package_command_pkey PRIMARY KEY (actor_id, idem_key);
END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='media.package_job'::regclass AND conname='package_job_actor_id_idem_key_key') THEN
ALTER TABLE ONLY media.package_job
    ADD CONSTRAINT package_job_actor_id_idem_key_key UNIQUE (actor_id, idem_key);
END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='media.package_job'::regclass AND conname='package_job_pkey') THEN
ALTER TABLE ONLY media.package_job
    ADD CONSTRAINT package_job_pkey PRIMARY KEY (id);
END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='media.package_object'::regclass AND conname='package_object_object_key_key') THEN
ALTER TABLE ONLY media.package_object
    ADD CONSTRAINT package_object_object_key_key UNIQUE (object_key);
END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='media.package_object'::regclass AND conname='package_object_pkey') THEN
ALTER TABLE ONLY media.package_object
    ADD CONSTRAINT package_object_pkey PRIMARY KEY (job_id, object_index);
END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='media.package_command'::regclass AND conname='package_command_job_id_fkey') THEN
ALTER TABLE ONLY media.package_command
    ADD CONSTRAINT package_command_job_id_fkey FOREIGN KEY (job_id) REFERENCES media.package_job(id);
END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='media.package_command'::regclass AND conname='package_command_actor_scope_user_fk') THEN
ALTER TABLE ONLY media.package_command
    ADD CONSTRAINT package_command_actor_scope_user_fk FOREIGN KEY (actor_id) REFERENCES identity."user"(id);
END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='media.package_command'::regclass AND conname='package_command_physical_org_fk') THEN
ALTER TABLE media.package_command ADD CONSTRAINT package_command_physical_org_fk FOREIGN KEY(org_id) REFERENCES workspace.organization(id);
END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='media.package_job'::regclass AND conname='package_job_library_id_fkey') THEN
ALTER TABLE ONLY media.package_job
    ADD CONSTRAINT package_job_library_id_fkey FOREIGN KEY (library_id) REFERENCES media.library(id);
END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='media.package_job'::regclass AND conname='package_job_actor_scope_user_fk') THEN
ALTER TABLE ONLY media.package_job
    ADD CONSTRAINT package_job_actor_scope_user_fk FOREIGN KEY (actor_id) REFERENCES identity."user"(id);
END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='media.package_job'::regclass AND conname='package_job_physical_org_fk') THEN
ALTER TABLE media.package_job ADD CONSTRAINT package_job_physical_org_fk FOREIGN KEY(org_id) REFERENCES workspace.organization(id);
END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='media.package_job'::regclass AND conname='package_job_project_id_fkey') THEN
ALTER TABLE ONLY media.package_job
    ADD CONSTRAINT package_job_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id);
END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='media.package_object'::regclass AND conname='package_object_job_id_fkey') THEN
ALTER TABLE ONLY media.package_object
    ADD CONSTRAINT package_object_job_id_fkey FOREIGN KEY (job_id) REFERENCES media.package_job(id);
END IF; END $$;

CREATE INDEX IF NOT EXISTS package_actor_jobs ON media.package_job USING btree (org_id, actor_id, created_at DESC, id);

CREATE UNIQUE INDEX IF NOT EXISTS package_library_active ON media.package_job USING btree (library_id) WHERE (status = 'needs_reconciliation'::text);

CREATE INDEX IF NOT EXISTS package_project_work ON media.package_job USING btree (project_id, status);

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgrelid='media.package_command'::regclass AND tgname='package_command_immutable') THEN
CREATE TRIGGER package_command_immutable BEFORE DELETE OR UPDATE ON media.package_command FOR EACH ROW EXECUTE FUNCTION media.reject_package_command_mutation();
END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgrelid='media.package_job'::regclass AND tgname='package_frozen_immutable') THEN
CREATE TRIGGER package_frozen_immutable BEFORE UPDATE ON media.package_job FOR EACH ROW EXECUTE FUNCTION media.reject_package_frozen_mutation();
END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgrelid='media.package_job'::regclass AND tgname='package_scope') THEN
CREATE TRIGGER package_scope BEFORE INSERT ON media.package_job FOR EACH ROW EXECUTE FUNCTION media.check_package_scope();
END IF; END $$;

GRANT SELECT,INSERT ON TABLE media.package_command TO lanverse_app;

GRANT SELECT,INSERT ON TABLE media.package_job TO lanverse_app;

GRANT UPDATE (status, revision, library_revision, project_revision, result, updated_at) ON TABLE media.package_job TO lanverse_app;

GRANT SELECT,INSERT ON TABLE media.package_object TO lanverse_app;

GRANT UPDATE (status) ON TABLE media.package_object TO lanverse_app;
