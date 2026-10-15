-- 对象存储受控升级：原资源证明和逐对象回填回执，不推断默认桶。
-- 仅在隔离副本演练并审阅后执行；schema.sql 仍是空库完整结构源。
CREATE TABLE IF NOT EXISTS media.storage_legacy_binding (
 singleton integer PRIMARY KEY CHECK(singleton=1),
 profile_id text NOT NULL,
 bucket text NOT NULL,
 resource_identity text NOT NULL CHECK(resource_identity ~ '^[a-f0-9]{64}$'),
 evidence_sha256 text NOT NULL CHECK(evidence_sha256 ~ '^[a-f0-9]{64}$'),
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(profile_id,bucket) REFERENCES media.storage_profile(id,bucket)
);
CREATE TABLE IF NOT EXISTS media.storage_legacy_receipt (
 profile_id text NOT NULL,
 bucket text NOT NULL,
 object_key text NOT NULL,
 snapshot_sha256 text NOT NULL CHECK(snapshot_sha256 ~ '^[a-f0-9]{64}$'),
 location_id uuid NOT NULL,
 byte_size bigint NOT NULL CHECK(byte_size>=0),
 sha256 text NOT NULL CHECK(sha256 ~ '^[a-f0-9]{64}$'),
 mime_type text NOT NULL,
 reference_count integer NOT NULL CHECK(reference_count>0),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(profile_id,bucket,object_key,snapshot_sha256),
 FOREIGN KEY(location_id,object_key) REFERENCES media.object_location(id,object_key),
 FOREIGN KEY(profile_id,bucket) REFERENCES media.storage_profile(id,bucket)
);
CREATE OR REPLACE FUNCTION media.keep_storage_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'storage evidence is immutable' USING ERRCODE='23514';
END $$;
DO $$ DECLARE name text; BEGIN
 FOREACH name IN ARRAY ARRAY['storage_legacy_binding','storage_legacy_receipt'] LOOP
  IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid=format('media.%I',name)::regclass AND tgname='keep_storage_evidence') THEN
   EXECUTE format('CREATE TRIGGER keep_storage_evidence BEFORE UPDATE OR DELETE ON media.%I FOR EACH ROW EXECUTE FUNCTION media.keep_storage_evidence()',name);
  END IF;
 END LOOP;
END $$;
GRANT SELECT,INSERT ON media.storage_legacy_binding,media.storage_legacy_receipt TO lanverse_app;

