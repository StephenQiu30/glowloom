-- Storage ledger expansion only; copied from the current canonical schema.
-- PostgreSQL 18. Reviewed old-schema upgrade; never replay schema.sql on a business DB.
-- Run explicitly with psql -X --single-transaction -v ON_ERROR_STOP=1 -f <this-file>.
-- No profile, legacy binding, route, object location, receipt or readiness evidence is seeded.
-- Nullable owner columns and existing bare-key business keys remain during byte-proved backfill.
-- This is NOT a finalization or activation migration. Do not start roles on a new route yet.
SET LOCAL lock_timeout='5s';
SELECT pg_advisory_xact_lock(hashtextextended('glowloom.storage-ledger-expand-v1',0));
DO $$ DECLARE r record; BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='glowloom_app' AND NOT rolcanlogin AND NOT rolsuper) THEN
  RAISE EXCEPTION 'pre-provisioned restricted glowloom_app role required';
 END IF;
 FOR r IN SELECT * FROM (VALUES
 ('media.media_asset','object_key','object_location_id'),
 ('media.rendition','object_key','object_location_id'),
 ('media.package_object','object_key','object_location_id'),
 ('media.project_copy_object','source_object_key','source_location_id'),
 ('media.project_copy_object','target_object_key','target_location_id'),
 ('media.transfer_object','source_object_key','source_location_id'),
 ('media.transfer_object','target_object_key','target_location_id'),
 ('media.purge_object','object_key','object_location_id'),
 ('mediatool.depth_object','object_key','object_location_id'),
 ('script.object_intent','object_key','object_location_id'),
 ('script.object_state','object_key','object_location_id')
 ) v(tbl,key_col,location_col) LOOP
  IF to_regclass(r.tbl) IS NULL THEN RAISE EXCEPTION 'required owning table missing: %',r.tbl; END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid=to_regclass(r.tbl) AND attname=r.key_col AND atttypid='text'::regtype AND NOT attisdropped) THEN
   RAISE EXCEPTION 'review owning key type first: %.%',r.tbl,r.key_col;
  END IF;
  IF EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid=to_regclass(r.tbl) AND attname=r.location_col AND (atttypid<>'uuid'::regtype OR attisdropped)) THEN
   RAISE EXCEPTION 'unexpected location column type: %.%',r.tbl,r.location_col;
  END IF;
 END LOOP;
 IF to_regclass('identity.user') IS NULL OR to_regclass('workspace.organization') IS NULL THEN RAISE EXCEPTION 'identity/workspace dependencies missing'; END IF;
END $$;


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
    CONSTRAINT object_location_state_check CHECK ((state = ANY (ARRAY['reserved'::text, 'present'::text, 'accepted'::text, 'delete_pending'::text, 'deleted'::text, 'unknown'::text])))
);

CREATE TABLE IF NOT EXISTS media.storage_activation (
    revision text NOT NULL,
    expected_revision text NOT NULL,
    profile_id text NOT NULL,
    activated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT storage_activation_revision_check CHECK ((revision ~ '^[a-z][a-z0-9-]{0,62}$'::text))
);

CREATE TABLE IF NOT EXISTS media.storage_activation_guard (
    revision text NOT NULL,
    config_sha256 text NOT NULL,
    deployment_evidence_sha256 text NOT NULL,
    activated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT storage_activation_guard_config_sha256_check CHECK ((config_sha256 ~ '^[a-f0-9]{64}$'::text))
);

