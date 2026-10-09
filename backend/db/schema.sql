-- Lanverse 当前数据库结构的唯一事实源。直接维护本文件，禁止另存手写 DDL 副本。
-- PostgreSQL 18；仅用于空业务库，不包含业务数据、登录账号或密码。
-- 由独立结构所有者执行，预先创建 lanverse_app NOLOGIN NOSUPERUSER 权限角色。
-- 初始化：psql "$LV_SCHEMA_DB_DSN" -X --single-transaction -v ON_ERROR_STOP=1 -f backend/db/schema.sql
-- 既有业务库的升级须先核对实际结构并审阅增量 SQL；禁止重放本文件或自动删除数据。

-- 尽早拒绝错误目标与未配置的应用角色，调用方的事务保证失败不留下部分结构。
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_roles
    WHERE rolname = 'lanverse_app' AND NOT rolcanlogin AND NOT rolsuper
  ) THEN
    RAISE EXCEPTION 'lanverse_app must be a pre-provisioned NOLOGIN, NOSUPERUSER role';
  END IF;
  IF EXISTS (
    SELECT 1 FROM pg_namespace
    WHERE nspname <> 'public' AND nspname <> 'information_schema' AND nspname !~ '^pg_'
  ) OR EXISTS (
    SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p', 'v', 'm', 'S', 'f')
  ) THEN
    RAISE EXCEPTION 'schema.sql requires an empty business database';
  END IF;
END $$;

-- 函数中引用随后定义的行类型；函数体由完整初始化后的行为测试验证。
SET LOCAL check_function_bodies = false;
SET LOCAL search_path = pg_catalog, public;
SET LOCAL timezone = 'UTC';

-- 命名空间与扩展
CREATE SCHEMA audit;

CREATE SCHEMA bible;

CREATE SCHEMA billing;

CREATE SCHEMA canvas;

CREATE SCHEMA catalog;

CREATE SCHEMA identity;

CREATE SCHEMA infra;

CREATE SCHEMA media;

CREATE SCHEMA mediatool;

CREATE SCHEMA operation;

CREATE SCHEMA script;

CREATE SCHEMA workspace;

CREATE EXTENSION IF NOT EXISTS citext WITH SCHEMA public;

COMMENT ON EXTENSION citext IS 'data type for case-insensitive character strings';

CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;

COMMENT ON EXTENSION pg_trgm IS 'text similarity measurement and index searching based on trigrams';

-- 触发器函数
CREATE FUNCTION audit.reject_audit_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  RAISE EXCEPTION 'audit.audit_log is append-only' USING ERRCODE = '42501';
END $$;

CREATE FUNCTION billing.keep_ledger_entry_append_only() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'ledger entries are append only';
  END IF;
  IF OLD.is_delete OR NOT NEW.is_delete OR
     (to_jsonb(NEW) - 'is_delete' - 'update_time') IS DISTINCT FROM
     (to_jsonb(OLD) - 'is_delete' - 'update_time') THEN
    RAISE EXCEPTION 'ledger entries are append only';
  END IF;
  NEW.update_time := now();
  RETURN NEW;
END;
$$;

CREATE FUNCTION billing.set_budget_update_time() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  NEW.update_time := now();
  RETURN NEW;
END;
$$;

CREATE FUNCTION media.check_library_asset_scope() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE held media.library%ROWTYPE; original media.media_asset%ROWTYPE;
BEGIN
  SELECT * INTO STRICT held FROM media.library WHERE id=NEW.library_id;
  IF NEW.asset_id IS NOT NULL THEN
    SELECT * INTO STRICT original FROM media.media_asset WHERE id=NEW.asset_id;
    IF (held.kind='project' AND (original.project_id IS DISTINCT FROM held.project_id OR original.personal_actor_id IS NOT NULL))
      OR (held.kind='personal' AND (original.project_id IS NOT NULL OR original.personal_org_id IS DISTINCT FROM held.org_id OR original.personal_actor_id IS DISTINCT FROM held.personal_actor_id)) THEN
      RAISE EXCEPTION 'library asset ownership mismatch' USING ERRCODE='23514';
    END IF;
  END IF;
  IF held.kind='project' AND NEW.favorite THEN
    RAISE EXCEPTION 'project library has no personal favorite' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END;
$$;

CREATE FUNCTION media.check_package_scope() RETURNS trigger
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

CREATE FUNCTION media.check_purge_scope() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
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

CREATE FUNCTION media.check_transfer_scope() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
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

CREATE FUNCTION media.check_upload_receipt_scope() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM media.media_asset a WHERE a.id=NEW.asset_id AND
  ((NEW.project_id IS NOT NULL AND a.project_id=NEW.project_id AND a.personal_org_id IS NULL AND a.personal_actor_id IS NULL)
   OR (NEW.project_id IS NULL AND a.project_id IS NULL AND a.personal_org_id=NEW.personal_org_id AND a.personal_actor_id=NEW.principal_id))) THEN
  RAISE EXCEPTION 'upload receipt ownership does not match its original' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION media.reject_library_command_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN RAISE EXCEPTION 'library command is immutable' USING ERRCODE='42501'; END;
$$;

CREATE FUNCTION media.reject_package_command_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN RAISE EXCEPTION 'media package command is immutable' USING ERRCODE='42501'; END $$;

CREATE FUNCTION media.reject_package_frozen_mutation() RETURNS trigger
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

CREATE FUNCTION media.reject_purge_command_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN RAISE EXCEPTION 'media purge command is immutable' USING ERRCODE='42501'; END;
$$;

CREATE FUNCTION media.reject_transfer_command_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN RAISE EXCEPTION 'media transfer command is immutable' USING ERRCODE='42501'; END;
$$;

CREATE FUNCTION workspace.protect_project_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF NEW.aspect_ratio IS DISTINCT FROM OLD.aspect_ratio
     OR NEW.style_type IS DISTINCT FROM OLD.style_type THEN
    RAISE EXCEPTION 'project aspect_ratio and style_type are immutable';
  END IF;
  NEW.update_time := now();
  RETURN NEW;
END;
$$;

CREATE FUNCTION workspace.reject_project_change_command_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 RAISE EXCEPTION 'project change command is immutable' USING ERRCODE='42501';
END;
$$;

CREATE FUNCTION workspace.reject_project_folder_command_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 RAISE EXCEPTION 'project folder command is immutable' USING ERRCODE='42501';
END;
$$;

CREATE FUNCTION workspace.set_style_preset_update_time() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  NEW.update_time := now();
  RETURN NEW;
END;
$$;

-- 当前表结构（含字段、默认值与 CHECK）
CREATE TABLE audit.audit_log (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid,
    actor_id uuid,
    actor_kind text NOT NULL,
    action text NOT NULL,
    object_type text NOT NULL,
    object_id text NOT NULL,
    before jsonb,
    after jsonb,
    request_id text,
    trace_id text,
    ip inet,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    CONSTRAINT audit_log_actor_kind_check CHECK ((actor_kind = ANY (ARRAY['user'::text, 'agent'::text, 'system'::text]))),
    CONSTRAINT audit_log_is_delete_check CHECK ((NOT is_delete))
)
PARTITION BY RANGE (create_time);

CREATE TABLE bible."character" (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    revision bigint NOT NULL,
    current_version_id uuid NOT NULL,
    confirmed_version_id uuid,
    redirect_id uuid,
    is_delete boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT character_check CHECK (((redirect_id IS NULL) OR (redirect_id <> id))),
    CONSTRAINT character_revision_check CHECK (((revision >= 1) AND (revision <= 2147483647)))
);

CREATE TABLE bible.character_confirmation (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    entry_id uuid NOT NULL,
    version_id uuid NOT NULL,
    revision bigint NOT NULL,
    actor_id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT character_confirmation_revision_check CHECK ((revision > 0))
);

CREATE TABLE bible.character_redirect (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    source_id uuid NOT NULL,
    target_id uuid NOT NULL,
    source_version_id uuid NOT NULL,
    target_version_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT character_redirect_check CHECK ((source_id <> target_id))
);

CREATE TABLE bible.character_split (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    source_id uuid NOT NULL,
    source_version_id uuid NOT NULL,
    target_id uuid NOT NULL,
    target_version_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT character_split_check CHECK ((source_id <> target_id))
);