CREATE TABLE IF NOT EXISTS media.storage_work_intent (
 id uuid PRIMARY KEY,
 kind text NOT NULL CHECK(kind IN ('operation','script','export','depth','transcription','package','transfer','project-copy')),
 owner_id uuid NOT NULL CHECK(owner_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 work_id uuid NOT NULL CHECK(work_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 profile_id text NOT NULL,
 bucket text NOT NULL,
 config_revision text NOT NULL,
 state text NOT NULL DEFAULT 'open' CHECK(state IN ('open','accepted','cancelled')),
 closed_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(kind,owner_id,work_id),
 UNIQUE(id,profile_id,bucket),
 FOREIGN KEY(profile_id,bucket) REFERENCES media.storage_profile(id,bucket),
 CHECK((state='open' AND closed_at IS NULL) OR (state<>'open' AND closed_at IS NOT NULL))
);
ALTER TABLE media.object_location ADD COLUMN IF NOT EXISTS owner_intent_id uuid;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.object_location'::regclass AND conname='object_location_work_owner_fkey') THEN
  ALTER TABLE media.object_location ADD CONSTRAINT object_location_work_owner_fkey FOREIGN KEY(owner_intent_id,profile_id,bucket) REFERENCES media.storage_work_intent(id,profile_id,bucket);
  ALTER TABLE media.object_location ADD CONSTRAINT object_location_single_owner_check CHECK(num_nonnulls(owner_session_id,owner_intent_id)<=1);
 END IF;
END $$;
CREATE OR REPLACE FUNCTION media.keep_storage_work() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='object_location' THEN
  IF OLD.owner_intent_id IS DISTINCT FROM NEW.owner_intent_id THEN
   RAISE EXCEPTION 'storage work object owner is immutable' USING ERRCODE='23514';
  END IF;
 ELSE
  IF (to_jsonb(OLD)-'state'-'closed_at') IS DISTINCT FROM (to_jsonb(NEW)-'state'-'closed_at')
   OR (OLD.state<>'open' AND NEW.state<>OLD.state)
   OR (OLD.closed_at IS NOT NULL AND NEW.closed_at IS DISTINCT FROM OLD.closed_at) THEN
   RAISE EXCEPTION 'storage work target is immutable' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END $$;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.storage_work_intent'::regclass AND tgname='keep_storage_work') THEN
  CREATE TRIGGER keep_storage_work BEFORE UPDATE ON media.storage_work_intent FOR EACH ROW EXECUTE FUNCTION media.keep_storage_work();
 END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.object_location'::regclass AND tgname='keep_storage_work') THEN
  CREATE TRIGGER keep_storage_work BEFORE UPDATE ON media.object_location FOR EACH ROW EXECUTE FUNCTION media.keep_storage_work();
 END IF;
END $$;
CREATE INDEX IF NOT EXISTS storage_work_open_profile_idx ON media.storage_work_intent(profile_id,state);
GRANT SELECT,INSERT,UPDATE ON media.storage_work_intent TO lanverse_app;

CREATE TABLE IF NOT EXISTS media.storage_runtime (
 instance_id uuid PRIMARY KEY,
 role text NOT NULL CHECK(role IN ('api','worker','relay','setup')),
 revision text NOT NULL,
 config_sha256 text NOT NULL CHECK(config_sha256 ~ '^[a-f0-9]{64}$'),
 capabilities jsonb NOT NULL CHECK(jsonb_typeof(capabilities)='array'),
 expires_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS media.storage_deployment_evidence (
 config_sha256 text PRIMARY KEY CHECK(config_sha256 ~ '^[a-f0-9]{64}$'),
 evidence_sha256 text NOT NULL CHECK(evidence_sha256 ~ '^[a-f0-9]{64}$'),
 recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS media.storage_probe_evidence (
 id uuid PRIMARY KEY,
 profile_id text NOT NULL REFERENCES media.storage_profile(id),
 resource_identity text NOT NULL,
 config_sha256 text NOT NULL CHECK(config_sha256 ~ '^[a-f0-9]{64}$'),
 evidence_sha256 text NOT NULL CHECK(evidence_sha256 ~ '^[a-f0-9]{64}$'),
 expires_at timestamptz NOT NULL,
 recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS media.storage_activation_guard (
 revision text PRIMARY KEY REFERENCES media.storage_activation(revision),
 config_sha256 text NOT NULL CHECK(config_sha256 ~ '^[a-f0-9]{64}$'),
 deployment_evidence_sha256 text NOT NULL,
 activated_at timestamptz NOT NULL DEFAULT now()
);
DO $$ DECLARE name text; BEGIN
 FOREACH name IN ARRAY ARRAY['storage_deployment_evidence','storage_probe_evidence','storage_activation_guard'] LOOP
  IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid=format('media.%I',name)::regclass AND tgname='keep_storage_evidence') THEN
   EXECUTE format('CREATE TRIGGER keep_storage_evidence BEFORE UPDATE OR DELETE ON media.%I FOR EACH ROW EXECUTE FUNCTION media.keep_storage_evidence()',name);
  END IF;
 END LOOP;
END $$;
GRANT SELECT,INSERT ON media.storage_deployment_evidence,media.storage_probe_evidence,media.storage_activation_guard TO lanverse_app;
GRANT SELECT,INSERT,UPDATE ON media.storage_runtime TO lanverse_app;

CREATE TABLE IF NOT EXISTS media.storage_config_revision (
 revision text PRIMARY KEY,
 config_sha256 text NOT NULL UNIQUE CHECK(config_sha256 ~ '^[a-f0-9]{64}$'),
 profile_id text NOT NULL REFERENCES media.storage_profile(id),
 upload_policy jsonb NOT NULL CHECK(jsonb_typeof(upload_policy)='object'),
 session_ttl_seconds bigint NOT NULL CHECK(session_ttl_seconds BETWEEN 60 AND 604800),
 created_at timestamptz NOT NULL DEFAULT now()
);
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.storage_config_revision'::regclass AND tgname='keep_storage_evidence') THEN
  CREATE TRIGGER keep_storage_evidence BEFORE UPDATE OR DELETE ON media.storage_config_revision FOR EACH ROW EXECUTE FUNCTION media.keep_storage_evidence();
 END IF;
END $$;
GRANT SELECT,INSERT ON media.storage_config_revision TO lanverse_app;

CREATE OR REPLACE FUNCTION media.keep_object_binding() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE old_id text; new_id text;
BEGIN
 old_id=to_jsonb(OLD)->>TG_ARGV[0];
 new_id=to_jsonb(NEW)->>TG_ARGV[0];
 IF old_id IS NOT NULL AND new_id IS DISTINCT FROM old_id THEN
  RAISE EXCEPTION 'physical object binding is immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
DO $$ DECLARE item record; trigger_name text; BEGIN
 FOR item IN SELECT * FROM (VALUES
  ('media','media_asset','object_location_id'),('media','rendition','object_location_id'),
  ('media','package_object','object_location_id'),('media','project_copy_object','source_location_id'),('media','project_copy_object','target_location_id'),
  ('media','transfer_object','source_location_id'),('media','transfer_object','target_location_id'),('media','purge_object','object_location_id'),
  ('mediatool','depth_object','object_location_id'),('script','object_intent','object_location_id'),('script','object_state','object_location_id')
 ) AS refs(schema_name,table_name,location_column) LOOP
  trigger_name='keep_'||item.location_column;
  IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid=format('%I.%I',item.schema_name,item.table_name)::regclass AND tgname=trigger_name) THEN
   EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE ON %I.%I FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding(%L)',trigger_name,item.schema_name,item.table_name,item.location_column);
  END IF;
 END LOOP;
END $$;

CREATE TABLE IF NOT EXISTS media.storage_legacy_tombstone (
 profile_id text NOT NULL,
 bucket text NOT NULL,
 object_key text NOT NULL,
 snapshot_sha256 text NOT NULL CHECK(snapshot_sha256 ~ '^[a-f0-9]{64}$'),
 location_id uuid NOT NULL,
 reference_count integer NOT NULL CHECK(reference_count>0),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(profile_id,bucket,object_key,snapshot_sha256),
 FOREIGN KEY(location_id,object_key) REFERENCES media.object_location(id,object_key),
 FOREIGN KEY(profile_id,bucket) REFERENCES media.storage_profile(id,bucket)
);
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.storage_legacy_tombstone'::regclass AND tgname='keep_storage_evidence') THEN
  CREATE TRIGGER keep_storage_evidence BEFORE UPDATE OR DELETE ON media.storage_legacy_tombstone FOR EACH ROW EXECUTE FUNCTION media.keep_storage_evidence();
 END IF;
END $$;
GRANT SELECT,INSERT ON media.storage_legacy_tombstone TO lanverse_app;