CREATE TABLE IF NOT EXISTS media.storage_bucket_evidence (
    id uuid NOT NULL,
    profile_id text NOT NULL,
    resource_identity text NOT NULL,
    config_sha256 text NOT NULL,
    evidence_sha256 text NOT NULL,
    contract text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT storage_bucket_evidence_config_sha256_check CHECK ((config_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT storage_bucket_evidence_contract_check CHECK ((contract = 'private-flat-never-versioned-standard-v1'::text)),
    CONSTRAINT storage_bucket_evidence_evidence_sha256_check CHECK ((evidence_sha256 ~ '^[a-f0-9]{64}$'::text))
);

CREATE TABLE IF NOT EXISTS media.storage_config_revision (
    revision text NOT NULL,
    config_sha256 text NOT NULL,
    profile_id text NOT NULL,
    upload_policy jsonb NOT NULL,
    session_ttl_seconds bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT storage_config_revision_config_sha256_check CHECK ((config_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT storage_config_revision_session_ttl_seconds_check CHECK (((session_ttl_seconds >= 60) AND (session_ttl_seconds <= 604800))),
    CONSTRAINT storage_config_revision_upload_policy_check CHECK ((jsonb_typeof(upload_policy) = 'object'::text))
);

CREATE TABLE IF NOT EXISTS media.storage_deployment_evidence (
    config_sha256 text NOT NULL,
    evidence_sha256 text NOT NULL,
    recorded_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT storage_deployment_evidence_config_sha256_check CHECK ((config_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT storage_deployment_evidence_evidence_sha256_check CHECK ((evidence_sha256 ~ '^[a-f0-9]{64}$'::text))
);

CREATE TABLE IF NOT EXISTS media.storage_legacy_binding (
    singleton integer NOT NULL,
    profile_id text NOT NULL,
    bucket text NOT NULL,
    resource_identity text NOT NULL,
    evidence_sha256 text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT storage_legacy_binding_evidence_sha256_check CHECK ((evidence_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT storage_legacy_binding_resource_identity_check CHECK ((resource_identity ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT storage_legacy_binding_singleton_check CHECK ((singleton = 1))
);

CREATE TABLE IF NOT EXISTS media.storage_legacy_receipt (
    profile_id text NOT NULL,
    bucket text NOT NULL,
    object_key text NOT NULL,
    snapshot_sha256 text NOT NULL,
    location_id uuid NOT NULL,
    byte_size bigint NOT NULL,
    sha256 text NOT NULL,
    mime_type text NOT NULL,
    reference_count integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT storage_legacy_receipt_byte_size_check CHECK ((byte_size >= 0)),
    CONSTRAINT storage_legacy_receipt_reference_count_check CHECK ((reference_count > 0)),
    CONSTRAINT storage_legacy_receipt_sha256_check CHECK ((sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT storage_legacy_receipt_snapshot_sha256_check CHECK ((snapshot_sha256 ~ '^[a-f0-9]{64}$'::text))
);

CREATE TABLE IF NOT EXISTS media.storage_legacy_tombstone (
    profile_id text NOT NULL,
    bucket text NOT NULL,
    object_key text NOT NULL,
    snapshot_sha256 text NOT NULL,
    location_id uuid NOT NULL,
    reference_count integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT storage_legacy_tombstone_reference_count_check CHECK ((reference_count > 0)),
    CONSTRAINT storage_legacy_tombstone_snapshot_sha256_check CHECK ((snapshot_sha256 ~ '^[a-f0-9]{64}$'::text))
);

CREATE TABLE IF NOT EXISTS media.storage_probe_evidence (
    id uuid NOT NULL,
    profile_id text NOT NULL,
    resource_identity text NOT NULL,
    config_sha256 text NOT NULL,
    evidence_sha256 text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    recorded_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT storage_probe_evidence_config_sha256_check CHECK ((config_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT storage_probe_evidence_evidence_sha256_check CHECK ((evidence_sha256 ~ '^[a-f0-9]{64}$'::text))
);

CREATE TABLE IF NOT EXISTS media.storage_probe_intent (
    id uuid NOT NULL,
    profile_id text NOT NULL,
    bucket text NOT NULL,
    config_sha256 text NOT NULL,
    sentinel_key text NOT NULL,
    removal_key text,
    mode text NOT NULL,
    state text DEFAULT 'running'::text NOT NULL,
    evidence_sha256 text,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT storage_probe_intent_check CHECK ((((mode = 'read_write'::text) AND (removal_key = (sentinel_key || '.remove'::text))) OR ((mode = 'read_only'::text) AND (removal_key IS NULL)))),
    CONSTRAINT storage_probe_intent_config_sha256_check CHECK ((config_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT storage_probe_intent_evidence_sha256_check CHECK ((evidence_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT storage_probe_intent_mode_check CHECK ((mode = ANY (ARRAY['read_only'::text, 'read_write'::text]))),
    CONSTRAINT storage_probe_intent_state_check CHECK ((state = ANY (ARRAY['running'::text, 'complete'::text, 'cleaning'::text, 'cleaned'::text])))
);

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
    CONSTRAINT storage_profile_resource_identity_check CHECK ((resource_identity ~ '^[a-f0-9]{64}$'::text))
);

CREATE TABLE IF NOT EXISTS media.storage_route (
    singleton integer DEFAULT 1 NOT NULL,
    revision text NOT NULL,
    profile_id text NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT storage_route_revision_check CHECK ((revision ~ '^[a-z][a-z0-9-]{0,62}$'::text)),
    CONSTRAINT storage_route_singleton_check CHECK ((singleton = 1))
);

CREATE TABLE IF NOT EXISTS media.storage_runtime (
    instance_id uuid NOT NULL,
    role text NOT NULL,
    revision text NOT NULL,
    config_sha256 text NOT NULL,
    capabilities jsonb NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT storage_runtime_capabilities_check CHECK ((jsonb_typeof(capabilities) = 'array'::text)),
    CONSTRAINT storage_runtime_config_sha256_check CHECK ((config_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT storage_runtime_role_check CHECK ((role = ANY (ARRAY['api'::text, 'worker'::text, 'relay'::text, 'setup'::text])))
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
    CONSTRAINT storage_work_intent_work_id_check CHECK ((work_id <> '00000000-0000-0000-0000-000000000000'::uuid))
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
    CONSTRAINT upload_admission_scope_kind_check CHECK ((scope_kind = ANY (ARRAY['project'::text, 'personal'::text])))
);

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

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.object_location'::regclass AND conname='object_location_id_object_key_key') THEN
  ALTER TABLE ONLY media.object_location
    ADD CONSTRAINT object_location_id_object_key_key UNIQUE (id, object_key);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.object_location'::regclass AND conname='object_location_pkey') THEN
  ALTER TABLE ONLY media.object_location
    ADD CONSTRAINT object_location_pkey PRIMARY KEY (id);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.object_location'::regclass AND conname='object_location_profile_id_bucket_object_key_key') THEN
  ALTER TABLE ONLY media.object_location
    ADD CONSTRAINT object_location_profile_id_bucket_object_key_key UNIQUE (profile_id, bucket, object_key);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_activation_guard'::regclass AND conname='storage_activation_guard_pkey') THEN
  ALTER TABLE ONLY media.storage_activation_guard
    ADD CONSTRAINT storage_activation_guard_pkey PRIMARY KEY (revision);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_activation'::regclass AND conname='storage_activation_pkey') THEN
  ALTER TABLE ONLY media.storage_activation
    ADD CONSTRAINT storage_activation_pkey PRIMARY KEY (revision);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_bucket_evidence'::regclass AND conname='storage_bucket_evidence_pkey') THEN
  ALTER TABLE ONLY media.storage_bucket_evidence
    ADD CONSTRAINT storage_bucket_evidence_pkey PRIMARY KEY (id);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_config_revision'::regclass AND conname='storage_config_revision_pkey') THEN
  ALTER TABLE ONLY media.storage_config_revision
    ADD CONSTRAINT storage_config_revision_pkey PRIMARY KEY (revision);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_deployment_evidence'::regclass AND conname='storage_deployment_evidence_pkey') THEN
  ALTER TABLE ONLY media.storage_deployment_evidence
    ADD CONSTRAINT storage_deployment_evidence_pkey PRIMARY KEY (config_sha256);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_legacy_binding'::regclass AND conname='storage_legacy_binding_pkey') THEN
  ALTER TABLE ONLY media.storage_legacy_binding
    ADD CONSTRAINT storage_legacy_binding_pkey PRIMARY KEY (singleton);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_legacy_receipt'::regclass AND conname='storage_legacy_receipt_pkey') THEN
  ALTER TABLE ONLY media.storage_legacy_receipt
    ADD CONSTRAINT storage_legacy_receipt_pkey PRIMARY KEY (profile_id, bucket, object_key, snapshot_sha256);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_legacy_tombstone'::regclass AND conname='storage_legacy_tombstone_pkey') THEN
  ALTER TABLE ONLY media.storage_legacy_tombstone
    ADD CONSTRAINT storage_legacy_tombstone_pkey PRIMARY KEY (profile_id, bucket, object_key, snapshot_sha256);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_probe_evidence'::regclass AND conname='storage_probe_evidence_pkey') THEN
  ALTER TABLE ONLY media.storage_probe_evidence
    ADD CONSTRAINT storage_probe_evidence_pkey PRIMARY KEY (id);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_probe_intent'::regclass AND conname='storage_probe_intent_pkey') THEN
  ALTER TABLE ONLY media.storage_probe_intent
    ADD CONSTRAINT storage_probe_intent_pkey PRIMARY KEY (id);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_profile'::regclass AND conname='storage_profile_id_bucket_key') THEN
  ALTER TABLE ONLY media.storage_profile
    ADD CONSTRAINT storage_profile_id_bucket_key UNIQUE (id, bucket);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_profile'::regclass AND conname='storage_profile_pkey') THEN
  ALTER TABLE ONLY media.storage_profile
    ADD CONSTRAINT storage_profile_pkey PRIMARY KEY (id);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_profile'::regclass AND conname='storage_profile_resource_identity_key') THEN
  ALTER TABLE ONLY media.storage_profile
    ADD CONSTRAINT storage_profile_resource_identity_key UNIQUE (resource_identity);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_profile'::regclass AND conname='storage_profile_resource_pair_key') THEN
  ALTER TABLE ONLY media.storage_profile
    ADD CONSTRAINT storage_profile_resource_pair_key UNIQUE (id, resource_identity);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_route'::regclass AND conname='storage_route_pkey') THEN
  ALTER TABLE ONLY media.storage_route
    ADD CONSTRAINT storage_route_pkey PRIMARY KEY (singleton);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_runtime'::regclass AND conname='storage_runtime_pkey') THEN
  ALTER TABLE ONLY media.storage_runtime
    ADD CONSTRAINT storage_runtime_pkey PRIMARY KEY (instance_id);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_work_intent'::regclass AND conname='storage_work_intent_id_profile_id_bucket_key') THEN
  ALTER TABLE ONLY media.storage_work_intent
    ADD CONSTRAINT storage_work_intent_id_profile_id_bucket_key UNIQUE (id, profile_id, bucket);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_work_intent'::regclass AND conname='storage_work_intent_kind_owner_id_work_id_key') THEN
  ALTER TABLE ONLY media.storage_work_intent
    ADD CONSTRAINT storage_work_intent_kind_owner_id_work_id_key UNIQUE (kind, owner_id, work_id);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_work_intent'::regclass AND conname='storage_work_intent_pkey') THEN
  ALTER TABLE ONLY media.storage_work_intent
    ADD CONSTRAINT storage_work_intent_pkey PRIMARY KEY (id);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.upload_admission'::regclass AND conname='upload_admission_id_profile_id_bucket_key') THEN
  ALTER TABLE ONLY media.upload_admission
    ADD CONSTRAINT upload_admission_id_profile_id_bucket_key UNIQUE (id, profile_id, bucket);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.upload_admission'::regclass AND conname='upload_admission_org_id_actor_id_scope_id_request_key_key') THEN
  ALTER TABLE ONLY media.upload_admission
    ADD CONSTRAINT upload_admission_org_id_actor_id_scope_id_request_key_key UNIQUE (org_id, actor_id, scope_id, request_key);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.upload_admission'::regclass AND conname='upload_admission_pkey') THEN
  ALTER TABLE ONLY media.upload_admission
    ADD CONSTRAINT upload_admission_pkey PRIMARY KEY (id);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.media_asset'::regclass AND conname='media_asset_object_location_id_fkey') THEN
  ALTER TABLE ONLY media.media_asset
    ADD CONSTRAINT media_asset_object_location_id_fkey FOREIGN KEY (object_location_id, object_key) REFERENCES media.object_location(id, object_key);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.object_location'::regclass AND conname='object_location_profile_id_bucket_fkey') THEN
  ALTER TABLE ONLY media.object_location
    ADD CONSTRAINT object_location_profile_id_bucket_fkey FOREIGN KEY (profile_id, bucket) REFERENCES media.storage_profile(id, bucket);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.object_location'::regclass AND conname='object_location_upload_owner_fkey') THEN
  ALTER TABLE ONLY media.object_location
    ADD CONSTRAINT object_location_upload_owner_fkey FOREIGN KEY (owner_session_id, profile_id, bucket) REFERENCES media.upload_admission(id, profile_id, bucket);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.object_location'::regclass AND conname='object_location_work_owner_fkey') THEN
  ALTER TABLE ONLY media.object_location
    ADD CONSTRAINT object_location_work_owner_fkey FOREIGN KEY (owner_intent_id, profile_id, bucket) REFERENCES media.storage_work_intent(id, profile_id, bucket);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.package_object'::regclass AND conname='package_object_object_location_id_fkey') THEN
  ALTER TABLE ONLY media.package_object
    ADD CONSTRAINT package_object_object_location_id_fkey FOREIGN KEY (object_location_id, object_key) REFERENCES media.object_location(id, object_key);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.project_copy_object'::regclass AND conname='project_copy_object_source_location_id_fkey') THEN
  ALTER TABLE ONLY media.project_copy_object
    ADD CONSTRAINT project_copy_object_source_location_id_fkey FOREIGN KEY (source_location_id, source_object_key) REFERENCES media.object_location(id, object_key);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.project_copy_object'::regclass AND conname='project_copy_object_target_location_id_fkey') THEN
  ALTER TABLE ONLY media.project_copy_object
    ADD CONSTRAINT project_copy_object_target_location_id_fkey FOREIGN KEY (target_location_id, target_object_key) REFERENCES media.object_location(id, object_key);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.purge_object'::regclass AND conname='purge_object_object_location_id_fkey') THEN
  ALTER TABLE ONLY media.purge_object
    ADD CONSTRAINT purge_object_object_location_id_fkey FOREIGN KEY (object_location_id, object_key) REFERENCES media.object_location(id, object_key);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.rendition'::regclass AND conname='rendition_object_location_id_fkey') THEN
  ALTER TABLE ONLY media.rendition
    ADD CONSTRAINT rendition_object_location_id_fkey FOREIGN KEY (object_location_id, object_key) REFERENCES media.object_location(id, object_key);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_activation_guard'::regclass AND conname='storage_activation_guard_revision_fkey') THEN
  ALTER TABLE ONLY media.storage_activation_guard
    ADD CONSTRAINT storage_activation_guard_revision_fkey FOREIGN KEY (revision) REFERENCES media.storage_activation(revision);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_activation'::regclass AND conname='storage_activation_profile_id_fkey') THEN
  ALTER TABLE ONLY media.storage_activation
    ADD CONSTRAINT storage_activation_profile_id_fkey FOREIGN KEY (profile_id) REFERENCES media.storage_profile(id);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_bucket_evidence'::regclass AND conname='storage_bucket_evidence_profile_id_resource_identity_fkey') THEN
  ALTER TABLE ONLY media.storage_bucket_evidence
    ADD CONSTRAINT storage_bucket_evidence_profile_id_resource_identity_fkey FOREIGN KEY (profile_id, resource_identity) REFERENCES media.storage_profile(id, resource_identity);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_config_revision'::regclass AND conname='storage_config_revision_profile_id_fkey') THEN
  ALTER TABLE ONLY media.storage_config_revision
    ADD CONSTRAINT storage_config_revision_profile_id_fkey FOREIGN KEY (profile_id) REFERENCES media.storage_profile(id);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_legacy_binding'::regclass AND conname='storage_legacy_binding_profile_id_bucket_fkey') THEN
  ALTER TABLE ONLY media.storage_legacy_binding
    ADD CONSTRAINT storage_legacy_binding_profile_id_bucket_fkey FOREIGN KEY (profile_id, bucket) REFERENCES media.storage_profile(id, bucket);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_legacy_receipt'::regclass AND conname='storage_legacy_receipt_location_id_object_key_fkey') THEN
  ALTER TABLE ONLY media.storage_legacy_receipt
    ADD CONSTRAINT storage_legacy_receipt_location_id_object_key_fkey FOREIGN KEY (location_id, object_key) REFERENCES media.object_location(id, object_key);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_legacy_receipt'::regclass AND conname='storage_legacy_receipt_profile_id_bucket_fkey') THEN
  ALTER TABLE ONLY media.storage_legacy_receipt
    ADD CONSTRAINT storage_legacy_receipt_profile_id_bucket_fkey FOREIGN KEY (profile_id, bucket) REFERENCES media.storage_profile(id, bucket);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_legacy_tombstone'::regclass AND conname='storage_legacy_tombstone_location_id_object_key_fkey') THEN
  ALTER TABLE ONLY media.storage_legacy_tombstone
    ADD CONSTRAINT storage_legacy_tombstone_location_id_object_key_fkey FOREIGN KEY (location_id, object_key) REFERENCES media.object_location(id, object_key);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_legacy_tombstone'::regclass AND conname='storage_legacy_tombstone_profile_id_bucket_fkey') THEN
  ALTER TABLE ONLY media.storage_legacy_tombstone
    ADD CONSTRAINT storage_legacy_tombstone_profile_id_bucket_fkey FOREIGN KEY (profile_id, bucket) REFERENCES media.storage_profile(id, bucket);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_probe_evidence'::regclass AND conname='storage_probe_evidence_profile_id_fkey') THEN
  ALTER TABLE ONLY media.storage_probe_evidence
    ADD CONSTRAINT storage_probe_evidence_profile_id_fkey FOREIGN KEY (profile_id) REFERENCES media.storage_profile(id);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_probe_intent'::regclass AND conname='storage_probe_intent_profile_id_bucket_fkey') THEN
  ALTER TABLE ONLY media.storage_probe_intent
    ADD CONSTRAINT storage_probe_intent_profile_id_bucket_fkey FOREIGN KEY (profile_id, bucket) REFERENCES media.storage_profile(id, bucket);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_route'::regclass AND conname='storage_route_profile_id_fkey') THEN
  ALTER TABLE ONLY media.storage_route
    ADD CONSTRAINT storage_route_profile_id_fkey FOREIGN KEY (profile_id) REFERENCES media.storage_profile(id);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_work_intent'::regclass AND conname='storage_work_intent_profile_id_bucket_fkey') THEN
  ALTER TABLE ONLY media.storage_work_intent
    ADD CONSTRAINT storage_work_intent_profile_id_bucket_fkey FOREIGN KEY (profile_id, bucket) REFERENCES media.storage_profile(id, bucket);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.transfer_object'::regclass AND conname='transfer_object_source_location_id_fkey') THEN
  ALTER TABLE ONLY media.transfer_object
    ADD CONSTRAINT transfer_object_source_location_id_fkey FOREIGN KEY (source_location_id, source_object_key) REFERENCES media.object_location(id, object_key);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.transfer_object'::regclass AND conname='transfer_object_target_location_id_fkey') THEN
  ALTER TABLE ONLY media.transfer_object
    ADD CONSTRAINT transfer_object_target_location_id_fkey FOREIGN KEY (target_location_id, target_object_key) REFERENCES media.object_location(id, object_key);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.upload_admission'::regclass AND conname='upload_admission_accepted_asset_id_fkey') THEN
  ALTER TABLE ONLY media.upload_admission
    ADD CONSTRAINT upload_admission_accepted_asset_id_fkey FOREIGN KEY (accepted_asset_id) REFERENCES media.media_asset(id);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.upload_admission'::regclass AND conname='upload_admission_actor_id_fkey') THEN
  ALTER TABLE ONLY media.upload_admission
    ADD CONSTRAINT upload_admission_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES identity."user"(id);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.upload_admission'::regclass AND conname='upload_admission_org_id_fkey') THEN
  ALTER TABLE ONLY media.upload_admission
    ADD CONSTRAINT upload_admission_org_id_fkey FOREIGN KEY (org_id) REFERENCES workspace.organization(id);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.upload_admission'::regclass AND conname='upload_admission_profile_id_bucket_fkey') THEN
  ALTER TABLE ONLY media.upload_admission
    ADD CONSTRAINT upload_admission_profile_id_bucket_fkey FOREIGN KEY (profile_id, bucket) REFERENCES media.storage_profile(id, bucket);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='mediatool.depth_object'::regclass AND conname='depth_object_object_location_id_fkey') THEN
  ALTER TABLE ONLY mediatool.depth_object
    ADD CONSTRAINT depth_object_object_location_id_fkey FOREIGN KEY (object_location_id, object_key) REFERENCES media.object_location(id, object_key);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='script.object_intent'::regclass AND conname='object_intent_object_location_id_fkey') THEN
  ALTER TABLE ONLY script.object_intent
    ADD CONSTRAINT object_intent_object_location_id_fkey FOREIGN KEY (object_location_id, object_key) REFERENCES media.object_location(id, object_key);
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='script.object_state'::regclass AND conname='object_state_object_location_id_fkey') THEN
  ALTER TABLE ONLY script.object_state
    ADD CONSTRAINT object_state_object_location_id_fkey FOREIGN KEY (object_location_id, object_key) REFERENCES media.object_location(id, object_key);
 END IF;
END $$;

CREATE OR REPLACE FUNCTION media.keep_object_binding() RETURNS trigger
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
END $$;

CREATE OR REPLACE FUNCTION media.keep_storage_evidence() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 RAISE EXCEPTION 'storage evidence is immutable' USING ERRCODE='23514';
END $$;

CREATE OR REPLACE FUNCTION media.keep_storage_identity() RETURNS trigger
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
END $$;

CREATE OR REPLACE FUNCTION media.keep_storage_probe_intent() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF (to_jsonb(OLD)-'state'-'evidence_sha256'-'updated_at') IS DISTINCT FROM (to_jsonb(NEW)-'state'-'evidence_sha256'-'updated_at')
  OR (OLD.state='running' AND NEW.state NOT IN ('running','complete','cleaning'))
  OR (OLD.state='complete' AND NEW.state<>'complete')
  OR (OLD.state='cleaning' AND NEW.state NOT IN ('cleaning','cleaned'))
  OR (OLD.state='cleaned' AND NEW.state<>'cleaned')
  OR (OLD.evidence_sha256 IS NOT NULL AND OLD.evidence_sha256 IS DISTINCT FROM NEW.evidence_sha256) THEN
  RAISE EXCEPTION 'probe identity/outcome cannot be rebound' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION media.keep_storage_work() RETURNS trigger
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
END $$;

CREATE OR REPLACE FUNCTION media.keep_storage_write_deadline() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF OLD.write_until IS NOT NULL AND (NEW.write_until IS NULL OR NEW.write_until<OLD.write_until) THEN
  RAISE EXCEPTION 'storage write deadline cannot regress' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION media.keep_upload_admission() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF (to_jsonb(NEW)-'accepted_asset_id') IS DISTINCT FROM (to_jsonb(OLD)-'accepted_asset_id')
   OR (OLD.accepted_asset_id IS NOT NULL AND NEW.accepted_asset_id IS DISTINCT FROM OLD.accepted_asset_id) THEN
  RAISE EXCEPTION 'upload admission is immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.media_asset'::regclass AND tgname='keep_object_location_id' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_object_location_id BEFORE UPDATE ON media.media_asset FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('object_location_id');
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.package_object'::regclass AND tgname='keep_object_location_id' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_object_location_id BEFORE UPDATE ON media.package_object FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('object_location_id');
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.purge_object'::regclass AND tgname='keep_object_location_id' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_object_location_id BEFORE UPDATE ON media.purge_object FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('object_location_id');
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.rendition'::regclass AND tgname='keep_object_location_id' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_object_location_id BEFORE UPDATE ON media.rendition FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('object_location_id');
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.project_copy_object'::regclass AND tgname='keep_source_location_id' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_source_location_id BEFORE UPDATE ON media.project_copy_object FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('source_location_id');
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.transfer_object'::regclass AND tgname='keep_source_location_id' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_source_location_id BEFORE UPDATE ON media.transfer_object FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('source_location_id');
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.storage_activation_guard'::regclass AND tgname='keep_storage_evidence' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_storage_evidence BEFORE DELETE OR UPDATE ON media.storage_activation_guard FOR EACH ROW EXECUTE FUNCTION media.keep_storage_evidence();
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.storage_bucket_evidence'::regclass AND tgname='keep_storage_evidence' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_storage_evidence BEFORE DELETE OR UPDATE ON media.storage_bucket_evidence FOR EACH ROW EXECUTE FUNCTION media.keep_storage_evidence();
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.storage_config_revision'::regclass AND tgname='keep_storage_evidence' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_storage_evidence BEFORE DELETE OR UPDATE ON media.storage_config_revision FOR EACH ROW EXECUTE FUNCTION media.keep_storage_evidence();
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.storage_deployment_evidence'::regclass AND tgname='keep_storage_evidence' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_storage_evidence BEFORE DELETE OR UPDATE ON media.storage_deployment_evidence FOR EACH ROW EXECUTE FUNCTION media.keep_storage_evidence();
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.storage_legacy_binding'::regclass AND tgname='keep_storage_evidence' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_storage_evidence BEFORE DELETE OR UPDATE ON media.storage_legacy_binding FOR EACH ROW EXECUTE FUNCTION media.keep_storage_evidence();
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.storage_legacy_receipt'::regclass AND tgname='keep_storage_evidence' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_storage_evidence BEFORE DELETE OR UPDATE ON media.storage_legacy_receipt FOR EACH ROW EXECUTE FUNCTION media.keep_storage_evidence();
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.storage_legacy_tombstone'::regclass AND tgname='keep_storage_evidence' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_storage_evidence BEFORE DELETE OR UPDATE ON media.storage_legacy_tombstone FOR EACH ROW EXECUTE FUNCTION media.keep_storage_evidence();
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.storage_probe_evidence'::regclass AND tgname='keep_storage_evidence' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_storage_evidence BEFORE DELETE OR UPDATE ON media.storage_probe_evidence FOR EACH ROW EXECUTE FUNCTION media.keep_storage_evidence();
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.object_location'::regclass AND tgname='keep_storage_identity' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_storage_identity BEFORE UPDATE ON media.object_location FOR EACH ROW EXECUTE FUNCTION media.keep_storage_identity();
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.storage_activation'::regclass AND tgname='keep_storage_identity' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_storage_identity BEFORE DELETE OR UPDATE ON media.storage_activation FOR EACH ROW EXECUTE FUNCTION media.keep_storage_identity();
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.storage_profile'::regclass AND tgname='keep_storage_identity' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_storage_identity BEFORE DELETE OR UPDATE ON media.storage_profile FOR EACH ROW EXECUTE FUNCTION media.keep_storage_identity();
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.storage_probe_intent'::regclass AND tgname='keep_storage_probe_intent' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_storage_probe_intent BEFORE UPDATE ON media.storage_probe_intent FOR EACH ROW EXECUTE FUNCTION media.keep_storage_probe_intent();
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.object_location'::regclass AND tgname='keep_storage_work' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_storage_work BEFORE UPDATE ON media.object_location FOR EACH ROW EXECUTE FUNCTION media.keep_storage_work();
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.storage_work_intent'::regclass AND tgname='keep_storage_work' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_storage_work BEFORE UPDATE ON media.storage_work_intent FOR EACH ROW EXECUTE FUNCTION media.keep_storage_work();
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.object_location'::regclass AND tgname='keep_storage_write_deadline' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_storage_write_deadline BEFORE UPDATE ON media.object_location FOR EACH ROW EXECUTE FUNCTION media.keep_storage_write_deadline();
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.project_copy_object'::regclass AND tgname='keep_target_location_id' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_target_location_id BEFORE UPDATE ON media.project_copy_object FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('target_location_id');
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.transfer_object'::regclass AND tgname='keep_target_location_id' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_target_location_id BEFORE UPDATE ON media.transfer_object FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('target_location_id');
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.upload_admission'::regclass AND tgname='keep_upload_admission' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_upload_admission BEFORE UPDATE ON media.upload_admission FOR EACH ROW EXECUTE FUNCTION media.keep_upload_admission();
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='mediatool.depth_object'::regclass AND tgname='keep_object_location_id' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_object_location_id BEFORE UPDATE ON mediatool.depth_object FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('object_location_id');
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='script.object_intent'::regclass AND tgname='keep_object_location_id' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_object_location_id BEFORE UPDATE ON script.object_intent FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('object_location_id');
 END IF;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='script.object_state'::regclass AND tgname='keep_object_location_id' AND NOT tgisinternal) THEN
  CREATE TRIGGER keep_object_location_id BEFORE UPDATE ON script.object_state FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('object_location_id');
 END IF;
END $$;

CREATE INDEX IF NOT EXISTS object_location_cleanup_idx ON media.object_location USING btree (state, updated_at) WHERE (state = ANY (ARRAY['reserved'::text, 'present'::text, 'delete_pending'::text, 'unknown'::text]));

CREATE INDEX IF NOT EXISTS storage_work_open_profile_idx ON media.storage_work_intent USING btree (profile_id, state);

GRANT SELECT,INSERT,UPDATE ON TABLE media.object_location TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.storage_activation TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.storage_activation_guard TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.storage_bucket_evidence TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.storage_config_revision TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.storage_deployment_evidence TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.storage_legacy_binding TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.storage_legacy_receipt TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.storage_legacy_tombstone TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.storage_probe_evidence TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE media.storage_probe_intent TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.storage_profile TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE media.storage_route TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE media.storage_runtime TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE media.storage_work_intent TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE media.upload_admission TO glowloom_app;

-- Deliberately no registration, activation, backfill, NOT NULL or bare-key key removal.