CREATE TABLE bible.character_version (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    entry_id uuid NOT NULL,
    version_no bigint NOT NULL,
    previous_id uuid,
    actor_id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL,
    origin text NOT NULL,
    result_source jsonb,
    content jsonb NOT NULL,
    content_sha256 text NOT NULL,
    CONSTRAINT character_version_check CHECK ((((version_no = 1) AND (previous_id IS NULL)) OR ((version_no > 1) AND (previous_id IS NOT NULL)))),
    CONSTRAINT character_version_check1 CHECK ((((origin = 'manual'::text) AND (result_source IS NULL)) OR ((origin = 'ai'::text) AND (jsonb_typeof(result_source) = 'object'::text)))),
    CONSTRAINT character_version_content_check CHECK (((jsonb_typeof(content) = 'object'::text) AND (pg_column_size(content) <= 4194304))),
    CONSTRAINT character_version_content_sha256_check CHECK ((content_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT character_version_origin_check CHECK ((origin = ANY (ARRAY['manual'::text, 'ai'::text]))),
    CONSTRAINT character_version_version_no_check CHECK (((version_no >= 1) AND (version_no <= 2147483647)))
);

CREATE TABLE bible.command (
    actor_id uuid NOT NULL,
    request_id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    kind text NOT NULL,
    action text NOT NULL,
    request_hash text NOT NULL,
    response jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT command_kind_check CHECK ((kind = ANY (ARRAY['character'::text, 'location'::text, 'prop'::text]))),
    CONSTRAINT command_request_hash_check CHECK ((request_hash ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT command_response_check CHECK (((jsonb_typeof(response) = 'object'::text) AND (pg_column_size(response) <= 16384)))
);

CREATE TABLE bible.copy_transfer_receipt (
    snapshot_id uuid NOT NULL,
    manifest_sha256 text NOT NULL,
    content_sha256 text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT copy_transfer_receipt_content_sha256_check CHECK ((content_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT copy_transfer_receipt_manifest_sha256_check CHECK ((manifest_sha256 ~ '^[a-f0-9]{64}$'::text))
);

CREATE TABLE bible.copy_cleanup_receipt (
    snapshot_id uuid CONSTRAINT copy_transfer_receipt_snapshot_id_not_null NOT NULL,
    manifest_sha256 text CONSTRAINT copy_transfer_receipt_manifest_sha256_not_null NOT NULL,
    content_sha256 text CONSTRAINT copy_transfer_receipt_content_sha256_not_null NOT NULL,
    created_at timestamp with time zone CONSTRAINT copy_transfer_receipt_created_at_not_null NOT NULL,
    CONSTRAINT copy_transfer_receipt_content_sha256_check CHECK ((content_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT copy_transfer_receipt_manifest_sha256_check CHECK ((manifest_sha256 ~ '^[a-f0-9]{64}$'::text))
);

CREATE TABLE bible.copy_receipt (
    snapshot_id uuid CONSTRAINT copy_transfer_receipt_snapshot_id_not_null NOT NULL,
    manifest_sha256 text CONSTRAINT copy_transfer_receipt_manifest_sha256_not_null NOT NULL,
    content_sha256 text CONSTRAINT copy_transfer_receipt_content_sha256_not_null NOT NULL,
    created_at timestamp with time zone CONSTRAINT copy_transfer_receipt_created_at_not_null NOT NULL,
    CONSTRAINT copy_transfer_receipt_content_sha256_check CHECK ((content_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT copy_transfer_receipt_manifest_sha256_check CHECK ((manifest_sha256 ~ '^[a-f0-9]{64}$'::text))
);

CREATE TABLE bible.copy_snapshot (
    id uuid NOT NULL,
    job_id uuid NOT NULL,
    org_id uuid NOT NULL,
    source_project_id uuid NOT NULL,
    target_project_id uuid NOT NULL,
    manifest jsonb NOT NULL,
    manifest_sha256 text NOT NULL,
    content_sha256 text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT copy_snapshot_check CHECK ((source_project_id <> target_project_id)),
    CONSTRAINT copy_snapshot_content_sha256_check CHECK ((content_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT copy_snapshot_manifest_check CHECK ((jsonb_typeof(manifest) = 'object'::text)),
    CONSTRAINT copy_snapshot_manifest_sha256_check CHECK ((manifest_sha256 ~ '^[a-f0-9]{64}$'::text))
);

CREATE TABLE bible.location (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    revision bigint NOT NULL,
    current_version_id uuid NOT NULL,
    confirmed_version_id uuid,
    is_delete boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT location_revision_check CHECK (((revision >= 1) AND (revision <= 2147483647)))
);

CREATE TABLE bible.location_confirmation (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    entry_id uuid NOT NULL,
    version_id uuid NOT NULL,
    revision bigint NOT NULL,
    actor_id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT location_confirmation_revision_check CHECK ((revision > 0))
);

CREATE TABLE bible.location_version (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    entry_id uuid NOT NULL,
    version_no bigint NOT NULL,
    previous_id uuid,
    actor_id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL,
    origin text NOT NULL,
    result_source jsonb,
    content jsonb NOT NULL,
    content_sha256 text NOT NULL,
    CONSTRAINT location_version_check CHECK ((((version_no = 1) AND (previous_id IS NULL)) OR ((version_no > 1) AND (previous_id IS NOT NULL)))),
    CONSTRAINT location_version_check1 CHECK ((((origin = 'manual'::text) AND (result_source IS NULL)) OR ((origin = 'ai'::text) AND (jsonb_typeof(result_source) = 'object'::text)))),
    CONSTRAINT location_version_content_check CHECK (((jsonb_typeof(content) = 'object'::text) AND (pg_column_size(content) <= 4194304))),
    CONSTRAINT location_version_content_sha256_check CHECK ((content_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT location_version_origin_check CHECK ((origin = ANY (ARRAY['manual'::text, 'ai'::text]))),
    CONSTRAINT location_version_version_no_check CHECK (((version_no >= 1) AND (version_no <= 2147483647)))
);

CREATE TABLE bible.look (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    character_id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE TABLE bible.look_version (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    look_id uuid NOT NULL,
    character_version_id uuid NOT NULL,
    "position" integer NOT NULL,
    name text NOT NULL,
    description text NOT NULL,
    is_default boolean NOT NULL,
    applies_to jsonb,
    CONSTRAINT look_version_description_check CHECK ((char_length(description) <= 8192)),
    CONSTRAINT look_version_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 512))),
    CONSTRAINT look_version_position_check CHECK ((("position" >= 0) AND ("position" <= 199)))
);

CREATE TABLE bible.prop (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    revision bigint NOT NULL,
    current_version_id uuid NOT NULL,
    confirmed_version_id uuid,
    is_delete boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT prop_revision_check CHECK (((revision >= 1) AND (revision <= 2147483647)))
);

CREATE TABLE bible.prop_confirmation (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    entry_id uuid NOT NULL,
    version_id uuid NOT NULL,
    revision bigint NOT NULL,
    actor_id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT prop_confirmation_revision_check CHECK ((revision > 0))
);

CREATE TABLE bible.prop_version (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    entry_id uuid NOT NULL,
    version_no bigint NOT NULL,
    previous_id uuid,
    actor_id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL,
    origin text NOT NULL,
    result_source jsonb,
    content jsonb NOT NULL,
    content_sha256 text NOT NULL,
    CONSTRAINT prop_version_check CHECK ((((version_no = 1) AND (previous_id IS NULL)) OR ((version_no > 1) AND (previous_id IS NOT NULL)))),
    CONSTRAINT prop_version_check1 CHECK ((((origin = 'manual'::text) AND (result_source IS NULL)) OR ((origin = 'ai'::text) AND (jsonb_typeof(result_source) = 'object'::text)))),
    CONSTRAINT prop_version_content_check CHECK (((jsonb_typeof(content) = 'object'::text) AND (pg_column_size(content) <= 4194304))),
    CONSTRAINT prop_version_content_sha256_check CHECK ((content_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT prop_version_origin_check CHECK ((origin = ANY (ARRAY['manual'::text, 'ai'::text]))),
    CONSTRAINT prop_version_version_no_check CHECK (((version_no >= 1) AND (version_no <= 2147483647)))
);

CREATE TABLE bible.reference_version (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    look_version_id uuid NOT NULL,
    "position" integer NOT NULL,
    role text NOT NULL,
    media_asset_id uuid NOT NULL,
    media_revision bigint NOT NULL,
    media_sha256 text NOT NULL,
    media_bytes bigint NOT NULL,
    rendition_id uuid NOT NULL,
    rendition_sha256 text NOT NULL,
    CONSTRAINT reference_version_media_bytes_check CHECK ((media_bytes > 0)),
    CONSTRAINT reference_version_media_revision_check CHECK ((media_revision > 0)),
    CONSTRAINT reference_version_media_sha256_check CHECK ((media_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT reference_version_position_check CHECK ((("position" >= 0) AND ("position" <= 7))),
    CONSTRAINT reference_version_rendition_sha256_check CHECK ((rendition_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT reference_version_role_check CHECK ((role = ANY (ARRAY['primary'::text, 'front'::text, 'side'::text, 'back'::text, 'turnaround_sheet'::text, 'expression_sheet'::text])))
);

CREATE TABLE bible.voice_version (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    character_version_id uuid NOT NULL,
    source_kind text NOT NULL,
    content jsonb NOT NULL,
    CONSTRAINT voice_version_content_check CHECK ((jsonb_typeof(content) = 'object'::text)),
    CONSTRAINT voice_version_source_kind_check CHECK ((source_kind = ANY (ARRAY['catalog'::text, 'sample'::text])))
);

CREATE TABLE billing.budget (
    id uuid NOT NULL,
    project_id uuid NOT NULL,
    limit_micros bigint NOT NULL,
    reserved_micros bigint DEFAULT 0 NOT NULL,
    settled_micros bigint DEFAULT 0 NOT NULL,
    is_overrun boolean DEFAULT false NOT NULL,
    revision integer DEFAULT 1 NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    update_by uuid,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    CONSTRAINT budget_limit_micros_check CHECK ((limit_micros >= 0)),
    CONSTRAINT budget_reserved_micros_check CHECK ((reserved_micros >= 0)),
    CONSTRAINT budget_revision_check CHECK ((revision > 0)),
    CONSTRAINT budget_settled_micros_check CHECK ((settled_micros >= 0)),
    CONSTRAINT ck_budget_balance CHECK ((is_overrun OR ((reserved_micros + settled_micros) <= limit_micros)))
);

CREATE TABLE billing.ledger_entry (
    id uuid NOT NULL,
    project_id uuid NOT NULL,
    entry_type text NOT NULL,
    amount_micros bigint NOT NULL,
    operation_id uuid,
    episode_id uuid,
    shot_id uuid,
    model_key text,
    region text,
    orig_currency character(3),
    orig_amount_micros bigint,
    fx_rate numeric(12,6),
    note text DEFAULT ''::text NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    create_by uuid,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    CONSTRAINT ledger_entry_entry_type_check CHECK ((entry_type = ANY (ARRAY['reserve'::text, 'settle'::text, 'release'::text, 'adjust'::text, 'budget_change'::text, 'provider_overage'::text])))
);

CREATE TABLE billing.reservation (
    id uuid NOT NULL,
    project_id uuid NOT NULL,
    operation_id uuid NOT NULL,
    amount_micros bigint NOT NULL,
    status text NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    closed_at timestamp with time zone,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    CONSTRAINT reservation_amount_micros_check CHECK ((amount_micros >= 0)),
    CONSTRAINT reservation_status_check CHECK ((status = ANY (ARRAY['held'::text, 'settled'::text, 'released'::text])))
);

CREATE TABLE canvas.command_log (
    id uuid NOT NULL,
    document_id uuid NOT NULL,
    revision integer NOT NULL,
    commands jsonb NOT NULL,
    create_by uuid,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL
);

CREATE TABLE canvas.document (
    id uuid NOT NULL,
    project_id uuid NOT NULL,
    name text NOT NULL,
    scope jsonb DEFAULT '{}'::jsonb NOT NULL,
    revision integer DEFAULT 1 NOT NULL,
    viewport jsonb DEFAULT '{}'::jsonb NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    CONSTRAINT document_revision_check CHECK ((revision > 0))
);

CREATE TABLE canvas.edge (
    id uuid NOT NULL,
    document_id uuid NOT NULL,
    edge_type text NOT NULL,
    source_node_id uuid NOT NULL,
    target_node_id uuid NOT NULL,
    role text,
    binding jsonb,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    CONSTRAINT edge_edge_type_check CHECK ((edge_type = ANY (ARRAY['input'::text, 'reference'::text, 'promote'::text, 'annotation'::text])))
);

CREATE TABLE canvas.node (
    id uuid NOT NULL,
    document_id uuid NOT NULL,
    node_type text NOT NULL,
    node_action text,
    ref_type text,
    ref_id uuid,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    x double precision NOT NULL,
    y double precision NOT NULL,
    width double precision,
    height double precision,
    parent_id uuid,
    z_index integer DEFAULT 0 NOT NULL,
    last_operation_id uuid,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    title text DEFAULT '备注'::text NOT NULL,
    CONSTRAINT ck_canvas_node_title CHECK (((char_length(title) >= 1) AND (char_length(title) <= 128))),
    CONSTRAINT node_node_action_check CHECK ((node_action = ANY (ARRAY['resource'::text, 'generate'::text, 'edit'::text, 'tool'::text])))
);

CREATE TABLE canvas.project_copy_receipt (
    snapshot_id uuid NOT NULL,
    content_sha256 text NOT NULL,
    document_count integer NOT NULL,
    cleared_operation_bindings integer NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT project_copy_receipt_cleared_operation_bindings_check CHECK ((cleared_operation_bindings >= 0)),
    CONSTRAINT project_copy_receipt_content_sha256_check CHECK ((content_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT project_copy_receipt_document_count_check CHECK ((document_count >= 0))
);

CREATE TABLE canvas.project_copy_snapshot (
    id uuid NOT NULL,
    job_id uuid NOT NULL,
    org_id uuid NOT NULL,
    source_project_id uuid NOT NULL,
    target_project_id uuid NOT NULL,
    manifest_sha256 text NOT NULL,
    document_count integer NOT NULL,
    asset_mapping jsonb NOT NULL,
    documents jsonb NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT project_copy_snapshot_asset_mapping_check CHECK ((jsonb_typeof(asset_mapping) = 'object'::text)),
    CONSTRAINT project_copy_snapshot_check CHECK ((source_project_id <> target_project_id)),
    CONSTRAINT project_copy_snapshot_document_count_check CHECK (((document_count >= 0) AND (document_count <= 256))),
    CONSTRAINT project_copy_snapshot_documents_check CHECK (((jsonb_typeof(documents) = 'array'::text) AND (octet_length((documents)::text) <= 33554432))),
    CONSTRAINT project_copy_snapshot_manifest_sha256_check CHECK ((manifest_sha256 ~ '^[0-9a-f]{64}$'::text))
);

CREATE TABLE catalog.capability (
    id uuid NOT NULL,
    key text NOT NULL,
    output_type text NOT NULL,
    modes text[] NOT NULL,
    input_roles text[] NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    CONSTRAINT capability_key_check CHECK ((btrim(key) <> ''::text)),
    CONSTRAINT capability_modes_check CHECK ((cardinality(modes) > 0)),
    CONSTRAINT capability_output_type_check CHECK ((output_type = ANY (ARRAY['json'::text, 'image'::text, 'video'::text, 'audio'::text, 'none'::text])))
);

CREATE TABLE catalog.model_profile (
    id uuid NOT NULL,
    model_key text NOT NULL,
    provider_id uuid NOT NULL,
    capability text NOT NULL,
    display_name text NOT NULL,
    status text DEFAULT 'disabled'::text NOT NULL,
    current_version_id uuid,
    revision integer DEFAULT 1 NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    CONSTRAINT model_profile_display_name_check CHECK ((btrim(display_name) <> ''::text)),
    CONSTRAINT model_profile_model_key_check CHECK ((btrim(model_key) <> ''::text)),
    CONSTRAINT model_profile_revision_check CHECK ((revision > 0)),
    CONSTRAINT model_profile_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text])))
);

CREATE TABLE catalog.model_profile_version (
    id uuid NOT NULL,
    model_profile_id uuid NOT NULL,
    version_no integer NOT NULL,
    provider_model_id text NOT NULL,
    modes text[] NOT NULL,
    limits jsonb NOT NULL,
    param_schema jsonb NOT NULL,
    supports_query boolean NOT NULL,
    supports_cancel boolean NOT NULL,
    supports_callback boolean NOT NULL,
    expected_max_ms integer NOT NULL,
    moderation text DEFAULT 'provider'::text NOT NULL,
    queue text DEFAULT 'agent'::text NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    create_by uuid,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    CONSTRAINT model_profile_version_expected_max_ms_check CHECK ((expected_max_ms > 0)),
    CONSTRAINT model_profile_version_limits_check CHECK ((jsonb_typeof(limits) = 'object'::text)),
    CONSTRAINT model_profile_version_moderation_check CHECK ((moderation = ANY (ARRAY['provider'::text, 'platform'::text, 'both'::text]))),
    CONSTRAINT model_profile_version_modes_check CHECK ((cardinality(modes) > 0)),
    CONSTRAINT model_profile_version_param_schema_check CHECK ((jsonb_typeof(param_schema) = 'array'::text)),
    CONSTRAINT model_profile_version_provider_model_id_check CHECK ((btrim(provider_model_id) <> ''::text)),
    CONSTRAINT model_profile_version_queue_check CHECK ((btrim(queue) <> ''::text)),
    CONSTRAINT model_profile_version_version_no_check CHECK ((version_no > 0))
);

CREATE TABLE catalog.price_rule_version (
    id uuid NOT NULL,
    model_profile_id uuid NOT NULL,
    version_no integer NOT NULL,
    unit text NOT NULL,
    rule jsonb NOT NULL,
    currency character(3) DEFAULT 'CNY'::bpchar NOT NULL,
    fx_rate_to_cny numeric(12,6),
    effective_from timestamp with time zone NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    create_by uuid,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    CONSTRAINT ck_price_rule_foreign_exchange CHECK (((currency = 'CNY'::bpchar) OR (fx_rate_to_cny IS NOT NULL))),
    CONSTRAINT price_rule_version_currency_check CHECK ((currency ~ '^[A-Z]{3}$'::text)),
    CONSTRAINT price_rule_version_fx_rate_to_cny_check CHECK ((fx_rate_to_cny > (0)::numeric)),
    CONSTRAINT price_rule_version_rule_check CHECK ((jsonb_typeof(rule) = 'object'::text)),
    CONSTRAINT price_rule_version_unit_check CHECK ((unit = ANY (ARRAY['per_image'::text, 'per_second'::text, 'per_request'::text, 'per_1k_tokens'::text, 'per_1k_chars'::text]))),
    CONSTRAINT price_rule_version_version_no_check CHECK ((version_no > 0))
);

CREATE TABLE catalog.provider (
    id uuid NOT NULL,
    key text NOT NULL,
    name text NOT NULL,
    adapter_key text NOT NULL,
    region text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    concurrency_limit integer DEFAULT 10 NOT NULL,
    rate_limit_per_min integer DEFAULT 60 NOT NULL,
    revision integer DEFAULT 1 NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    CONSTRAINT provider_adapter_key_check CHECK ((btrim(adapter_key) <> ''::text)),
    CONSTRAINT provider_concurrency_limit_check CHECK ((concurrency_limit > 0)),
    CONSTRAINT provider_key_check CHECK ((btrim(key) <> ''::text)),
    CONSTRAINT provider_name_check CHECK ((btrim(name) <> ''::text)),
    CONSTRAINT provider_rate_limit_per_min_check CHECK ((rate_limit_per_min > 0)),
    CONSTRAINT provider_region_check CHECK ((region = ANY (ARRAY['domestic'::text, 'overseas'::text]))),
    CONSTRAINT provider_revision_check CHECK ((revision > 0)),
    CONSTRAINT provider_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text])))
);

CREATE TABLE catalog.provider_credential (
    id uuid NOT NULL,
    provider_id uuid NOT NULL,
    label text NOT NULL,
    ciphertext bytea NOT NULL,
    key_id text NOT NULL,
    last4 text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    last_tested_at timestamp with time zone,
    last_test_result text,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    CONSTRAINT provider_credential_ciphertext_check CHECK ((octet_length(ciphertext) > 0)),
    CONSTRAINT provider_credential_key_id_check CHECK ((btrim(key_id) <> ''::text)),
    CONSTRAINT provider_credential_label_check CHECK ((btrim(label) <> ''::text)),
    CONSTRAINT provider_credential_last4_check CHECK ((char_length(last4) = 4)),
    CONSTRAINT provider_credential_last_test_result_check CHECK ((last_test_result = ANY (ARRAY['ok'::text, 'auth_failed'::text, 'unreachable'::text, 'timeout'::text, 'unsupported'::text]))),
    CONSTRAINT provider_credential_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text])))
);

CREATE TABLE identity."user" (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    login_name public.citext NOT NULL,
    display_name text NOT NULL,
    role text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    password_hash text NOT NULL,
    must_change_password boolean DEFAULT true NOT NULL,
    password_changed_at timestamp with time zone,
    failed_login_count integer DEFAULT 0 NOT NULL,
    locked_until timestamp with time zone,
    session_epoch integer DEFAULT 1 NOT NULL,
    last_login_at timestamp with time zone,
    revision integer DEFAULT 1 NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    CONSTRAINT user_display_name_check CHECK (((char_length(display_name) >= 1) AND (char_length(display_name) <= 50))),
    CONSTRAINT user_failed_login_count_check CHECK ((failed_login_count >= 0)),
    CONSTRAINT user_revision_check CHECK ((revision > 0)),
    CONSTRAINT user_role_check CHECK ((role = ANY (ARRAY['admin'::text, 'producer'::text]))),
    CONSTRAINT user_session_epoch_check CHECK ((session_epoch > 0)),
    CONSTRAINT user_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text])))
);

-- M1 认证增量：登录名全局唯一，会话摘要、滚动失败窗口与安全事件。
CREATE UNIQUE INDEX uq_user_login_global ON identity."user" (lower(btrim(login_name::text))) WHERE NOT is_delete;

CREATE TABLE identity.user_session (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL,
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    credential_revision bigint NOT NULL CHECK (credential_revision > 0),
    created_at timestamptz NOT NULL,
    last_active_at timestamptz NOT NULL,
    absolute_expires_at timestamptz NOT NULL,
    persistent boolean NOT NULL,
    revoked_at timestamptz,
    CHECK (last_active_at >= created_at AND absolute_expires_at > created_at)
);
CREATE INDEX user_session_account_idx ON identity.user_session (account_id);

CREATE TABLE identity.login_guard (
    login_key text PRIMARY KEY CHECK (login_key ~ '^[a-z0-9._-]{1,64}$'),
    failure_times timestamptz[] NOT NULL DEFAULT '{}',
    locked_until timestamptz,
    CHECK (cardinality(failure_times) <= 5)
);

CREATE TABLE identity.auth_event (
    id uuid PRIMARY KEY,
    account_id uuid,
    action text NOT NULL CHECK (action IN ('registered','logged_in','login_failed','logged_out','password_changed')),
    created_at timestamptz NOT NULL
);

CREATE TABLE infra.idempotency_record (
    id uuid NOT NULL,
    actor_id uuid NOT NULL,
    idem_key text NOT NULL,
    request_hash text NOT NULL,
    status_code integer,
    response_body jsonb,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL
);

CREATE TABLE infra.outbox (
    id uuid NOT NULL,
    topic text NOT NULL,
    partition_key text NOT NULL,
    payload jsonb NOT NULL,
    headers jsonb DEFAULT '{}'::jsonb NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    published_at timestamp with time zone,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL
)
PARTITION BY RANGE (create_time);

CREATE TABLE infra.processed_event (
    id uuid NOT NULL,
    consumer text NOT NULL,
    event_id uuid NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL
);

CREATE TABLE media.library (
    id uuid NOT NULL,
    kind text NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid,
    personal_actor_id uuid,
    revision integer DEFAULT 0 NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT library_check CHECK ((((kind = 'project'::text) AND (project_id IS NOT NULL) AND (personal_actor_id IS NULL)) OR ((kind = 'personal'::text) AND (project_id IS NULL) AND (personal_actor_id IS NOT NULL)))),
    CONSTRAINT library_kind_check CHECK ((kind = ANY (ARRAY['project'::text, 'personal'::text]))),
    CONSTRAINT library_revision_check CHECK ((revision >= 0))
);

CREATE TABLE media.library_command (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    library_id uuid NOT NULL,
    idem_key uuid NOT NULL,
    action text NOT NULL,
    request_sha256 text NOT NULL,
    response_body bytea NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT library_command_action_check CHECK ((action = ANY (ARRAY['create_folder'::text, 'update_folder'::text, 'delete_folder'::text, 'create_text'::text, 'update_item'::text, 'move_items'::text, 'recycle_items'::text, 'restore_items'::text, 'remove_items'::text]))),
    CONSTRAINT library_command_request_sha256_check CHECK ((request_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT library_command_response_body_check CHECK (((octet_length(response_body) >= 2) AND (octet_length(response_body) <= 1048576)))
);

CREATE TABLE media.library_folder (
    id uuid NOT NULL,
    library_id uuid NOT NULL,
    library_kind text NOT NULL,
    parent_id uuid,
    name text NOT NULL,
    name_key text NOT NULL,
    "position" integer NOT NULL,
    style text DEFAULT ''::text NOT NULL,
    theme text DEFAULT ''::text NOT NULL,
    revision integer DEFAULT 1 NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT library_folder_check CHECK ((name_key = lower(btrim(name)))),
    CONSTRAINT library_folder_check1 CHECK (((parent_id IS NULL) OR (parent_id <> id))),
    CONSTRAINT library_folder_check2 CHECK ((((library_kind = 'personal'::text) AND (parent_id IS NULL) AND (char_length(name) <= 40) AND (style = ''::text) AND (theme = ''::text)) OR ((library_kind = 'project'::text) AND (style = ANY (ARRAY['glass'::text, 'stacked'::text, 'midnight'::text, 'paper'::text, 'cinema'::text, 'compact'::text])) AND (theme = ANY (ARRAY['aurora'::text, 'obsidian'::text, 'ember'::text, 'pearl'::text]))))),
    CONSTRAINT library_folder_name_check CHECK (((name = btrim(name)) AND ((char_length(name) >= 1) AND (char_length(name) <= 60)))),
    CONSTRAINT library_folder_position_check CHECK (("position" >= 0)),
    CONSTRAINT library_folder_revision_check CHECK ((revision > 0))
);

CREATE TABLE media.library_item (
    id uuid NOT NULL,
    library_id uuid NOT NULL,
    asset_id uuid,
    plain_text text,
    folder_id uuid,
    title text NOT NULL,
    category text NOT NULL,
    tags jsonb DEFAULT '[]'::jsonb NOT NULL,
    source_label text DEFAULT ''::text NOT NULL,
    note text DEFAULT ''::text NOT NULL,
    favorite boolean DEFAULT false NOT NULL,
    catalog_state text DEFAULT 'active'::text NOT NULL,
    trashed_at timestamp with time zone,
    "position" integer NOT NULL,
    revision integer DEFAULT 1 NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    purged_at timestamp with time zone,
    CONSTRAINT library_item_catalog_state_check CHECK ((catalog_state = ANY (ARRAY['active'::text, 'trashed'::text, 'removed'::text]))),
    CONSTRAINT library_item_category_check CHECK ((category = ANY (ARRAY['character'::text, 'environment'::text, 'prop'::text, 'material'::text, 'other'::text]))),
    CONSTRAINT library_item_check CHECK (((asset_id IS NULL) <> (plain_text IS NULL))),
    CONSTRAINT library_item_check1 CHECK (((asset_id IS NULL) OR (id = asset_id))),
    CONSTRAINT library_item_check2 CHECK (((catalog_state = 'trashed'::text) = (trashed_at IS NOT NULL))),
    CONSTRAINT library_item_note_check CHECK ((char_length(note) <= 8000)),
    CONSTRAINT library_item_plain_text_check CHECK (((plain_text IS NULL) OR (octet_length(plain_text) <= 65536))),
    CONSTRAINT library_item_position_check CHECK (("position" >= 0)),
    CONSTRAINT library_item_purged_check CHECK (((purged_at IS NULL) OR ((catalog_state = 'removed'::text) AND (folder_id IS NULL) AND (trashed_at IS NULL) AND (title = '已永久清理'::text) AND (tags = '[]'::jsonb) AND (source_label = ''::text) AND (note = ''::text) AND (NOT favorite) AND ((plain_text IS NULL) OR (plain_text = ''::text))))),
    CONSTRAINT library_item_revision_check CHECK ((revision > 0)),
    CONSTRAINT library_item_source_label_check CHECK ((char_length(source_label) <= 240)),
    CONSTRAINT library_item_tags_check CHECK (((jsonb_typeof(tags) = 'array'::text) AND (jsonb_array_length(tags) <= 32))),
    CONSTRAINT library_item_title_check CHECK ((((char_length(title) >= 1) AND (char_length(title) <= 240)) AND (btrim(title) <> ''::text)))
);

CREATE TABLE media.media_asset (
    id uuid NOT NULL,
    project_id uuid,
    kind text NOT NULL,
    origin text NOT NULL,
    status text NOT NULL,
    object_key text NOT NULL,
    file_name text DEFAULT ''::text NOT NULL,
    mime_type text NOT NULL,
    byte_size bigint NOT NULL,
    sha256 text,
    width integer,
    height integer,
    duration_ms integer,
    fps numeric(6,3),
    audio_channels integer,
    codec text,
    source_operation_id uuid,
    provider_key text,
    model_key text,
    region text,
    moderation_status text DEFAULT 'pending'::text NOT NULL,
    moderation_detail jsonb,
    aigc_marked boolean DEFAULT false NOT NULL,
    contains_real_person boolean DEFAULT false NOT NULL,
    consent_record_id uuid,
    upload_id text,
    failure_reason text,
    delete_time timestamp with time zone,
    purge_after timestamp with time zone,
    revision integer DEFAULT 1 NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    personal_org_id uuid,
    personal_actor_id uuid,
    CONSTRAINT media_asset_byte_size_check CHECK ((byte_size >= 0)),
    CONSTRAINT media_asset_kind_check CHECK ((kind = ANY (ARRAY['image'::text, 'video'::text, 'audio'::text, 'document'::text, 'model'::text]))),
    CONSTRAINT media_asset_model_facts_check CHECK (((kind <> 'model'::text) OR ((origin = 'upload'::text) AND (((mime_type = 'model/gltf-binary'::text) AND (codec = 'glb2'::text)) OR ((mime_type = 'model/gltf+json'::text) AND (codec = 'gltf2'::text))) AND ((byte_size >= 1) AND (byte_size <= 67108864)) AND (codec IS NOT NULL) AND (width IS NULL) AND (height IS NULL) AND (duration_ms IS NULL) AND (fps IS NULL) AND (audio_channels IS NULL)))),
    CONSTRAINT media_asset_moderation_status_check CHECK ((moderation_status = ANY (ARRAY['pending'::text, 'passed'::text, 'rejected'::text, 'skipped'::text]))),
    CONSTRAINT media_asset_origin_check CHECK ((origin = ANY (ARRAY['upload'::text, 'generated'::text, 'system'::text]))),
    CONSTRAINT media_asset_ownership_check CHECK ((((project_id IS NOT NULL) AND (project_id <> '00000000-0000-0000-0000-000000000000'::uuid) AND (personal_org_id IS NULL) AND (personal_actor_id IS NULL)) OR ((project_id IS NULL) AND (personal_org_id IS NOT NULL) AND (personal_actor_id IS NOT NULL) AND (personal_org_id <> '00000000-0000-0000-0000-000000000000'::uuid) AND (personal_actor_id <> '00000000-0000-0000-0000-000000000000'::uuid) AND (origin = ANY (ARRAY['upload'::text, 'system'::text])) AND (source_operation_id IS NULL) AND (provider_key IS NULL) AND (model_key IS NULL) AND (region IS NULL)))),
    CONSTRAINT media_asset_region_check CHECK ((region = ANY (ARRAY['domestic'::text, 'overseas'::text]))),
    CONSTRAINT media_asset_revision_check CHECK ((revision > 0)),
    CONSTRAINT media_asset_status_check CHECK ((status = ANY (ARRAY['uploading'::text, 'processing'::text, 'ready'::text, 'rejected'::text, 'failed'::text])))
);

CREATE TABLE media.package_command (
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

CREATE TABLE media.package_job (
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

CREATE TABLE media.package_object (
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

CREATE TABLE media.project_copy_object (
    snapshot_id uuid NOT NULL,
    target_asset_id uuid NOT NULL,
    rendition_kind text DEFAULT ''::text NOT NULL,
    source_object_key text NOT NULL,
    target_object_key text NOT NULL,
    byte_size bigint,
    content_type text NOT NULL,
    source_verified boolean DEFAULT false NOT NULL,
    write_started boolean DEFAULT false NOT NULL,
    sha256 text,
    status text DEFAULT 'pending'::text NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT project_copy_object_byte_size_check CHECK ((byte_size > 0)),
    CONSTRAINT project_copy_object_check CHECK ((source_object_key <> target_object_key)),
    CONSTRAINT project_copy_object_sha256_check CHECK ((sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT project_copy_object_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'verified'::text, 'removed'::text])))
);

CREATE TABLE media.project_copy_receipt (
    snapshot_id uuid NOT NULL,
    content_sha256 text NOT NULL,
    asset_count integer NOT NULL,
    rendition_count integer NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT project_copy_receipt_asset_count_check CHECK ((asset_count >= 0)),
    CONSTRAINT project_copy_receipt_content_sha256_check CHECK ((content_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT project_copy_receipt_rendition_count_check CHECK ((rendition_count >= 0))
);

CREATE TABLE media.project_copy_snapshot (
    id uuid NOT NULL,
    job_id uuid NOT NULL,
    org_id uuid NOT NULL,
    source_project_id uuid NOT NULL,
    target_project_id uuid NOT NULL,
    manifest_sha256 text NOT NULL,
    asset_count integer NOT NULL,
    rendition_count integer NOT NULL,
    content jsonb NOT NULL,
    asset_mapping jsonb NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT project_copy_snapshot_asset_count_check CHECK ((asset_count >= 0)),
    CONSTRAINT project_copy_snapshot_asset_mapping_check CHECK ((jsonb_typeof(asset_mapping) = 'object'::text)),
    CONSTRAINT project_copy_snapshot_check CHECK ((source_project_id <> target_project_id)),
    CONSTRAINT project_copy_snapshot_content_check CHECK (((jsonb_typeof(content) = 'object'::text) AND (octet_length((content)::text) <= 33554432))),
    CONSTRAINT project_copy_snapshot_manifest_sha256_check CHECK ((manifest_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT project_copy_snapshot_rendition_count_check CHECK ((rendition_count >= 0))
);

CREATE TABLE media.purge_command (
    actor_id uuid NOT NULL,
    org_id uuid NOT NULL,
    idem_key uuid NOT NULL,
    job_id uuid NOT NULL,
    action text NOT NULL,
    request_sha256 text NOT NULL,
    response_body bytea NOT NULL,
    event_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT purge_command_action_check CHECK ((action = ANY (ARRAY['create'::text, 'cancel'::text, 'reconcile'::text]))),
    CONSTRAINT purge_command_request_sha256_check CHECK ((request_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT purge_command_response_body_check CHECK (((octet_length(response_body) >= 2) AND (octet_length(response_body) <= 1048576)))
);

CREATE TABLE media.purge_item (
    job_id uuid NOT NULL,
    item_index integer NOT NULL,
    item_id uuid NOT NULL,
    asset_id uuid,
    frozen bytea NOT NULL,
    status text NOT NULL,
    failure_code text,
    CONSTRAINT purge_item_failure_code_check CHECK ((failure_code = ANY (ARRAY['in_use'::text, 'source_unavailable'::text, 'source_changed'::text, 'object_missing'::text, 'object_mismatch'::text, 'object_remove_unknown'::text, 'object_receipt_unknown'::text, 'worker_interrupted'::text, 'cancelled'::text]))),
    CONSTRAINT purge_item_frozen_check CHECK (((octet_length(frozen) >= 2) AND (octet_length(frozen) <= 1048576))),
    CONSTRAINT purge_item_item_index_check CHECK (((item_index >= 0) AND (item_index <= 199))),
    CONSTRAINT purge_item_status_check CHECK ((status = ANY (ARRAY['blocked'::text, 'queued'::text, 'running'::text, 'needs_reconciliation'::text, 'succeeded'::text, 'cancelled'::text])))
);

CREATE TABLE media.purge_job (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    library_id uuid NOT NULL,
    scope_kind text NOT NULL,
    project_id uuid,
    item_count integer NOT NULL,
    status text NOT NULL,
    stage text NOT NULL,
    attempt integer DEFAULT 1 NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    cancellation_requested boolean DEFAULT false NOT NULL,
    needs_reconciliation boolean DEFAULT false NOT NULL,
    execution_unconfirmed boolean DEFAULT false NOT NULL,
    execution_id uuid,
    worker_fence uuid,
    lease_until timestamp with time zone,
    process_ended boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT purge_job_attempt_check CHECK ((attempt > 0)),
    CONSTRAINT purge_job_check CHECK (((scope_kind = 'project'::text) = (project_id IS NOT NULL))),
    CONSTRAINT purge_job_check1 CHECK ((((execution_id IS NULL) AND (worker_fence IS NULL) AND (lease_until IS NULL) AND process_ended) OR ((execution_id IS NOT NULL) AND (worker_fence IS NOT NULL) AND (lease_until IS NOT NULL)))),
    CONSTRAINT purge_job_item_count_check CHECK (((item_count >= 1) AND (item_count <= 200))),
    CONSTRAINT purge_job_revision_check CHECK ((revision > 0)),
    CONSTRAINT purge_job_scope_kind_check CHECK ((scope_kind = ANY (ARRAY['personal'::text, 'project'::text]))),
    CONSTRAINT purge_job_stage_check CHECK ((stage = ANY (ARRAY['frozen'::text, 'verifying'::text, 'removing'::text, 'completed'::text]))),
    CONSTRAINT purge_job_status_check CHECK ((status = ANY (ARRAY['queued'::text, 'running'::text, 'needs_reconciliation'::text, 'succeeded'::text, 'partial_failed'::text, 'failed'::text, 'cancel_requested'::text, 'cancelled'::text])))
);

CREATE TABLE media.purge_object (
    job_id uuid NOT NULL,
    item_index integer NOT NULL,
    object_key text NOT NULL,
    rendition_kind text NOT NULL,
    byte_size bigint,
    sha256 text,
    verified boolean DEFAULT false NOT NULL,
    removal_started boolean DEFAULT false NOT NULL,
    removed boolean DEFAULT false NOT NULL,
    CONSTRAINT purge_object_byte_size_check CHECK (((byte_size >= 1) AND (byte_size <= '2147483648'::bigint))),
    CONSTRAINT purge_object_check CHECK (((NOT removal_started) OR (verified AND (sha256 IS NOT NULL) AND (byte_size IS NOT NULL)))),
    CONSTRAINT purge_object_check1 CHECK (((NOT removed) OR removal_started)),
    CONSTRAINT purge_object_rendition_kind_check CHECK ((rendition_kind = ANY (ARRAY[''::text, 'thumb_256'::text, 'thumb_640'::text, 'poster'::text, 'proxy_720p'::text, 'waveform'::text]))),
    CONSTRAINT purge_object_sha256_check CHECK ((sha256 ~ '^[0-9a-f]{64}$'::text))
);

CREATE TABLE media.rendition (
    id uuid NOT NULL,
    media_asset_id uuid NOT NULL,
    kind text NOT NULL,
    object_key text NOT NULL,
    width integer,
    height integer,
    byte_size bigint,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    CONSTRAINT rendition_kind_check CHECK ((kind = ANY (ARRAY['thumb_256'::text, 'thumb_640'::text, 'poster'::text, 'proxy_720p'::text, 'waveform'::text])))
);

CREATE TABLE media.transfer_command (
    actor_id uuid NOT NULL,
    org_id uuid NOT NULL,
    idem_key uuid NOT NULL,
    job_id uuid NOT NULL,
    action text NOT NULL,
    request_sha256 text NOT NULL,
    response_body bytea NOT NULL,
    event_id uuid,
    event_action text,
    event_attempt integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT transfer_command_action_check CHECK ((action = ANY (ARRAY['create'::text, 'cancel'::text, 'retry'::text, 'reconcile'::text]))),
    CONSTRAINT transfer_command_check CHECK ((((event_id IS NULL) AND (event_action IS NULL) AND (event_attempt IS NULL)) OR ((event_id IS NOT NULL) AND (event_action IS NOT NULL) AND (event_attempt IS NOT NULL)))),
    CONSTRAINT transfer_command_event_action_check CHECK ((event_action = ANY (ARRAY['start'::text, 'cancel'::text, 'reconcile'::text]))),
    CONSTRAINT transfer_command_event_attempt_check CHECK ((event_attempt > 0)),
    CONSTRAINT transfer_command_request_sha256_check CHECK ((request_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT transfer_command_response_body_check CHECK (((octet_length(response_body) >= 2) AND (octet_length(response_body) <= 1048576)))
);

CREATE TABLE media.transfer_item (
    job_id uuid NOT NULL,
    item_index integer NOT NULL,
    source_item_id uuid NOT NULL,
    target_item_id uuid NOT NULL,
    source_asset_id uuid,
    target_asset_id uuid,
    frozen bytea NOT NULL,
    status text NOT NULL,
    failure_code text,
    CONSTRAINT transfer_item_check CHECK (((source_asset_id IS NULL) = (target_asset_id IS NULL))),
    CONSTRAINT transfer_item_check1 CHECK (((target_asset_id IS NULL) OR (target_asset_id = target_item_id))),
    CONSTRAINT transfer_item_failure_code_check CHECK ((failure_code = ANY (ARRAY['source_changed'::text, 'source_unavailable'::text, 'target_folder_changed'::text, 'object_missing'::text, 'object_mismatch'::text, 'object_write_unknown'::text, 'object_receipt_unknown'::text, 'object_cleanup_unknown'::text, 'registration_failed'::text, 'worker_interrupted'::text, 'cancelled'::text]))),
    CONSTRAINT transfer_item_frozen_check CHECK (((octet_length(frozen) >= 2) AND (octet_length(frozen) <= 1048576))),
    CONSTRAINT transfer_item_item_index_check CHECK (((item_index >= 0) AND (item_index <= 199))),
    CONSTRAINT transfer_item_status_check CHECK ((status = ANY (ARRAY['queued'::text, 'running'::text, 'succeeded'::text, 'failed'::text, 'needs_reconciliation'::text, 'cancelled'::text])))
);

CREATE TABLE media.transfer_job (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    source_library_id uuid NOT NULL,
    target_library_id uuid NOT NULL,
    source_kind text NOT NULL,
    target_kind text NOT NULL,
    source_project_id uuid,
    target_project_id uuid,
    target_folder_id uuid,
    target_folder_revision integer NOT NULL,
    source_revision integer NOT NULL,
    target_revision integer NOT NULL,
    project_revision integer NOT NULL,
    manifest_sha256 text NOT NULL,
    item_count integer NOT NULL,
    status text NOT NULL,
    stage text NOT NULL,
    attempt integer DEFAULT 1 NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    needs_reconciliation boolean DEFAULT false NOT NULL,
    cancellation_requested boolean DEFAULT false NOT NULL,
    execution_unconfirmed boolean DEFAULT false NOT NULL,
    execution_id uuid,
    worker_fence uuid,
    lease_until timestamp with time zone,
    process_ended boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT transfer_job_attempt_check CHECK ((attempt > 0)),
    CONSTRAINT transfer_job_check CHECK (((source_library_id <> target_library_id) AND (source_kind <> target_kind))),
    CONSTRAINT transfer_job_check1 CHECK (((source_kind = 'project'::text) = (source_project_id IS NOT NULL))),
    CONSTRAINT transfer_job_check2 CHECK (((target_kind = 'project'::text) = (target_project_id IS NOT NULL))),
    CONSTRAINT transfer_job_check3 CHECK ((((target_folder_id IS NULL) AND (target_folder_revision = 0)) OR ((target_folder_id IS NOT NULL) AND (target_folder_revision > 0)))),
    CONSTRAINT transfer_job_check4 CHECK ((((execution_id IS NULL) AND (worker_fence IS NULL) AND (lease_until IS NULL) AND process_ended) OR ((execution_id IS NOT NULL) AND (worker_fence IS NOT NULL) AND (lease_until IS NOT NULL)))),
    CONSTRAINT transfer_job_item_count_check CHECK (((item_count >= 1) AND (item_count <= 200))),
    CONSTRAINT transfer_job_manifest_sha256_check CHECK ((manifest_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT transfer_job_project_revision_check CHECK ((project_revision > 0)),
    CONSTRAINT transfer_job_revision_check CHECK ((revision > 0)),
    CONSTRAINT transfer_job_source_kind_check CHECK ((source_kind = ANY (ARRAY['personal'::text, 'project'::text]))),
    CONSTRAINT transfer_job_source_revision_check CHECK ((source_revision >= 0)),
    CONSTRAINT transfer_job_stage_check CHECK ((stage = ANY (ARRAY['frozen'::text, 'copying'::text, 'registering'::text, 'cleanup'::text, 'completed'::text]))),
    CONSTRAINT transfer_job_status_check CHECK ((status = ANY (ARRAY['queued'::text, 'running'::text, 'needs_reconciliation'::text, 'succeeded'::text, 'partial_failed'::text, 'failed'::text, 'cancel_requested'::text, 'cancelled'::text]))),
    CONSTRAINT transfer_job_target_folder_revision_check CHECK ((target_folder_revision >= 0)),
    CONSTRAINT transfer_job_target_kind_check CHECK ((target_kind = ANY (ARRAY['personal'::text, 'project'::text]))),
    CONSTRAINT transfer_job_target_revision_check CHECK ((target_revision >= 0))
);

CREATE TABLE media.transfer_object (
    job_id uuid NOT NULL,
    item_index integer NOT NULL,
    rendition_kind text DEFAULT ''::text NOT NULL,
    source_object_key text NOT NULL,
    target_object_key text NOT NULL,
    byte_size bigint,
    content_type text NOT NULL,
    sha256 text,
    source_verified boolean DEFAULT false NOT NULL,
    write_started boolean DEFAULT false NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    CONSTRAINT transfer_object_byte_size_check CHECK (((byte_size >= 1) AND (byte_size <= '2147483648'::bigint))),
    CONSTRAINT transfer_object_rendition_kind_check CHECK ((rendition_kind = ANY (ARRAY[''::text, 'thumb_256'::text, 'thumb_640'::text, 'poster'::text, 'proxy_720p'::text, 'waveform'::text]))),
    CONSTRAINT transfer_object_sha256_check CHECK ((sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT transfer_object_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'verified'::text, 'removed'::text])))
);

CREATE TABLE media.upload_request (
    project_id uuid,
    principal_id uuid NOT NULL,
    request_key uuid NOT NULL,
    sha256 text NOT NULL,
    file_name text NOT NULL,
    byte_size bigint NOT NULL,
    asset_id uuid NOT NULL,
    response jsonb NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    personal_org_id uuid,
    CONSTRAINT upload_request_byte_size_check CHECK (((byte_size >= 1) AND (byte_size <= 524288000))),
    CONSTRAINT upload_request_file_name_check CHECK (((octet_length(file_name) >= 1) AND (octet_length(file_name) <= 255))),
    CONSTRAINT upload_request_request_key_check CHECK ((request_key <> '00000000-0000-0000-0000-000000000000'::uuid)),
    CONSTRAINT upload_request_response_check CHECK (((jsonb_typeof(response) = 'object'::text) AND (octet_length((response)::text) <= 4096))),
    CONSTRAINT upload_request_scope_check CHECK ((((project_id IS NOT NULL) AND (project_id <> '00000000-0000-0000-0000-000000000000'::uuid) AND (personal_org_id IS NULL)) OR ((project_id IS NULL) AND (personal_org_id IS NOT NULL) AND (personal_org_id <> '00000000-0000-0000-0000-000000000000'::uuid)))),
    CONSTRAINT upload_request_sha256_check CHECK ((sha256 ~ '^[0-9a-f]{64}$'::text))
);

CREATE TABLE mediatool.depth_command (
    actor_id uuid NOT NULL,
    request_id uuid NOT NULL,
    job_id uuid NOT NULL,
    request_hash text NOT NULL,
    response jsonb NOT NULL,
    event_id uuid,
    event_action text,
    event_attempt integer,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT depth_command_check CHECK (((event_id IS NULL) = (event_action IS NULL))),
    CONSTRAINT depth_command_check1 CHECK (((event_id IS NULL) = (event_attempt IS NULL))),
    CONSTRAINT depth_command_event_action_check CHECK ((event_action = ANY (ARRAY['start'::text, 'cancel'::text, 'reconcile'::text]))),
    CONSTRAINT depth_command_request_hash_check CHECK ((request_hash ~ '^[a-f0-9]{64}$'::text))
);

CREATE TABLE mediatool.depth_job (
    id uuid NOT NULL,
    project_id uuid NOT NULL,
    org_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    actor_role text NOT NULL,
    canvas_id uuid NOT NULL,
    node_id uuid NOT NULL,
    source_revision bigint NOT NULL,
    source_asset_id uuid NOT NULL,
    source_asset_revision bigint NOT NULL,
    source_sha256 text NOT NULL,
    profile_id text NOT NULL,
    frozen jsonb NOT NULL,
    frozen_sha256 text NOT NULL,
    status text NOT NULL,
    stage text NOT NULL,
    attempt integer NOT NULL,
    revision bigint NOT NULL,
    process_state text NOT NULL,
    active_worker uuid,
    failure_code text,
    retryable boolean DEFAULT false NOT NULL,
    needs_reconciliation boolean DEFAULT false NOT NULL,
    execution_unconfirmed boolean DEFAULT false NOT NULL,
    cancellation_requested boolean DEFAULT false NOT NULL,
    reconciliation_requested boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT depth_job_actor_role_check CHECK ((actor_role = ANY (ARRAY['admin'::text, 'producer'::text]))),
    CONSTRAINT depth_job_attempt_check CHECK (((attempt >= 1) AND (attempt <= 100))),
    CONSTRAINT depth_job_check CHECK ((NOT (retryable AND (needs_reconciliation OR execution_unconfirmed)))),
    CONSTRAINT depth_job_check1 CHECK (((NOT execution_unconfirmed) OR (needs_reconciliation AND (active_worker IS NOT NULL)))),
    CONSTRAINT depth_job_check2 CHECK (((status <> ALL (ARRAY['review_required'::text, 'succeeded'::text, 'cancelled'::text])) OR ((active_worker IS NULL) AND (NOT execution_unconfirmed) AND (process_state = ANY (ARRAY['none'::text, 'ended'::text]))))),
    CONSTRAINT depth_job_frozen_check CHECK (((jsonb_typeof(frozen) = 'object'::text) AND (octet_length((frozen)::text) <= 1048576))),
    CONSTRAINT depth_job_frozen_sha256_check CHECK ((frozen_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT depth_job_process_state_check CHECK ((process_state = ANY (ARRAY['none'::text, 'started'::text, 'ended'::text, 'unknown'::text]))),
    CONSTRAINT depth_job_profile_id_check CHECK ((profile_id = 'vda-small-relative-v1'::text)),
    CONSTRAINT depth_job_revision_check CHECK ((revision > 0)),
    CONSTRAINT depth_job_source_asset_revision_check CHECK ((source_asset_revision > 0)),
    CONSTRAINT depth_job_source_revision_check CHECK ((source_revision > 0)),
    CONSTRAINT depth_job_source_sha256_check CHECK ((source_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT depth_job_stage_check CHECK (((length(stage) >= 1) AND (length(stage) <= 32))),
    CONSTRAINT depth_job_status_check CHECK ((status = ANY (ARRAY['queued'::text, 'running'::text, 'review_required'::text, 'succeeded'::text, 'failed'::text, 'cancel_requested'::text, 'cancelled'::text])))
);

CREATE TABLE mediatool.depth_object (
    job_id uuid NOT NULL,
    attempt integer NOT NULL,
    kind text NOT NULL,
    object_key text NOT NULL,
    sha256 text NOT NULL,
    byte_size bigint NOT NULL,
    mime_type text NOT NULL,
    write_started boolean DEFAULT false NOT NULL,
    delete_started boolean DEFAULT false NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT depth_object_byte_size_check CHECK (((byte_size >= 1) AND (byte_size <= 524288000))),
    CONSTRAINT depth_object_kind_check CHECK ((kind = ANY (ARRAY['original'::text, 'poster'::text, 'proxy_720p'::text]))),
    CONSTRAINT depth_object_sha256_check CHECK ((sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT depth_object_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'verified'::text, 'removed'::text])))
);

CREATE TABLE mediatool.depth_result_intent (
    job_id uuid NOT NULL,
    attempt integer NOT NULL,
    asset_id uuid NOT NULL,
    output_sha256 text NOT NULL,
    artifact_sha256 text NOT NULL,
    artifact jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT depth_result_intent_artifact_check CHECK (((jsonb_typeof(artifact) = 'object'::text) AND (octet_length((artifact)::text) <= 1048576))),
    CONSTRAINT depth_result_intent_artifact_sha256_check CHECK ((artifact_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT depth_result_intent_attempt_check CHECK (((attempt >= 1) AND (attempt <= 100))),
    CONSTRAINT depth_result_intent_output_sha256_check CHECK ((output_sha256 ~ '^[a-f0-9]{64}$'::text))
);

CREATE TABLE mediatool.export_command (
    actor_id uuid NOT NULL,
    request_id uuid NOT NULL,
    job_id uuid NOT NULL,
    request_hash text NOT NULL,
    response jsonb NOT NULL,
    event_id uuid,
    event_action text,
    event_attempt integer,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT export_command_check CHECK (((event_id IS NULL) = (event_action IS NULL))),
    CONSTRAINT export_command_check1 CHECK (((event_id IS NULL) = (event_attempt IS NULL))),
    CONSTRAINT export_command_event_action_check CHECK ((event_action = ANY (ARRAY['start'::text, 'cancel'::text]))),
    CONSTRAINT export_command_request_hash_check CHECK ((request_hash ~ '^[a-f0-9]{64}$'::text))
);

CREATE TABLE mediatool.export_job (
    id uuid NOT NULL,
    project_id uuid NOT NULL,
    org_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    actor_role text NOT NULL,
    canvas_id uuid NOT NULL,
    node_id uuid NOT NULL,
    source_revision bigint NOT NULL,
    frozen jsonb NOT NULL,
    status text NOT NULL,
    stage text NOT NULL,
    progress integer NOT NULL,
    attempt integer NOT NULL,
    revision bigint NOT NULL,
    asset_id uuid,
    sha256 text,
    failure_code text,
    active_worker uuid,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    output_kind text DEFAULT 'video'::text NOT NULL,
    CONSTRAINT export_job_actor_role_check CHECK ((actor_role = ANY (ARRAY['admin'::text, 'producer'::text]))),
    CONSTRAINT export_job_attempt_check CHECK (((attempt >= 1) AND (attempt <= 100))),
    CONSTRAINT export_job_check CHECK (((asset_id IS NULL) = (sha256 IS NULL))),
    CONSTRAINT export_job_check1 CHECK (((status <> ALL (ARRAY['review_required'::text, 'succeeded'::text])) OR (asset_id IS NOT NULL))),
    CONSTRAINT export_job_check2 CHECK (((status <> 'succeeded'::text) OR (progress = 100))),
    CONSTRAINT export_job_frozen_check CHECK (((jsonb_typeof(frozen) = 'object'::text) AND (octet_length((frozen)::text) <= 1048576))),
    CONSTRAINT export_job_output_kind_check CHECK ((output_kind = ANY (ARRAY['video'::text, 'audio'::text]))),
    CONSTRAINT export_job_progress_check CHECK (((progress >= 0) AND (progress <= 100))),
    CONSTRAINT export_job_revision_check CHECK ((revision > 0)),
    CONSTRAINT export_job_sha256_check CHECK ((sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT export_job_source_revision_check CHECK ((source_revision > 0)),
    CONSTRAINT export_job_status_check CHECK ((status = ANY (ARRAY['queued'::text, 'running'::text, 'review_required'::text, 'succeeded'::text, 'failed'::text, 'cancel_requested'::text, 'cancelled'::text])))
);

CREATE TABLE mediatool.transcription_command (
    actor_id uuid NOT NULL,
    request_id uuid NOT NULL,
    job_id uuid NOT NULL,
    request_hash text NOT NULL,
    response jsonb NOT NULL,
    event_id uuid,
    event_action text,
    event_attempt integer,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT transcription_command_check CHECK (((event_id IS NULL) = (event_action IS NULL))),
    CONSTRAINT transcription_command_check1 CHECK (((event_id IS NULL) = (event_attempt IS NULL))),
    CONSTRAINT transcription_command_event_action_check CHECK ((event_action = ANY (ARRAY['start'::text, 'cancel'::text]))),
    CONSTRAINT transcription_command_request_hash_check CHECK ((request_hash ~ '^[a-f0-9]{64}$'::text))
);

CREATE TABLE mediatool.transcription_job (
    id uuid NOT NULL,
    project_id uuid NOT NULL,
    org_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    actor_role text NOT NULL,
    canvas_id uuid NOT NULL,
    node_id uuid NOT NULL,
    source_revision bigint NOT NULL,
    language text NOT NULL,
    frozen jsonb NOT NULL,
    status text NOT NULL,
    stage text NOT NULL,
    progress integer NOT NULL,
    attempt integer NOT NULL,
    revision bigint NOT NULL,
    inference_state text NOT NULL,
    result jsonb,
    result_sha256 text,
    failure_code text,
    active_worker uuid,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT transcription_job_actor_role_check CHECK ((actor_role = ANY (ARRAY['admin'::text, 'producer'::text]))),
    CONSTRAINT transcription_job_attempt_check CHECK (((attempt >= 1) AND (attempt <= 100))),
    CONSTRAINT transcription_job_check CHECK (((result IS NULL) = (result_sha256 IS NULL))),
    CONSTRAINT transcription_job_check1 CHECK (((status <> 'succeeded'::text) OR ((progress = 100) AND (result IS NOT NULL) AND (inference_state = 'terminal'::text)))),
    CONSTRAINT transcription_job_check2 CHECK (((status <> 'cancelled'::text) OR (inference_state = ANY (ARRAY['none'::text, 'terminal'::text])))),
    CONSTRAINT transcription_job_frozen_check CHECK (((jsonb_typeof(frozen) = 'object'::text) AND (octet_length((frozen)::text) <= 1048576))),
    CONSTRAINT transcription_job_inference_state_check CHECK ((inference_state = ANY (ARRAY['none'::text, 'submitted'::text, 'terminal'::text, 'unknown'::text]))),
    CONSTRAINT transcription_job_language_check CHECK (((length(language) >= 2) AND (length(language) <= 4))),
    CONSTRAINT transcription_job_progress_check CHECK (((progress >= 0) AND (progress <= 100))),
    CONSTRAINT transcription_job_result_check CHECK (((result IS NULL) OR ((jsonb_typeof(result) = 'object'::text) AND (octet_length((result)::text) <= 8388608)))),
    CONSTRAINT transcription_job_result_sha256_check CHECK ((result_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT transcription_job_revision_check CHECK ((revision > 0)),
    CONSTRAINT transcription_job_source_revision_check CHECK ((source_revision > 0)),
    CONSTRAINT transcription_job_status_check CHECK ((status = ANY (ARRAY['queued'::text, 'running'::text, 'succeeded'::text, 'failed'::text, 'cancel_requested'::text, 'cancelled'::text])))
);

CREATE TABLE operation.batch (
    id uuid NOT NULL,
    project_id uuid NOT NULL,
    kind text NOT NULL,
    scope jsonb NOT NULL,
    status text NOT NULL,
    paused_reason text,
    total_count integer NOT NULL,
    succeeded_count integer DEFAULT 0 NOT NULL,
    failed_count integer DEFAULT 0 NOT NULL,
    unknown_count integer DEFAULT 0 NOT NULL,
    quote_total_micros bigint NOT NULL,
    workflow_id text,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    cancel_requested_at timestamp with time zone,
    CONSTRAINT batch_failed_count_check CHECK ((failed_count >= 0)),
    CONSTRAINT batch_kind_check CHECK ((kind = ANY (ARRAY['keyframe'::text, 'video'::text, 'tts'::text, 'reference'::text, 'parse'::text, 'storyboard'::text, 'mixed'::text]))),
    CONSTRAINT batch_quote_total_micros_check CHECK ((quote_total_micros >= 0)),
    CONSTRAINT batch_scope_check CHECK ((jsonb_typeof(scope) = 'object'::text)),
    CONSTRAINT batch_status_check CHECK ((status = ANY (ARRAY['quoted'::text, 'confirmed'::text, 'running'::text, 'finished'::text, 'cancelled'::text, 'expired'::text]))),
    CONSTRAINT batch_succeeded_count_check CHECK ((succeeded_count >= 0)),
    CONSTRAINT batch_total_count_check CHECK ((total_count > 0)),
    CONSTRAINT batch_unknown_count_check CHECK ((unknown_count >= 0)),
    CONSTRAINT ck_batch_counts CHECK (((((succeeded_count)::bigint + failed_count) + unknown_count) <= total_count))
);

CREATE TABLE operation.batch_launch (
    operation_id uuid NOT NULL,
    batch_id uuid NOT NULL,
    project_id uuid NOT NULL,
    provider_id uuid,
    state text NOT NULL,
    acquired_at timestamp with time zone DEFAULT now() NOT NULL,
    released_at timestamp with time zone,
    CONSTRAINT batch_launch_state_check CHECK ((state = ANY (ARRAY['active'::text, 'done'::text]))),
    CONSTRAINT ck_batch_launch_release CHECK ((((state = 'active'::text) AND (released_at IS NULL)) OR ((state = 'done'::text) AND (released_at IS NOT NULL))))
);

CREATE TABLE operation.confirmation_request (
    org_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    request_id uuid NOT NULL,
    fingerprint bytea NOT NULL,
    outcome jsonb NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT confirmation_request_fingerprint_check CHECK ((octet_length(fingerprint) = 32)),
    CONSTRAINT confirmation_request_outcome_check CHECK ((jsonb_typeof(outcome) = 'object'::text))
);

CREATE TABLE operation.operation (
    id uuid NOT NULL,
    project_id uuid NOT NULL,
    batch_id uuid,
    target_type text,
    target_id uuid,
    target_key text,
    target_version_no integer,
    capability text NOT NULL,
    mode text NOT NULL,
    model_profile_version_id uuid,
    price_rule_version_id uuid,
    params jsonb DEFAULT '{}'::jsonb NOT NULL,
    output_count integer DEFAULT 1 NOT NULL,
    input_hash text NOT NULL,
    origin text NOT NULL,
    status text NOT NULL,
    failure_code text,
    failure_message text,
    retryable boolean,
    quote_micros bigint,
    quote_detail jsonb,
    quote_expires_at timestamp with time zone,
    reused_from_id uuid,
    force_regenerate boolean DEFAULT false NOT NULL,
    reservation_id uuid,
    provider_request_key text,
    workflow_id text,
    confirmed_at timestamp with time zone,
    confirmed_by uuid,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    settled_micros bigint,
    cost_estimated boolean DEFAULT false NOT NULL,
    region text,
    trace_id text,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    source_context jsonb,
    prompt_preparation jsonb,
    CONSTRAINT ck_operation_agent_session CHECK (((target_type <> 'agent_session'::text) OR ((model_profile_version_id IS NULL) AND (price_rule_version_id IS NULL)))),
    CONSTRAINT ck_operation_canvas_source CHECK (((source_context IS NULL) OR ((target_type = 'free'::text) AND (origin = 'canvas'::text) AND (jsonb_typeof(source_context) = 'object'::text) AND (source_context ?& ARRAY['canvas_id'::text, 'node_id'::text, 'revision'::text]) AND ((source_context - ARRAY['canvas_id'::text, 'node_id'::text, 'row_id'::text, 'revision'::text]) = '{}'::jsonb) AND (jsonb_typeof((source_context -> 'canvas_id'::text)) = 'string'::text) AND (jsonb_typeof((source_context -> 'node_id'::text)) = 'string'::text) AND (jsonb_typeof((source_context -> 'revision'::text)) = 'number'::text) AND (((source_context ->> 'revision'::text))::bigint > 0) AND ((NOT (source_context ? 'row_id'::text)) OR (jsonb_typeof((source_context -> 'row_id'::text)) = 'string'::text))))),
    CONSTRAINT ck_operation_prompt_preparation CHECK (((prompt_preparation IS NULL) OR (((jsonb_typeof(prompt_preparation) = 'object'::text) AND (octet_length((prompt_preparation)::text) <= 4096) AND ((prompt_preparation - ARRAY['version'::text, 'operation'::text, 'policy'::text, 'template_id'::text, 'template_version'::text, 'customization_id'::text, 'customization_revision'::text, 'request_sha256'::text, 'user_prompt_sha256'::text, 'content_sha256'::text]) = '{}'::jsonb) AND ((prompt_preparation -> 'version'::text) = '1'::jsonb) AND (origin = 'canvas'::text) AND (target_type = 'free'::text) AND ((prompt_preparation ->> 'operation'::text) = ANY (ARRAY['chapter_assets_extract'::text, 'character_extract'::text, 'character_turnaround'::text, 'storyboard_plan'::text, 'storyboard_repair'::text, 'storyboard_first_frame'::text, 'storyboard_video'::text, 'short_drama_outline'::text, 'skill_draft'::text])) AND ((prompt_preparation ->> 'request_sha256'::text) ~ '^[0-9a-f]{64}$'::text) AND ((prompt_preparation ->> 'user_prompt_sha256'::text) ~ '^[0-9a-f]{64}$'::text) AND ((prompt_preparation ->> 'content_sha256'::text) ~ '^[0-9a-f]{64}$'::text) AND ((((prompt_preparation ->> 'operation'::text) = ANY (ARRAY['character_turnaround'::text, 'storyboard_first_frame'::text])) AND (capability = 'image.generate'::text)) OR (((prompt_preparation ->> 'operation'::text) = 'storyboard_video'::text) AND (capability = 'video.generate'::text)) OR (((prompt_preparation ->> 'operation'::text) <> ALL (ARRAY['character_turnaround'::text, 'storyboard_first_frame'::text, 'storyboard_video'::text])) AND (capability = 'text.structured'::text))) AND ((((prompt_preparation ->> 'policy'::text) = 'bypass_video'::text) AND ((prompt_preparation ->> 'operation'::text) = 'storyboard_video'::text) AND (NOT (prompt_preparation ?| ARRAY['template_id'::text, 'template_version'::text, 'customization_id'::text, 'customization_revision'::text])) AND ((prompt_preparation ->> 'content_sha256'::text) = (prompt_preparation ->> 'user_prompt_sha256'::text))) OR (((prompt_preparation ->> 'policy'::text) = 'compiled'::text) AND ((prompt_preparation ->> 'operation'::text) <> 'storyboard_video'::text) AND ((prompt_preparation ->> 'template_id'::text) ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'::text) AND ((prompt_preparation ->> 'template_id'::text) <> '00000000-0000-0000-0000-000000000000'::text) AND ((prompt_preparation -> 'template_version'::text) = '1'::jsonb) AND (((NOT (prompt_preparation ? 'customization_id'::text)) AND (NOT (prompt_preparation ? 'customization_revision'::text))) OR (((prompt_preparation ->> 'customization_id'::text) ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'::text) AND ((prompt_preparation ->> 'customization_id'::text) <> '00000000-0000-0000-0000-000000000000'::text) AND (jsonb_typeof((prompt_preparation -> 'customization_revision'::text)) = 'number'::text) AND ((prompt_preparation ->> 'customization_revision'::text) ~ '^[1-9][0-9]{0,9}$'::text) AND (((prompt_preparation ->> 'customization_revision'::text))::numeric <= (2147483647)::numeric)))))) IS TRUE))),
    CONSTRAINT ck_operation_quote_amount CHECK (((status <> 'quoted'::text) OR ((quote_micros IS NOT NULL) AND (quote_expires_at IS NOT NULL) AND (quote_expires_at > create_time)))),
    CONSTRAINT ck_operation_reuse_self CHECK ((reused_from_id IS DISTINCT FROM id)),
    CONSTRAINT ck_operation_upload CHECK (((origin = 'upload'::text) OR ((region IS NOT NULL) AND ((model_profile_version_id IS NOT NULL) OR (NOT (target_type IS DISTINCT FROM 'agent_session'::text)))))),
    CONSTRAINT operation_capability_check CHECK ((btrim(capability) <> ''::text)),
    CONSTRAINT operation_input_hash_check CHECK ((btrim(input_hash) <> ''::text)),
    CONSTRAINT operation_mode_check CHECK ((btrim(mode) <> ''::text)),
    CONSTRAINT operation_origin_check CHECK ((origin = ANY (ARRAY['pipeline'::text, 'batch'::text, 'canvas'::text, 'agent'::text, 'upload'::text, 'system'::text]))),
    CONSTRAINT operation_output_count_check CHECK (((output_count >= 1) AND (output_count <= 8))),
    CONSTRAINT operation_params_check CHECK ((jsonb_typeof(params) = 'object'::text)),
    CONSTRAINT operation_quote_micros_check CHECK ((quote_micros >= 0)),
    CONSTRAINT operation_region_check CHECK ((region = ANY (ARRAY['domestic'::text, 'overseas'::text]))),
    CONSTRAINT operation_settled_micros_check CHECK ((settled_micros >= 0)),
    CONSTRAINT operation_status_check CHECK ((status = ANY (ARRAY['draft'::text, 'quoted'::text, 'confirmed'::text, 'submitting'::text, 'submitted'::text, 'succeeded'::text, 'ingesting'::text, 'completed'::text, 'failed'::text, 'unknown'::text, 'reconciling'::text, 'manual'::text, 'cancelling'::text, 'cancelled'::text, 'expired'::text]))),
    CONSTRAINT operation_target_type_check CHECK ((target_type = ANY (ARRAY['shot_frame'::text, 'shot_take'::text, 'reference_slot'::text, 'dialogue_audio'::text, 'voice_preview'::text, 'episode_split'::text, 'episode_parse'::text, 'bible_extract'::text, 'scene_storyboard'::text, 'agent_session'::text, 'free'::text])))
);

COMMENT ON COLUMN operation.operation.prompt_preparation IS 'Immutable template policy evidence; final text lives only in operation_input.prompt. Existing column UPDATE grants deliberately exclude this column.';

CREATE TABLE operation.operation_event (
    id uuid NOT NULL,
    operation_id uuid NOT NULL,
    from_status text,
    to_status text NOT NULL,
    reason text,
    detail jsonb,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    CONSTRAINT operation_event_detail_check CHECK (((detail IS NULL) OR (jsonb_typeof(detail) = 'object'::text)))
);

CREATE TABLE operation.operation_input (
    id uuid NOT NULL,
    operation_id uuid NOT NULL,
    seq_no integer NOT NULL,
    role text NOT NULL,
    ref_type text NOT NULL,
    ref_id uuid,
    ref_version text,
    text_value text,
    media_asset_id uuid,
    mask_asset_id uuid,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    CONSTRAINT operation_input_ref_type_check CHECK ((btrim(ref_type) <> ''::text)),
    CONSTRAINT operation_input_role_check CHECK ((btrim(role) <> ''::text)),
    CONSTRAINT operation_input_seq_no_check CHECK ((seq_no >= 0))
);

CREATE TABLE operation.operation_output (
    id uuid NOT NULL,
    project_id uuid NOT NULL,
    operation_id uuid NOT NULL,
    seq_no integer NOT NULL,
    kind text NOT NULL,
    media_asset_id uuid,
    json_payload jsonb,
    moderation_status text DEFAULT 'pending'::text NOT NULL,
    moderation_reason text,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    CONSTRAINT ck_operation_output_content CHECK ((((kind = 'media'::text) AND (media_asset_id IS NOT NULL) AND (json_payload IS NULL)) OR ((kind = 'json'::text) AND (media_asset_id IS NULL) AND (json_payload IS NOT NULL) AND (jsonb_typeof(json_payload) = 'object'::text)))),
    CONSTRAINT operation_output_kind_check CHECK ((kind = ANY (ARRAY['media'::text, 'json'::text]))),
    CONSTRAINT operation_output_moderation_status_check CHECK ((moderation_status = ANY (ARRAY['pending'::text, 'passed'::text, 'rejected'::text, 'skipped'::text]))),
    CONSTRAINT operation_output_seq_no_check CHECK ((seq_no >= 0))
);

CREATE TABLE operation.provider_call (
    id uuid NOT NULL,
    project_id uuid NOT NULL,
    operation_id uuid NOT NULL,
    attempt integer NOT NULL,
    action text NOT NULL,
    provider_key text NOT NULL,
    provider_task_id text,
    request_summary jsonb NOT NULL,
    response_summary jsonb,
    http_status integer,
    outcome text NOT NULL,
    usage jsonb,
    cost_micros bigint,
    latency_ms integer,
    region text NOT NULL,
    run_id uuid,
    call_seq integer,
    request_key text,
    model_profile_version_id uuid,
    price_rule_version_id uuid,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    dispatch_started_at timestamp with time zone,
    receipt jsonb,
    CONSTRAINT ck_provider_call_receipt CHECK (((receipt IS NULL) OR COALESCE(((jsonb_typeof(receipt) = 'object'::text) AND (octet_length((receipt)::text) <= 4096) AND ((receipt - ARRAY['version'::text, 'identity'::text, 'manifest_sha256'::text, 'outputs'::text]) = '{}'::jsonb) AND (jsonb_typeof((receipt -> 'version'::text)) = 'number'::text) AND ((receipt ->> 'version'::text) = '1'::text) AND (jsonb_typeof((receipt -> 'manifest_sha256'::text)) = 'string'::text) AND ((receipt ->> 'manifest_sha256'::text) ~ '^[0-9a-f]{64}$'::text) AND (jsonb_typeof((receipt -> 'identity'::text)) = 'object'::text) AND (((receipt -> 'identity'::text) - ARRAY['project_id'::text, 'operation_id'::text, 'action'::text, 'attempt'::text, 'request_key'::text, 'model_profile_version_id'::text, 'price_rule_version_id'::text]) = '{}'::jsonb) AND (jsonb_typeof((receipt #> '{identity,project_id}'::text[])) = 'string'::text) AND ((receipt #>> '{identity,project_id}'::text[]) = (project_id)::text) AND (jsonb_typeof((receipt #> '{identity,operation_id}'::text[])) = 'string'::text) AND ((receipt #>> '{identity,operation_id}'::text[]) = (operation_id)::text) AND (jsonb_typeof((receipt #> '{identity,action}'::text[])) = 'string'::text) AND ((receipt #>> '{identity,action}'::text[]) = 'submit'::text) AND (action = 'submit'::text) AND (jsonb_typeof((receipt #> '{identity,attempt}'::text[])) = 'number'::text) AND ((receipt #>> '{identity,attempt}'::text[]) = (attempt)::text) AND (jsonb_typeof((receipt #> '{identity,request_key}'::text[])) = 'string'::text) AND ((receipt #>> '{identity,request_key}'::text[]) = request_key) AND ((octet_length((receipt #>> '{identity,request_key}'::text[])) >= 1) AND (octet_length((receipt #>> '{identity,request_key}'::text[])) <= 256)) AND (btrim((receipt #>> '{identity,request_key}'::text[])) = (receipt #>> '{identity,request_key}'::text[])) AND (jsonb_typeof((receipt #> '{identity,model_profile_version_id}'::text[])) = 'string'::text) AND ((receipt #>> '{identity,model_profile_version_id}'::text[]) = (model_profile_version_id)::text) AND (jsonb_typeof((receipt #> '{identity,price_rule_version_id}'::text[])) = 'string'::text) AND ((receipt #>> '{identity,price_rule_version_id}'::text[]) = (price_rule_version_id)::text) AND
CASE
    WHEN (jsonb_typeof((receipt -> 'outputs'::text)) = 'array'::text) THEN (jsonb_array_length((receipt -> 'outputs'::text)) = 1)
    ELSE false
END AND (jsonb_typeof((receipt #> '{outputs,0}'::text[])) = 'object'::text) AND (((receipt #> '{outputs,0}'::text[]) - ARRAY['sequence'::text, 'size_bytes'::text, 'mime_type'::text, 'sha256'::text]) = '{}'::jsonb) AND (jsonb_typeof((receipt #> '{outputs,0,sequence}'::text[])) = 'number'::text) AND ((receipt #>> '{outputs,0,sequence}'::text[]) = '1'::text) AND
CASE
    WHEN ((jsonb_typeof((receipt #> '{outputs,0,size_bytes}'::text[])) = 'number'::text) AND ((receipt #>> '{outputs,0,size_bytes}'::text[]) ~ '^[0-9]+$'::text)) THEN ((((receipt #>> '{outputs,0,size_bytes}'::text[]))::numeric >= (1)::numeric) AND (((receipt #>> '{outputs,0,size_bytes}'::text[]))::numeric <= (33554432)::numeric))
    ELSE false
END AND (jsonb_typeof((receipt #> '{outputs,0,mime_type}'::text[])) = 'string'::text) AND ((receipt #>> '{outputs,0,mime_type}'::text[]) = ANY (ARRAY['image/png'::text, 'image/jpeg'::text, 'image/webp'::text])) AND (jsonb_typeof((receipt #> '{outputs,0,sha256}'::text[])) = 'string'::text) AND ((receipt #>> '{outputs,0,sha256}'::text[]) ~ '^[0-9a-f]{64}$'::text) AND (dispatch_started_at IS NOT NULL) AND ((request_summary ->> 'dispatch_contract'::text) = 'v1'::text) AND (outcome = 'ok'::text) AND ((response_summary ->> 'state'::text) = 'completed'::text) AND (provider_task_id IS NULL)), false))),
    CONSTRAINT provider_call_action_check CHECK ((action = ANY (ARRAY['submit'::text, 'query'::text, 'cancel'::text, 'callback'::text, 'moderate'::text, 'llm'::text]))),
    CONSTRAINT provider_call_attempt_check CHECK ((attempt > 0)),
    CONSTRAINT provider_call_call_seq_check CHECK (((call_seq IS NULL) OR (call_seq > 0))),
    CONSTRAINT provider_call_cost_micros_check CHECK (((cost_micros IS NULL) OR (cost_micros >= 0))),
    CONSTRAINT provider_call_latency_ms_check CHECK (((latency_ms IS NULL) OR (latency_ms >= 0))),
    CONSTRAINT provider_call_outcome_check CHECK ((outcome = ANY (ARRAY['ok'::text, 'error'::text, 'timeout'::text, 'unknown'::text]))),
    CONSTRAINT provider_call_provider_key_check CHECK ((btrim(provider_key) <> ''::text)),
    CONSTRAINT provider_call_region_check CHECK ((region = ANY (ARRAY['domestic'::text, 'overseas'::text]))),
    CONSTRAINT provider_call_request_summary_check CHECK ((jsonb_typeof(request_summary) = 'object'::text)),
    CONSTRAINT provider_call_response_summary_check CHECK (((response_summary IS NULL) OR (jsonb_typeof(response_summary) = 'object'::text))),
    CONSTRAINT provider_call_usage_check CHECK (((usage IS NULL) OR (jsonb_typeof(usage) = 'object'::text)))
)
PARTITION BY RANGE (create_time);

CREATE TABLE operation.quote_request (
    org_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    request_id uuid NOT NULL,
    fingerprint bytea NOT NULL,
    result jsonb NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT quote_request_fingerprint_check CHECK ((octet_length(fingerprint) = 32)),
    CONSTRAINT quote_request_result_check CHECK ((jsonb_typeof(result) = 'object'::text))
);

CREATE TABLE script.action_line (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    scene_id uuid NOT NULL,
    line_key uuid NOT NULL,
    seq_no integer NOT NULL,
    content text NOT NULL,
    span_start integer NOT NULL,
    span_end integer NOT NULL,
    CONSTRAINT action_line_check CHECK ((span_end > span_start)),
    CONSTRAINT action_line_seq_no_check CHECK ((seq_no > 0))
);

CREATE TABLE script.command (
    actor_id uuid NOT NULL,
    request_id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    action text NOT NULL,
    request_hash text NOT NULL,
    plan jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT command_plan_check CHECK ((jsonb_typeof(plan) = 'object'::text)),
    CONSTRAINT command_request_hash_check CHECK ((request_hash ~ '^[a-f0-9]{64}$'::text))
);

CREATE TABLE script.command_result (
    actor_id uuid NOT NULL,
    request_id uuid NOT NULL,
    response jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT command_result_response_check CHECK ((jsonb_typeof(response) = 'object'::text))
);

CREATE TABLE script.command_state (
    actor_id uuid NOT NULL,
    request_id uuid NOT NULL,
    id uuid NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    status text NOT NULL,
    failure_code text DEFAULT ''::text NOT NULL,
    cancellation_requested boolean DEFAULT false NOT NULL,
    needs_reconciliation boolean DEFAULT false NOT NULL,
    io_owner_id uuid,
    io_state text DEFAULT 'idle'::text NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT command_state_io_state_check CHECK ((io_state = ANY (ARRAY['idle'::text, 'running'::text, 'ended'::text, 'unknown'::text]))),
    CONSTRAINT command_state_revision_check CHECK ((revision > 0)),
    CONSTRAINT command_state_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'completed'::text, 'cancelled'::text])))
);

CREATE TABLE script.copy_object_intent (
    target_key text NOT NULL,
    snapshot_id uuid NOT NULL,
    source_key text NOT NULL,
    sha256 text NOT NULL,
    byte_size bigint NOT NULL,
    mime text NOT NULL,
    CONSTRAINT copy_object_intent_byte_size_check CHECK (((byte_size >= 0) AND (byte_size <= 37748736))),
    CONSTRAINT copy_object_intent_check CHECK ((source_key <> target_key)),
    CONSTRAINT copy_object_intent_sha256_check CHECK ((sha256 ~ '^[a-f0-9]{64}$'::text))
);

CREATE TABLE script.copy_object_state (
    target_key text NOT NULL,
    put_started boolean NOT NULL,
    confirmed boolean NOT NULL,
    removed boolean NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE TABLE script.copy_receipt (
    snapshot_id uuid NOT NULL,
    receipt jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT copy_receipt_receipt_check CHECK ((jsonb_typeof(receipt) = 'object'::text))
);

CREATE TABLE script.copy_snapshot (
    id uuid NOT NULL,
    job_id uuid NOT NULL,
    org_id uuid NOT NULL,
    source_project_id uuid NOT NULL,
    target_project_id uuid NOT NULL,
    manifest jsonb NOT NULL,
    manifest_sha256 text NOT NULL,
    content_sha256 text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT copy_snapshot_check CHECK ((source_project_id <> target_project_id)),
    CONSTRAINT copy_snapshot_content_sha256_check CHECK ((content_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT copy_snapshot_manifest_check CHECK ((jsonb_typeof(manifest) = 'object'::text)),
    CONSTRAINT copy_snapshot_manifest_sha256_check CHECK ((manifest_sha256 ~ '^[a-f0-9]{64}$'::text))
);

CREATE TABLE script.dialogue_line (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    scene_id uuid NOT NULL,
    line_key uuid NOT NULL,
    seq_no integer NOT NULL,
    kind text NOT NULL,
    content text NOT NULL,
    speaker_text text NOT NULL,
    character_id uuid,
    emotion text NOT NULL,
    content_hash text NOT NULL,
    span_start integer NOT NULL,
    span_end integer NOT NULL,
    character_version_id uuid,
    CONSTRAINT dialogue_character_version_pair CHECK (((character_version_id IS NULL) OR (character_id IS NOT NULL))),
    CONSTRAINT dialogue_line_check CHECK ((span_end > span_start)),
    CONSTRAINT dialogue_line_content_hash_check CHECK ((content_hash ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT dialogue_line_kind_check CHECK ((kind = ANY (ARRAY['dialogue'::text, 'voiceover'::text, 'inner'::text]))),
    CONSTRAINT dialogue_line_seq_no_check CHECK ((seq_no > 0))
);

CREATE TABLE script.episode (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    script_version_id uuid NOT NULL,
    split_set_id uuid NOT NULL,
    seq_no integer NOT NULL,
    title text NOT NULL,
    span_start integer NOT NULL,
    span_end integer NOT NULL,
    revision bigint NOT NULL,
    current_structure_id uuid,
    confirmed_structure_id uuid,
    previous_episode_id uuid,
    inherit_status text DEFAULT 'not_inherited'::text NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    CONSTRAINT episode_check CHECK ((span_end > span_start)),
    CONSTRAINT episode_inherit_status_check CHECK ((inherit_status = ANY (ARRAY['not_inherited'::text, 'pending'::text, 'inherited'::text]))),
    CONSTRAINT episode_revision_check CHECK ((revision > 0)),
    CONSTRAINT episode_seq_no_check CHECK ((seq_no > 0)),
    CONSTRAINT episode_span_start_check CHECK ((span_start >= 0)),
    CONSTRAINT episode_title_check CHECK (((char_length(title) >= 1) AND (char_length(title) <= 512)))
);

CREATE TABLE script.episode_structure (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    episode_id uuid NOT NULL,
    version_no bigint NOT NULL,
    source_hash text NOT NULL,
    document jsonb NOT NULL,
    actor_id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT episode_structure_document_check CHECK ((jsonb_typeof(document) = 'object'::text)),
    CONSTRAINT episode_structure_source_hash_check CHECK ((source_hash ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT episode_structure_version_no_check CHECK ((version_no > 0))
);

CREATE TABLE script.import_attempt (
    job_id uuid NOT NULL,
    attempt integer NOT NULL,
    expected_script_revision bigint NOT NULL,
    base_version_id uuid,
    publication_key uuid NOT NULL,
    positions integer[] NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT import_attempt_attempt_check CHECK (((attempt >= 1) AND (attempt <= 100))),
    CONSTRAINT import_attempt_expected_script_revision_check CHECK ((expected_script_revision >= 0))
);

CREATE TABLE script.import_command (
    actor_id uuid NOT NULL,
    request_id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    job_id uuid NOT NULL,
    action text NOT NULL,
    request_hash text NOT NULL,
    expected_revision bigint NOT NULL,
    event_id uuid NOT NULL,
    event_action text NOT NULL,
    event_attempt integer NOT NULL,
    response jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT import_command_action_check CHECK ((action = ANY (ARRAY['create'::text, 'cancel'::text, 'retry'::text, 'reconcile'::text]))),
    CONSTRAINT import_command_event_action_check CHECK ((event_action = ANY (ARRAY['start'::text, 'cancel'::text, 'reconcile'::text]))),
    CONSTRAINT import_command_expected_revision_check CHECK ((expected_revision >= 0)),
    CONSTRAINT import_command_request_hash_check CHECK ((request_hash ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT import_command_response_check CHECK ((jsonb_typeof(response) = 'object'::text))
);

CREATE TABLE script.import_file (
    job_id uuid NOT NULL,
    "position" integer NOT NULL,
    source_id uuid NOT NULL,
    asset_id uuid NOT NULL,
    project_id uuid NOT NULL,
    media_revision bigint NOT NULL,
    sha256 text NOT NULL,
    byte_size bigint NOT NULL,
    mime text NOT NULL,
    file_name text NOT NULL,
    CONSTRAINT import_file_byte_size_check CHECK (((byte_size >= 0) AND (byte_size <= 20971520))),
    CONSTRAINT import_file_media_revision_check CHECK ((media_revision > 0)),
    CONSTRAINT import_file_mime_check CHECK ((mime = ANY (ARRAY['text/plain'::text, 'application/vnd.openxmlformats-officedocument.wordprocessingml.document'::text]))),
    CONSTRAINT import_file_position_check CHECK ((("position" >= 0) AND ("position" <= 199))),
    CONSTRAINT import_file_sha256_check CHECK ((sha256 ~ '^[a-f0-9]{64}$'::text))
);

CREATE TABLE script.import_file_result (
    job_id uuid NOT NULL,
    attempt integer NOT NULL,
    "position" integer NOT NULL,
    result jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT import_file_result_result_check CHECK ((jsonb_typeof(result) = 'object'::text))
);

CREATE TABLE script.import_job (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    actor_role text NOT NULL,
    request_id uuid NOT NULL,
    request_hash text NOT NULL,
    expected_script_revision bigint NOT NULL,
    base_version_id uuid,
    rights_confirmed_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT import_job_actor_role_check CHECK ((actor_role = ANY (ARRAY['admin'::text, 'producer'::text]))),
    CONSTRAINT import_job_expected_script_revision_check CHECK ((expected_script_revision >= 0)),
    CONSTRAINT import_job_request_hash_check CHECK ((request_hash ~ '^[a-f0-9]{64}$'::text))
);

CREATE TABLE script.import_publication (
    job_id uuid NOT NULL,
    attempt integer NOT NULL,
    response jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT import_publication_response_check CHECK ((jsonb_typeof(response) = 'object'::text))
);

CREATE TABLE script.import_state (
    job_id uuid NOT NULL,
    revision bigint NOT NULL,
    attempt integer NOT NULL,
    status text NOT NULL,
    stage text NOT NULL,
    latest_script_revision bigint NOT NULL,
    latest_version_id uuid,
    cancellation_requested boolean DEFAULT false NOT NULL,
    reconciliation_requested boolean DEFAULT false NOT NULL,
    needs_reconciliation boolean DEFAULT false NOT NULL,
    failure_code text DEFAULT ''::text NOT NULL,
    io_owner_id uuid,
    io_state text NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT import_state_attempt_check CHECK (((attempt >= 1) AND (attempt <= 100))),
    CONSTRAINT import_state_io_state_check CHECK ((io_state = ANY (ARRAY['idle'::text, 'running'::text, 'ended'::text, 'unknown'::text]))),
    CONSTRAINT import_state_latest_script_revision_check CHECK ((latest_script_revision >= 0)),
    CONSTRAINT import_state_revision_check CHECK ((revision > 0)),
    CONSTRAINT import_state_stage_check CHECK ((stage = ANY (ARRAY['queued'::text, 'extracting'::text, 'normalizing'::text, 'storing'::text, 'committing'::text, 'completed'::text, 'failed'::text, 'cancelling'::text, 'awaiting_reconciliation'::text]))),
    CONSTRAINT import_state_status_check CHECK ((status = ANY (ARRAY['queued'::text, 'running'::text, 'partial'::text, 'succeeded'::text, 'failed'::text, 'cancel_requested'::text, 'cancelled'::text])))
);

CREATE TABLE script.object_intent (
    actor_id uuid NOT NULL,
    request_id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    object_key text NOT NULL,
    sha256 text NOT NULL,
    byte_size bigint NOT NULL,
    mime text NOT NULL,
    CONSTRAINT object_intent_byte_size_check CHECK (((byte_size >= 0) AND (byte_size <= 37748736))),
    CONSTRAINT object_intent_sha256_check CHECK ((sha256 ~ '^[a-f0-9]{64}$'::text))
);

CREATE TABLE script.object_state (
    object_key text NOT NULL,
    put_started boolean DEFAULT false NOT NULL,
    confirmed boolean DEFAULT false NOT NULL,
    removed boolean DEFAULT false NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE TABLE script.project_state (
    project_id uuid NOT NULL,
    org_id uuid NOT NULL,
    revision bigint NOT NULL,
    draft_version_id uuid NOT NULL,
    adopted_version_id uuid,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT project_state_revision_check CHECK ((revision > 0))
);

CREATE TABLE script.request (
    actor_id uuid NOT NULL,
    request_id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    action text NOT NULL,
    request_hash text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT request_request_hash_check CHECK ((request_hash ~ '^[a-f0-9]{64}$'::text))
);

CREATE TABLE script.review_command (
    actor_id uuid NOT NULL,
    request_id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    action text NOT NULL,
    request_hash text NOT NULL,
    response jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT review_command_action_check CHECK ((action = ANY (ARRAY['resplit'::text, 'confirm_split'::text, 'save_structure'::text, 'confirm_structure'::text, 'adopt'::text]))),
    CONSTRAINT review_command_request_hash_check CHECK ((request_hash ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT review_command_response_check CHECK ((jsonb_typeof(response) = 'object'::text))
);

CREATE TABLE script.scene (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    episode_structure_id uuid NOT NULL,
    scene_key uuid NOT NULL,
    seq_no integer NOT NULL,
    heading text NOT NULL,
    location_text text NOT NULL,
    time_of_day text NOT NULL,
    span_start integer NOT NULL,
    span_end integer NOT NULL,
    CONSTRAINT scene_check CHECK ((span_end > span_start)),
    CONSTRAINT scene_seq_no_check CHECK ((seq_no > 0))
);

CREATE TABLE script.script_source (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    source_lineage_id uuid NOT NULL,
    previous_source_id uuid,
    source_revision bigint NOT NULL,
    origin text NOT NULL,
    source_kind text NOT NULL,
    title text NOT NULL,
    status text NOT NULL,
    rights_actor_id uuid NOT NULL,
    rights_confirmed_at timestamp with time zone NOT NULL,
    media_asset_id uuid,
    media_revision bigint,
    media_sha256 text,
    original_key text NOT NULL,
    original_sha256 text NOT NULL,
    original_bytes bigint NOT NULL,
    original_mime text NOT NULL,
    rich_key text NOT NULL,
    rich_sha256 text NOT NULL,
    rich_bytes bigint NOT NULL,
    content_hash text NOT NULL,
    char_count integer NOT NULL,
    provenance jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT script_source_char_count_check CHECK (((char_count >= 0) AND (char_count <= 1500000))),
    CONSTRAINT script_source_check CHECK ((((media_asset_id IS NULL) AND (media_revision IS NULL) AND (media_sha256 IS NULL)) OR ((origin = 'file'::text) AND (media_asset_id IS NOT NULL) AND (media_revision > 0) AND (media_sha256 ~ '^[a-f0-9]{64}$'::text)))),
    CONSTRAINT script_source_check1 CHECK ((((source_revision = 1) AND (previous_source_id IS NULL) AND (source_lineage_id = id)) OR ((source_revision > 1) AND (previous_source_id IS NOT NULL)))),
    CONSTRAINT script_source_content_hash_check CHECK ((content_hash ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT script_source_origin_check CHECK ((origin = ANY (ARRAY['manual'::text, 'beeftv'::text, 'file'::text]))),
    CONSTRAINT script_source_original_bytes_check CHECK (((original_bytes >= 0) AND (original_bytes <= 20971520))),
    CONSTRAINT script_source_original_sha256_check CHECK ((original_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT script_source_provenance_check CHECK ((jsonb_typeof(provenance) = 'object'::text)),
    CONSTRAINT script_source_rich_bytes_check CHECK (((rich_bytes >= 0) AND (rich_bytes <= 8388608))),
    CONSTRAINT script_source_rich_sha256_check CHECK ((rich_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT script_source_source_kind_check CHECK ((source_kind = ANY (ARRAY['chapter'::text, 'episode'::text, 'document'::text]))),
    CONSTRAINT script_source_source_revision_check CHECK ((source_revision > 0)),
    CONSTRAINT script_source_status_check CHECK ((status = ANY (ARRAY['draft'::text, 'ready'::text, 'completed'::text]))),
    CONSTRAINT script_source_title_check CHECK (((char_length(title) >= 1) AND (char_length(title) <= 512)))
);

CREATE TABLE script.script_version (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    version_no bigint NOT NULL,
    source_ids uuid[] NOT NULL,
    source_spans jsonb NOT NULL,
    text_key text NOT NULL,
    text_sha256 text NOT NULL,
    text_bytes bigint NOT NULL,
    rich_key text NOT NULL,
    rich_sha256 text NOT NULL,
    rich_bytes bigint NOT NULL,
    content_hash text NOT NULL,
    document_sha256 text NOT NULL,
    source_manifest_sha256 text NOT NULL,
    char_count integer NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT script_version_char_count_check CHECK (((char_count >= 0) AND (char_count <= 1500000))),
    CONSTRAINT script_version_check CHECK ((content_hash = text_sha256)),
    CONSTRAINT script_version_check1 CHECK ((document_sha256 = rich_sha256)),
    CONSTRAINT script_version_rich_bytes_check CHECK (((rich_bytes >= 0) AND (rich_bytes <= 33554432))),
    CONSTRAINT script_version_rich_sha256_check CHECK ((rich_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT script_version_source_manifest_sha256_check CHECK ((source_manifest_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT script_version_source_spans_check CHECK ((jsonb_typeof(source_spans) = 'array'::text)),
    CONSTRAINT script_version_text_bytes_check CHECK (((text_bytes >= 0) AND (text_bytes <= 6000000))),
    CONSTRAINT script_version_text_sha256_check CHECK ((text_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT script_version_version_no_check CHECK ((version_no > 0))
);

CREATE TABLE script.source_control (
    actor_id uuid NOT NULL,
    request_id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    intent_id uuid NOT NULL,
    action text NOT NULL,
    expected_revision bigint NOT NULL,
    request_hash text NOT NULL,
    response jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT source_control_action_check CHECK ((action = ANY (ARRAY['cancel'::text, 'reconcile'::text]))),
    CONSTRAINT source_control_expected_revision_check CHECK ((expected_revision > 0)),
    CONSTRAINT source_control_request_hash_check CHECK ((request_hash ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT source_control_response_check CHECK ((jsonb_typeof(response) = 'object'::text))
);

CREATE TABLE script.split_confirmation (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    version_id uuid NOT NULL,
    candidate_set_id uuid NOT NULL,
    formal_set_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    revision bigint NOT NULL,
    preface jsonb,
    episodes jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT split_confirmation_episodes_check CHECK ((jsonb_typeof(episodes) = 'array'::text)),
    CONSTRAINT split_confirmation_revision_check CHECK ((revision > 0))
);

CREATE TABLE script.split_set (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    version_id uuid NOT NULL,
    kind text NOT NULL,
    origin text NOT NULL,
    preface jsonb,
    boundaries jsonb NOT NULL,
    warnings jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT split_set_boundaries_check CHECK ((jsonb_typeof(boundaries) = 'array'::text)),
    CONSTRAINT split_set_kind_check CHECK ((kind = ANY (ARRAY['candidate'::text, 'formal'::text]))),
    CONSTRAINT split_set_origin_check CHECK ((origin = ANY (ARRAY['sources'::text, 'rules'::text, 'manual'::text]))),
    CONSTRAINT split_set_warnings_check CHECK ((jsonb_typeof(warnings) = 'array'::text))
);

CREATE TABLE script.version_head (
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    version_id uuid NOT NULL,
    split_revision bigint DEFAULT 0 NOT NULL,
    candidate_split_set_id uuid NOT NULL,
    confirmed_split_set_id uuid,
    CONSTRAINT version_head_split_revision_check CHECK ((split_revision >= 0))
);

CREATE TABLE script.version_source (
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    version_id uuid NOT NULL,
    source_id uuid NOT NULL,
    "position" integer NOT NULL,
    CONSTRAINT version_source_position_check CHECK (("position" >= 0))
);

CREATE TABLE workspace.organization (
    id uuid NOT NULL,
    name text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    CONSTRAINT organization_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text])))
);

CREATE TABLE workspace.project (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    aspect_ratio text NOT NULL,
    style_type text NOT NULL,
    style_subtype text,
    style_preset_id uuid,
    resolution text DEFAULT '1080p'::text NOT NULL,
    allow_overseas_models boolean DEFAULT false NOT NULL,
    default_models jsonb DEFAULT '{}'::jsonb NOT NULL,
    aigc_mark_style jsonb DEFAULT '{"preset": "bottom_right"}'::jsonb NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    archived_at timestamp with time zone,
    delete_time timestamp with time zone,
    purge_after timestamp with time zone,
    revision integer DEFAULT 1 NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    cover_asset_id uuid,
    CONSTRAINT ck_project_style CHECK (((style_type = 'stylized'::text) = (style_subtype IS NOT NULL))),
    CONSTRAINT project_aspect_ratio_check CHECK ((aspect_ratio = ANY (ARRAY['9:16'::text, '16:9'::text]))),
    CONSTRAINT project_cover_nonzero CHECK (((cover_asset_id IS NULL) OR (cover_asset_id <> '00000000-0000-0000-0000-000000000000'::uuid))),
    CONSTRAINT project_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 50))),
    CONSTRAINT project_resolution_check CHECK ((resolution = '1080p'::text)),
    CONSTRAINT project_revision_check CHECK ((revision > 0)),
    CONSTRAINT project_status_check CHECK ((status = ANY (ARRAY['active'::text, 'archived'::text, 'copying'::text]))),
    CONSTRAINT project_style_subtype_check CHECK ((style_subtype = ANY (ARRAY['anime_jp'::text, 'guofeng_xianxia'::text, 'cartoon_3d'::text, 'manhwa'::text]))),
    CONSTRAINT project_style_type_check CHECK ((style_type = ANY (ARRAY['realistic'::text, 'stylized'::text])))
);

COMMENT ON COLUMN workspace.project.cover_asset_id IS 'Current-project ready/passed image; eligibility and current authorization are verified by the media owner in the project transaction.';

CREATE TABLE workspace.project_change_command (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    idem_key uuid NOT NULL,
    action text NOT NULL,
    request_sha256 text NOT NULL,
    status_code smallint NOT NULL,
    response_body jsonb NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT project_change_command_action_check CHECK ((action = ANY (ARRAY['patch'::text, 'archive'::text, 'unarchive'::text, 'delete'::text, 'restore'::text]))),
    CONSTRAINT project_change_command_request_sha256_check CHECK ((request_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT project_change_command_response_body_check CHECK ((jsonb_typeof(response_body) = 'object'::text)),
    CONSTRAINT project_change_command_status_code_check CHECK ((status_code = 200))
);

CREATE TABLE workspace.project_copy_command (
    id uuid NOT NULL,
    copy_job_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    idem_key uuid NOT NULL,
    request_sha256 text NOT NULL,
    response_body jsonb NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT project_copy_command_request_sha256_check CHECK ((request_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT project_copy_command_response_body_check CHECK ((jsonb_typeof(response_body) = 'object'::text))
);

CREATE TABLE workspace.project_copy_job (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    request_id uuid DEFAULT gen_random_uuid() NOT NULL,
    idem_key uuid NOT NULL,
    request_sha256 text NOT NULL,
    admission_response jsonb NOT NULL,
    source_project_id uuid NOT NULL,
    source_revision integer NOT NULL,
    target_project_id uuid NOT NULL,
    target_name text NOT NULL,
    status text NOT NULL,
    stage text NOT NULL,
    revision integer DEFAULT 1 NOT NULL,
    attempt integer DEFAULT 0 NOT NULL,
    worker_id uuid,
    started_at timestamp with time zone,
    manifest jsonb NOT NULL,
    workspace_snapshot jsonb NOT NULL,
    media_receipt jsonb,
    canvas_receipt jsonb,
    failure_code text DEFAULT ''::text NOT NULL,
    retryable boolean DEFAULT false NOT NULL,
    needs_reconciliation boolean DEFAULT false NOT NULL,
    cancellation_requested boolean DEFAULT false NOT NULL,
    reconciliation_requested boolean DEFAULT false NOT NULL,
    execution_unconfirmed boolean DEFAULT false NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    script_receipt jsonb,
    bible_receipt jsonb,
    execution_actor_id uuid,
    CONSTRAINT project_copy_job_admission_response_check CHECK ((jsonb_typeof(admission_response) = 'object'::text)),
    CONSTRAINT project_copy_job_attempt_check CHECK ((attempt >= 0)),
    CONSTRAINT project_copy_job_bible_receipt_check CHECK (((bible_receipt IS NULL) OR (jsonb_typeof(bible_receipt) = 'object'::text))),
    CONSTRAINT project_copy_job_check CHECK ((source_project_id <> target_project_id)),
    CONSTRAINT project_copy_job_check1 CHECK ((NOT (retryable AND needs_reconciliation))),
    CONSTRAINT project_copy_job_manifest_check CHECK ((jsonb_typeof(manifest) = 'object'::text)),
    CONSTRAINT project_copy_job_request_sha256_check CHECK ((request_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT project_copy_job_revision_check CHECK ((revision > 0)),
    CONSTRAINT project_copy_job_script_receipt_check CHECK (((script_receipt IS NULL) OR (jsonb_typeof(script_receipt) = 'object'::text))),
    CONSTRAINT project_copy_job_source_revision_check CHECK ((source_revision > 0)),
    CONSTRAINT project_copy_job_stage_check CHECK ((stage = ANY (ARRAY['media'::text, 'bible'::text, 'script'::text, 'canvases'::text, 'finalizing'::text, 'cleanup'::text, 'complete'::text]))),
    CONSTRAINT project_copy_job_status_check CHECK ((status = ANY (ARRAY['queued'::text, 'running'::text, 'failed'::text, 'cancel_requested'::text, 'cancelled'::text, 'succeeded'::text]))),
    CONSTRAINT project_copy_job_target_name_check CHECK (((char_length(target_name) >= 1) AND (char_length(target_name) <= 50))),
    CONSTRAINT project_copy_job_workspace_snapshot_check CHECK ((jsonb_typeof(workspace_snapshot) = 'object'::text))
);

CREATE TABLE workspace.project_folder (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    name text NOT NULL,
    cover_project_id uuid,
    cover_asset_id uuid,
    revision integer DEFAULT 1 NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    delete_time timestamp with time zone,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT project_folder_check CHECK (((cover_project_id IS NULL) = (cover_asset_id IS NULL))),
    CONSTRAINT project_folder_check1 CHECK ((is_delete = (delete_time IS NOT NULL))),
    CONSTRAINT project_folder_name_check CHECK (((char_length(btrim(name)) >= 1) AND (char_length(btrim(name)) <= 160))),
    CONSTRAINT project_folder_revision_check CHECK ((revision > 0))
);

CREATE TABLE workspace.project_folder_command (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    idem_key uuid NOT NULL,
    action text NOT NULL,
    request_sha256 text NOT NULL,
    response_body jsonb NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT project_folder_command_action_check CHECK ((action = ANY (ARRAY['create'::text, 'patch'::text, 'move'::text, 'recycle'::text]))),
    CONSTRAINT project_folder_command_request_sha256_check CHECK ((request_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT project_folder_command_response_body_check CHECK ((jsonb_typeof(response_body) = 'object'::text))
);

CREATE TABLE workspace.project_folder_placement (
    org_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    project_id uuid NOT NULL,
    folder_id uuid,
    revision integer DEFAULT 1 NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT project_folder_placement_revision_check CHECK ((revision > 0))
);

CREATE TABLE workspace.prompt_customization (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    owner_id uuid NOT NULL,
    operation text NOT NULL,
    mode text NOT NULL,
    content text NOT NULL,
    base_template_id uuid NOT NULL,
    revision integer DEFAULT 1 NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_prompt_customization_content CHECK ((((mode = 'inherit'::text) AND (content = ''::text)) OR ((mode = ANY (ARRAY['append'::text, 'rewrite'::text])) AND (btrim(content) <> ''::text)))),
    CONSTRAINT prompt_customization_content_check CHECK ((char_length(content) <= 12000)),
    CONSTRAINT prompt_customization_mode_check CHECK ((mode = ANY (ARRAY['inherit'::text, 'append'::text, 'rewrite'::text]))),
    CONSTRAINT prompt_customization_operation_check CHECK ((btrim(operation) <> ''::text)),
    CONSTRAINT prompt_customization_revision_check CHECK ((revision > 0))
);

CREATE TABLE workspace.style_preset (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid,
    name text NOT NULL,
    style_type text NOT NULL,
    style_subtype text,
    prompt_fragment text DEFAULT ''::text NOT NULL,
    negative_prompt text DEFAULT ''::text NOT NULL,
    reference_asset_ids uuid[] DEFAULT '{}'::uuid[] NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    CONSTRAINT style_preset_style_subtype_check CHECK ((style_subtype = ANY (ARRAY['anime_jp'::text, 'guofeng_xianxia'::text, 'cartoon_3d'::text, 'manhwa'::text]))),
    CONSTRAINT style_preset_style_type_check CHECK ((style_type = ANY (ARRAY['realistic'::text, 'stylized'::text])))
);

-- 主键、唯一约束与外键
ALTER TABLE ONLY audit.audit_log
    ADD CONSTRAINT audit_log_pkey PRIMARY KEY (id, create_time);

ALTER TABLE ONLY bible.character_confirmation
    ADD CONSTRAINT character_confirmation_entry_id_revision_key UNIQUE (entry_id, revision);

ALTER TABLE ONLY bible.character_confirmation
    ADD CONSTRAINT character_confirmation_org_id_project_id_id_key UNIQUE (org_id, project_id, id);

ALTER TABLE ONLY bible.character_confirmation
    ADD CONSTRAINT character_confirmation_pkey PRIMARY KEY (id);

ALTER TABLE ONLY bible."character"
    ADD CONSTRAINT character_org_id_project_id_id_key UNIQUE (org_id, project_id, id);

ALTER TABLE ONLY bible."character"
    ADD CONSTRAINT character_pkey PRIMARY KEY (id);

ALTER TABLE ONLY bible.character_redirect
    ADD CONSTRAINT character_redirect_org_id_project_id_id_key UNIQUE (org_id, project_id, id);

ALTER TABLE ONLY bible.character_redirect
    ADD CONSTRAINT character_redirect_pkey PRIMARY KEY (id);

ALTER TABLE ONLY bible.character_split
    ADD CONSTRAINT character_split_org_id_project_id_id_key UNIQUE (org_id, project_id, id);

ALTER TABLE ONLY bible.character_split
    ADD CONSTRAINT character_split_pkey PRIMARY KEY (id);

ALTER TABLE ONLY bible.character_version
    ADD CONSTRAINT character_version_entry_id_version_no_key UNIQUE (entry_id, version_no);

ALTER TABLE ONLY bible.character_version
    ADD CONSTRAINT character_version_org_id_project_id_id_key UNIQUE (org_id, project_id, id);

ALTER TABLE ONLY bible.character_version
    ADD CONSTRAINT character_version_pkey PRIMARY KEY (id);

ALTER TABLE ONLY bible.command
    ADD CONSTRAINT command_pkey PRIMARY KEY (actor_id, request_id);

ALTER TABLE ONLY bible.copy_cleanup_receipt
    ADD CONSTRAINT copy_cleanup_receipt_pkey PRIMARY KEY (snapshot_id);

ALTER TABLE ONLY bible.copy_receipt
    ADD CONSTRAINT copy_receipt_pkey PRIMARY KEY (snapshot_id);

ALTER TABLE ONLY bible.copy_snapshot
    ADD CONSTRAINT copy_snapshot_job_id_key UNIQUE (job_id);

ALTER TABLE ONLY bible.copy_snapshot
    ADD CONSTRAINT copy_snapshot_pkey PRIMARY KEY (id);

ALTER TABLE ONLY bible.copy_snapshot
    ADD CONSTRAINT copy_snapshot_target_project_id_key UNIQUE (target_project_id);

ALTER TABLE ONLY bible.copy_transfer_receipt
    ADD CONSTRAINT copy_transfer_receipt_pkey PRIMARY KEY (snapshot_id);

ALTER TABLE ONLY bible.location_confirmation
    ADD CONSTRAINT location_confirmation_entry_id_revision_key UNIQUE (entry_id, revision);

ALTER TABLE ONLY bible.location_confirmation
    ADD CONSTRAINT location_confirmation_org_id_project_id_id_key UNIQUE (org_id, project_id, id);

ALTER TABLE ONLY bible.location_confirmation
    ADD CONSTRAINT location_confirmation_pkey PRIMARY KEY (id);

ALTER TABLE ONLY bible.location
    ADD CONSTRAINT location_org_id_project_id_id_key UNIQUE (org_id, project_id, id);

ALTER TABLE ONLY bible.location
    ADD CONSTRAINT location_pkey PRIMARY KEY (id);

ALTER TABLE ONLY bible.location_version
    ADD CONSTRAINT location_version_entry_id_version_no_key UNIQUE (entry_id, version_no);

ALTER TABLE ONLY bible.location_version
    ADD CONSTRAINT location_version_org_id_project_id_id_key UNIQUE (org_id, project_id, id);

ALTER TABLE ONLY bible.location_version
    ADD CONSTRAINT location_version_pkey PRIMARY KEY (id);

ALTER TABLE ONLY bible.look
    ADD CONSTRAINT look_org_id_project_id_id_key UNIQUE (org_id, project_id, id);

ALTER TABLE ONLY bible.look
    ADD CONSTRAINT look_pkey PRIMARY KEY (id);

ALTER TABLE ONLY bible.look_version
    ADD CONSTRAINT look_version_character_version_id_look_id_key UNIQUE (character_version_id, look_id);

ALTER TABLE ONLY bible.look_version
    ADD CONSTRAINT look_version_character_version_id_position_key UNIQUE (character_version_id, "position");

ALTER TABLE ONLY bible.look_version
    ADD CONSTRAINT look_version_org_id_project_id_id_key UNIQUE (org_id, project_id, id);

ALTER TABLE ONLY bible.look_version
    ADD CONSTRAINT look_version_pkey PRIMARY KEY (id);

ALTER TABLE ONLY bible.prop_confirmation
    ADD CONSTRAINT prop_confirmation_entry_id_revision_key UNIQUE (entry_id, revision);

ALTER TABLE ONLY bible.prop_confirmation
    ADD CONSTRAINT prop_confirmation_org_id_project_id_id_key UNIQUE (org_id, project_id, id);

ALTER TABLE ONLY bible.prop_confirmation
    ADD CONSTRAINT prop_confirmation_pkey PRIMARY KEY (id);

ALTER TABLE ONLY bible.prop
    ADD CONSTRAINT prop_org_id_project_id_id_key UNIQUE (org_id, project_id, id);

ALTER TABLE ONLY bible.prop
    ADD CONSTRAINT prop_pkey PRIMARY KEY (id);

ALTER TABLE ONLY bible.prop_version
    ADD CONSTRAINT prop_version_entry_id_version_no_key UNIQUE (entry_id, version_no);

ALTER TABLE ONLY bible.prop_version
    ADD CONSTRAINT prop_version_org_id_project_id_id_key UNIQUE (org_id, project_id, id);

ALTER TABLE ONLY bible.prop_version
    ADD CONSTRAINT prop_version_pkey PRIMARY KEY (id);

ALTER TABLE ONLY bible.reference_version
    ADD CONSTRAINT reference_version_look_version_id_position_key UNIQUE (look_version_id, "position");

ALTER TABLE ONLY bible.reference_version
    ADD CONSTRAINT reference_version_look_version_id_role_key UNIQUE (look_version_id, role);

ALTER TABLE ONLY bible.reference_version
    ADD CONSTRAINT reference_version_org_id_project_id_id_key UNIQUE (org_id, project_id, id);

ALTER TABLE ONLY bible.reference_version
    ADD CONSTRAINT reference_version_pkey PRIMARY KEY (id);

ALTER TABLE ONLY bible.voice_version
    ADD CONSTRAINT voice_version_character_version_id_key UNIQUE (character_version_id);

ALTER TABLE ONLY bible.voice_version
    ADD CONSTRAINT voice_version_org_id_project_id_id_key UNIQUE (org_id, project_id, id);

ALTER TABLE ONLY bible.voice_version
    ADD CONSTRAINT voice_version_pkey PRIMARY KEY (id);

ALTER TABLE ONLY billing.budget
    ADD CONSTRAINT budget_pkey PRIMARY KEY (id);

ALTER TABLE ONLY billing.ledger_entry
    ADD CONSTRAINT ledger_entry_pkey PRIMARY KEY (id);

ALTER TABLE ONLY billing.reservation
    ADD CONSTRAINT reservation_operation_id_key UNIQUE (operation_id);

ALTER TABLE ONLY billing.reservation
    ADD CONSTRAINT reservation_pkey PRIMARY KEY (id);

ALTER TABLE ONLY billing.budget
    ADD CONSTRAINT uq_budget_project UNIQUE (project_id);

ALTER TABLE ONLY billing.reservation
    ADD CONSTRAINT uq_reservation_project_operation_id UNIQUE (project_id, operation_id, id);

ALTER TABLE canvas.node
    ADD CONSTRAINT ck_canvas_node_parent_self CHECK (((parent_id IS NULL) OR (parent_id <> id))) NOT VALID;

ALTER TABLE ONLY canvas.command_log
    ADD CONSTRAINT command_log_pkey PRIMARY KEY (id);

ALTER TABLE ONLY canvas.document
    ADD CONSTRAINT document_pkey PRIMARY KEY (id);

ALTER TABLE ONLY canvas.edge
    ADD CONSTRAINT edge_pkey PRIMARY KEY (id);

ALTER TABLE ONLY canvas.node
    ADD CONSTRAINT node_document_id_id_key UNIQUE (document_id, id);

ALTER TABLE ONLY canvas.node
    ADD CONSTRAINT node_pkey PRIMARY KEY (id);

ALTER TABLE ONLY canvas.project_copy_receipt
    ADD CONSTRAINT project_copy_receipt_pkey PRIMARY KEY (snapshot_id);

ALTER TABLE ONLY canvas.project_copy_snapshot
    ADD CONSTRAINT project_copy_snapshot_job_id_key UNIQUE (job_id);

ALTER TABLE ONLY canvas.project_copy_snapshot
    ADD CONSTRAINT project_copy_snapshot_pkey PRIMARY KEY (id);

ALTER TABLE ONLY canvas.command_log
    ADD CONSTRAINT uq_canvas_command_rev UNIQUE (document_id, revision);

ALTER TABLE ONLY catalog.capability
    ADD CONSTRAINT capability_key_key UNIQUE (key);

ALTER TABLE ONLY catalog.capability
    ADD CONSTRAINT capability_pkey PRIMARY KEY (id);

ALTER TABLE ONLY catalog.model_profile
    ADD CONSTRAINT model_profile_model_key_key UNIQUE (model_key);

ALTER TABLE ONLY catalog.model_profile
    ADD CONSTRAINT model_profile_pkey PRIMARY KEY (id);

ALTER TABLE ONLY catalog.model_profile_version
    ADD CONSTRAINT model_profile_version_pkey PRIMARY KEY (id);

ALTER TABLE ONLY catalog.price_rule_version
    ADD CONSTRAINT price_rule_version_pkey PRIMARY KEY (id);

ALTER TABLE ONLY catalog.provider_credential
    ADD CONSTRAINT provider_credential_pkey PRIMARY KEY (id);

ALTER TABLE ONLY catalog.provider
    ADD CONSTRAINT provider_key_key UNIQUE (key);

ALTER TABLE ONLY catalog.provider
    ADD CONSTRAINT provider_pkey PRIMARY KEY (id);

ALTER TABLE ONLY catalog.model_profile_version
    ADD CONSTRAINT uq_model_profile_version UNIQUE (model_profile_id, version_no);

ALTER TABLE ONLY catalog.model_profile_version
    ADD CONSTRAINT uq_model_profile_version_owner UNIQUE (model_profile_id, id);

ALTER TABLE ONLY catalog.price_rule_version
    ADD CONSTRAINT uq_price_rule_version UNIQUE (model_profile_id, version_no);

ALTER TABLE ONLY identity."user"
    ADD CONSTRAINT user_org_id_id_unique UNIQUE (org_id, id);

ALTER TABLE ONLY identity."user"
    ADD CONSTRAINT user_pkey PRIMARY KEY (id);

ALTER TABLE ONLY infra.idempotency_record
    ADD CONSTRAINT idempotency_record_pkey PRIMARY KEY (id);

ALTER TABLE ONLY infra.outbox
    ADD CONSTRAINT outbox_pkey PRIMARY KEY (id, create_time);

ALTER TABLE ONLY infra.processed_event
    ADD CONSTRAINT processed_event_pkey PRIMARY KEY (id);

ALTER TABLE ONLY infra.idempotency_record
    ADD CONSTRAINT uq_idempotency_record_natural UNIQUE (actor_id, idem_key);

ALTER TABLE ONLY infra.processed_event
    ADD CONSTRAINT uq_processed_event_natural UNIQUE (consumer, event_id);

ALTER TABLE ONLY media.library_command
    ADD CONSTRAINT library_command_actor_id_idem_key_key UNIQUE (actor_id, idem_key);

ALTER TABLE ONLY media.library_command
    ADD CONSTRAINT library_command_pkey PRIMARY KEY (id);

ALTER TABLE ONLY media.library_folder
    ADD CONSTRAINT library_folder_library_id_id_key UNIQUE (library_id, id);

ALTER TABLE ONLY media.library_folder
    ADD CONSTRAINT library_folder_library_id_parent_id_name_key_key UNIQUE NULLS NOT DISTINCT (library_id, parent_id, name_key);

ALTER TABLE ONLY media.library_folder
    ADD CONSTRAINT library_folder_pkey PRIMARY KEY (id);

ALTER TABLE ONLY media.library
    ADD CONSTRAINT library_id_kind_key UNIQUE (id, kind);

ALTER TABLE ONLY media.library_item
    ADD CONSTRAINT library_item_library_id_asset_id_key UNIQUE (library_id, asset_id);

ALTER TABLE ONLY media.library_item
    ADD CONSTRAINT library_item_pkey PRIMARY KEY (id);

ALTER TABLE ONLY media.library
    ADD CONSTRAINT library_pkey PRIMARY KEY (id);

ALTER TABLE media.media_asset
    ADD CONSTRAINT media_asset_document_facts_check CHECK (((kind <> 'document'::text) OR ((origin = ANY (ARRAY['upload'::text, 'system'::text])) AND ((byte_size >= 1) AND (byte_size <= 20971520)) AND (sha256 IS NOT NULL) AND (sha256 ~ '^[0-9a-f]{64}$'::text) AND (codec IS NOT NULL) AND (width IS NULL) AND (height IS NULL) AND (duration_ms IS NULL) AND (fps IS NULL) AND (audio_channels IS NULL) AND (file_name IS NOT NULL) AND ((octet_length(file_name) >= 1) AND (octet_length(file_name) <= 255)) AND (file_name = btrim(file_name)) AND (file_name !~ '[[:cntrl:]/\\]'::text) AND (((mime_type = 'text/plain'::text) AND (codec = 'txt'::text) AND (lower(file_name) ~~ '%.txt'::text) AND (object_key ~~ '%.txt'::text)) OR ((mime_type = 'application/vnd.openxmlformats-officedocument.wordprocessingml.document'::text) AND (codec = 'docx'::text) AND (lower(file_name) ~~ '%.docx'::text) AND (object_key ~~ '%.docx'::text)))))) NOT VALID;

ALTER TABLE ONLY media.media_asset
    ADD CONSTRAINT media_asset_object_key_key UNIQUE (object_key);

ALTER TABLE ONLY media.media_asset
    ADD CONSTRAINT media_asset_pkey PRIMARY KEY (id);

ALTER TABLE ONLY media.package_command
    ADD CONSTRAINT package_command_pkey PRIMARY KEY (actor_id, idem_key);

ALTER TABLE ONLY media.package_job
    ADD CONSTRAINT package_job_actor_id_idem_key_key UNIQUE (actor_id, idem_key);

ALTER TABLE ONLY media.package_job
    ADD CONSTRAINT package_job_pkey PRIMARY KEY (id);

ALTER TABLE ONLY media.package_object
    ADD CONSTRAINT package_object_object_key_key UNIQUE (object_key);

ALTER TABLE ONLY media.package_object
    ADD CONSTRAINT package_object_pkey PRIMARY KEY (job_id, object_index);

ALTER TABLE ONLY media.project_copy_object
    ADD CONSTRAINT project_copy_object_pkey PRIMARY KEY (snapshot_id, target_asset_id, rendition_kind);

ALTER TABLE ONLY media.project_copy_object
    ADD CONSTRAINT project_copy_object_target_object_key_key UNIQUE (target_object_key);

ALTER TABLE ONLY media.project_copy_receipt
    ADD CONSTRAINT project_copy_receipt_pkey PRIMARY KEY (snapshot_id);

ALTER TABLE ONLY media.project_copy_snapshot
    ADD CONSTRAINT project_copy_snapshot_job_id_key UNIQUE (job_id);

ALTER TABLE ONLY media.project_copy_snapshot
    ADD CONSTRAINT project_copy_snapshot_pkey PRIMARY KEY (id);

ALTER TABLE ONLY media.purge_command
    ADD CONSTRAINT purge_command_pkey PRIMARY KEY (actor_id, idem_key);

ALTER TABLE ONLY media.purge_item
    ADD CONSTRAINT purge_item_job_id_item_id_key UNIQUE (job_id, item_id);

ALTER TABLE ONLY media.purge_item
    ADD CONSTRAINT purge_item_pkey PRIMARY KEY (job_id, item_index);

ALTER TABLE ONLY media.purge_job
    ADD CONSTRAINT purge_job_pkey PRIMARY KEY (id);

ALTER TABLE ONLY media.purge_object
    ADD CONSTRAINT purge_object_pkey PRIMARY KEY (job_id, item_index, rendition_kind);

ALTER TABLE ONLY media.rendition
    ADD CONSTRAINT rendition_pkey PRIMARY KEY (id);

ALTER TABLE ONLY media.transfer_command
    ADD CONSTRAINT transfer_command_pkey PRIMARY KEY (actor_id, idem_key);

ALTER TABLE ONLY media.transfer_item
    ADD CONSTRAINT transfer_item_job_id_source_item_id_key UNIQUE (job_id, source_item_id);

ALTER TABLE ONLY media.transfer_item
    ADD CONSTRAINT transfer_item_pkey PRIMARY KEY (job_id, item_index);

ALTER TABLE ONLY media.transfer_item
    ADD CONSTRAINT transfer_item_target_item_id_key UNIQUE (target_item_id);

ALTER TABLE ONLY media.transfer_job
    ADD CONSTRAINT transfer_job_pkey PRIMARY KEY (id);

ALTER TABLE ONLY media.transfer_object
    ADD CONSTRAINT transfer_object_pkey PRIMARY KEY (job_id, item_index, rendition_kind);

ALTER TABLE ONLY media.transfer_object
    ADD CONSTRAINT transfer_object_target_object_key_key UNIQUE (target_object_key);

ALTER TABLE ONLY media.upload_request
    ADD CONSTRAINT upload_request_personal_unique UNIQUE (personal_org_id, principal_id, request_key);

ALTER TABLE ONLY media.upload_request
    ADD CONSTRAINT upload_request_project_unique UNIQUE (project_id, principal_id, request_key);

ALTER TABLE ONLY media.media_asset
    ADD CONSTRAINT uq_media_asset_project_id UNIQUE (project_id, id);

ALTER TABLE ONLY media.rendition
    ADD CONSTRAINT uq_rendition_natural UNIQUE (media_asset_id, kind);

ALTER TABLE ONLY mediatool.depth_command
    ADD CONSTRAINT depth_command_event_id_key UNIQUE (event_id);

ALTER TABLE ONLY mediatool.depth_command
    ADD CONSTRAINT depth_command_pkey PRIMARY KEY (actor_id, request_id);

ALTER TABLE ONLY mediatool.depth_job
    ADD CONSTRAINT depth_job_pkey PRIMARY KEY (id);

ALTER TABLE ONLY mediatool.depth_object
    ADD CONSTRAINT depth_object_object_key_key UNIQUE (object_key);

ALTER TABLE ONLY mediatool.depth_object
    ADD CONSTRAINT depth_object_pkey PRIMARY KEY (job_id, attempt, kind);

ALTER TABLE ONLY mediatool.depth_result_intent
    ADD CONSTRAINT depth_result_intent_asset_id_key UNIQUE (asset_id);

ALTER TABLE ONLY mediatool.depth_result_intent
    ADD CONSTRAINT depth_result_intent_pkey PRIMARY KEY (job_id, attempt);

ALTER TABLE ONLY mediatool.export_command
    ADD CONSTRAINT export_command_event_id_key UNIQUE (event_id);

ALTER TABLE ONLY mediatool.export_command
    ADD CONSTRAINT export_command_pkey PRIMARY KEY (actor_id, request_id);

ALTER TABLE ONLY mediatool.export_job
    ADD CONSTRAINT export_job_pkey PRIMARY KEY (id);

ALTER TABLE ONLY mediatool.transcription_command
    ADD CONSTRAINT transcription_command_event_id_key UNIQUE (event_id);

ALTER TABLE ONLY mediatool.transcription_command
    ADD CONSTRAINT transcription_command_pkey PRIMARY KEY (actor_id, request_id);

ALTER TABLE ONLY mediatool.transcription_job
    ADD CONSTRAINT transcription_job_pkey PRIMARY KEY (id);

ALTER TABLE ONLY operation.batch_launch
    ADD CONSTRAINT batch_launch_pkey PRIMARY KEY (operation_id);

ALTER TABLE ONLY operation.batch
    ADD CONSTRAINT batch_pkey PRIMARY KEY (id);

ALTER TABLE ONLY operation.confirmation_request
    ADD CONSTRAINT confirmation_request_pkey PRIMARY KEY (org_id, actor_id, request_id);

ALTER TABLE ONLY operation.operation_event
    ADD CONSTRAINT operation_event_pkey PRIMARY KEY (id);

ALTER TABLE ONLY operation.operation_input
    ADD CONSTRAINT operation_input_pkey PRIMARY KEY (id);

ALTER TABLE ONLY operation.operation_output
    ADD CONSTRAINT operation_output_pkey PRIMARY KEY (id);

ALTER TABLE ONLY operation.operation
    ADD CONSTRAINT operation_pkey PRIMARY KEY (id);

ALTER TABLE ONLY operation.operation
    ADD CONSTRAINT operation_provider_request_key_key UNIQUE (provider_request_key);

ALTER TABLE ONLY operation.operation
    ADD CONSTRAINT operation_workflow_id_key UNIQUE (workflow_id);

ALTER TABLE ONLY operation.provider_call
    ADD CONSTRAINT provider_call_pkey PRIMARY KEY (id, create_time);

ALTER TABLE ONLY operation.quote_request
    ADD CONSTRAINT quote_request_pkey PRIMARY KEY (org_id, actor_id, request_id);

ALTER TABLE ONLY operation.batch
    ADD CONSTRAINT uq_batch_project_id UNIQUE (project_id, id);

ALTER TABLE ONLY operation.operation_input
    ADD CONSTRAINT uq_operation_input_natural UNIQUE (operation_id, seq_no);

ALTER TABLE ONLY operation.operation_output
    ADD CONSTRAINT uq_operation_output UNIQUE (operation_id, seq_no);

ALTER TABLE ONLY operation.operation
    ADD CONSTRAINT uq_operation_project_id UNIQUE (project_id, id);

ALTER TABLE ONLY script.action_line
    ADD CONSTRAINT action_line_org_id_project_id_id_key UNIQUE (org_id, project_id, id);

ALTER TABLE ONLY script.action_line
    ADD CONSTRAINT action_line_pkey PRIMARY KEY (id);

ALTER TABLE ONLY script.action_line
    ADD CONSTRAINT action_line_scene_id_line_key_key UNIQUE (scene_id, line_key);

ALTER TABLE ONLY script.command
    ADD CONSTRAINT command_pkey PRIMARY KEY (actor_id, request_id);

ALTER TABLE ONLY script.command_result
    ADD CONSTRAINT command_result_pkey PRIMARY KEY (actor_id, request_id);

ALTER TABLE ONLY script.command_state
    ADD CONSTRAINT command_state_id_key UNIQUE (id);

ALTER TABLE ONLY script.command_state
    ADD CONSTRAINT command_state_pkey PRIMARY KEY (actor_id, request_id);

ALTER TABLE ONLY script.copy_object_intent
    ADD CONSTRAINT copy_object_intent_pkey PRIMARY KEY (target_key);

ALTER TABLE ONLY script.copy_object_intent
    ADD CONSTRAINT copy_object_intent_snapshot_id_source_key_key UNIQUE (snapshot_id, source_key);

ALTER TABLE ONLY script.copy_object_state
    ADD CONSTRAINT copy_object_state_pkey PRIMARY KEY (target_key);

ALTER TABLE ONLY script.copy_receipt
    ADD CONSTRAINT copy_receipt_pkey PRIMARY KEY (snapshot_id);

ALTER TABLE ONLY script.copy_snapshot
    ADD CONSTRAINT copy_snapshot_job_id_key UNIQUE (job_id);

ALTER TABLE ONLY script.copy_snapshot
    ADD CONSTRAINT copy_snapshot_pkey PRIMARY KEY (id);

ALTER TABLE ONLY script.copy_snapshot
    ADD CONSTRAINT copy_snapshot_target_project_id_key UNIQUE (target_project_id);

ALTER TABLE ONLY script.dialogue_line
    ADD CONSTRAINT dialogue_line_org_id_project_id_id_key UNIQUE (org_id, project_id, id);

ALTER TABLE ONLY script.dialogue_line
    ADD CONSTRAINT dialogue_line_pkey PRIMARY KEY (id);

ALTER TABLE ONLY script.dialogue_line
    ADD CONSTRAINT dialogue_line_scene_id_line_key_key UNIQUE (scene_id, line_key);

ALTER TABLE ONLY script.episode
    ADD CONSTRAINT episode_org_id_project_id_id_key UNIQUE (org_id, project_id, id);

ALTER TABLE ONLY script.episode
    ADD CONSTRAINT episode_pkey PRIMARY KEY (id);

ALTER TABLE ONLY script.episode_structure
    ADD CONSTRAINT episode_structure_episode_id_version_no_key UNIQUE (episode_id, version_no);

ALTER TABLE ONLY script.episode_structure
    ADD CONSTRAINT episode_structure_org_id_project_id_id_key UNIQUE (org_id, project_id, id);

ALTER TABLE ONLY script.episode_structure
    ADD CONSTRAINT episode_structure_pkey PRIMARY KEY (id);

ALTER TABLE ONLY script.import_attempt
    ADD CONSTRAINT import_attempt_pkey PRIMARY KEY (job_id, attempt);

ALTER TABLE ONLY script.import_attempt
    ADD CONSTRAINT import_attempt_publication_key_key UNIQUE (publication_key);

ALTER TABLE ONLY script.import_command
    ADD CONSTRAINT import_command_event_id_key UNIQUE (event_id);

ALTER TABLE ONLY script.import_command
    ADD CONSTRAINT import_command_pkey PRIMARY KEY (actor_id, request_id);

ALTER TABLE ONLY script.import_file
    ADD CONSTRAINT import_file_job_id_asset_id_key UNIQUE (job_id, asset_id);

ALTER TABLE ONLY script.import_file
    ADD CONSTRAINT import_file_pkey PRIMARY KEY (job_id, "position");

ALTER TABLE ONLY script.import_file_result
    ADD CONSTRAINT import_file_result_pkey PRIMARY KEY (job_id, attempt, "position");

ALTER TABLE ONLY script.import_file
    ADD CONSTRAINT import_file_source_id_key UNIQUE (source_id);

ALTER TABLE ONLY script.import_job
    ADD CONSTRAINT import_job_actor_id_request_id_key UNIQUE (actor_id, request_id);

ALTER TABLE ONLY script.import_job
    ADD CONSTRAINT import_job_org_id_project_id_id_key UNIQUE (org_id, project_id, id);

ALTER TABLE ONLY script.import_job
    ADD CONSTRAINT import_job_pkey PRIMARY KEY (id);

ALTER TABLE ONLY script.import_publication
    ADD CONSTRAINT import_publication_pkey PRIMARY KEY (job_id, attempt);

ALTER TABLE ONLY script.import_state
    ADD CONSTRAINT import_state_pkey PRIMARY KEY (job_id);

ALTER TABLE ONLY script.object_intent
    ADD CONSTRAINT object_intent_pkey PRIMARY KEY (object_key);

ALTER TABLE ONLY script.object_state
    ADD CONSTRAINT object_state_pkey PRIMARY KEY (object_key);

ALTER TABLE ONLY script.project_state
    ADD CONSTRAINT project_state_pkey PRIMARY KEY (project_id);

ALTER TABLE ONLY script.request
    ADD CONSTRAINT request_pkey PRIMARY KEY (actor_id, request_id);

ALTER TABLE ONLY script.review_command
    ADD CONSTRAINT review_command_pkey PRIMARY KEY (actor_id, request_id);

ALTER TABLE ONLY script.scene
    ADD CONSTRAINT scene_episode_structure_id_scene_key_key UNIQUE (episode_structure_id, scene_key);

ALTER TABLE ONLY script.scene
    ADD CONSTRAINT scene_episode_structure_id_seq_no_key UNIQUE (episode_structure_id, seq_no);

ALTER TABLE ONLY script.scene
    ADD CONSTRAINT scene_org_id_project_id_id_key UNIQUE (org_id, project_id, id);

ALTER TABLE ONLY script.scene
    ADD CONSTRAINT scene_pkey PRIMARY KEY (id);

ALTER TABLE ONLY script.script_source
    ADD CONSTRAINT script_source_org_id_project_id_id_key UNIQUE (org_id, project_id, id);

ALTER TABLE ONLY script.script_source
    ADD CONSTRAINT script_source_pkey PRIMARY KEY (id);

ALTER TABLE ONLY script.script_source
    ADD CONSTRAINT script_source_project_id_source_lineage_id_source_revision_key UNIQUE (project_id, source_lineage_id, source_revision);

ALTER TABLE ONLY script.script_version
    ADD CONSTRAINT script_version_org_id_project_id_id_key UNIQUE (org_id, project_id, id);

ALTER TABLE ONLY script.script_version
    ADD CONSTRAINT script_version_pkey PRIMARY KEY (id);

ALTER TABLE ONLY script.script_version
    ADD CONSTRAINT script_version_project_id_source_manifest_sha256_key UNIQUE (project_id, source_manifest_sha256);

ALTER TABLE ONLY script.script_version
    ADD CONSTRAINT script_version_project_id_version_no_key UNIQUE (project_id, version_no);

ALTER TABLE ONLY script.source_control
    ADD CONSTRAINT source_control_pkey PRIMARY KEY (actor_id, request_id);

ALTER TABLE ONLY script.split_confirmation
    ADD CONSTRAINT split_confirmation_org_id_project_id_id_key UNIQUE (org_id, project_id, id);

ALTER TABLE ONLY script.split_confirmation
    ADD CONSTRAINT split_confirmation_pkey PRIMARY KEY (id);

ALTER TABLE ONLY script.split_confirmation
    ADD CONSTRAINT split_confirmation_version_id_revision_key UNIQUE (version_id, revision);

ALTER TABLE ONLY script.split_set
    ADD CONSTRAINT split_set_org_id_project_id_id_key UNIQUE (org_id, project_id, id);

ALTER TABLE ONLY script.split_set
    ADD CONSTRAINT split_set_pkey PRIMARY KEY (id);

ALTER TABLE ONLY script.version_head
    ADD CONSTRAINT version_head_pkey PRIMARY KEY (version_id);

ALTER TABLE ONLY script.version_source
    ADD CONSTRAINT version_source_pkey PRIMARY KEY (version_id, "position");

ALTER TABLE ONLY script.version_source
    ADD CONSTRAINT version_source_version_id_source_id_key UNIQUE (version_id, source_id);

ALTER TABLE ONLY workspace.organization
    ADD CONSTRAINT organization_pkey PRIMARY KEY (id);

ALTER TABLE ONLY workspace.project_change_command
    ADD CONSTRAINT project_change_command_actor_id_idem_key_key UNIQUE (actor_id, idem_key);

ALTER TABLE ONLY workspace.project_change_command
    ADD CONSTRAINT project_change_command_pkey PRIMARY KEY (id);

ALTER TABLE ONLY workspace.project_copy_command
    ADD CONSTRAINT project_copy_command_actor_id_idem_key_key UNIQUE (actor_id, idem_key);

ALTER TABLE ONLY workspace.project_copy_command
    ADD CONSTRAINT project_copy_command_pkey PRIMARY KEY (id);

ALTER TABLE ONLY workspace.project_copy_job
    ADD CONSTRAINT project_copy_job_actor_id_idem_key_key UNIQUE (actor_id, idem_key);

ALTER TABLE ONLY workspace.project_copy_job
    ADD CONSTRAINT project_copy_job_pkey PRIMARY KEY (id);

ALTER TABLE ONLY workspace.project_copy_job
    ADD CONSTRAINT project_copy_job_target_project_id_key UNIQUE (target_project_id);

ALTER TABLE ONLY workspace.project_folder_command
    ADD CONSTRAINT project_folder_command_actor_id_idem_key_key UNIQUE (actor_id, idem_key);

ALTER TABLE ONLY workspace.project_folder_command
    ADD CONSTRAINT project_folder_command_pkey PRIMARY KEY (id);

ALTER TABLE ONLY workspace.project_folder
    ADD CONSTRAINT project_folder_id_org_id_actor_id_key UNIQUE (id, org_id, actor_id);

ALTER TABLE ONLY workspace.project_folder
    ADD CONSTRAINT project_folder_pkey PRIMARY KEY (id);

ALTER TABLE ONLY workspace.project_folder_placement
    ADD CONSTRAINT project_folder_placement_pkey PRIMARY KEY (actor_id, project_id);

ALTER TABLE ONLY workspace.project
    ADD CONSTRAINT project_pkey PRIMARY KEY (id);

ALTER TABLE ONLY workspace.prompt_customization
    ADD CONSTRAINT prompt_customization_pkey PRIMARY KEY (id);

ALTER TABLE ONLY workspace.style_preset
    ADD CONSTRAINT style_preset_pkey PRIMARY KEY (id);

ALTER TABLE ONLY workspace.prompt_customization
    ADD CONSTRAINT uq_prompt_customization_owner_operation UNIQUE (org_id, owner_id, operation);

-- 索引
CREATE INDEX ix_audit_action ON ONLY audit.audit_log USING btree (action, create_time DESC);

CREATE INDEX ix_audit_org_time ON ONLY audit.audit_log USING btree (org_id, create_time DESC, id DESC);

CREATE INDEX ix_audit_project_time ON ONLY audit.audit_log USING btree (project_id, create_time DESC);

CREATE INDEX character_project_page ON bible."character" USING btree (org_id, project_id, created_at, id);

CREATE INDEX location_project_page ON bible.location USING btree (org_id, project_id, created_at, id);

CREATE UNIQUE INDEX look_one_default ON bible.look_version USING btree (character_version_id) WHERE is_default;

CREATE INDEX prop_project_page ON bible.prop USING btree (org_id, project_id, created_at, id);

CREATE INDEX ix_ledger_episode ON billing.ledger_entry USING btree (episode_id) WHERE (episode_id IS NOT NULL);

CREATE INDEX ix_ledger_project_time ON billing.ledger_entry USING btree (project_id, create_time);

CREATE INDEX ix_reservation_held ON billing.reservation USING btree (project_id) WHERE (status = 'held'::text);

CREATE INDEX ix_canvas_document_project ON canvas.document USING btree (project_id) WHERE (NOT is_delete);

CREATE INDEX ix_canvas_node_doc ON canvas.node USING btree (document_id);

CREATE INDEX ix_model_profile_provider_not_deleted ON catalog.model_profile USING btree (provider_id) WHERE (NOT is_delete);

CREATE INDEX ix_price_rule_effective ON catalog.price_rule_version USING btree (model_profile_id, effective_from DESC);

CREATE UNIQUE INDEX uq_provider_credential_active ON catalog.provider_credential USING btree (provider_id) WHERE ((status = 'active'::text) AND (NOT is_delete));

CREATE INDEX idx_user_directory ON identity."user" USING btree (org_id, create_time DESC, id DESC) WHERE (NOT is_delete);

ALTER TABLE identity.user_session ADD FOREIGN KEY (account_id) REFERENCES identity."user" (id);
ALTER TABLE identity.auth_event ADD FOREIGN KEY (account_id) REFERENCES identity."user" (id);

CREATE UNIQUE INDEX uq_user_login_name ON identity."user" USING btree (org_id, login_name) WHERE (NOT is_delete);

CREATE INDEX ix_outbox_pending ON ONLY infra.outbox USING btree (create_time) WHERE (published_at IS NULL);

CREATE INDEX ix_outbox_published_retention ON ONLY infra.outbox USING btree (published_at, create_time, id) WHERE (published_at IS NOT NULL);

CREATE INDEX ix_processed_event_create_time ON infra.processed_event USING btree (create_time, id);

CREATE INDEX ix_media_name_trgm ON media.media_asset USING gin (file_name public.gin_trgm_ops);

CREATE INDEX ix_media_project_kind ON media.media_asset USING btree (project_id, kind, create_time DESC) WHERE (NOT is_delete);

CREATE INDEX ix_media_sha256 ON media.media_asset USING btree (project_id, sha256) WHERE (NOT is_delete);

CREATE INDEX library_item_folder ON media.library_item USING btree (library_id, folder_id, catalog_state, id);

CREATE INDEX library_item_scoped_order ON media.library_item USING btree (library_id, catalog_state, update_time DESC, id);

CREATE INDEX library_item_search ON media.library_item USING gin (title public.gin_trgm_ops);

CREATE UNIQUE INDEX library_personal_scope ON media.library USING btree (org_id, personal_actor_id) WHERE (kind = 'personal'::text);

CREATE UNIQUE INDEX library_project_scope ON media.library USING btree (project_id) WHERE (kind = 'project'::text);

CREATE INDEX media_personal_current ON media.media_asset USING btree (personal_org_id, personal_actor_id, update_time DESC, id) WHERE ((project_id IS NULL) AND (NOT is_delete));

CREATE INDEX package_actor_jobs ON media.package_job USING btree (org_id, actor_id, created_at DESC, id);

CREATE UNIQUE INDEX package_library_active ON media.package_job USING btree (library_id) WHERE (status = 'needs_reconciliation'::text);

CREATE INDEX package_project_work ON media.package_job USING btree (project_id, status);

CREATE INDEX purge_actor_jobs ON media.purge_job USING btree (org_id, actor_id, created_at DESC, id);

CREATE INDEX purge_project_work ON media.purge_job USING btree (project_id, status);

CREATE UNIQUE INDEX purge_reserved_item ON media.purge_item USING btree (item_id) WHERE (status = ANY (ARRAY['queued'::text, 'running'::text, 'needs_reconciliation'::text, 'succeeded'::text]));

CREATE INDEX transfer_actor_jobs ON media.transfer_job USING btree (org_id, actor_id, created_at DESC, id);

CREATE INDEX transfer_source_work ON media.transfer_job USING btree (source_project_id, status);

CREATE INDEX transfer_target_work ON media.transfer_job USING btree (target_project_id, status);

CREATE INDEX depth_job_project_source ON mediatool.depth_job USING btree (project_id, canvas_id, node_id, id DESC);

CREATE INDEX export_job_project_source ON mediatool.export_job USING btree (project_id, canvas_id, node_id, id DESC);

CREATE INDEX transcription_job_project_source ON mediatool.transcription_job USING btree (project_id, canvas_id, node_id, id DESC);

CREATE INDEX ix_batch_launch_active_project ON operation.batch_launch USING btree (project_id) WHERE (state = 'active'::text);

CREATE INDEX ix_batch_launch_active_provider ON operation.batch_launch USING btree (provider_id) WHERE ((state = 'active'::text) AND (provider_id IS NOT NULL));

CREATE INDEX ix_batch_quoted ON operation.batch USING btree (create_time, id) WHERE ((status = 'quoted'::text) AND (NOT is_delete));

CREATE INDEX ix_batch_starter_confirmed ON operation.batch USING btree (update_time, id) WHERE ((status = 'confirmed'::text) AND (NOT is_delete));

CREATE INDEX ix_operation_batch_active ON operation.operation USING btree (batch_id, status) WHERE ((batch_id IS NOT NULL) AND (NOT is_delete));

CREATE INDEX ix_operation_canvas_source ON operation.operation USING btree (((source_context ->> 'canvas_id'::text)), ((source_context ->> 'node_id'::text)), create_time DESC) WHERE ((source_context IS NOT NULL) AND (NOT is_delete));

CREATE INDEX ix_operation_due_quote ON operation.operation USING btree (quote_expires_at, id) WHERE ((status = 'quoted'::text) AND (NOT is_delete));

CREATE INDEX ix_operation_event_op ON operation.operation_event USING btree (operation_id, create_time);

CREATE INDEX ix_operation_inflight ON operation.operation USING btree (status, started_at) WHERE (status = ANY (ARRAY['submitting'::text, 'submitted'::text, 'unknown'::text, 'reconciling'::text, 'manual'::text, 'ingesting'::text]));

CREATE INDEX ix_operation_live_batch_status ON operation.operation USING btree (batch_id, status) WHERE ((batch_id IS NOT NULL) AND (NOT is_delete));

CREATE INDEX ix_operation_output_project_media ON operation.operation_output USING btree (project_id, media_asset_id) WHERE ((media_asset_id IS NOT NULL) AND (NOT is_delete));

CREATE INDEX ix_operation_project_status ON operation.operation USING btree (project_id, status, create_time DESC);

CREATE INDEX ix_operation_reuse ON operation.operation USING btree (project_id, input_hash) WHERE (status = 'completed'::text);

CREATE INDEX ix_operation_target ON operation.operation USING btree (target_type, target_id, create_time DESC);

CREATE INDEX ix_provider_call_operation ON ONLY operation.provider_call USING btree (operation_id, create_time);

CREATE UNIQUE INDEX script_episode_active_seq ON script.episode USING btree (split_set_id, seq_no) WHERE (NOT is_delete);

CREATE INDEX script_source_project ON script.script_source USING btree (org_id, project_id, created_at, id);

CREATE INDEX script_version_content ON script.script_version USING btree (project_id, content_hash);

CREATE INDEX script_version_document ON script.script_version USING btree (project_id, document_sha256);

CREATE INDEX ix_project_copy_org_recent ON workspace.project_copy_job USING btree (org_id, create_time DESC, id DESC);

CREATE INDEX ix_project_copy_source_active ON workspace.project_copy_job USING btree (source_project_id, status) WHERE (status <> ALL (ARRAY['succeeded'::text, 'cancelled'::text]));

CREATE INDEX ix_project_folder_actor_recent ON workspace.project_folder USING btree (org_id, actor_id, update_time DESC, id DESC) WHERE (NOT is_delete);

CREATE INDEX ix_project_folder_members ON workspace.project_folder_placement USING btree (org_id, actor_id, folder_id, project_id);

CREATE INDEX ix_project_org_status ON workspace.project USING btree (org_id, status, update_time DESC) WHERE (NOT is_delete);

-- 触发器
CREATE TRIGGER audit_log_append_only BEFORE DELETE OR UPDATE ON audit.audit_log FOR EACH ROW EXECUTE FUNCTION audit.reject_audit_mutation();

CREATE TRIGGER trg_budget_update_time BEFORE UPDATE ON billing.budget FOR EACH ROW EXECUTE FUNCTION billing.set_budget_update_time();

CREATE TRIGGER trg_ledger_entry_append_only BEFORE DELETE OR UPDATE ON billing.ledger_entry FOR EACH ROW EXECUTE FUNCTION billing.keep_ledger_entry_append_only();

CREATE TRIGGER library_command_immutable BEFORE DELETE OR UPDATE ON media.library_command FOR EACH ROW EXECUTE FUNCTION media.reject_library_command_mutation();

CREATE TRIGGER library_item_scope BEFORE INSERT OR UPDATE ON media.library_item FOR EACH ROW EXECUTE FUNCTION media.check_library_asset_scope();

CREATE TRIGGER package_command_immutable BEFORE DELETE OR UPDATE ON media.package_command FOR EACH ROW EXECUTE FUNCTION media.reject_package_command_mutation();

CREATE TRIGGER package_frozen_immutable BEFORE UPDATE ON media.package_job FOR EACH ROW EXECUTE FUNCTION media.reject_package_frozen_mutation();

CREATE TRIGGER package_scope BEFORE INSERT ON media.package_job FOR EACH ROW EXECUTE FUNCTION media.check_package_scope();

CREATE TRIGGER purge_command_immutable BEFORE DELETE OR UPDATE ON media.purge_command FOR EACH ROW EXECUTE FUNCTION media.reject_purge_command_mutation();

CREATE TRIGGER purge_scope BEFORE INSERT ON media.purge_job FOR EACH ROW EXECUTE FUNCTION media.check_purge_scope();

CREATE TRIGGER transfer_command_immutable BEFORE DELETE OR UPDATE ON media.transfer_command FOR EACH ROW EXECUTE FUNCTION media.reject_transfer_command_mutation();

CREATE TRIGGER transfer_scope BEFORE INSERT ON media.transfer_job FOR EACH ROW EXECUTE FUNCTION media.check_transfer_scope();

CREATE TRIGGER upload_request_scope_guard BEFORE INSERT ON media.upload_request FOR EACH ROW EXECUTE FUNCTION media.check_upload_receipt_scope();

CREATE TRIGGER project_change_command_immutable BEFORE DELETE OR UPDATE ON workspace.project_change_command FOR EACH ROW EXECUTE FUNCTION workspace.reject_project_change_command_mutation();

CREATE TRIGGER project_folder_command_immutable BEFORE DELETE OR UPDATE ON workspace.project_folder_command FOR EACH ROW EXECUTE FUNCTION workspace.reject_project_folder_command_mutation();

CREATE TRIGGER trg_project_protect_update BEFORE UPDATE ON workspace.project FOR EACH ROW EXECUTE FUNCTION workspace.protect_project_update();

CREATE TRIGGER trg_style_preset_update_time BEFORE UPDATE ON workspace.style_preset FOR EACH ROW EXECUTE FUNCTION workspace.set_style_preset_update_time();

ALTER TABLE ONLY bible.character_confirmation
    ADD CONSTRAINT character_confirmation_org_id_project_id_entry_id_fkey FOREIGN KEY (org_id, project_id, entry_id) REFERENCES bible."character"(org_id, project_id, id);

ALTER TABLE ONLY bible.character_confirmation
    ADD CONSTRAINT character_confirmation_org_id_project_id_version_id_fkey FOREIGN KEY (org_id, project_id, version_id) REFERENCES bible.character_version(org_id, project_id, id);

ALTER TABLE ONLY bible."character"
    ADD CONSTRAINT character_confirmed_version_fk FOREIGN KEY (org_id, project_id, confirmed_version_id) REFERENCES bible.character_version(org_id, project_id, id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY bible."character"
    ADD CONSTRAINT character_current_version_fk FOREIGN KEY (org_id, project_id, current_version_id) REFERENCES bible.character_version(org_id, project_id, id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY bible."character"
    ADD CONSTRAINT character_redirect_fk FOREIGN KEY (org_id, project_id, redirect_id) REFERENCES bible."character"(org_id, project_id, id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY bible.character_redirect
    ADD CONSTRAINT character_redirect_org_id_project_id_source_id_fkey FOREIGN KEY (org_id, project_id, source_id) REFERENCES bible."character"(org_id, project_id, id);

ALTER TABLE ONLY bible.character_redirect
    ADD CONSTRAINT character_redirect_org_id_project_id_source_version_id_fkey FOREIGN KEY (org_id, project_id, source_version_id) REFERENCES bible.character_version(org_id, project_id, id);

ALTER TABLE ONLY bible.character_redirect
    ADD CONSTRAINT character_redirect_org_id_project_id_target_id_fkey FOREIGN KEY (org_id, project_id, target_id) REFERENCES bible."character"(org_id, project_id, id);

ALTER TABLE ONLY bible.character_redirect
    ADD CONSTRAINT character_redirect_org_id_project_id_target_version_id_fkey FOREIGN KEY (org_id, project_id, target_version_id) REFERENCES bible.character_version(org_id, project_id, id);

ALTER TABLE ONLY bible.character_split
    ADD CONSTRAINT character_split_org_id_project_id_source_id_fkey FOREIGN KEY (org_id, project_id, source_id) REFERENCES bible."character"(org_id, project_id, id);

ALTER TABLE ONLY bible.character_split
    ADD CONSTRAINT character_split_org_id_project_id_source_version_id_fkey FOREIGN KEY (org_id, project_id, source_version_id) REFERENCES bible.character_version(org_id, project_id, id);

ALTER TABLE ONLY bible.character_split
    ADD CONSTRAINT character_split_org_id_project_id_target_id_fkey FOREIGN KEY (org_id, project_id, target_id) REFERENCES bible."character"(org_id, project_id, id);

ALTER TABLE ONLY bible.character_split
    ADD CONSTRAINT character_split_org_id_project_id_target_version_id_fkey FOREIGN KEY (org_id, project_id, target_version_id) REFERENCES bible.character_version(org_id, project_id, id);

ALTER TABLE ONLY bible.character_version
    ADD CONSTRAINT character_version_org_id_project_id_entry_id_fkey FOREIGN KEY (org_id, project_id, entry_id) REFERENCES bible."character"(org_id, project_id, id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY bible.character_version
    ADD CONSTRAINT character_version_org_id_project_id_previous_id_fkey FOREIGN KEY (org_id, project_id, previous_id) REFERENCES bible.character_version(org_id, project_id, id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY bible.copy_cleanup_receipt
    ADD CONSTRAINT copy_cleanup_receipt_snapshot_id_fkey FOREIGN KEY (snapshot_id) REFERENCES bible.copy_snapshot(id);

ALTER TABLE ONLY bible.copy_receipt
    ADD CONSTRAINT copy_receipt_snapshot_id_fkey FOREIGN KEY (snapshot_id) REFERENCES bible.copy_snapshot(id);

ALTER TABLE ONLY bible.copy_transfer_receipt
    ADD CONSTRAINT copy_transfer_receipt_snapshot_id_fkey FOREIGN KEY (snapshot_id) REFERENCES bible.copy_snapshot(id);

ALTER TABLE ONLY bible.location_confirmation
    ADD CONSTRAINT location_confirmation_org_id_project_id_entry_id_fkey FOREIGN KEY (org_id, project_id, entry_id) REFERENCES bible.location(org_id, project_id, id);

ALTER TABLE ONLY bible.location_confirmation
    ADD CONSTRAINT location_confirmation_org_id_project_id_version_id_fkey FOREIGN KEY (org_id, project_id, version_id) REFERENCES bible.location_version(org_id, project_id, id);

ALTER TABLE ONLY bible.location
    ADD CONSTRAINT location_confirmed_version_fk FOREIGN KEY (org_id, project_id, confirmed_version_id) REFERENCES bible.location_version(org_id, project_id, id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY bible.location
    ADD CONSTRAINT location_current_version_fk FOREIGN KEY (org_id, project_id, current_version_id) REFERENCES bible.location_version(org_id, project_id, id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY bible.location_version
    ADD CONSTRAINT location_version_org_id_project_id_entry_id_fkey FOREIGN KEY (org_id, project_id, entry_id) REFERENCES bible.location(org_id, project_id, id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY bible.location_version
    ADD CONSTRAINT location_version_org_id_project_id_previous_id_fkey FOREIGN KEY (org_id, project_id, previous_id) REFERENCES bible.location_version(org_id, project_id, id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY bible.look
    ADD CONSTRAINT look_org_id_project_id_character_id_fkey FOREIGN KEY (org_id, project_id, character_id) REFERENCES bible."character"(org_id, project_id, id);

ALTER TABLE ONLY bible.look_version
    ADD CONSTRAINT look_version_org_id_project_id_character_version_id_fkey FOREIGN KEY (org_id, project_id, character_version_id) REFERENCES bible.character_version(org_id, project_id, id);

ALTER TABLE ONLY bible.look_version
    ADD CONSTRAINT look_version_org_id_project_id_look_id_fkey FOREIGN KEY (org_id, project_id, look_id) REFERENCES bible.look(org_id, project_id, id);

ALTER TABLE ONLY bible.prop_confirmation
    ADD CONSTRAINT prop_confirmation_org_id_project_id_entry_id_fkey FOREIGN KEY (org_id, project_id, entry_id) REFERENCES bible.prop(org_id, project_id, id);

ALTER TABLE ONLY bible.prop_confirmation
    ADD CONSTRAINT prop_confirmation_org_id_project_id_version_id_fkey FOREIGN KEY (org_id, project_id, version_id) REFERENCES bible.prop_version(org_id, project_id, id);

ALTER TABLE ONLY bible.prop
    ADD CONSTRAINT prop_confirmed_version_fk FOREIGN KEY (org_id, project_id, confirmed_version_id) REFERENCES bible.prop_version(org_id, project_id, id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY bible.prop
    ADD CONSTRAINT prop_current_version_fk FOREIGN KEY (org_id, project_id, current_version_id) REFERENCES bible.prop_version(org_id, project_id, id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY bible.prop_version
    ADD CONSTRAINT prop_version_org_id_project_id_entry_id_fkey FOREIGN KEY (org_id, project_id, entry_id) REFERENCES bible.prop(org_id, project_id, id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY bible.prop_version
    ADD CONSTRAINT prop_version_org_id_project_id_previous_id_fkey FOREIGN KEY (org_id, project_id, previous_id) REFERENCES bible.prop_version(org_id, project_id, id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY bible.reference_version
    ADD CONSTRAINT reference_version_org_id_project_id_look_version_id_fkey FOREIGN KEY (org_id, project_id, look_version_id) REFERENCES bible.look_version(org_id, project_id, id);

ALTER TABLE ONLY bible.voice_version
    ADD CONSTRAINT voice_version_org_id_project_id_character_version_id_fkey FOREIGN KEY (org_id, project_id, character_version_id) REFERENCES bible.character_version(org_id, project_id, id);

ALTER TABLE ONLY billing.reservation
    ADD CONSTRAINT fk_reservation_operation_project FOREIGN KEY (project_id, operation_id) REFERENCES operation.operation(project_id, id);

ALTER TABLE ONLY billing.reservation
    ADD CONSTRAINT reservation_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY canvas.command_log
    ADD CONSTRAINT command_log_document_id_fkey FOREIGN KEY (document_id) REFERENCES canvas.document(id);

ALTER TABLE ONLY canvas.document
    ADD CONSTRAINT document_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY canvas.edge
    ADD CONSTRAINT edge_document_id_fkey FOREIGN KEY (document_id) REFERENCES canvas.document(id);

ALTER TABLE ONLY canvas.edge
    ADD CONSTRAINT edge_document_id_source_node_id_fkey FOREIGN KEY (document_id, source_node_id) REFERENCES canvas.node(document_id, id);

ALTER TABLE ONLY canvas.edge
    ADD CONSTRAINT edge_document_id_target_node_id_fkey FOREIGN KEY (document_id, target_node_id) REFERENCES canvas.node(document_id, id);

ALTER TABLE ONLY canvas.node
    ADD CONSTRAINT fk_canvas_node_parent FOREIGN KEY (document_id, parent_id) REFERENCES canvas.node(document_id, id) DEFERRABLE INITIALLY DEFERRED NOT VALID;

ALTER TABLE ONLY canvas.node
    ADD CONSTRAINT node_document_id_fkey FOREIGN KEY (document_id) REFERENCES canvas.document(id);

ALTER TABLE ONLY canvas.project_copy_receipt
    ADD CONSTRAINT project_copy_receipt_snapshot_id_fkey FOREIGN KEY (snapshot_id) REFERENCES canvas.project_copy_snapshot(id);

ALTER TABLE ONLY canvas.project_copy_snapshot
    ADD CONSTRAINT project_copy_snapshot_job_id_fkey FOREIGN KEY (job_id) REFERENCES workspace.project_copy_job(id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY canvas.project_copy_snapshot
    ADD CONSTRAINT project_copy_snapshot_org_id_fkey FOREIGN KEY (org_id) REFERENCES workspace.organization(id);

ALTER TABLE ONLY canvas.project_copy_snapshot
    ADD CONSTRAINT project_copy_snapshot_source_project_id_fkey FOREIGN KEY (source_project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY canvas.project_copy_snapshot
    ADD CONSTRAINT project_copy_snapshot_target_project_id_fkey FOREIGN KEY (target_project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY catalog.model_profile
    ADD CONSTRAINT fk_model_profile_current_version FOREIGN KEY (id, current_version_id) REFERENCES catalog.model_profile_version(model_profile_id, id);

ALTER TABLE ONLY catalog.model_profile
    ADD CONSTRAINT model_profile_capability_fkey FOREIGN KEY (capability) REFERENCES catalog.capability(key);

ALTER TABLE ONLY catalog.model_profile
    ADD CONSTRAINT model_profile_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES catalog.provider(id);

ALTER TABLE ONLY catalog.model_profile_version
    ADD CONSTRAINT model_profile_version_model_profile_id_fkey FOREIGN KEY (model_profile_id) REFERENCES catalog.model_profile(id);

ALTER TABLE ONLY catalog.price_rule_version
    ADD CONSTRAINT price_rule_version_model_profile_id_fkey FOREIGN KEY (model_profile_id) REFERENCES catalog.model_profile(id);

ALTER TABLE ONLY catalog.provider_credential
    ADD CONSTRAINT provider_credential_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES catalog.provider(id);

ALTER TABLE ONLY media.media_asset
    ADD CONSTRAINT fk_media_source_operation_project FOREIGN KEY (project_id, source_operation_id) REFERENCES operation.operation(project_id, id);

ALTER TABLE ONLY media.library_command
    ADD CONSTRAINT library_command_library_id_fkey FOREIGN KEY (library_id) REFERENCES media.library(id);

ALTER TABLE ONLY media.library_command
    ADD CONSTRAINT library_command_org_id_actor_id_fkey FOREIGN KEY (org_id, actor_id) REFERENCES identity."user"(org_id, id);

ALTER TABLE ONLY media.library_folder
    ADD CONSTRAINT library_folder_library_id_library_kind_fkey FOREIGN KEY (library_id, library_kind) REFERENCES media.library(id, kind);

ALTER TABLE ONLY media.library_folder
    ADD CONSTRAINT library_folder_library_id_parent_id_fkey FOREIGN KEY (library_id, parent_id) REFERENCES media.library_folder(library_id, id);

ALTER TABLE ONLY media.library_item
    ADD CONSTRAINT library_item_asset_id_fkey FOREIGN KEY (asset_id) REFERENCES media.media_asset(id);

ALTER TABLE ONLY media.library_item
    ADD CONSTRAINT library_item_library_id_fkey FOREIGN KEY (library_id) REFERENCES media.library(id);

ALTER TABLE ONLY media.library_item
    ADD CONSTRAINT library_item_library_id_folder_id_fkey FOREIGN KEY (library_id, folder_id) REFERENCES media.library_folder(library_id, id);

ALTER TABLE ONLY media.library
    ADD CONSTRAINT library_org_id_fkey FOREIGN KEY (org_id) REFERENCES workspace.organization(id);

ALTER TABLE ONLY media.library
    ADD CONSTRAINT library_org_id_personal_actor_id_fkey FOREIGN KEY (org_id, personal_actor_id) REFERENCES identity."user"(org_id, id);

ALTER TABLE ONLY media.library
    ADD CONSTRAINT library_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY media.media_asset
    ADD CONSTRAINT media_asset_personal_owner_fk FOREIGN KEY (personal_org_id, personal_actor_id) REFERENCES identity."user"(org_id, id);

ALTER TABLE ONLY media.media_asset
    ADD CONSTRAINT media_asset_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY media.package_command
    ADD CONSTRAINT package_command_job_id_fkey FOREIGN KEY (job_id) REFERENCES media.package_job(id);

ALTER TABLE ONLY media.package_command
    ADD CONSTRAINT package_command_org_id_actor_id_fkey FOREIGN KEY (org_id, actor_id) REFERENCES identity."user"(org_id, id);

ALTER TABLE ONLY media.package_job
    ADD CONSTRAINT package_job_library_id_fkey FOREIGN KEY (library_id) REFERENCES media.library(id);

ALTER TABLE ONLY media.package_job
    ADD CONSTRAINT package_job_org_id_actor_id_fkey FOREIGN KEY (org_id, actor_id) REFERENCES identity."user"(org_id, id);

ALTER TABLE ONLY media.package_job
    ADD CONSTRAINT package_job_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY media.package_object
    ADD CONSTRAINT package_object_job_id_fkey FOREIGN KEY (job_id) REFERENCES media.package_job(id);

ALTER TABLE ONLY media.project_copy_object
    ADD CONSTRAINT project_copy_object_snapshot_id_fkey FOREIGN KEY (snapshot_id) REFERENCES media.project_copy_snapshot(id);

ALTER TABLE ONLY media.project_copy_receipt
    ADD CONSTRAINT project_copy_receipt_snapshot_id_fkey FOREIGN KEY (snapshot_id) REFERENCES media.project_copy_snapshot(id);

ALTER TABLE ONLY media.project_copy_snapshot
    ADD CONSTRAINT project_copy_snapshot_job_id_fkey FOREIGN KEY (job_id) REFERENCES workspace.project_copy_job(id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY media.project_copy_snapshot
    ADD CONSTRAINT project_copy_snapshot_org_id_fkey FOREIGN KEY (org_id) REFERENCES workspace.organization(id);

ALTER TABLE ONLY media.project_copy_snapshot
    ADD CONSTRAINT project_copy_snapshot_source_project_id_fkey FOREIGN KEY (source_project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY media.project_copy_snapshot
    ADD CONSTRAINT project_copy_snapshot_target_project_id_fkey FOREIGN KEY (target_project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY media.purge_command
    ADD CONSTRAINT purge_command_job_id_fkey FOREIGN KEY (job_id) REFERENCES media.purge_job(id);

ALTER TABLE ONLY media.purge_command
    ADD CONSTRAINT purge_command_org_id_actor_id_fkey FOREIGN KEY (org_id, actor_id) REFERENCES identity."user"(org_id, id);

ALTER TABLE ONLY media.purge_item
    ADD CONSTRAINT purge_item_asset_id_fkey FOREIGN KEY (asset_id) REFERENCES media.media_asset(id);

ALTER TABLE ONLY media.purge_item
    ADD CONSTRAINT purge_item_item_id_fkey FOREIGN KEY (item_id) REFERENCES media.library_item(id);

ALTER TABLE ONLY media.purge_item
    ADD CONSTRAINT purge_item_job_id_fkey FOREIGN KEY (job_id) REFERENCES media.purge_job(id);

ALTER TABLE ONLY media.purge_job
    ADD CONSTRAINT purge_job_library_id_fkey FOREIGN KEY (library_id) REFERENCES media.library(id);

ALTER TABLE ONLY media.purge_job
    ADD CONSTRAINT purge_job_org_id_actor_id_fkey FOREIGN KEY (org_id, actor_id) REFERENCES identity."user"(org_id, id);

ALTER TABLE ONLY media.purge_job
    ADD CONSTRAINT purge_job_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY media.purge_object
    ADD CONSTRAINT purge_object_job_id_item_index_fkey FOREIGN KEY (job_id, item_index) REFERENCES media.purge_item(job_id, item_index);

ALTER TABLE ONLY media.rendition
    ADD CONSTRAINT rendition_media_asset_id_fkey FOREIGN KEY (media_asset_id) REFERENCES media.media_asset(id);

ALTER TABLE ONLY media.transfer_command
    ADD CONSTRAINT transfer_command_job_id_fkey FOREIGN KEY (job_id) REFERENCES media.transfer_job(id);

ALTER TABLE ONLY media.transfer_command
    ADD CONSTRAINT transfer_command_org_id_actor_id_fkey FOREIGN KEY (org_id, actor_id) REFERENCES identity."user"(org_id, id);

ALTER TABLE ONLY media.transfer_item
    ADD CONSTRAINT transfer_item_job_id_fkey FOREIGN KEY (job_id) REFERENCES media.transfer_job(id);

ALTER TABLE ONLY media.transfer_item
    ADD CONSTRAINT transfer_item_source_asset_id_fkey FOREIGN KEY (source_asset_id) REFERENCES media.media_asset(id);

ALTER TABLE ONLY media.transfer_job
    ADD CONSTRAINT transfer_job_org_id_actor_id_fkey FOREIGN KEY (org_id, actor_id) REFERENCES identity."user"(org_id, id);

ALTER TABLE ONLY media.transfer_job
    ADD CONSTRAINT transfer_job_source_library_id_fkey FOREIGN KEY (source_library_id) REFERENCES media.library(id);

ALTER TABLE ONLY media.transfer_job
    ADD CONSTRAINT transfer_job_source_project_id_fkey FOREIGN KEY (source_project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY media.transfer_job
    ADD CONSTRAINT transfer_job_target_library_id_fkey FOREIGN KEY (target_library_id) REFERENCES media.library(id);

ALTER TABLE ONLY media.transfer_job
    ADD CONSTRAINT transfer_job_target_project_id_fkey FOREIGN KEY (target_project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY media.transfer_object
    ADD CONSTRAINT transfer_object_job_id_item_index_fkey FOREIGN KEY (job_id, item_index) REFERENCES media.transfer_item(job_id, item_index);

ALTER TABLE ONLY media.upload_request
    ADD CONSTRAINT upload_request_asset_id_fkey FOREIGN KEY (asset_id) REFERENCES media.media_asset(id);

ALTER TABLE ONLY media.upload_request
    ADD CONSTRAINT upload_request_personal_actor_fk FOREIGN KEY (personal_org_id, principal_id) REFERENCES identity."user"(org_id, id);

ALTER TABLE ONLY media.upload_request
    ADD CONSTRAINT upload_request_principal_id_fkey FOREIGN KEY (principal_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY media.upload_request
    ADD CONSTRAINT upload_request_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY mediatool.depth_command
    ADD CONSTRAINT depth_command_job_id_fkey FOREIGN KEY (job_id) REFERENCES mediatool.depth_job(id);

ALTER TABLE ONLY mediatool.depth_job
    ADD CONSTRAINT depth_job_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY mediatool.depth_object
    ADD CONSTRAINT depth_object_job_id_attempt_fkey FOREIGN KEY (job_id, attempt) REFERENCES mediatool.depth_result_intent(job_id, attempt);

ALTER TABLE ONLY mediatool.depth_result_intent
    ADD CONSTRAINT depth_result_intent_job_id_fkey FOREIGN KEY (job_id) REFERENCES mediatool.depth_job(id);

ALTER TABLE ONLY mediatool.export_command
    ADD CONSTRAINT export_command_job_id_fkey FOREIGN KEY (job_id) REFERENCES mediatool.export_job(id);

ALTER TABLE ONLY mediatool.export_job
    ADD CONSTRAINT export_job_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY mediatool.transcription_command
    ADD CONSTRAINT transcription_command_job_id_fkey FOREIGN KEY (job_id) REFERENCES mediatool.transcription_job(id);

ALTER TABLE ONLY mediatool.transcription_job
    ADD CONSTRAINT transcription_job_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY operation.batch_launch
    ADD CONSTRAINT batch_launch_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES operation.operation(id);

ALTER TABLE ONLY operation.batch_launch
    ADD CONSTRAINT batch_launch_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES catalog.provider(id);

ALTER TABLE ONLY operation.batch
    ADD CONSTRAINT batch_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY operation.batch_launch
    ADD CONSTRAINT fk_batch_launch_batch_project FOREIGN KEY (project_id, batch_id) REFERENCES operation.batch(project_id, id);

ALTER TABLE ONLY operation.operation
    ADD CONSTRAINT fk_operation_batch_project FOREIGN KEY (project_id, batch_id) REFERENCES operation.batch(project_id, id);

ALTER TABLE ONLY operation.operation_output
    ADD CONSTRAINT fk_operation_output_media_project FOREIGN KEY (project_id, media_asset_id) REFERENCES media.media_asset(project_id, id);

ALTER TABLE ONLY operation.operation_output
    ADD CONSTRAINT fk_operation_output_operation_project FOREIGN KEY (project_id, operation_id) REFERENCES operation.operation(project_id, id);

ALTER TABLE ONLY operation.operation
    ADD CONSTRAINT fk_operation_reservation_identity FOREIGN KEY (project_id, id, reservation_id) REFERENCES billing.reservation(project_id, operation_id, id);

ALTER TABLE ONLY operation.operation
    ADD CONSTRAINT fk_operation_reused_project FOREIGN KEY (project_id, reused_from_id) REFERENCES operation.operation(project_id, id);

ALTER TABLE operation.provider_call
    ADD CONSTRAINT fk_provider_call_operation_project FOREIGN KEY (project_id, operation_id) REFERENCES operation.operation(project_id, id);

ALTER TABLE ONLY operation.operation_event
    ADD CONSTRAINT operation_event_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES operation.operation(id);

ALTER TABLE ONLY operation.operation_input
    ADD CONSTRAINT operation_input_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES operation.operation(id);

ALTER TABLE ONLY operation.operation
    ADD CONSTRAINT operation_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY script.action_line
    ADD CONSTRAINT action_line_org_id_project_id_scene_id_fkey FOREIGN KEY (org_id, project_id, scene_id) REFERENCES script.scene(org_id, project_id, id);

ALTER TABLE ONLY script.command
    ADD CONSTRAINT command_actor_id_request_id_fkey FOREIGN KEY (actor_id, request_id) REFERENCES script.request(actor_id, request_id);

ALTER TABLE ONLY script.command_result
    ADD CONSTRAINT command_result_actor_id_request_id_fkey FOREIGN KEY (actor_id, request_id) REFERENCES script.command(actor_id, request_id);

ALTER TABLE ONLY script.command_state
    ADD CONSTRAINT command_state_actor_id_request_id_fkey FOREIGN KEY (actor_id, request_id) REFERENCES script.command(actor_id, request_id);

ALTER TABLE ONLY script.copy_object_intent
    ADD CONSTRAINT copy_object_intent_snapshot_id_fkey FOREIGN KEY (snapshot_id) REFERENCES script.copy_snapshot(id);

ALTER TABLE ONLY script.copy_object_state
    ADD CONSTRAINT copy_object_state_target_key_fkey FOREIGN KEY (target_key) REFERENCES script.copy_object_intent(target_key);

ALTER TABLE ONLY script.copy_receipt
    ADD CONSTRAINT copy_receipt_snapshot_id_fkey FOREIGN KEY (snapshot_id) REFERENCES script.copy_snapshot(id);

ALTER TABLE ONLY script.dialogue_line
    ADD CONSTRAINT dialogue_character_version_fk FOREIGN KEY (org_id, project_id, character_version_id) REFERENCES bible.character_version(org_id, project_id, id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY script.dialogue_line
    ADD CONSTRAINT dialogue_line_org_id_project_id_scene_id_fkey FOREIGN KEY (org_id, project_id, scene_id) REFERENCES script.scene(org_id, project_id, id);

ALTER TABLE ONLY script.episode
    ADD CONSTRAINT episode_org_id_project_id_confirmed_structure_id_fkey FOREIGN KEY (org_id, project_id, confirmed_structure_id) REFERENCES script.episode_structure(org_id, project_id, id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY script.episode
    ADD CONSTRAINT episode_org_id_project_id_current_structure_id_fkey FOREIGN KEY (org_id, project_id, current_structure_id) REFERENCES script.episode_structure(org_id, project_id, id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY script.episode
    ADD CONSTRAINT episode_org_id_project_id_previous_episode_id_fkey FOREIGN KEY (org_id, project_id, previous_episode_id) REFERENCES script.episode(org_id, project_id, id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY script.episode
    ADD CONSTRAINT episode_org_id_project_id_script_version_id_fkey FOREIGN KEY (org_id, project_id, script_version_id) REFERENCES script.script_version(org_id, project_id, id);

ALTER TABLE ONLY script.episode
    ADD CONSTRAINT episode_org_id_project_id_split_set_id_fkey FOREIGN KEY (org_id, project_id, split_set_id) REFERENCES script.split_set(org_id, project_id, id);

ALTER TABLE ONLY script.episode_structure
    ADD CONSTRAINT episode_structure_org_id_project_id_episode_id_fkey FOREIGN KEY (org_id, project_id, episode_id) REFERENCES script.episode(org_id, project_id, id);

ALTER TABLE ONLY script.import_attempt
    ADD CONSTRAINT import_attempt_job_id_fkey FOREIGN KEY (job_id) REFERENCES script.import_job(id);

ALTER TABLE ONLY script.import_command
    ADD CONSTRAINT import_command_actor_id_request_id_fkey FOREIGN KEY (actor_id, request_id) REFERENCES script.request(actor_id, request_id);

ALTER TABLE ONLY script.import_command
    ADD CONSTRAINT import_command_job_id_fkey FOREIGN KEY (job_id) REFERENCES script.import_job(id);

ALTER TABLE ONLY script.import_file
    ADD CONSTRAINT import_file_job_id_fkey FOREIGN KEY (job_id) REFERENCES script.import_job(id);

ALTER TABLE ONLY script.import_file_result
    ADD CONSTRAINT import_file_result_job_id_attempt_fkey FOREIGN KEY (job_id, attempt) REFERENCES script.import_attempt(job_id, attempt);

ALTER TABLE ONLY script.import_file_result
    ADD CONSTRAINT import_file_result_job_id_position_fkey FOREIGN KEY (job_id, "position") REFERENCES script.import_file(job_id, "position");

ALTER TABLE ONLY script.import_job
    ADD CONSTRAINT import_job_actor_id_request_id_fkey FOREIGN KEY (actor_id, request_id) REFERENCES script.request(actor_id, request_id);

ALTER TABLE ONLY script.import_job
    ADD CONSTRAINT import_job_org_id_project_id_base_version_id_fkey FOREIGN KEY (org_id, project_id, base_version_id) REFERENCES script.script_version(org_id, project_id, id);

ALTER TABLE ONLY script.import_publication
    ADD CONSTRAINT import_publication_job_id_attempt_fkey FOREIGN KEY (job_id, attempt) REFERENCES script.import_attempt(job_id, attempt);

ALTER TABLE ONLY script.import_state
    ADD CONSTRAINT import_state_job_id_fkey FOREIGN KEY (job_id) REFERENCES script.import_job(id);

ALTER TABLE ONLY script.object_intent
    ADD CONSTRAINT object_intent_actor_id_request_id_fkey FOREIGN KEY (actor_id, request_id) REFERENCES script.command(actor_id, request_id);

ALTER TABLE ONLY script.object_state
    ADD CONSTRAINT object_state_object_key_fkey FOREIGN KEY (object_key) REFERENCES script.object_intent(object_key);

ALTER TABLE ONLY script.project_state
    ADD CONSTRAINT project_state_org_id_project_id_adopted_version_id_fkey FOREIGN KEY (org_id, project_id, adopted_version_id) REFERENCES script.script_version(org_id, project_id, id);

ALTER TABLE ONLY script.project_state
    ADD CONSTRAINT project_state_org_id_project_id_draft_version_id_fkey FOREIGN KEY (org_id, project_id, draft_version_id) REFERENCES script.script_version(org_id, project_id, id);

ALTER TABLE ONLY script.review_command
    ADD CONSTRAINT review_command_actor_id_request_id_fkey FOREIGN KEY (actor_id, request_id) REFERENCES script.request(actor_id, request_id);

ALTER TABLE ONLY script.scene
    ADD CONSTRAINT scene_org_id_project_id_episode_structure_id_fkey FOREIGN KEY (org_id, project_id, episode_structure_id) REFERENCES script.episode_structure(org_id, project_id, id);

ALTER TABLE ONLY script.script_source
    ADD CONSTRAINT script_source_org_id_project_id_previous_source_id_fkey FOREIGN KEY (org_id, project_id, previous_source_id) REFERENCES script.script_source(org_id, project_id, id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY script.script_source
    ADD CONSTRAINT script_source_org_id_project_id_source_lineage_id_fkey FOREIGN KEY (org_id, project_id, source_lineage_id) REFERENCES script.script_source(org_id, project_id, id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY script.source_control
    ADD CONSTRAINT source_control_actor_id_request_id_fkey FOREIGN KEY (actor_id, request_id) REFERENCES script.request(actor_id, request_id);

ALTER TABLE ONLY script.source_control
    ADD CONSTRAINT source_control_intent_id_fkey FOREIGN KEY (intent_id) REFERENCES script.command_state(id);

ALTER TABLE ONLY script.split_confirmation
    ADD CONSTRAINT split_confirmation_org_id_project_id_candidate_set_id_fkey FOREIGN KEY (org_id, project_id, candidate_set_id) REFERENCES script.split_set(org_id, project_id, id);

ALTER TABLE ONLY script.split_confirmation
    ADD CONSTRAINT split_confirmation_org_id_project_id_formal_set_id_fkey FOREIGN KEY (org_id, project_id, formal_set_id) REFERENCES script.split_set(org_id, project_id, id);

ALTER TABLE ONLY script.split_confirmation
    ADD CONSTRAINT split_confirmation_org_id_project_id_version_id_fkey FOREIGN KEY (org_id, project_id, version_id) REFERENCES script.script_version(org_id, project_id, id);

ALTER TABLE ONLY script.split_set
    ADD CONSTRAINT split_set_org_id_project_id_version_id_fkey FOREIGN KEY (org_id, project_id, version_id) REFERENCES script.script_version(org_id, project_id, id);

ALTER TABLE ONLY script.version_head
    ADD CONSTRAINT version_head_org_id_project_id_candidate_split_set_id_fkey FOREIGN KEY (org_id, project_id, candidate_split_set_id) REFERENCES script.split_set(org_id, project_id, id);

ALTER TABLE ONLY script.version_head
    ADD CONSTRAINT version_head_org_id_project_id_confirmed_split_set_id_fkey FOREIGN KEY (org_id, project_id, confirmed_split_set_id) REFERENCES script.split_set(org_id, project_id, id);

ALTER TABLE ONLY script.version_head
    ADD CONSTRAINT version_head_org_id_project_id_version_id_fkey FOREIGN KEY (org_id, project_id, version_id) REFERENCES script.script_version(org_id, project_id, id);

ALTER TABLE ONLY script.version_source
    ADD CONSTRAINT version_source_org_id_project_id_source_id_fkey FOREIGN KEY (org_id, project_id, source_id) REFERENCES script.script_source(org_id, project_id, id);

ALTER TABLE ONLY script.version_source
    ADD CONSTRAINT version_source_org_id_project_id_version_id_fkey FOREIGN KEY (org_id, project_id, version_id) REFERENCES script.script_version(org_id, project_id, id);

ALTER TABLE ONLY workspace.style_preset
    ADD CONSTRAINT fk_style_preset_project FOREIGN KEY (project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY workspace.project_change_command
    ADD CONSTRAINT project_change_command_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY workspace.project_change_command
    ADD CONSTRAINT project_change_command_org_id_fkey FOREIGN KEY (org_id) REFERENCES workspace.organization(id);

ALTER TABLE ONLY workspace.project_change_command
    ADD CONSTRAINT project_change_command_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY workspace.project_copy_command
    ADD CONSTRAINT project_copy_command_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY workspace.project_copy_command
    ADD CONSTRAINT project_copy_command_copy_job_id_fkey FOREIGN KEY (copy_job_id) REFERENCES workspace.project_copy_job(id);

ALTER TABLE ONLY workspace.project_copy_job
    ADD CONSTRAINT project_copy_job_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY workspace.project_copy_job
    ADD CONSTRAINT project_copy_job_execution_actor_id_fkey FOREIGN KEY (execution_actor_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY workspace.project_copy_job
    ADD CONSTRAINT project_copy_job_org_id_fkey FOREIGN KEY (org_id) REFERENCES workspace.organization(id);

ALTER TABLE ONLY workspace.project_copy_job
    ADD CONSTRAINT project_copy_job_source_project_id_fkey FOREIGN KEY (source_project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY workspace.project_copy_job
    ADD CONSTRAINT project_copy_job_target_project_id_fkey FOREIGN KEY (target_project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY workspace.project
    ADD CONSTRAINT project_cover_asset_id_fkey FOREIGN KEY (cover_asset_id) REFERENCES media.media_asset(id);

ALTER TABLE ONLY workspace.project_folder
    ADD CONSTRAINT project_folder_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY workspace.project_folder_command
    ADD CONSTRAINT project_folder_command_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY workspace.project_folder_command
    ADD CONSTRAINT project_folder_command_org_id_fkey FOREIGN KEY (org_id) REFERENCES workspace.organization(id);

ALTER TABLE ONLY workspace.project_folder
    ADD CONSTRAINT project_folder_cover_asset_id_fkey FOREIGN KEY (cover_asset_id) REFERENCES media.media_asset(id);

ALTER TABLE ONLY workspace.project_folder
    ADD CONSTRAINT project_folder_cover_project_id_fkey FOREIGN KEY (cover_project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY workspace.project_folder
    ADD CONSTRAINT project_folder_org_id_fkey FOREIGN KEY (org_id) REFERENCES workspace.organization(id);

ALTER TABLE ONLY workspace.project_folder_placement
    ADD CONSTRAINT project_folder_placement_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY workspace.project_folder_placement
    ADD CONSTRAINT project_folder_placement_folder_id_org_id_actor_id_fkey FOREIGN KEY (folder_id, org_id, actor_id) REFERENCES workspace.project_folder(id, org_id, actor_id);

ALTER TABLE ONLY workspace.project_folder_placement
    ADD CONSTRAINT project_folder_placement_org_id_fkey FOREIGN KEY (org_id) REFERENCES workspace.organization(id);

ALTER TABLE ONLY workspace.project_folder_placement
    ADD CONSTRAINT project_folder_placement_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id) ON DELETE CASCADE;

ALTER TABLE ONLY workspace.project
    ADD CONSTRAINT project_org_id_fkey FOREIGN KEY (org_id) REFERENCES workspace.organization(id);

ALTER TABLE ONLY workspace.project
    ADD CONSTRAINT project_style_preset_id_fkey FOREIGN KEY (style_preset_id) REFERENCES workspace.style_preset(id);

ALTER TABLE ONLY workspace.prompt_customization
    ADD CONSTRAINT prompt_customization_org_id_fkey FOREIGN KEY (org_id) REFERENCES workspace.organization(id);

ALTER TABLE ONLY workspace.prompt_customization
    ADD CONSTRAINT prompt_customization_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY workspace.style_preset
    ADD CONSTRAINT style_preset_org_id_fkey FOREIGN KEY (org_id) REFERENCES workspace.organization(id);

-- 运行时最小权限；应用登录账号不得成为表所有者
GRANT USAGE ON SCHEMA audit TO lanverse_app;

GRANT USAGE ON SCHEMA bible TO lanverse_app;

GRANT USAGE ON SCHEMA billing TO lanverse_app;

GRANT USAGE ON SCHEMA canvas TO lanverse_app;

GRANT USAGE ON SCHEMA catalog TO lanverse_app;

GRANT USAGE ON SCHEMA identity TO lanverse_app;

GRANT USAGE ON SCHEMA infra TO lanverse_app;

GRANT USAGE ON SCHEMA media TO lanverse_app;

GRANT USAGE ON SCHEMA mediatool TO lanverse_app;

GRANT USAGE ON SCHEMA operation TO lanverse_app;

GRANT USAGE ON SCHEMA script TO lanverse_app;

GRANT USAGE ON SCHEMA workspace TO lanverse_app;

GRANT SELECT,INSERT ON TABLE audit.audit_log TO lanverse_app;

GRANT SELECT,INSERT ON TABLE bible."character" TO lanverse_app;

GRANT UPDATE(revision) ON TABLE bible."character" TO lanverse_app;

GRANT UPDATE(current_version_id) ON TABLE bible."character" TO lanverse_app;

GRANT UPDATE(confirmed_version_id) ON TABLE bible."character" TO lanverse_app;

GRANT UPDATE(redirect_id) ON TABLE bible."character" TO lanverse_app;

GRANT UPDATE(is_delete) ON TABLE bible."character" TO lanverse_app;

GRANT UPDATE(updated_at) ON TABLE bible."character" TO lanverse_app;

GRANT SELECT,INSERT ON TABLE bible.character_confirmation TO lanverse_app;

GRANT SELECT,INSERT ON TABLE bible.character_redirect TO lanverse_app;

GRANT SELECT,INSERT ON TABLE bible.character_split TO lanverse_app;

GRANT SELECT,INSERT ON TABLE bible.character_version TO lanverse_app;

GRANT SELECT,INSERT ON TABLE bible.command TO lanverse_app;

GRANT SELECT,INSERT ON TABLE bible.copy_cleanup_receipt TO lanverse_app;

GRANT SELECT,INSERT ON TABLE bible.copy_receipt TO lanverse_app;

GRANT SELECT,INSERT ON TABLE bible.copy_snapshot TO lanverse_app;

GRANT SELECT,INSERT ON TABLE bible.copy_transfer_receipt TO lanverse_app;

GRANT SELECT,INSERT ON TABLE bible.location TO lanverse_app;

GRANT UPDATE (revision, current_version_id, confirmed_version_id, is_delete, updated_at) ON TABLE bible.location TO lanverse_app;

GRANT SELECT,INSERT ON TABLE bible.location_confirmation TO lanverse_app;

GRANT SELECT,INSERT ON TABLE bible.location_version TO lanverse_app;

GRANT SELECT,INSERT ON TABLE bible.look TO lanverse_app;

GRANT SELECT,INSERT ON TABLE bible.look_version TO lanverse_app;

GRANT SELECT,INSERT ON TABLE bible.prop TO lanverse_app;

GRANT UPDATE (revision, current_version_id, confirmed_version_id, is_delete, updated_at) ON TABLE bible.prop TO lanverse_app;

GRANT SELECT,INSERT ON TABLE bible.prop_confirmation TO lanverse_app;

GRANT SELECT,INSERT ON TABLE bible.prop_version TO lanverse_app;

GRANT SELECT,INSERT ON TABLE bible.reference_version TO lanverse_app;

GRANT SELECT,INSERT ON TABLE bible.voice_version TO lanverse_app;

GRANT SELECT,INSERT,UPDATE ON TABLE billing.budget TO lanverse_app;

GRANT SELECT,INSERT ON TABLE billing.ledger_entry TO lanverse_app;

GRANT UPDATE (update_time, is_delete) ON TABLE billing.ledger_entry TO lanverse_app;

GRANT SELECT,INSERT ON TABLE billing.reservation TO lanverse_app;

GRANT UPDATE (status, closed_at, update_time) ON TABLE billing.reservation TO lanverse_app;

GRANT SELECT,INSERT,UPDATE ON TABLE canvas.command_log TO lanverse_app;

GRANT SELECT,INSERT,UPDATE ON TABLE canvas.document TO lanverse_app;

GRANT SELECT,INSERT,UPDATE ON TABLE canvas.edge TO lanverse_app;

GRANT SELECT,INSERT,UPDATE ON TABLE canvas.node TO lanverse_app;

GRANT SELECT,INSERT ON TABLE canvas.project_copy_receipt TO lanverse_app;

GRANT SELECT,INSERT ON TABLE canvas.project_copy_snapshot TO lanverse_app;

GRANT SELECT,INSERT ON TABLE catalog.capability TO lanverse_app;

GRANT SELECT,INSERT,UPDATE ON TABLE catalog.model_profile TO lanverse_app;

GRANT SELECT,INSERT ON TABLE catalog.model_profile_version TO lanverse_app;

GRANT SELECT,INSERT ON TABLE catalog.price_rule_version TO lanverse_app;

GRANT SELECT,INSERT,UPDATE ON TABLE catalog.provider TO lanverse_app;

GRANT SELECT,INSERT,UPDATE ON TABLE catalog.provider_credential TO lanverse_app;

GRANT SELECT,INSERT,UPDATE ON TABLE identity."user" TO lanverse_app;

GRANT SELECT,INSERT,UPDATE ON TABLE identity.user_session TO lanverse_app;
GRANT SELECT,INSERT,UPDATE ON TABLE identity.login_guard TO lanverse_app;
GRANT SELECT,INSERT ON TABLE identity.auth_event TO lanverse_app;

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE infra.idempotency_record TO lanverse_app;

GRANT SELECT,INSERT,DELETE ON TABLE infra.outbox TO lanverse_app;

GRANT UPDATE (published_at, update_time) ON TABLE infra.outbox TO lanverse_app;

GRANT SELECT,INSERT,DELETE ON TABLE infra.processed_event TO lanverse_app;

GRANT UPDATE (update_time) ON TABLE infra.processed_event TO lanverse_app;

GRANT SELECT,INSERT ON TABLE media.library TO lanverse_app;

GRANT UPDATE (revision, update_time) ON TABLE media.library TO lanverse_app;

GRANT SELECT,INSERT ON TABLE media.library_command TO lanverse_app;

GRANT SELECT,INSERT,DELETE ON TABLE media.library_folder TO lanverse_app;

GRANT UPDATE (parent_id, name, name_key, "position", style, theme, revision, update_time) ON TABLE media.library_folder TO lanverse_app;

GRANT SELECT,INSERT ON TABLE media.library_item TO lanverse_app;

GRANT UPDATE (plain_text, folder_id, title, category, tags, source_label, note, favorite, catalog_state, trashed_at, "position", revision, update_time, purged_at) ON TABLE media.library_item TO lanverse_app;

GRANT SELECT,INSERT ON TABLE media.media_asset TO lanverse_app;

GRANT UPDATE (status, mime_type, byte_size, sha256, width, height, duration_ms, fps, audio_channels, codec, moderation_status, moderation_detail, failure_reason, delete_time, purge_after, revision, update_time, is_delete) ON TABLE media.media_asset TO lanverse_app;

GRANT SELECT,INSERT ON TABLE media.package_command TO lanverse_app;

GRANT SELECT,INSERT ON TABLE media.package_job TO lanverse_app;

GRANT UPDATE (status, revision, library_revision, project_revision, result, updated_at) ON TABLE media.package_job TO lanverse_app;

GRANT SELECT,INSERT ON TABLE media.package_object TO lanverse_app;

GRANT UPDATE (status) ON TABLE media.package_object TO lanverse_app;

GRANT SELECT,INSERT ON TABLE media.project_copy_object TO lanverse_app;

GRANT UPDATE (byte_size, source_verified, write_started, sha256, status, update_time) ON TABLE media.project_copy_object TO lanverse_app;

GRANT SELECT,INSERT ON TABLE media.project_copy_receipt TO lanverse_app;

GRANT SELECT,INSERT ON TABLE media.project_copy_snapshot TO lanverse_app;

GRANT SELECT,INSERT ON TABLE media.purge_command TO lanverse_app;

GRANT SELECT,INSERT ON TABLE media.purge_item TO lanverse_app;

GRANT UPDATE (status, failure_code) ON TABLE media.purge_item TO lanverse_app;

GRANT SELECT,INSERT ON TABLE media.purge_job TO lanverse_app;

GRANT UPDATE (status, stage, attempt, revision, cancellation_requested, needs_reconciliation, execution_unconfirmed, execution_id, worker_fence, lease_until, process_ended, updated_at) ON TABLE media.purge_job TO lanverse_app;

GRANT SELECT,INSERT ON TABLE media.purge_object TO lanverse_app;

GRANT UPDATE (byte_size, sha256, verified, removal_started, removed) ON TABLE media.purge_object TO lanverse_app;

GRANT SELECT,INSERT ON TABLE media.rendition TO lanverse_app;

GRANT UPDATE (update_time, is_delete) ON TABLE media.rendition TO lanverse_app;

GRANT SELECT,INSERT ON TABLE media.transfer_command TO lanverse_app;

GRANT SELECT,INSERT ON TABLE media.transfer_item TO lanverse_app;

GRANT UPDATE (status, failure_code) ON TABLE media.transfer_item TO lanverse_app;

GRANT SELECT,INSERT ON TABLE media.transfer_job TO lanverse_app;

GRANT UPDATE (status, stage, attempt, revision, needs_reconciliation, cancellation_requested, execution_unconfirmed, execution_id, worker_fence, lease_until, process_ended, updated_at) ON TABLE media.transfer_job TO lanverse_app;

GRANT SELECT,INSERT ON TABLE media.transfer_object TO lanverse_app;

GRANT UPDATE (byte_size, sha256, source_verified, write_started, status) ON TABLE media.transfer_object TO lanverse_app;

GRANT SELECT,INSERT ON TABLE media.upload_request TO lanverse_app;

GRANT SELECT,INSERT ON TABLE mediatool.depth_command TO lanverse_app;

GRANT SELECT,INSERT ON TABLE mediatool.depth_job TO lanverse_app;

GRANT UPDATE (status, stage, attempt, revision, process_state, active_worker, failure_code, retryable, needs_reconciliation, execution_unconfirmed, cancellation_requested, reconciliation_requested, updated_at) ON TABLE mediatool.depth_job TO lanverse_app;

GRANT SELECT,INSERT ON TABLE mediatool.depth_object TO lanverse_app;

GRANT UPDATE (write_started, delete_started, status, updated_at) ON TABLE mediatool.depth_object TO lanverse_app;

GRANT SELECT,INSERT ON TABLE mediatool.depth_result_intent TO lanverse_app;

GRANT SELECT,INSERT ON TABLE mediatool.export_command TO lanverse_app;

GRANT SELECT,INSERT ON TABLE mediatool.export_job TO lanverse_app;

GRANT UPDATE (actor_id, actor_role, status, stage, progress, attempt, revision, asset_id, sha256, failure_code, active_worker, updated_at) ON TABLE mediatool.export_job TO lanverse_app;

GRANT SELECT,INSERT ON TABLE mediatool.transcription_command TO lanverse_app;

GRANT SELECT,INSERT ON TABLE mediatool.transcription_job TO lanverse_app;

GRANT UPDATE (actor_id, actor_role, status, stage, progress, attempt, revision, inference_state, result, result_sha256, failure_code, active_worker, updated_at) ON TABLE mediatool.transcription_job TO lanverse_app;

GRANT SELECT,INSERT ON TABLE operation.batch TO lanverse_app;

GRANT UPDATE (status, paused_reason, total_count, succeeded_count, failed_count, unknown_count, quote_total_micros, workflow_id, update_time, cancel_requested_at) ON TABLE operation.batch TO lanverse_app;

GRANT SELECT,INSERT ON TABLE operation.batch_launch TO lanverse_app;

GRANT UPDATE (state, released_at) ON TABLE operation.batch_launch TO lanverse_app;

GRANT SELECT,INSERT ON TABLE operation.confirmation_request TO lanverse_app;

GRANT SELECT,INSERT ON TABLE operation.operation TO lanverse_app;

GRANT UPDATE (status, failure_code, failure_message, retryable, reservation_id, provider_request_key, workflow_id, confirmed_at, confirmed_by, started_at, finished_at, settled_micros, cost_estimated, update_time) ON TABLE operation.operation TO lanverse_app;

GRANT SELECT,INSERT ON TABLE operation.operation_event TO lanverse_app;

GRANT SELECT,INSERT ON TABLE operation.operation_input TO lanverse_app;

GRANT SELECT,INSERT ON TABLE operation.operation_output TO lanverse_app;

GRANT UPDATE (moderation_status, moderation_reason, update_time) ON TABLE operation.operation_output TO lanverse_app;

GRANT SELECT,INSERT,UPDATE ON TABLE operation.provider_call TO lanverse_app;

GRANT SELECT,INSERT ON TABLE operation.quote_request TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.action_line TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.command TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.command_result TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.command_state TO lanverse_app;

GRANT UPDATE (revision, status, failure_code, cancellation_requested, needs_reconciliation, io_owner_id, io_state, updated_at) ON TABLE script.command_state TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.copy_object_intent TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.copy_object_state TO lanverse_app;

GRANT UPDATE (put_started, confirmed, removed, updated_at) ON TABLE script.copy_object_state TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.copy_receipt TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.copy_snapshot TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.dialogue_line TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.episode TO lanverse_app;

GRANT UPDATE (title, revision, current_structure_id, confirmed_structure_id, previous_episode_id, inherit_status, is_delete) ON TABLE script.episode TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.episode_structure TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.import_attempt TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.import_command TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.import_file TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.import_file_result TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.import_job TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.import_publication TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.import_state TO lanverse_app;

GRANT UPDATE (revision, attempt, status, stage, latest_script_revision, latest_version_id, cancellation_requested, reconciliation_requested, needs_reconciliation, failure_code, io_owner_id, io_state, updated_at) ON TABLE script.import_state TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.object_intent TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.object_state TO lanverse_app;

GRANT UPDATE (put_started, confirmed, removed, updated_at) ON TABLE script.object_state TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.project_state TO lanverse_app;

GRANT UPDATE (revision, draft_version_id, adopted_version_id, updated_at) ON TABLE script.project_state TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.request TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.review_command TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.scene TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.script_source TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.script_version TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.source_control TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.split_confirmation TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.split_set TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.version_head TO lanverse_app;

GRANT UPDATE (split_revision, candidate_split_set_id, confirmed_split_set_id) ON TABLE script.version_head TO lanverse_app;

GRANT SELECT,INSERT ON TABLE script.version_source TO lanverse_app;

GRANT SELECT,INSERT ON TABLE workspace.organization TO lanverse_app;

GRANT UPDATE (update_time) ON TABLE workspace.organization TO lanverse_app;

GRANT SELECT,INSERT,UPDATE ON TABLE workspace.project TO lanverse_app;

GRANT SELECT,INSERT ON TABLE workspace.project_change_command TO lanverse_app;

GRANT SELECT,INSERT ON TABLE workspace.project_copy_command TO lanverse_app;

GRANT SELECT,INSERT ON TABLE workspace.project_copy_job TO lanverse_app;

GRANT UPDATE (status, stage, revision, attempt, worker_id, started_at, media_receipt, canvas_receipt, failure_code, retryable, needs_reconciliation, cancellation_requested, reconciliation_requested, execution_unconfirmed, update_time, script_receipt, bible_receipt, execution_actor_id) ON TABLE workspace.project_copy_job TO lanverse_app;

GRANT SELECT,INSERT ON TABLE workspace.project_folder TO lanverse_app;

GRANT UPDATE (name, cover_project_id, cover_asset_id, revision, is_delete, delete_time, update_time) ON TABLE workspace.project_folder TO lanverse_app;

GRANT SELECT,INSERT ON TABLE workspace.project_folder_command TO lanverse_app;

GRANT SELECT,INSERT ON TABLE workspace.project_folder_placement TO lanverse_app;

GRANT UPDATE (folder_id, revision, update_time) ON TABLE workspace.project_folder_placement TO lanverse_app;

GRANT SELECT,INSERT,UPDATE ON TABLE workspace.prompt_customization TO lanverse_app;

GRANT SELECT,INSERT ON TABLE workspace.style_preset TO lanverse_app;

-- 分区是按初始化时间派生的物理对象，不能把一次 dump 的年月固定为结构合同。
-- 每个父表预建当前 UTC 月及未来三个月，并保留默认分区。
-- 索引、约束与触发器由 PostgreSQL 从父表继承；应用始终经父表访问。
DO $$
DECLARE
  first_month timestamp := date_trunc('month', now() AT TIME ZONE 'UTC');
  parent_table text;
  month_index integer;
  month_start timestamptz;
  month_end timestamptz;
BEGIN
  FOREACH parent_table IN ARRAY ARRAY['audit.audit_log', 'infra.outbox', 'operation.provider_call'] LOOP
    FOR month_index IN 0..3 LOOP
      month_start := (first_month + make_interval(months => month_index)) AT TIME ZONE 'UTC';
      month_end := (first_month + make_interval(months => month_index + 1)) AT TIME ZONE 'UTC';
      EXECUTE format(
        'CREATE TABLE %s PARTITION OF %s FOR VALUES FROM (%L) TO (%L)',
        parent_table || '_' || to_char(month_start AT TIME ZONE 'UTC', 'YYYYMM'),
        parent_table, month_start, month_end
      );
    END LOOP;
    EXECUTE format('CREATE TABLE %s PARTITION OF %s DEFAULT', parent_table || '_default', parent_table);
  END LOOP;
END $$;
