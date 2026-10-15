-- 既有业务库增量：先登记物理身份与位置，禁止猜测或自动回填旧 MinIO。
-- 在独立副本审阅、核对后以单事务执行；不得重放 schema.sql。
-- 本阶段保留旧键约束；完成全部消费者迁移及回填后才可替换并收紧非空。
CREATE TABLE IF NOT EXISTS media.storage_profile (
    id text PRIMARY KEY CHECK (id ~ '^[a-z][a-z0-9-]{0,62}$'),
    provider text NOT NULL CHECK (provider IN ('minio','cos','tos')),
    endpoint text NOT NULL,
    signing_endpoint text NOT NULL,
    region text NOT NULL,
    bucket text NOT NULL,
    addressing_style text NOT NULL,
    resource_identity text NOT NULL UNIQUE CHECK (resource_identity ~ '^[a-f0-9]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(id,bucket)
);

CREATE TABLE IF NOT EXISTS media.storage_route (
    singleton integer PRIMARY KEY DEFAULT 1 CHECK (singleton=1),
    revision text NOT NULL CHECK (revision ~ '^[a-z][a-z0-9-]{0,62}$'),
    profile_id text NOT NULL REFERENCES media.storage_profile(id),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS media.storage_activation (
    revision text PRIMARY KEY CHECK (revision ~ '^[a-z][a-z0-9-]{0,62}$'),
    expected_revision text NOT NULL,
    profile_id text NOT NULL REFERENCES media.storage_profile(id),
    activated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS media.object_location (
    id uuid PRIMARY KEY,
    profile_id text NOT NULL,
    bucket text NOT NULL,
    object_key text NOT NULL CHECK (octet_length(object_key) BETWEEN 1 AND 696 AND object_key !~ '[[:cntrl:]\\]' AND object_key !~ '(^|/)(\.|\.\.|)(/|$)'),
    state text NOT NULL DEFAULT 'reserved' CHECK (state IN ('reserved','present','accepted','delete_pending','deleted','unknown')),
    byte_size bigint CHECK (byte_size>=0),
    sha256 text CHECK (sha256 ~ '^[a-f0-9]{64}$'),
    mime_type text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY(profile_id,bucket) REFERENCES media.storage_profile(id,bucket),
    UNIQUE(profile_id,bucket,object_key),
    UNIQUE(id,object_key)
);

CREATE OR REPLACE FUNCTION media.keep_storage_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_TABLE_NAME='storage_profile' OR TG_TABLE_NAME='storage_activation' THEN
        RAISE EXCEPTION 'storage identity is immutable' USING ERRCODE='23514';
    END IF;
    IF (NEW.id,NEW.profile_id,NEW.bucket,NEW.object_key,NEW.owner_session_id) IS DISTINCT FROM (OLD.id,OLD.profile_id,OLD.bucket,OLD.object_key,OLD.owner_session_id) THEN
        RAISE EXCEPTION 'object location identity is immutable' USING ERRCODE='23514';
    END IF;
    NEW.updated_at=now();
    RETURN NEW;
END $$;

DO $$ BEGIN
    IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.storage_profile'::regclass AND tgname='keep_storage_identity') THEN
        CREATE TRIGGER keep_storage_identity BEFORE UPDATE OR DELETE ON media.storage_profile FOR EACH ROW EXECUTE FUNCTION media.keep_storage_identity();
    END IF;
    IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.storage_activation'::regclass AND tgname='keep_storage_identity') THEN
        CREATE TRIGGER keep_storage_identity BEFORE UPDATE OR DELETE ON media.storage_activation FOR EACH ROW EXECUTE FUNCTION media.keep_storage_identity();
    END IF;
    IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.object_location'::regclass AND tgname='keep_storage_identity') THEN
        CREATE TRIGGER keep_storage_identity BEFORE UPDATE ON media.object_location FOR EACH ROW EXECUTE FUNCTION media.keep_storage_identity();
    END IF;
END $$;

ALTER TABLE media.media_asset ADD COLUMN IF NOT EXISTS object_location_id uuid;
ALTER TABLE media.rendition ADD COLUMN IF NOT EXISTS object_location_id uuid;
ALTER TABLE media.package_object ADD COLUMN IF NOT EXISTS object_location_id uuid;
ALTER TABLE media.project_copy_object ADD COLUMN IF NOT EXISTS source_location_id uuid;
ALTER TABLE media.project_copy_object ADD COLUMN IF NOT EXISTS target_location_id uuid;
ALTER TABLE media.transfer_object ADD COLUMN IF NOT EXISTS source_location_id uuid;
ALTER TABLE media.transfer_object ADD COLUMN IF NOT EXISTS target_location_id uuid;
ALTER TABLE media.purge_object ADD COLUMN IF NOT EXISTS object_location_id uuid;
ALTER TABLE mediatool.depth_object ADD COLUMN IF NOT EXISTS object_location_id uuid;
ALTER TABLE script.object_intent ADD COLUMN IF NOT EXISTS object_location_id uuid;
ALTER TABLE script.object_state ADD COLUMN IF NOT EXISTS object_location_id uuid;

-- 复合 FK 同时约束位置与原键，防止把某一行关联到另一对象。
DO $$ DECLARE item record; constraint_name text; BEGIN
    FOR item IN SELECT * FROM (VALUES
        ('media','media_asset','object_location_id','object_key'),
        ('media','rendition','object_location_id','object_key'),
        ('media','package_object','object_location_id','object_key'),
        ('media','project_copy_object','source_location_id','source_object_key'),
        ('media','project_copy_object','target_location_id','target_object_key'),
        ('media','transfer_object','source_location_id','source_object_key'),
        ('media','transfer_object','target_location_id','target_object_key'),
        ('media','purge_object','object_location_id','object_key'),
        ('mediatool','depth_object','object_location_id','object_key'),
        ('script','object_intent','object_location_id','object_key'),
        ('script','object_state','object_location_id','object_key')
    ) AS objects(schema_name,table_name,location_column,key_column) LOOP
        constraint_name=item.table_name||'_'||item.location_column||'_fkey';
        IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid=format('%I.%I',item.schema_name,item.table_name)::regclass AND conname=constraint_name) THEN
            EXECUTE format('ALTER TABLE %I.%I ADD CONSTRAINT %I FOREIGN KEY(%I,%I) REFERENCES media.object_location(id,object_key)',item.schema_name,item.table_name,constraint_name,item.location_column,item.key_column);
        END IF;
    END LOOP;
END $$;

GRANT SELECT,INSERT ON media.storage_profile,media.storage_activation TO lanverse_app;
GRANT SELECT,INSERT,UPDATE ON media.storage_route,media.object_location TO lanverse_app;

CREATE TABLE IF NOT EXISTS media.upload_admission (
 id uuid PRIMARY KEY,
 org_id uuid NOT NULL REFERENCES workspace.organization(id),
 actor_id uuid NOT NULL REFERENCES identity."user"(id),
 scope_id uuid NOT NULL,
 scope_kind text NOT NULL CHECK(scope_kind IN ('project','personal')),
 request_key uuid NOT NULL,
 profile_id text NOT NULL,
 bucket text NOT NULL,
 config_revision text NOT NULL,
 policy jsonb NOT NULL CHECK(jsonb_typeof(policy)='object'),
 accepted_asset_id uuid REFERENCES media.media_asset(id),
 expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(org_id,actor_id,scope_id,request_key),
 UNIQUE(id,profile_id,bucket),
 FOREIGN KEY(profile_id,bucket) REFERENCES media.storage_profile(id,bucket)
);
ALTER TABLE media.object_location ADD COLUMN IF NOT EXISTS owner_session_id uuid;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.object_location'::regclass AND conname='object_location_upload_owner_fkey') THEN
  ALTER TABLE media.object_location ADD CONSTRAINT object_location_upload_owner_fkey FOREIGN KEY(owner_session_id,profile_id,bucket) REFERENCES media.upload_admission(id,profile_id,bucket);
 END IF;
END $$;
CREATE INDEX IF NOT EXISTS object_location_cleanup_idx ON media.object_location(state,updated_at) WHERE state IN ('reserved','present','delete_pending','unknown');
GRANT SELECT,INSERT,UPDATE ON media.upload_admission TO lanverse_app;

CREATE OR REPLACE FUNCTION media.keep_upload_admission() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (to_jsonb(NEW)-'accepted_asset_id') IS DISTINCT FROM (to_jsonb(OLD)-'accepted_asset_id')
   OR (OLD.accepted_asset_id IS NOT NULL AND NEW.accepted_asset_id IS DISTINCT FROM OLD.accepted_asset_id) THEN
  RAISE EXCEPTION 'upload admission is immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.upload_admission'::regclass AND tgname='keep_upload_admission') THEN
  CREATE TRIGGER keep_upload_admission BEFORE UPDATE ON media.upload_admission FOR EACH ROW EXECUTE FUNCTION media.keep_upload_admission();
 END IF;
END $$;
