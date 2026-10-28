-- Additive canonical storage prerequisites for an existing database.
-- User-authorized local application verified on 2026-10-28. No data backfill.
-- Canonical prerequisite closure for subject_voice_incremental.sql.
-- Creates storage metadata only; no profile/route/config/asset data is inserted,
-- rewritten, backfilled, rebound or deleted. Existing incompatible data aborts.
BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='60s';
SELECT pg_advisory_xact_lock(63498120261029);

DO $preflight$ BEGIN
 IF to_regclass('media.media_asset') IS NULL OR to_regclass('media.rendition') IS NULL
 OR to_regclass('identity."user"') IS NULL OR to_regclass('workspace.organization') IS NULL THEN
  RAISE EXCEPTION 'required existing media/identity/workspace owner tables are missing';
 END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='glowloom_app') THEN
  RAISE EXCEPTION 'expected existing glowloom_app role is missing';
 END IF;
END $preflight$;

CREATE TABLE IF NOT EXISTS media.storage_profile (
    id text NOT NULL,
    provider text NOT NULL,
    endpoint text NOT NULL,
    signing_endpoint text NOT NULL,
    region text NOT NULL,
    bucket text NOT NULL,
    addressing_style text NOT NULL,
    resource_identity text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT storage_profile_id_check CHECK ((id ~ '^[a-z][a-z0-9-]{0,62}$'::text)),
    CONSTRAINT storage_profile_provider_check CHECK ((provider = ANY (ARRAY['minio'::text, 'cos'::text, 'tos'::text]))),
    CONSTRAINT storage_profile_resource_identity_check CHECK ((resource_identity ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT storage_profile_id_bucket_key UNIQUE (id, bucket),
    CONSTRAINT storage_profile_pkey PRIMARY KEY (id),
    CONSTRAINT storage_profile_resource_identity_key UNIQUE (resource_identity),
    CONSTRAINT storage_profile_resource_pair_key UNIQUE (id, resource_identity)
);

CREATE TABLE IF NOT EXISTS media.storage_work_intent (
    id uuid NOT NULL,
    kind text NOT NULL,
    owner_id uuid NOT NULL,
    work_id uuid NOT NULL,
    profile_id text NOT NULL,
    bucket text NOT NULL,
    config_revision text NOT NULL,
    state text DEFAULT 'open'::text NOT NULL,
    closed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT storage_work_intent_check CHECK ((((state = 'open'::text) AND (closed_at IS NULL)) OR ((state <> 'open'::text) AND (closed_at IS NOT NULL)))),
    CONSTRAINT storage_work_intent_kind_check CHECK ((kind = ANY (ARRAY['operation'::text, 'script'::text, 'export'::text, 'depth'::text, 'transcription'::text, 'package'::text, 'transfer'::text, 'project-copy'::text, 'subject-voice'::text]))),
    CONSTRAINT storage_work_intent_owner_id_check CHECK ((owner_id <> '00000000-0000-0000-0000-000000000000'::uuid)),
    CONSTRAINT storage_work_intent_state_check CHECK ((state = ANY (ARRAY['open'::text, 'accepted'::text, 'cancelled'::text]))),
    CONSTRAINT storage_work_intent_work_id_check CHECK ((work_id <> '00000000-0000-0000-0000-000000000000'::uuid)),
    CONSTRAINT storage_work_intent_id_profile_id_bucket_key UNIQUE (id, profile_id, bucket),
    CONSTRAINT storage_work_intent_kind_owner_id_work_id_key UNIQUE (kind, owner_id, work_id),
    CONSTRAINT storage_work_intent_pkey PRIMARY KEY (id),
    CONSTRAINT storage_work_intent_profile_id_bucket_fkey FOREIGN KEY (profile_id, bucket) REFERENCES media.storage_profile(id, bucket)
);

CREATE TABLE IF NOT EXISTS media.upload_admission (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    scope_id uuid NOT NULL,
    scope_kind text NOT NULL,
    request_key uuid NOT NULL,
    profile_id text NOT NULL,
    bucket text NOT NULL,
    config_revision text NOT NULL,
    policy jsonb NOT NULL,
    accepted_asset_id uuid,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT upload_admission_policy_check CHECK ((jsonb_typeof(policy) = 'object'::text)),
    CONSTRAINT upload_admission_scope_kind_check CHECK ((scope_kind = ANY (ARRAY['project'::text, 'personal'::text]))),
    CONSTRAINT upload_admission_id_profile_id_bucket_key UNIQUE (id, profile_id, bucket),
    CONSTRAINT upload_admission_org_id_actor_id_scope_id_request_key_key UNIQUE (org_id, actor_id, scope_id, request_key),
    CONSTRAINT upload_admission_pkey PRIMARY KEY (id),
    CONSTRAINT upload_admission_accepted_asset_id_fkey FOREIGN KEY (accepted_asset_id) REFERENCES media.media_asset(id),
    CONSTRAINT upload_admission_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES identity."user"(id),
    CONSTRAINT upload_admission_org_id_fkey FOREIGN KEY (org_id) REFERENCES workspace.organization(id),
    CONSTRAINT upload_admission_profile_id_bucket_fkey FOREIGN KEY (profile_id, bucket) REFERENCES media.storage_profile(id, bucket)
);

CREATE TABLE IF NOT EXISTS media.object_location (
    id uuid NOT NULL,
    profile_id text NOT NULL,
    bucket text NOT NULL,
    object_key text NOT NULL,
    state text DEFAULT 'reserved'::text NOT NULL,
    byte_size bigint,
    sha256 text,
    mime_type text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    owner_session_id uuid,
    owner_intent_id uuid,
    write_until timestamp with time zone,
    CONSTRAINT object_location_byte_size_check CHECK ((byte_size >= 0)),
    CONSTRAINT object_location_object_key_check CHECK ((((octet_length(object_key) >= 1) AND (octet_length(object_key) <= 696)) AND (object_key !~ '[[:cntrl:]\\]'::text) AND (object_key !~ '(^|/)(\.|\.\.|)(/|$)'::text))),
    CONSTRAINT object_location_sha256_check CHECK ((sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT object_location_single_owner_check CHECK ((num_nonnulls(owner_session_id, owner_intent_id) <= 1)),
    CONSTRAINT object_location_state_check CHECK ((state = ANY (ARRAY['reserved'::text, 'present'::text, 'accepted'::text, 'delete_pending'::text, 'deleted'::text, 'unknown'::text]))),
    CONSTRAINT object_location_id_object_key_key UNIQUE (id, object_key),
    CONSTRAINT object_location_pkey PRIMARY KEY (id),
    CONSTRAINT object_location_profile_id_bucket_object_key_key UNIQUE (profile_id, bucket, object_key),
    CONSTRAINT object_location_profile_id_bucket_fkey FOREIGN KEY (profile_id, bucket) REFERENCES media.storage_profile(id, bucket),
    CONSTRAINT object_location_upload_owner_fkey FOREIGN KEY (owner_session_id, profile_id, bucket) REFERENCES media.upload_admission(id, profile_id, bucket),
    CONSTRAINT object_location_work_owner_fkey FOREIGN KEY (owner_intent_id, profile_id, bucket) REFERENCES media.storage_work_intent(id, profile_id, bucket)
);

DO $function_guard$ BEGIN
 IF to_regprocedure('media.keep_object_binding()') IS NULL THEN
  EXECUTE $canonical_definition$CREATE FUNCTION media.keep_object_binding() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE old_id text; new_id text;
BEGIN
 old_id=to_jsonb(OLD)->>TG_ARGV[0];
 new_id=to_jsonb(NEW)->>TG_ARGV[0];
 IF old_id IS NOT NULL AND new_id IS DISTINCT FROM old_id THEN
  RAISE EXCEPTION 'physical object binding is immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;$canonical_definition$;
 ELSIF NOT EXISTS(SELECT 1 FROM pg_proc p JOIN pg_language l ON l.oid=p.prolang
  WHERE p.oid=to_regprocedure('media.keep_object_binding()') AND p.prorettype='trigger'::regtype
  AND l.lanname='plpgsql' AND NOT p.prosecdef
  AND btrim(p.prosrc)=btrim($canonical_body$
DECLARE old_id text; new_id text;
BEGIN
 old_id=to_jsonb(OLD)->>TG_ARGV[0];
 new_id=to_jsonb(NEW)->>TG_ARGV[0];
 IF old_id IS NOT NULL AND new_id IS DISTINCT FROM old_id THEN
  RAISE EXCEPTION 'physical object binding is immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $canonical_body$)) THEN
  RAISE EXCEPTION 'existing media.keep_object_binding differs; explicit schema review required';
 END IF;
END $function_guard$;

DO $function_guard$ BEGIN
 IF to_regprocedure('media.keep_storage_identity()') IS NULL THEN
  EXECUTE $canonical_definition$CREATE FUNCTION media.keep_storage_identity() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF TG_TABLE_NAME='storage_profile' OR TG_TABLE_NAME='storage_activation' THEN
        RAISE EXCEPTION 'storage identity is immutable' USING ERRCODE='23514';
    END IF;
    IF (NEW.id,NEW.profile_id,NEW.bucket,NEW.object_key,NEW.owner_session_id) IS DISTINCT FROM (OLD.id,OLD.profile_id,OLD.bucket,OLD.object_key,OLD.owner_session_id) THEN
        RAISE EXCEPTION 'object location identity is immutable' USING ERRCODE='23514';
    END IF;
    NEW.updated_at=now();
    RETURN NEW;
END $$;$canonical_definition$;
 ELSIF NOT EXISTS(SELECT 1 FROM pg_proc p JOIN pg_language l ON l.oid=p.prolang
  WHERE p.oid=to_regprocedure('media.keep_storage_identity()') AND p.prorettype='trigger'::regtype
  AND l.lanname='plpgsql' AND NOT p.prosecdef
  AND btrim(p.prosrc)=btrim($canonical_body$
BEGIN
    IF TG_TABLE_NAME='storage_profile' OR TG_TABLE_NAME='storage_activation' THEN
        RAISE EXCEPTION 'storage identity is immutable' USING ERRCODE='23514';
    END IF;
    IF (NEW.id,NEW.profile_id,NEW.bucket,NEW.object_key,NEW.owner_session_id) IS DISTINCT FROM (OLD.id,OLD.profile_id,OLD.bucket,OLD.object_key,OLD.owner_session_id) THEN
        RAISE EXCEPTION 'object location identity is immutable' USING ERRCODE='23514';
    END IF;
    NEW.updated_at=now();
    RETURN NEW;
END $canonical_body$)) THEN
  RAISE EXCEPTION 'existing media.keep_storage_identity differs; explicit schema review required';
 END IF;
END $function_guard$;

DO $function_guard$ BEGIN
 IF to_regprocedure('media.keep_storage_work()') IS NULL THEN
  EXECUTE $canonical_definition$CREATE FUNCTION media.keep_storage_work() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
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
END $$;$canonical_definition$;
 ELSIF NOT EXISTS(SELECT 1 FROM pg_proc p JOIN pg_language l ON l.oid=p.prolang
  WHERE p.oid=to_regprocedure('media.keep_storage_work()') AND p.prorettype='trigger'::regtype
  AND l.lanname='plpgsql' AND NOT p.prosecdef
  AND btrim(p.prosrc)=btrim($canonical_body$
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
END $canonical_body$)) THEN
  RAISE EXCEPTION 'existing media.keep_storage_work differs; explicit schema review required';
 END IF;
END $function_guard$;

DO $function_guard$ BEGIN
 IF to_regprocedure('media.keep_storage_write_deadline()') IS NULL THEN
  EXECUTE $canonical_definition$CREATE FUNCTION media.keep_storage_write_deadline() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF OLD.write_until IS NOT NULL AND (NEW.write_until IS NULL OR NEW.write_until<OLD.write_until) THEN
  RAISE EXCEPTION 'storage write deadline cannot regress' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;$canonical_definition$;
 ELSIF NOT EXISTS(SELECT 1 FROM pg_proc p JOIN pg_language l ON l.oid=p.prolang
  WHERE p.oid=to_regprocedure('media.keep_storage_write_deadline()') AND p.prorettype='trigger'::regtype
  AND l.lanname='plpgsql' AND NOT p.prosecdef
  AND btrim(p.prosrc)=btrim($canonical_body$
BEGIN
 IF OLD.write_until IS NOT NULL AND (NEW.write_until IS NULL OR NEW.write_until<OLD.write_until) THEN
  RAISE EXCEPTION 'storage write deadline cannot regress' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $canonical_body$)) THEN
  RAISE EXCEPTION 'existing media.keep_storage_write_deadline differs; explicit schema review required';
 END IF;
END $function_guard$;

DO $function_guard$ BEGIN
 IF to_regprocedure('media.keep_upload_admission()') IS NULL THEN
  EXECUTE $canonical_definition$CREATE FUNCTION media.keep_upload_admission() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF (to_jsonb(NEW)-'accepted_asset_id') IS DISTINCT FROM (to_jsonb(OLD)-'accepted_asset_id')
   OR (OLD.accepted_asset_id IS NOT NULL AND NEW.accepted_asset_id IS DISTINCT FROM OLD.accepted_asset_id) THEN
  RAISE EXCEPTION 'upload admission is immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;$canonical_definition$;
 ELSIF NOT EXISTS(SELECT 1 FROM pg_proc p JOIN pg_language l ON l.oid=p.prolang
  WHERE p.oid=to_regprocedure('media.keep_upload_admission()') AND p.prorettype='trigger'::regtype
  AND l.lanname='plpgsql' AND NOT p.prosecdef
  AND btrim(p.prosrc)=btrim($canonical_body$
BEGIN
 IF (to_jsonb(NEW)-'accepted_asset_id') IS DISTINCT FROM (to_jsonb(OLD)-'accepted_asset_id')
   OR (OLD.accepted_asset_id IS NOT NULL AND NEW.accepted_asset_id IS DISTINCT FROM OLD.accepted_asset_id) THEN
  RAISE EXCEPTION 'upload admission is immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $canonical_body$)) THEN
  RAISE EXCEPTION 'existing media.keep_upload_admission differs; explicit schema review required';
 END IF;
END $function_guard$;

-- Validated FK scans the existing column but never returns or changes row values.
-- Any old non-null orphan aborts the entire migration; do not backfill by guessing.
DO $binding_fk$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.media_asset'::regclass AND conname='media_asset_object_location_id_fkey') THEN
  ALTER TABLE ONLY media.media_asset
    ADD CONSTRAINT media_asset_object_location_id_fkey FOREIGN KEY (object_location_id, object_key) REFERENCES media.object_location(id, object_key);
 END IF;
END $binding_fk$;

-- Validated FK scans the existing column but never returns or changes row values.
-- Any old non-null orphan aborts the entire migration; do not backfill by guessing.
DO $binding_fk$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.rendition'::regclass AND conname='rendition_object_location_id_fkey') THEN
  ALTER TABLE ONLY media.rendition
    ADD CONSTRAINT rendition_object_location_id_fkey FOREIGN KEY (object_location_id, object_key) REFERENCES media.object_location(id, object_key);
 END IF;
END $binding_fk$;

CREATE INDEX IF NOT EXISTS object_location_cleanup_idx ON media.object_location USING btree (state, updated_at) WHERE (state = ANY (ARRAY['reserved'::text, 'present'::text, 'delete_pending'::text, 'unknown'::text]));

CREATE INDEX IF NOT EXISTS storage_work_open_profile_idx ON media.storage_work_intent USING btree (profile_id, state);

DO $trigger_guard$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.media_asset'::regclass AND tgname='keep_object_location_id' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_object_location_id BEFORE UPDATE ON media.media_asset FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('object_location_id');
 ELSIF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.media_asset'::regclass AND tgname='keep_object_location_id' AND NOT tgisinternal AND tgfoid='media.keep_object_binding()'::regprocedure AND tgenabled='O') THEN
  RAISE EXCEPTION 'existing media.media_asset.keep_object_location_id trigger differs; explicit review required';
 END IF;
END $trigger_guard$;

DO $trigger_guard$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.rendition'::regclass AND tgname='keep_object_location_id' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_object_location_id BEFORE UPDATE ON media.rendition FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('object_location_id');
 ELSIF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.rendition'::regclass AND tgname='keep_object_location_id' AND NOT tgisinternal AND tgfoid='media.keep_object_binding()'::regprocedure AND tgenabled='O') THEN
  RAISE EXCEPTION 'existing media.rendition.keep_object_location_id trigger differs; explicit review required';
 END IF;
END $trigger_guard$;

DO $trigger_guard$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.object_location'::regclass AND tgname='keep_storage_identity' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_storage_identity BEFORE UPDATE ON media.object_location FOR EACH ROW EXECUTE FUNCTION media.keep_storage_identity();
 ELSIF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.object_location'::regclass AND tgname='keep_storage_identity' AND NOT tgisinternal AND tgfoid='media.keep_storage_identity()'::regprocedure AND tgenabled='O') THEN
  RAISE EXCEPTION 'existing media.object_location.keep_storage_identity trigger differs; explicit review required';
 END IF;
END $trigger_guard$;

DO $trigger_guard$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.storage_profile'::regclass AND tgname='keep_storage_identity' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_storage_identity BEFORE DELETE OR UPDATE ON media.storage_profile FOR EACH ROW EXECUTE FUNCTION media.keep_storage_identity();
 ELSIF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.storage_profile'::regclass AND tgname='keep_storage_identity' AND NOT tgisinternal AND tgfoid='media.keep_storage_identity()'::regprocedure AND tgenabled='O') THEN
  RAISE EXCEPTION 'existing media.storage_profile.keep_storage_identity trigger differs; explicit review required';
 END IF;
END $trigger_guard$;

DO $trigger_guard$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.object_location'::regclass AND tgname='keep_storage_work' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_storage_work BEFORE UPDATE ON media.object_location FOR EACH ROW EXECUTE FUNCTION media.keep_storage_work();
 ELSIF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.object_location'::regclass AND tgname='keep_storage_work' AND NOT tgisinternal AND tgfoid='media.keep_storage_work()'::regprocedure AND tgenabled='O') THEN
  RAISE EXCEPTION 'existing media.object_location.keep_storage_work trigger differs; explicit review required';
 END IF;
END $trigger_guard$;

DO $trigger_guard$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.storage_work_intent'::regclass AND tgname='keep_storage_work' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_storage_work BEFORE UPDATE ON media.storage_work_intent FOR EACH ROW EXECUTE FUNCTION media.keep_storage_work();
 ELSIF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.storage_work_intent'::regclass AND tgname='keep_storage_work' AND NOT tgisinternal AND tgfoid='media.keep_storage_work()'::regprocedure AND tgenabled='O') THEN
  RAISE EXCEPTION 'existing media.storage_work_intent.keep_storage_work trigger differs; explicit review required';
 END IF;
END $trigger_guard$;

DO $trigger_guard$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.object_location'::regclass AND tgname='keep_storage_write_deadline' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_storage_write_deadline BEFORE UPDATE ON media.object_location FOR EACH ROW EXECUTE FUNCTION media.keep_storage_write_deadline();
 ELSIF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.object_location'::regclass AND tgname='keep_storage_write_deadline' AND NOT tgisinternal AND tgfoid='media.keep_storage_write_deadline()'::regprocedure AND tgenabled='O') THEN
  RAISE EXCEPTION 'existing media.object_location.keep_storage_write_deadline trigger differs; explicit review required';
 END IF;
END $trigger_guard$;

DO $trigger_guard$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.upload_admission'::regclass AND tgname='keep_upload_admission' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_upload_admission BEFORE UPDATE ON media.upload_admission FOR EACH ROW EXECUTE FUNCTION media.keep_upload_admission();
 ELSIF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.upload_admission'::regclass AND tgname='keep_upload_admission' AND NOT tgisinternal AND tgfoid='media.keep_upload_admission()'::regprocedure AND tgenabled='O') THEN
  RAISE EXCEPTION 'existing media.upload_admission.keep_upload_admission trigger differs; explicit review required';
 END IF;
END $trigger_guard$;

GRANT SELECT,INSERT ON TABLE media.storage_profile TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE media.storage_work_intent TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE media.upload_admission TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE media.object_location TO glowloom_app;

DO $constraint_guard$ BEGIN
 IF EXISTS(SELECT 1 FROM (VALUES ('media.storage_profile','storage_profile_id_bucket_key'),
('media.storage_profile','storage_profile_pkey'),
('media.storage_profile','storage_profile_resource_identity_key'),
('media.storage_profile','storage_profile_resource_pair_key'),
('media.storage_profile','storage_profile_id_check'),
('media.storage_profile','storage_profile_provider_check'),
('media.storage_profile','storage_profile_resource_identity_check'),
('media.storage_work_intent','storage_work_intent_id_profile_id_bucket_key'),
('media.storage_work_intent','storage_work_intent_kind_owner_id_work_id_key'),
('media.storage_work_intent','storage_work_intent_pkey'),
('media.storage_work_intent','storage_work_intent_profile_id_bucket_fkey'),
('media.storage_work_intent','storage_work_intent_check'),
('media.storage_work_intent','storage_work_intent_kind_check'),
('media.storage_work_intent','storage_work_intent_owner_id_check'),
('media.storage_work_intent','storage_work_intent_state_check'),
('media.storage_work_intent','storage_work_intent_work_id_check'),
('media.upload_admission','upload_admission_id_profile_id_bucket_key'),
('media.upload_admission','upload_admission_org_id_actor_id_scope_id_request_key_key'),
('media.upload_admission','upload_admission_pkey'),
('media.upload_admission','upload_admission_accepted_asset_id_fkey'),
('media.upload_admission','upload_admission_actor_id_fkey'),
('media.upload_admission','upload_admission_org_id_fkey'),
('media.upload_admission','upload_admission_profile_id_bucket_fkey'),
('media.upload_admission','upload_admission_policy_check'),
('media.upload_admission','upload_admission_scope_kind_check'),
('media.object_location','object_location_id_object_key_key'),
('media.object_location','object_location_pkey'),
('media.object_location','object_location_profile_id_bucket_object_key_key'),
('media.object_location','object_location_profile_id_bucket_fkey'),
('media.object_location','object_location_upload_owner_fkey'),
('media.object_location','object_location_work_owner_fkey'),
('media.object_location','object_location_byte_size_check'),
('media.object_location','object_location_object_key_check'),
('media.object_location','object_location_sha256_check'),
('media.object_location','object_location_single_owner_check'),
('media.object_location','object_location_state_check'),
('media.media_asset','media_asset_object_location_id_fkey'),
('media.rendition','rendition_object_location_id_fkey')) expected(relation_name,constraint_name)
  WHERE NOT EXISTS(SELECT 1 FROM pg_constraint c WHERE c.conrelid=to_regclass(expected.relation_name) AND c.conname=expected.constraint_name AND c.convalidated)) THEN
  RAISE EXCEPTION 'canonical prerequisite constraint missing or unvalidated; explicit schema review required';
 END IF;
END $constraint_guard$;
COMMIT;
