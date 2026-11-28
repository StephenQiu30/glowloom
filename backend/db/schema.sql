-- 当前 PostgreSQL 完整结构与基础数据。开发、CI 和部署使用同一空库定义。
-- PostgreSQL 18。由结构所有者执行；管理员离线预建 glowloom_app、四域 NOLOGIN 角色与受限 glowloom_agent LOGIN。
-- 四域角色不得拥有对象/继承 glowloom_app；普通 LOGIN 只能加入单一 owner，凭据离线设置。
-- psql "$GLOWLOOM_SCHEMA_DB_DSN" -X --single-transaction -v ON_ERROR_STOP=1 -f backend/db/schema.sql -f backend/db/data.sql
-- 禁止对已有业务库重放本文件。

DO $$
DECLARE
  runtime_role text;
BEGIN
  FOREACH runtime_role IN ARRAY ARRAY['glowloom_identity','glowloom_creation','glowloom_media','glowloom_execution'] LOOP
    IF NOT EXISTS (
      SELECT 1 FROM pg_roles WHERE rolname = runtime_role
      AND NOT (rolcanlogin OR rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls)
    ) THEN
      RAISE EXCEPTION '% must be a pre-provisioned restricted NOLOGIN role', runtime_role;
    END IF;
    IF pg_has_role(runtime_role, 'glowloom_app', 'MEMBER') OR EXISTS (
      SELECT 1 FROM pg_roles WHERE rolname IN ('glowloom_identity','glowloom_creation','glowloom_media','glowloom_execution')
      AND rolname <> runtime_role AND pg_has_role(runtime_role, oid, 'MEMBER')
    ) THEN
      RAISE EXCEPTION '% must not inherit another runtime owner or glowloom_app', runtime_role;
    END IF;
    IF current_user = runtime_role OR pg_has_role(runtime_role, current_user, 'MEMBER') THEN
      RAISE EXCEPTION '% must not inherit the schema initializer', runtime_role;
    END IF;
  END LOOP;
  IF NOT EXISTS (
    SELECT 1 FROM pg_roles
    WHERE rolname = 'glowloom_app'
      AND NOT (rolcanlogin OR rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls)
  ) THEN
    RAISE EXCEPTION 'glowloom_app must be a pre-provisioned restricted NOLOGIN role';
  END IF;
  IF current_user = 'glowloom_app' OR pg_has_role('glowloom_app',current_user,'MEMBER') THEN
    RAISE EXCEPTION 'glowloom_app must not inherit the schema initializer';
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

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

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

CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;
CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;

-- Trusted service identity comes only from database memberships, never a GUC.
-- Offline schema owner/superuser imports retain legacy ownership. Ordinary
-- LOGINs (including NOINHERIT) must have exactly one capability; SET ROLE cannot
-- conceal a second capability held by the authenticated session identity.
CREATE FUNCTION infra.runtime_owner() RETURNS text
LANGUAGE plpgsql STABLE SECURITY INVOKER SET search_path = pg_catalog AS $$
DECLARE
  owner_count integer;
  owner_name text;
  offline_session boolean;
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname=current_user AND rolsuper)
     OR current_user = (SELECT pg_get_userbyid(nspowner) FROM pg_namespace WHERE nspname='infra') THEN
    RETURN 'legacy';
  END IF;
  SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname=session_user AND rolsuper)
     OR session_user = (SELECT pg_get_userbyid(nspowner) FROM pg_namespace WHERE nspname='infra')
    INTO offline_session;
  SELECT count(*), min(CASE WHEN rolname='glowloom_app' THEN 'legacy' ELSE substr(rolname,10) END)
    INTO owner_count, owner_name
    FROM pg_roles
    WHERE rolname IN ('glowloom_app','glowloom_identity','glowloom_creation','glowloom_media','glowloom_execution')
      AND (pg_has_role(current_user,oid,'MEMBER')
           OR (NOT offline_session AND pg_has_role(session_user,oid,'MEMBER')));
  IF owner_count <> 1 THEN
    RAISE EXCEPTION 'runtime owner requires exactly one database capability' USING ERRCODE='42501';
  END IF;
  RETURN owner_name;
END $$;
REVOKE ALL ON FUNCTION infra.runtime_owner() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION infra.runtime_owner() TO glowloom_app,glowloom_identity,glowloom_creation,glowloom_media,glowloom_execution;

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
 IF NEW.source_scope_id IS NULL OR NEW.target_scope_id IS NULL OR NEW.source_org_id IS NULL OR NEW.target_org_id IS NULL
 OR source.org_id<>NEW.source_org_id OR target.org_id<>NEW.target_org_id OR source.kind<>NEW.source_kind OR target.kind<>NEW.target_kind
 OR source.project_id IS DISTINCT FROM NEW.source_project_id OR target.project_id IS DISTINCT FROM NEW.target_project_id
 OR (source.kind='personal' AND source.personal_actor_id<>NEW.actor_id) OR (target.kind='personal' AND target.personal_actor_id<>NEW.actor_id)
 OR NOT EXISTS(SELECT 1 FROM workspace.creation_scope s WHERE s.id=NEW.source_scope_id AND s.physical_scope_id=source.org_id AND
 ((source.kind='personal' AND s.kind='personal' AND s.owner_user_id=NEW.actor_id) OR (source.kind='project' AND EXISTS(SELECT 1 FROM workspace.project p WHERE p.id=source.project_id AND p.owner_scope_id=s.id))))
 OR NOT EXISTS(SELECT 1 FROM workspace.creation_scope s WHERE s.id=NEW.target_scope_id AND s.physical_scope_id=target.org_id AND
 ((target.kind='personal' AND s.kind='personal' AND s.owner_user_id=NEW.actor_id) OR (target.kind='project' AND EXISTS(SELECT 1 FROM workspace.project p WHERE p.id=target.project_id AND p.owner_scope_id=s.id))))
 OR (source.kind='project' AND source.org_id<>NEW.org_id) OR (target.kind='project' AND target.org_id<>NEW.org_id)
 THEN RAISE EXCEPTION 'media transfer scope mismatch' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;

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

CREATE FUNCTION media.keep_object_binding() RETURNS trigger
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

CREATE FUNCTION media.keep_storage_evidence() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 RAISE EXCEPTION 'storage evidence is immutable' USING ERRCODE='23514';
END $$;

CREATE FUNCTION media.keep_storage_identity() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF TG_TABLE_NAME='storage_profile' THEN
        RAISE EXCEPTION 'storage identity is immutable' USING ERRCODE='23514';
    END IF;
    IF (NEW.id,NEW.profile_id,NEW.bucket,NEW.object_key,NEW.owner_session_id) IS DISTINCT FROM (OLD.id,OLD.profile_id,OLD.bucket,OLD.object_key,OLD.owner_session_id) THEN
        RAISE EXCEPTION 'object location identity is immutable' USING ERRCODE='23514';
    END IF;
    NEW.updated_at=now();
    RETURN NEW;
END $$;

CREATE FUNCTION media.keep_storage_probe_intent() RETURNS trigger
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

CREATE FUNCTION media.keep_storage_work() RETURNS trigger
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

CREATE FUNCTION media.keep_storage_write_deadline() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF OLD.write_until IS NOT NULL AND (NEW.write_until IS NULL OR NEW.write_until<OLD.write_until) THEN
  RAISE EXCEPTION 'storage write deadline cannot regress' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION media.keep_upload_admission() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF (to_jsonb(NEW)-'accepted_asset_id') IS DISTINCT FROM (to_jsonb(OLD)-'accepted_asset_id')
   OR (OLD.accepted_asset_id IS NOT NULL AND NEW.accepted_asset_id IS DISTINCT FROM OLD.accepted_asset_id) THEN
  RAISE EXCEPTION 'upload admission is immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION media.protect_transfer_ownership() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF ROW(NEW.actor_id,NEW.org_id,NEW.source_library_id,NEW.target_library_id,NEW.source_kind,NEW.target_kind,NEW.source_project_id,NEW.target_project_id,NEW.source_scope_id,NEW.target_scope_id,NEW.source_org_id,NEW.target_org_id)
 IS DISTINCT FROM ROW(OLD.actor_id,OLD.org_id,OLD.source_library_id,OLD.target_library_id,OLD.source_kind,OLD.target_kind,OLD.source_project_id,OLD.target_project_id,OLD.source_scope_id,OLD.target_scope_id,OLD.source_org_id,OLD.target_org_id)
 THEN RAISE EXCEPTION 'media transfer ownership is immutable' USING ERRCODE='23514'; END IF;
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

CREATE FUNCTION workspace.validate_secondary_scope() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_TABLE_NAME='project_copy_job' THEN
  IF TG_OP='UPDATE' AND (NEW.owner_scope_id,NEW.org_id,NEW.actor_id,NEW.source_project_id,NEW.target_project_id) IS DISTINCT FROM (OLD.owner_scope_id,OLD.org_id,OLD.actor_id,OLD.source_project_id,OLD.target_project_id) THEN
   RAISE EXCEPTION 'copy scope binding is immutable';
  END IF;
  IF TG_OP='INSERT' AND (NEW.owner_scope_id IS NULL OR NOT EXISTS(SELECT 1 FROM workspace.project p JOIN workspace.project t ON t.id=NEW.target_project_id WHERE p.id=NEW.source_project_id AND p.org_id=NEW.org_id AND t.org_id=NEW.org_id AND p.owner_scope_id=NEW.owner_scope_id AND t.owner_scope_id=NEW.owner_scope_id)) THEN
   RAISE EXCEPTION 'copy scope binding invalid';
  END IF;
 ELSE
  IF NEW.owner_scope_id IS NULL OR NOT EXISTS(SELECT 1 FROM workspace.project p WHERE p.id=NEW.project_id AND p.org_id=NEW.org_id AND p.owner_scope_id=NEW.owner_scope_id) OR (NEW.folder_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM workspace.project_folder f WHERE f.id=NEW.folder_id AND f.org_id=NEW.org_id AND f.actor_id=NEW.actor_id AND f.owner_scope_id=NEW.owner_scope_id)) THEN
   RAISE EXCEPTION 'folder placement scope binding invalid';
  END IF;
  IF TG_OP='UPDATE' AND (NEW.owner_scope_id,NEW.org_id,NEW.actor_id,NEW.project_id) IS DISTINCT FROM (OLD.owner_scope_id,OLD.org_id,OLD.actor_id,OLD.project_id) THEN
   RAISE EXCEPTION 'folder placement scope binding is immutable';
  END IF;
 END IF;
 RETURN NEW;
END;
$$;

CREATE TABLE audit.audit_log (
    service_owner text NOT NULL DEFAULT infra.runtime_owner() CHECK (service_owner IN ('legacy','identity','creation','media','execution')),
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


SET default_table_access_method = heap;

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

CREATE TABLE bible.copy_transfer_receipt (
    snapshot_id uuid NOT NULL,
    manifest_sha256 text NOT NULL,
    content_sha256 text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT copy_transfer_receipt_content_sha256_check CHECK ((content_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT copy_transfer_receipt_manifest_sha256_check CHECK ((manifest_sha256 ~ '^[a-f0-9]{64}$'::text))
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
    op_id uuid,
    client_id uuid,
    base_revision bigint,
    operation_version integer DEFAULT 0 NOT NULL,
    id uuid NOT NULL,
    document_id uuid NOT NULL,
    revision bigint NOT NULL,
    commands jsonb NOT NULL,
    create_by uuid,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL
);

CREATE INDEX canvas_operation_tombstones ON canvas.command_log USING gin(commands jsonb_path_ops) WHERE operation_version=1;

CREATE UNIQUE INDEX canvas_operation_identity ON canvas.command_log(document_id,create_by,op_id) WHERE op_id IS NOT NULL;

CREATE TABLE canvas.document (
    kind text DEFAULT 'canvas' NOT NULL CHECK (kind IN ('canvas','short_drama')),
    schema_version integer DEFAULT 1 NOT NULL CHECK (schema_version > 0),
    operation_version integer DEFAULT 1 NOT NULL CHECK (operation_version IN (0,1)),
    id uuid NOT NULL,
    project_id uuid NOT NULL,
    name text NOT NULL,
    scope jsonb DEFAULT '{}'::jsonb NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
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
    content_version bigint DEFAULT 1 NOT NULL CHECK (content_version > 0),
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
    input_revision bigint,
    CONSTRAINT ck_canvas_node_title CHECK (((char_length(title) >= 1) AND (char_length(title) <= 128))),
    CONSTRAINT node_input_revision_check CHECK ((input_revision > 0)),
    CONSTRAINT node_node_action_check CHECK ((node_action = ANY (ARRAY['resource'::text, 'generate'::text, 'edit'::text, 'tool'::text])))
);

CREATE TABLE canvas.node_input (
    node_id uuid NOT NULL,
    input_revision bigint NOT NULL,
    draft jsonb NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT node_input_draft_check CHECK ((jsonb_typeof(draft) = 'object'::text)),
    CONSTRAINT node_input_input_revision_check CHECK ((input_revision > 0))
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

CREATE TABLE canvas.save_receipt (
    actor_id uuid NOT NULL,
    idem_key uuid NOT NULL,
    document_id uuid NOT NULL,
    request_hash text NOT NULL,
    status text DEFAULT 'accepted' NOT NULL,
    result_revision bigint,
    response_body jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT save_receipt_request_hash_check CHECK ((request_hash ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT save_receipt_response_body_check CHECK ((jsonb_typeof(response_body) = 'object'::text)),
    CONSTRAINT save_receipt_result_revision_check CHECK ((status='accepted' AND result_revision IS NOT NULL AND result_revision>1) OR (status='rejected' AND result_revision IS NULL))
);

-- Runtime writes may redact only the response payload after project retention
-- expires. Active accepted/rejected receipts and their request identity are immutable.
CREATE FUNCTION canvas.guard_save_receipt() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF (SELECT rolsuper FROM pg_roles WHERE rolname=current_user) THEN
    IF TG_OP='DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
  END IF;
  IF TG_OP='UPDATE'
     AND (to_jsonb(NEW)-'response_body')=(to_jsonb(OLD)-'response_body')
     AND NEW.response_body='{"retention_expired":true}'::jsonb
     AND EXISTS (
       SELECT 1 FROM canvas.document d JOIN workspace.project p ON p.id=d.project_id
       WHERE d.id=OLD.document_id AND p.is_delete AND p.purge_after<=statement_timestamp()
     ) THEN
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'canvas save receipts are immutable' USING ERRCODE='42501';
END $$;

CREATE TRIGGER canvas_save_receipt_immutable BEFORE UPDATE OR DELETE ON canvas.save_receipt
FOR EACH ROW EXECUTE FUNCTION canvas.guard_save_receipt();


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

CREATE TABLE catalog.capability_default (
    capability text NOT NULL,
    release_id uuid NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT capability_default_revision_check CHECK ((revision > 0))
);

CREATE TABLE catalog.config_request (
    actor_id uuid NOT NULL,
    action text NOT NULL,
    target_id uuid NOT NULL,
    request_id uuid NOT NULL,
    fingerprint text NOT NULL,
    result jsonb NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE catalog.model_candidate (
    id uuid NOT NULL,
    check_id uuid NOT NULL,
    provider_model_id text NOT NULL,
    display_name text NOT NULL,
    capability text NOT NULL,
    metadata jsonb NOT NULL
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
    active_release_id uuid,
    candidate_snapshot jsonb,
    CONSTRAINT model_profile_candidate_snapshot_check CHECK (candidate_snapshot IS NULL OR (jsonb_typeof(candidate_snapshot) = 'object' AND octet_length(candidate_snapshot::text) <= 8192)),
    CONSTRAINT model_profile_display_name_check CHECK ((btrim(display_name) <> ''::text)),
    CONSTRAINT model_profile_model_key_check CHECK ((btrim(model_key) <> ''::text)),
    CONSTRAINT model_profile_revision_check CHECK ((revision > 0)),
    CONSTRAINT model_profile_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text, 'archived'::text])))
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

CREATE TABLE catalog.model_release (
    onboarding_check_id uuid,
    id uuid NOT NULL,
    model_id uuid NOT NULL,
    model_version_id uuid NOT NULL,
    price_version_id uuid NOT NULL,
    config_version_id uuid NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    upstream_cost jsonb NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    activated_at timestamp with time zone,
    CONSTRAINT model_release_revision_check CHECK ((revision > 0)),
    CONSTRAINT model_release_status_check CHECK ((status = ANY (ARRAY['draft'::text, 'active'::text, 'retired'::text]))),
    CONSTRAINT model_release_upstream_cost_check CHECK ((jsonb_typeof(upstream_cost) = 'object'::text))
);

CREATE TABLE catalog.model_validation (
    id uuid NOT NULL,
    release_id uuid NOT NULL,
    credential_id uuid NOT NULL,
    adapter_version text NOT NULL,
    mode text NOT NULL,
    input_digest text NOT NULL,
    operation_id uuid NOT NULL,
    status text NOT NULL,
    evidence jsonb DEFAULT '{}'::jsonb NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone,
    CONSTRAINT model_validation_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'running'::text, 'succeeded'::text, 'failed'::text, 'unknown'::text])))
);

CREATE TABLE catalog.price_rule_version (
    source_url text,
    reviewed_at timestamp with time zone,
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
    logo_asset_id uuid,
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
    current_config_version_id uuid,
    CONSTRAINT provider_adapter_key_check CHECK ((btrim(adapter_key) <> ''::text)),
    CONSTRAINT provider_concurrency_limit_check CHECK ((concurrency_limit > 0)),
    CONSTRAINT provider_key_check CHECK ((btrim(key) <> ''::text)),
    CONSTRAINT provider_name_check CHECK ((btrim(name) <> ''::text)),
    CONSTRAINT provider_rate_limit_per_min_check CHECK ((rate_limit_per_min > 0)),
    CONSTRAINT provider_region_check CHECK ((region = ANY (ARRAY['domestic'::text, 'overseas'::text]))),
    CONSTRAINT provider_revision_check CHECK ((revision > 0)),
    CONSTRAINT provider_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text, 'archived'::text])))
);

CREATE TABLE catalog.provider_check (
    model_id uuid,
    model_version_id uuid,
    model_result text DEFAULT '' NOT NULL,
    task_id text DEFAULT '' NOT NULL,
    dispatch_started_at timestamp with time zone,
    id uuid NOT NULL,
    actor_id uuid NOT NULL,
    provider_id uuid NOT NULL,
    config_version_id uuid NOT NULL,
    credential_id uuid,
    request_id uuid,
    network_result text NOT NULL DEFAULT '',
    http_status integer NOT NULL DEFAULT 0,
    elapsed_ms bigint NOT NULL DEFAULT 0,
    attempts integer NOT NULL DEFAULT 0,
    parent_check_id uuid,
    tracking_ended_at timestamp with time zone,
    tracking_note text NOT NULL DEFAULT '',
    kind text NOT NULL,
    status text NOT NULL,
    auth_result text DEFAULT 'unknown'::text NOT NULL,
    adapter_result text DEFAULT 'unknown'::text NOT NULL,
    discovery_result text DEFAULT 'unknown'::text NOT NULL,
    error_code text,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone,
    CONSTRAINT provider_check_kind_check CHECK ((kind = ANY (ARRAY['connectivity'::text, 'credential_auth'::text, 'reachability'::text, 'model_query'::text, 'discovery'::text, 'model_access'::text]))),
    CONSTRAINT provider_check_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'running'::text, 'completed'::text, 'failed'::text])))
);

CREATE TABLE catalog.provider_config_version (
    id uuid NOT NULL,
    provider_id uuid NOT NULL,
    preset_version_id uuid,
    version_no integer NOT NULL,
    config jsonb NOT NULL,
    content_hash text NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT provider_config_version_config_check CHECK ((jsonb_typeof(config) = 'object'::text)),
    CONSTRAINT provider_config_version_content_hash_check CHECK ((length(content_hash) = 64)),
    CONSTRAINT provider_config_version_version_no_check CHECK ((version_no > 0))
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
    CONSTRAINT provider_credential_status_check CHECK ((status = ANY (ARRAY['active'::text, 'draining'::text, 'disabled'::text])))
);

CREATE TABLE catalog.provider_preset (
    id uuid NOT NULL,
    slug text NOT NULL,
    origin text NOT NULL,
    parent_id uuid,
    archived boolean DEFAULT false NOT NULL,
    emergency_disabled boolean DEFAULT false NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT provider_preset_origin_check CHECK ((origin = ANY (ARRAY['builtin'::text, 'custom'::text]))),
    CONSTRAINT provider_preset_revision_check CHECK ((revision > 0)),
    CONSTRAINT provider_preset_slug_check CHECK ((slug ~ '^[a-z][a-z0-9_-]{0,63}$'::text))
);

CREATE TABLE catalog.provider_preset_version (
    id uuid NOT NULL,
    preset_id uuid NOT NULL,
    version_no integer NOT NULL,
    payload jsonb NOT NULL,
    content_hash text NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT provider_preset_version_content_hash_check CHECK ((length(content_hash) = 64)),
    CONSTRAINT provider_preset_version_payload_check CHECK ((jsonb_typeof(payload) = 'object'::text)),
    CONSTRAINT provider_preset_version_version_no_check CHECK ((version_no > 0))
);


CREATE TABLE identity.auth_challenge (
    id uuid NOT NULL,
    operation_id uuid NOT NULL,
    purpose text NOT NULL,
    email_key text NOT NULL,
    user_id uuid,
    email_version bigint,
    credential_epoch bigint,
    proof_digest bytea NOT NULL,
    proof_key_id text NOT NULL,
    flow_secret_digest bytea,
    attempts smallint DEFAULT 0 NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    invitation_id uuid,
    invitation_revision bigint,
    CONSTRAINT auth_challenge_attempts_check CHECK (((attempts >= 0) AND (attempts <= 5))),
    CONSTRAINT auth_challenge_check CHECK ((expires_at > created_at)),
    CONSTRAINT auth_challenge_credential_epoch_check CHECK ((credential_epoch > 0)),
    CONSTRAINT auth_challenge_email_version_check CHECK ((email_version > 0)),
    CONSTRAINT auth_challenge_flow_secret_digest_check CHECK ((octet_length(flow_secret_digest) = 32)),
    CONSTRAINT auth_challenge_invitation_revision_check CHECK ((invitation_revision > 0)),
    CONSTRAINT auth_challenge_proof_digest_check CHECK ((octet_length(proof_digest) = 32)),
    CONSTRAINT auth_challenge_purpose_check CHECK ((purpose = ANY (ARRAY['registration_code'::text, 'registration_grant'::text, 'password_reset'::text, 'email_old'::text, 'email_new'::text, 'deletion_confirm'::text, 'migration_bind'::text])))
);

CREATE TABLE identity.auth_event (
    id uuid NOT NULL,
    account_id uuid,
    action text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT auth_event_action_check CHECK ((action = ANY (ARRAY['registered'::text, 'logged_in'::text, 'login_failed'::text, 'logged_out'::text, 'password_changed'::text])))
);

CREATE TABLE identity.authority_proposal (
    id uuid NOT NULL,
    kind text NOT NULL,
    team_id uuid,
    source_user_id uuid NOT NULL,
    target_user_id uuid NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    reason text NOT NULL,
    source_authority_revision bigint NOT NULL,
    target_revision bigint NOT NULL,
    team_revision bigint,
    revision bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    target_email_version bigint DEFAULT 1 NOT NULL,
    target_credential_epoch bigint DEFAULT 1 NOT NULL,
    CONSTRAINT authority_proposal_check CHECK (((kind = 'platform_grant'::text) = (team_id IS NULL))),
    CONSTRAINT authority_proposal_check1 CHECK (((team_id IS NULL) = (team_revision IS NULL))),
    CONSTRAINT authority_proposal_check2 CHECK ((source_user_id <> target_user_id)),
    CONSTRAINT authority_proposal_kind_check CHECK ((kind = ANY (ARRAY['platform_grant'::text, 'team_transfer'::text, 'platform_takeover'::text]))),
    CONSTRAINT authority_proposal_reason_check CHECK (((char_length(reason) >= 5) AND (char_length(reason) <= 500))),
    CONSTRAINT authority_proposal_revision_check CHECK ((revision > 0)),
    CONSTRAINT authority_proposal_source_authority_revision_check CHECK ((source_authority_revision > 0)),
    CONSTRAINT authority_proposal_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'accepted'::text, 'declined'::text, 'cancelled'::text, 'expired'::text]))),
    CONSTRAINT authority_proposal_target_credential_epoch_check CHECK ((target_credential_epoch > 0)),
    CONSTRAINT authority_proposal_target_email_version_check CHECK ((target_email_version > 0)),
    CONSTRAINT authority_proposal_target_revision_check CHECK ((target_revision > 0))
);

CREATE TABLE identity.directory_export (
    id uuid NOT NULL,
    actor_id uuid NOT NULL,
    team_id uuid,
    scope_kind text NOT NULL,
    scope_id uuid NOT NULL,
    request_key uuid NOT NULL,
    actor_session_id uuid NOT NULL,
    credential_epoch bigint NOT NULL,
    platform_epoch bigint NOT NULL,
    team_revision bigint DEFAULT 0 NOT NULL,
    filter_snapshot jsonb NOT NULL,
    snapshot_payload bytea,
    snapshot_key_id text NOT NULL,
    as_of timestamp with time zone NOT NULL,
    status text NOT NULL,
    row_count integer NOT NULL,
    private_object_id uuid,
    storage_location_id uuid,
    storage_intent_id uuid,
    storage_profile_id text,
    storage_bucket text,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    error_category text DEFAULT ''::text NOT NULL,
    CONSTRAINT directory_export_check CHECK ((((scope_kind = 'platform'::text) AND (team_id IS NULL) AND (scope_id = '00000000-0000-0000-0000-000000000000'::uuid)) OR ((scope_kind = 'team'::text) AND (team_id IS NOT NULL) AND (scope_id = team_id)))),
    CONSTRAINT directory_export_storage_identity_check CHECK (((private_object_id IS NULL) AND (storage_location_id IS NULL) AND (storage_intent_id IS NULL) AND (storage_profile_id IS NULL) AND (storage_bucket IS NULL)) OR ((private_object_id IS NOT NULL) AND (storage_location_id IS NOT NULL) AND (storage_intent_id IS NOT NULL) AND (storage_profile_id IS NOT NULL) AND (storage_bucket IS NOT NULL))),
    CONSTRAINT directory_export_row_count_check CHECK (((row_count >= 0) AND (row_count <= 10000))),
    CONSTRAINT directory_export_scope_kind_check CHECK ((scope_kind = ANY (ARRAY['platform'::text, 'team'::text]))),
    CONSTRAINT directory_export_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'running'::text, 'completed'::text, 'failed'::text, 'expired'::text, 'revoked'::text])))
);

CREATE TABLE identity.email_budget (
    bucket_key text NOT NULL,
    sent_at timestamp with time zone[] DEFAULT '{}'::timestamp with time zone[] NOT NULL,
    failed_at timestamp with time zone[] DEFAULT '{}'::timestamp with time zone[] NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    CONSTRAINT email_budget_revision_check CHECK ((revision > 0))
);

CREATE TABLE identity.invitation (
    id uuid NOT NULL,
    kind text NOT NULL,
    team_id uuid,
    email_key text NOT NULL,
    team_role text,
    inviter_id uuid NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    token_hash bytea NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    accepted_user_id uuid,
    membership_revision bigint,
    revision bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    platform_issued boolean DEFAULT false NOT NULL,
    CONSTRAINT invitation_check CHECK ((((kind = 'account'::text) AND (team_id IS NULL) AND (team_role IS NULL)) OR ((kind = 'team'::text) AND (team_id IS NOT NULL) AND (team_role IS NOT NULL)))),
    CONSTRAINT invitation_kind_check CHECK ((kind = ANY (ARRAY['account'::text, 'team'::text]))),
    CONSTRAINT invitation_revision_check CHECK ((revision > 0)),
    CONSTRAINT invitation_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'accepted'::text, 'declined'::text, 'revoked'::text, 'expired'::text]))),
    CONSTRAINT invitation_team_role_check CHECK ((team_role = ANY (ARRAY['member'::text, 'team_admin'::text]))),
    CONSTRAINT invitation_token_hash_check CHECK ((octet_length(token_hash) = 32))
);

CREATE TABLE identity.login_guard (
    login_key text NOT NULL,
    failure_times timestamp with time zone[] DEFAULT '{}'::timestamp with time zone[] NOT NULL,
    locked_until timestamp with time zone,
    CONSTRAINT login_guard_failure_times_check CHECK ((cardinality(failure_times) <= 5)),
    CONSTRAINT login_guard_login_key_check CHECK ((login_key ~ '^[a-z0-9._-]{1,64}$'::text))
);

CREATE TABLE identity.mail_delivery (
    event_id uuid NOT NULL,
    challenge_id uuid,
    template text NOT NULL,
    recipient text NOT NULL,
    encrypted_payload bytea,
    key_id text,
    status text DEFAULT 'pending'::text NOT NULL,
    attempt_count integer DEFAULT 0 NOT NULL,
    next_attempt_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    lease_until timestamp with time zone,
    provider_message_id text,
    error_category text,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    invitation_id uuid,
    invitation_revision bigint,
    CONSTRAINT mail_delivery_attempt_count_check CHECK ((attempt_count >= 0)),
    CONSTRAINT mail_delivery_check CHECK (((encrypted_payload IS NULL) = (key_id IS NULL))),
    CONSTRAINT mail_delivery_invitation_revision_check CHECK ((invitation_revision > 0)),
    CONSTRAINT mail_delivery_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'leased'::text, 'submitted'::text, 'delivered'::text, 'bounced'::text, 'failed'::text, 'expired'::text])))
);

CREATE TABLE identity.platform_settings (
    singleton boolean DEFAULT true NOT NULL,
    registration_enabled boolean DEFAULT false NOT NULL,
    team_creation_enabled boolean DEFAULT false NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    updated_by uuid,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT platform_settings_revision_check CHECK ((revision > 0)),
    CONSTRAINT platform_settings_singleton_check CHECK (singleton)
);

CREATE TABLE identity.security_event (
    id uuid NOT NULL,
    actor_id uuid,
    target_user_id uuid,
    team_id uuid,
    scope_kind text NOT NULL,
    action text NOT NULL,
    result text NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    request_id text DEFAULT ''::text NOT NULL,
    safe_diff jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT security_event_safe_diff_check CHECK ((jsonb_typeof(safe_diff) = 'object'::text))
);

CREATE TABLE identity.session_refresh_token (
    id uuid NOT NULL,
    session_id uuid NOT NULL,
    generation bigint NOT NULL,
    token_digest bytea NOT NULL,
    issued_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone,
    revoked_at timestamp with time zone,
    rotation_request_id uuid,
    replay_until timestamp with time zone,
    receipt_ciphertext bytea,
    CONSTRAINT session_refresh_token_check CHECK (((consumed_at IS NULL) = (rotation_request_id IS NULL))),
    CONSTRAINT session_refresh_token_check1 CHECK (((replay_until IS NULL) = (receipt_ciphertext IS NULL))),
    CONSTRAINT session_refresh_token_check2 CHECK (((replay_until IS NULL) OR ((consumed_at IS NOT NULL) AND (replay_until > consumed_at) AND (replay_until <= (consumed_at + '00:00:10'::interval)) AND (octet_length(receipt_ciphertext) > 0)))),
    CONSTRAINT session_refresh_token_check3 CHECK (((consumed_at IS NULL) OR (consumed_at >= issued_at))),
    CONSTRAINT session_refresh_token_check4 CHECK (((revoked_at IS NULL) OR (revoked_at >= issued_at))),
    CONSTRAINT session_refresh_token_generation_check CHECK ((generation >= 0)),
    CONSTRAINT session_refresh_token_token_digest_check CHECK ((octet_length(token_digest) = 32))
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
    email_key text,
    email_verified_at timestamp with time zone,
    email_version bigint DEFAULT 1 NOT NULL,
    platform_role text DEFAULT 'creator'::text NOT NULL,
    platform_epoch bigint DEFAULT 1 NOT NULL,
    avatar_asset_id uuid,
    terms_version text,
    terms_accepted_at timestamp with time zone,
    theme text DEFAULT 'dark'::text NOT NULL,
    prompt_suffix text DEFAULT ''::text NOT NULL,
    bio text DEFAULT '' NOT NULL,
    canvas_background text DEFAULT 'dots' NOT NULL,
    CONSTRAINT user_bio_check CHECK (char_length(bio) <= 100),
    CONSTRAINT user_canvas_background_check CHECK (canvas_background IN ('dots','lines','blank')),
    deleted_at timestamp with time zone,
    CONSTRAINT user_display_name_check CHECK (((char_length(display_name) >= 1) AND (char_length(display_name) <= 50))),
    CONSTRAINT user_email_key_check CHECK (((email_key IS NULL) OR (((octet_length(email_key) >= 3) AND (octet_length(email_key) <= 254)) AND (email_key = lower(btrim(email_key))) AND (email_key !~ '[^\x21-\x7e]'::text) AND (email_key ~ '^[^@]+@[^@]+$'::text)))),
    CONSTRAINT user_email_proof_check CHECK (((email_key IS NULL) = (email_verified_at IS NULL))),
    CONSTRAINT user_email_version_check CHECK ((email_version > 0)),
    CONSTRAINT user_failed_login_count_check CHECK ((failed_login_count >= 0)),
    CONSTRAINT user_platform_epoch_check CHECK ((platform_epoch > 0)),
    CONSTRAINT user_platform_role_check CHECK ((platform_role = ANY (ARRAY['creator'::text, 'platform_admin'::text]))),
    CONSTRAINT user_prompt_suffix_check CHECK ((char_length(prompt_suffix) <= 2000)),
    CONSTRAINT user_revision_check CHECK ((revision > 0)),
    CONSTRAINT user_role_check CHECK ((role = ANY (ARRAY['admin'::text, 'producer'::text]))),
    CONSTRAINT user_session_epoch_check CHECK ((session_epoch > 0)),
    CONSTRAINT user_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text]))),
    CONSTRAINT user_terms_check CHECK (((terms_version IS NULL) = (terms_accepted_at IS NULL))),
    CONSTRAINT user_theme_check CHECK ((theme = ANY (ARRAY['light'::text, 'dark'::text, 'system'::text])))
);

CREATE TABLE identity.user_session (
    id uuid NOT NULL,
    account_id uuid NOT NULL,
    token_hash bytea NOT NULL,
    credential_revision bigint NOT NULL,
    created_at timestamp with time zone NOT NULL,
    last_active_at timestamp with time zone NOT NULL,
    absolute_expires_at timestamp with time zone NOT NULL,
    persistent boolean NOT NULL,
    protocol_version smallint DEFAULT 1 NOT NULL,
    idle_timeout_seconds integer DEFAULT 43200 NOT NULL,
    refresh_generation bigint DEFAULT 0 NOT NULL,
    revoked_at timestamp with time zone,
    device_summary text DEFAULT ''::text NOT NULL,
    reauth_at timestamp with time zone,
    reauth_epoch bigint,
    csrf_hash bytea,
    CONSTRAINT user_session_check CHECK (((last_active_at >= created_at) AND (absolute_expires_at > created_at))),
    CONSTRAINT user_session_credential_revision_check CHECK ((credential_revision > 0)),
    CONSTRAINT user_session_csrf_hash_check CHECK ((octet_length(csrf_hash) = 32)),
    CONSTRAINT user_session_device_summary_check CHECK ((char_length(device_summary) <= 256)),
    CONSTRAINT user_session_idle_timeout_seconds_check CHECK ((idle_timeout_seconds > 0)),
    CONSTRAINT user_session_protocol_version_check CHECK ((protocol_version = ANY (ARRAY[1, 2]))),
    CONSTRAINT user_session_reauth_check CHECK (((reauth_at IS NULL) = (reauth_epoch IS NULL))),
    CONSTRAINT user_session_reauth_epoch_check CHECK ((reauth_epoch > 0)),
    CONSTRAINT user_session_refresh_generation_check CHECK ((refresh_generation >= 0)),
    CONSTRAINT user_session_token_hash_check CHECK ((octet_length(token_hash) = 32))
);

-- Identity-owned original revocation commands outlive account/session cleanup.
-- No account/session FK: the source transaction validates ownership while holding
-- the account, actor-session and target-session policy locks. Recovery binds only
-- to these durable IDs; deleting a session must never erase source responsibility
-- or permit that session's revocation history to be recreated under a new command.
CREATE TABLE identity.session_revocation (
    session_id uuid PRIMARY KEY CHECK (session_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    account_id uuid NOT NULL CHECK (account_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    request_session_id uuid NOT NULL CHECK (request_session_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    coordination_id uuid NOT NULL UNIQUE CHECK (coordination_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    expected_epoch bigint NOT NULL CHECK (expected_epoch >= 0),
    requested_at timestamptz NOT NULL,
    completed_at timestamptz CHECK (completed_at >= requested_at)
);

-- Accepted public intents and their original evidence outlive ordinary credential
-- cleanup. Only same-owner durable journals are referenced; no account/session FK
-- can erase or prevent recovery of a previously committed responsibility.
CREATE TABLE identity.session_revocation_request (
    id uuid PRIMARY KEY CHECK (id <> '00000000-0000-0000-0000-000000000000'::uuid),
    idem_key uuid NOT NULL CHECK (idem_key <> '00000000-0000-0000-0000-000000000000'::uuid),
    account_id uuid NOT NULL CHECK (account_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    request_session_id uuid NOT NULL CHECK (request_session_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    action text NOT NULL CHECK (action IN ('revoke_session','revoke_others','refresh_reuse')),
    target_session_id uuid CHECK (target_session_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    requested_at timestamptz NOT NULL,
    completed_at timestamptz CHECK (completed_at >= requested_at),
    UNIQUE (account_id,request_session_id,idem_key),
    CHECK ((action='revoke_session' AND target_session_id IS NOT NULL) OR
           (action='revoke_others' AND target_session_id IS NULL) OR
           (action='refresh_reuse' AND target_session_id IS NOT NULL AND target_session_id=request_session_id))
);

CREATE TABLE identity.session_revocation_request_target (
    request_id uuid NOT NULL REFERENCES identity.session_revocation_request(id),
    session_id uuid NOT NULL REFERENCES identity.session_revocation(session_id),
    PRIMARY KEY (request_id,session_id)
);

CREATE TABLE identity.session_revocation_request_proof (
    request_id uuid NOT NULL REFERENCES identity.session_revocation_request(id),
    kind text NOT NULL CHECK (kind IN ('legacy','access','refresh')),
    token_digest bytea NOT NULL CHECK (octet_length(token_digest)=32),
    csrf_hash bytea NOT NULL CHECK (octet_length(csrf_hash)=32),
    PRIMARY KEY (request_id,kind)
);
CREATE INDEX session_revocation_request_proof_digest_idx
    ON identity.session_revocation_request_proof(kind,token_digest);

CREATE TABLE infra.idempotency_record (
    service_owner text NOT NULL DEFAULT infra.runtime_owner() CHECK (service_owner IN ('legacy','identity','creation','media','execution')),
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
    service_owner text NOT NULL DEFAULT infra.runtime_owner() CHECK (service_owner IN ('legacy','identity','creation','media','execution')),
    id uuid NOT NULL,
    topic text NOT NULL,
    partition_key text NOT NULL,
    payload jsonb NOT NULL,
    headers jsonb DEFAULT '{}'::jsonb NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    published_at timestamp with time zone,
    attempt_count integer DEFAULT 0 NOT NULL CHECK (attempt_count >= 0),
    next_attempt_at timestamp with time zone DEFAULT now() NOT NULL,
    claim_token uuid,
    lease_until timestamp with time zone,
    last_error_class text,
    dead_lettered_at timestamp with time zone,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL
)
PARTITION BY RANGE (create_time);

CREATE TABLE infra.outbox_dead_letter (
    service_owner text NOT NULL DEFAULT infra.runtime_owner() CHECK (service_owner IN ('legacy','identity','creation','media','execution')),
    event_id uuid NOT NULL,
    event_create_time timestamptz NOT NULL,
    topic text NOT NULL,
    partition_key text NOT NULL,
    payload jsonb NOT NULL,
    headers jsonb NOT NULL DEFAULT '{}'::jsonb,
    attempt_count integer NOT NULL CHECK (attempt_count > 0),
    error_class text NOT NULL,
    failed_at timestamptz NOT NULL,
    replayed_at timestamptz,
    PRIMARY KEY (service_owner, event_id, event_create_time)
);

CREATE TABLE infra.outbox_dead_letter_audit (
    service_owner text NOT NULL DEFAULT infra.runtime_owner() CHECK (service_owner IN ('legacy','identity','creation','media','execution')),
    id uuid NOT NULL,
    PRIMARY KEY (service_owner, id),
    event_id uuid NOT NULL,
    event_create_time timestamptz NOT NULL,
    operator_id uuid NOT NULL,
    reason text NOT NULL CHECK (length(reason) BETWEEN 1 AND 500),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE infra.processed_event (
    service_owner text NOT NULL DEFAULT infra.runtime_owner() CHECK (service_owner IN ('legacy','identity','creation','media','execution')),
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
    CONSTRAINT library_item_title_check CHECK (((char_length(title) >= 1) AND (char_length(title) <= 240) AND (btrim(title) <> ''::text)))
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
    source_ai_request_id uuid,
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
    object_location_id uuid NOT NULL,
    CONSTRAINT media_asset_byte_size_check CHECK ((byte_size >= 0)),
    CONSTRAINT media_asset_kind_check CHECK ((kind = ANY (ARRAY['image'::text, 'video'::text, 'audio'::text, 'document'::text, 'model'::text]))),
    CONSTRAINT media_asset_model_facts_check CHECK (((kind <> 'model'::text) OR ((origin = 'upload'::text) AND (((mime_type = 'model/gltf-binary'::text) AND (codec = 'glb2'::text)) OR ((mime_type = 'model/gltf+json'::text) AND (codec = 'gltf2'::text))) AND ((byte_size >= 1) AND (byte_size <= 67108864)) AND (codec IS NOT NULL) AND (width IS NULL) AND (height IS NULL) AND (duration_ms IS NULL) AND (fps IS NULL) AND (audio_channels IS NULL)))),
    CONSTRAINT media_asset_moderation_status_check CHECK ((moderation_status = ANY (ARRAY['pending'::text, 'passed'::text, 'rejected'::text, 'skipped'::text]))),
    CONSTRAINT media_asset_generated_source_check CHECK (
 (origin='generated' AND num_nonnulls(source_operation_id,source_ai_request_id)=1 AND provider_key IS NOT NULL AND model_key IS NOT NULL AND region IS NOT NULL)
 OR (origin<>'generated' AND source_operation_id IS NULL AND source_ai_request_id IS NULL)),
    CONSTRAINT media_asset_origin_check CHECK ((origin = ANY (ARRAY['upload'::text, 'generated'::text, 'system'::text]))),
    CONSTRAINT media_asset_ownership_check CHECK ((((project_id IS NOT NULL) AND (project_id <> '00000000-0000-0000-0000-000000000000'::uuid) AND (personal_org_id IS NULL) AND (personal_actor_id IS NULL)) OR ((project_id IS NULL) AND (personal_org_id IS NOT NULL) AND (personal_actor_id IS NOT NULL) AND (personal_org_id <> '00000000-0000-0000-0000-000000000000'::uuid) AND (personal_actor_id <> '00000000-0000-0000-0000-000000000000'::uuid) AND (origin = ANY (ARRAY['upload'::text, 'system'::text])) AND (source_operation_id IS NULL) AND (source_ai_request_id IS NULL) AND (provider_key IS NULL) AND (model_key IS NULL) AND (region IS NULL)))),
    CONSTRAINT media_asset_region_check CHECK ((region = ANY (ARRAY['domestic'::text, 'overseas'::text]))),
    CONSTRAINT media_asset_revision_check CHECK ((revision > 0)),
    CONSTRAINT media_asset_status_check CHECK ((status = ANY (ARRAY['uploading'::text, 'processing'::text, 'ready'::text, 'rejected'::text, 'failed'::text])))
);

CREATE TABLE media.object_location (
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
    object_location_id uuid NOT NULL,
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
    source_location_id uuid NOT NULL,
    target_location_id uuid NOT NULL,
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
    object_location_id uuid NOT NULL,
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
    object_location_id uuid NOT NULL,
    CONSTRAINT rendition_kind_check CHECK ((kind = ANY (ARRAY['thumb_256'::text, 'thumb_640'::text, 'poster'::text, 'proxy_720p'::text, 'waveform'::text])))
);

CREATE TABLE media.storage_bucket_evidence (
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

CREATE TABLE media.storage_config_revision (
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

CREATE TABLE media.storage_probe_evidence (
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

CREATE TABLE media.storage_probe_intent (
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

CREATE TABLE media.storage_profile (
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
    CONSTRAINT storage_profile_provider_check CHECK (provider = 'cos'),
    CONSTRAINT storage_profile_resource_identity_check CHECK ((resource_identity ~ '^[a-f0-9]{64}$'::text))
);

CREATE TABLE media.storage_route (
    singleton integer DEFAULT 1 NOT NULL,
    revision text NOT NULL,
    profile_id text NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT storage_route_revision_check CHECK ((revision ~ '^[1-9][0-9]{0,19}$'::text)),
    CONSTRAINT storage_route_singleton_check CHECK ((singleton = 1))
);

CREATE TABLE media.storage_route_lock (
    singleton integer NOT NULL,
    CONSTRAINT storage_route_lock_singleton_check CHECK ((singleton = 1))
);

INSERT INTO media.storage_route_lock(singleton) VALUES (1);

CREATE TABLE media.storage_work_intent (
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
    CONSTRAINT storage_work_intent_kind_check CHECK ((kind = ANY (ARRAY['operation'::text, 'script'::text, 'export'::text, 'depth'::text, 'transcription'::text, 'package'::text, 'transfer'::text, 'project-copy'::text, 'subject-voice'::text, 'avatar'::text]))),
    CONSTRAINT storage_work_intent_owner_id_check CHECK ((owner_id <> '00000000-0000-0000-0000-000000000000'::uuid)),
    CONSTRAINT storage_work_intent_state_check CHECK ((state = ANY (ARRAY['open'::text, 'accepted'::text, 'cancelled'::text]))),
    CONSTRAINT storage_work_intent_work_id_check CHECK ((work_id <> '00000000-0000-0000-0000-000000000000'::uuid))
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
    source_scope_id uuid,
    target_scope_id uuid,
    source_org_id uuid,
    target_org_id uuid,
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
    output_attempt integer NOT NULL CHECK (output_attempt > 0),
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
    source_location_id uuid NOT NULL,
    target_location_id uuid NOT NULL,
    CONSTRAINT transfer_object_byte_size_check CHECK (((byte_size >= 1) AND (byte_size <= '2147483648'::bigint))),
    CONSTRAINT transfer_object_rendition_kind_check CHECK ((rendition_kind = ANY (ARRAY[''::text, 'thumb_256'::text, 'thumb_640'::text, 'poster'::text, 'proxy_720p'::text, 'waveform'::text]))),
    CONSTRAINT transfer_object_sha256_check CHECK ((sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT transfer_object_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'verified'::text, 'removed'::text])))
);

CREATE TABLE media.upload_admission (
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

CREATE TABLE media.upload_request (
    project_id uuid,
    principal_id uuid NOT NULL,
    request_key uuid NOT NULL,
    sha256 text NOT NULL,
    file_name text NOT NULL,
    source_kind text DEFAULT 'local'::text NOT NULL,
    byte_size bigint NOT NULL,
    asset_id uuid NOT NULL,
    response jsonb NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    personal_org_id uuid,
    CONSTRAINT upload_request_byte_size_check CHECK (((byte_size >= 1) AND (byte_size <= 524288000))),
    CONSTRAINT upload_request_file_name_check CHECK (((octet_length(file_name) >= 1) AND (octet_length(file_name) <= 255))),
    CONSTRAINT upload_request_source_kind_check CHECK ((source_kind IN ('local','video_link')) AND (source_kind <> 'video_link' OR project_id IS NULL)),
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
    actor_role text,
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
    object_location_id uuid NOT NULL,
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
    actor_role text,
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
    artifact_id uuid,
    CONSTRAINT export_job_actor_role_check CHECK ((actor_role = ANY (ARRAY['admin'::text, 'producer'::text]))),
    CONSTRAINT export_job_attempt_check CHECK (((attempt >= 1) AND (attempt <= 100))),
    CONSTRAINT export_job_check CHECK((asset_id IS NULL AND artifact_id IS NULL AND sha256 IS NULL) OR (num_nonnulls(asset_id,artifact_id)=1 AND sha256 IS NOT NULL)),
    CONSTRAINT export_job_check1 CHECK(status NOT IN ('review_required','succeeded') OR num_nonnulls(asset_id,artifact_id)=1),
    CONSTRAINT export_job_check2 CHECK (((status <> 'succeeded'::text) OR (progress = 100))),
    CONSTRAINT export_job_frozen_check CHECK (((jsonb_typeof(frozen) = 'object'::text) AND (octet_length((frozen)::text) <= 1048576))),
    CONSTRAINT export_job_output_kind_check CHECK(output_kind IN ('video','audio','media_bundle','fcpxml','xmeml','jianying')),
    CONSTRAINT export_job_output_identity_check CHECK((output_kind IN ('video','audio') AND artifact_id IS NULL) OR (output_kind IN ('media_bundle','fcpxml','xmeml','jianying') AND asset_id IS NULL)),
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
    actor_role text,
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
    CONSTRAINT ck_operation_canvas_source CHECK (
        source_context IS NULL OR (
            target_type = 'free' AND origin = 'canvas'
            AND jsonb_typeof(source_context) = 'object'
            AND source_context ?& ARRAY['canvas_id', 'node_id', 'revision']
            AND source_context - ARRAY['canvas_id', 'node_id', 'row_id', 'revision', 'shot_id', 'shot_revision'] = '{}'::jsonb
            AND jsonb_typeof(source_context->'canvas_id') = 'string'
            AND jsonb_typeof(source_context->'node_id') = 'string'
            AND jsonb_typeof(source_context->'revision') = 'number'
            AND (source_context->>'revision')::bigint > 0
            AND (NOT (source_context ? 'row_id') OR jsonb_typeof(source_context->'row_id') = 'string')
            AND ((NOT (source_context ?| ARRAY['shot_id', 'shot_revision'])) OR (
                source_context ?& ARRAY['shot_id', 'shot_revision']
                AND NOT (source_context ? 'row_id')
                AND jsonb_typeof(source_context->'shot_id') = 'string'
                AND source_context->>'shot_id' ~ '^[a-f0-9]{8}(-[a-f0-9]{4}){3}-[a-f0-9]{12}$'
                AND source_context->>'shot_id' <> '00000000-0000-0000-0000-000000000000'
                AND jsonb_typeof(source_context->'shot_revision') = 'number'
                AND (source_context->>'shot_revision')::bigint > 0
            ))
        )
    ),
    CONSTRAINT ck_operation_prompt_preparation CHECK (((prompt_preparation IS NULL) OR (((jsonb_typeof(prompt_preparation) = 'object'::text) AND (octet_length((prompt_preparation)::text) <= 4096) AND ((prompt_preparation - ARRAY['version'::text, 'operation'::text, 'policy'::text, 'template_id'::text, 'template_version'::text, 'customization_id'::text, 'customization_revision'::text, 'request_sha256'::text, 'user_prompt_sha256'::text, 'content_sha256'::text]) = '{}'::jsonb) AND ((prompt_preparation -> 'version'::text) = '1'::jsonb) AND (origin = 'canvas'::text) AND (target_type = 'free'::text) AND ((prompt_preparation ->> 'operation'::text) = ANY (ARRAY['chapter_assets_extract'::text, 'character_extract'::text, 'character_turnaround'::text, 'storyboard_plan'::text, 'storyboard_repair'::text, 'storyboard_first_frame'::text, 'storyboard_video'::text, 'short_drama_outline'::text, 'skill_draft'::text])) AND ((prompt_preparation ->> 'request_sha256'::text) ~ '^[0-9a-f]{64}$'::text) AND ((prompt_preparation ->> 'user_prompt_sha256'::text) ~ '^[0-9a-f]{64}$'::text) AND ((prompt_preparation ->> 'content_sha256'::text) ~ '^[0-9a-f]{64}$'::text) AND ((((prompt_preparation ->> 'operation'::text) = ANY (ARRAY['character_turnaround'::text, 'storyboard_first_frame'::text])) AND (capability = 'image.generate'::text)) OR (((prompt_preparation ->> 'operation'::text) = 'storyboard_video'::text) AND (capability = 'video.generate'::text)) OR (((prompt_preparation ->> 'operation'::text) <> ALL (ARRAY['character_turnaround'::text, 'storyboard_first_frame'::text, 'storyboard_video'::text])) AND (capability = 'text.structured'::text))) AND ((((prompt_preparation ->> 'policy'::text) = 'bypass_video'::text) AND ((prompt_preparation ->> 'operation'::text) = 'storyboard_video'::text) AND (NOT (prompt_preparation ?| ARRAY['template_id'::text, 'template_version'::text, 'customization_id'::text, 'customization_revision'::text])) AND ((prompt_preparation ->> 'content_sha256'::text) = (prompt_preparation ->> 'user_prompt_sha256'::text))) OR (((prompt_preparation ->> 'policy'::text) = 'compiled'::text) AND ((prompt_preparation ->> 'operation'::text) <> 'storyboard_video'::text) AND ((prompt_preparation ->> 'template_id'::text) ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'::text) AND ((prompt_preparation ->> 'template_id'::text) <> '00000000-0000-0000-0000-000000000000'::text) AND ((prompt_preparation -> 'template_version'::text) = '1'::jsonb) AND (((NOT (prompt_preparation ? 'customization_id'::text)) AND (NOT (prompt_preparation ? 'customization_revision'::text))) OR (((prompt_preparation ->> 'customization_id'::text) ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'::text) AND ((prompt_preparation ->> 'customization_id'::text) <> '00000000-0000-0000-0000-000000000000'::text) AND (jsonb_typeof((prompt_preparation -> 'customization_revision'::text)) = 'number'::text) AND ((prompt_preparation ->> 'customization_revision'::text) ~ '^[1-9][0-9]{0,9}$'::text) AND (((prompt_preparation ->> 'customization_revision'::text))::numeric <= (2147483647)::numeric)))))) IS TRUE))),
    CONSTRAINT ck_operation_quote_amount CHECK (((status <> 'quoted'::text) OR ((quote_micros IS NOT NULL) AND (quote_expires_at IS NOT NULL) AND (quote_expires_at > create_time)))),
    CONSTRAINT ck_operation_reuse_self CHECK ((reused_from_id IS DISTINCT FROM id)),
    CONSTRAINT ck_operation_upload CHECK (((origin = 'upload'::text) OR ((region IS NOT NULL) AND (model_profile_version_id IS NOT NULL)))),
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
    CONSTRAINT operation_target_type_check CHECK ((target_type = ANY (ARRAY['shot_frame'::text, 'shot_take'::text, 'reference_slot'::text, 'dialogue_audio'::text, 'voice_preview'::text, 'episode_split'::text, 'episode_parse'::text, 'bible_extract'::text, 'scene_storyboard'::text, 'free'::text])))
);

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
    CONSTRAINT operation_input_subject_binding_check CHECK (ref_type <> 'subject_media' OR (ref_id IS NOT NULL AND ref_version IS NOT NULL AND ref_version ~ '^[a-f0-9]{64}$' AND media_asset_id IS NOT NULL AND text_value IS NULL AND mask_asset_id IS NULL AND role <> 'prompt')),
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

CREATE TABLE operation.prompt_resolution_receipt (
    actor_id uuid NOT NULL,
    request_key uuid NOT NULL,
    request_hash text NOT NULL,
    response jsonb NOT NULL
);

CREATE TABLE operation.prompt_submission (
    id uuid NOT NULL,
    actor_id uuid NOT NULL,
    project_id uuid NOT NULL,
    owner_key text NOT NULL,
    epoch bigint NOT NULL,
    operation_id uuid NOT NULL,
    confirm_key uuid NOT NULL,
    input_source jsonb NOT NULL,
    target_content_version bigint,
    CONSTRAINT ck_submission_result_target CHECK ((target_content_version IS NOT NULL) = (input_source->>'kind' IS NOT DISTINCT FROM 'canvas_input') AND target_content_version > 0),
    snapshot jsonb NOT NULL,
    quote jsonb NOT NULL,
    state text NOT NULL,
    resolution text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT prompt_submission_epoch_check CHECK ((epoch > 0)),
    CONSTRAINT prompt_submission_state_check CHECK ((state = ANY (ARRAY['quoted'::text, 'accepted'::text, 'closed'::text])))
);

CREATE TABLE operation.prompt_submission_slot (
    actor_id uuid NOT NULL,
    project_id uuid NOT NULL,
    owner_key text NOT NULL,
    epoch bigint DEFAULT 0 NOT NULL,
    submission_id uuid,
    CONSTRAINT prompt_submission_slot_epoch_check CHECK ((epoch >= 0))
);

CREATE TABLE operation.provider_binding (
    operation_id uuid NOT NULL,
    release_id uuid NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL
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

CREATE TABLE operation.provider_credential_binding (
    operation_id uuid NOT NULL,
    credential_id uuid NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE operation.provider_result (
    operation_id uuid NOT NULL,
    result jsonb NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT provider_result_result_check CHECK (((jsonb_typeof(result) = 'object'::text) AND (octet_length((result)::text) <= 2097152)))
);

CREATE TABLE operation.provider_trial (
    operation_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    max_charge_micros bigint NOT NULL,
    supplier_max_micros bigint NOT NULL,
    supplier_currency text NOT NULL,
    CONSTRAINT provider_trial_max_charge_micros_check CHECK ((max_charge_micros >= 0)),
    CONSTRAINT provider_trial_supplier_currency_check CHECK ((supplier_currency = ANY (ARRAY['CNY'::text, 'USD'::text]))),
    CONSTRAINT provider_trial_supplier_max_micros_check CHECK (((supplier_max_micros >= 0) AND (supplier_max_micros <= 1000000000)))
);

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
    CONSTRAINT command_plan_check CHECK ((jsonb_typeof(plan) = 'object'::text AND octet_length(plan::text) <= 113246208)),
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
    actor_role text,
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
    object_location_id uuid NOT NULL,
    CONSTRAINT object_intent_byte_size_check CHECK (((byte_size >= 0) AND (byte_size <= 37748736))),
    CONSTRAINT object_intent_sha256_check CHECK ((sha256 ~ '^[a-f0-9]{64}$'::text))
);

CREATE TABLE script.object_state (
    object_key text NOT NULL,
    put_started boolean DEFAULT false NOT NULL,
    confirmed boolean DEFAULT false NOT NULL,
    removed boolean DEFAULT false NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    object_location_id uuid NOT NULL
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

CREATE TABLE workspace.creation_scope (
    id uuid NOT NULL,
    kind text NOT NULL,
    owner_user_id uuid,
    team_id uuid,
    physical_scope_id uuid NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT creation_scope_check CHECK ((((kind = 'personal'::text) AND (owner_user_id IS NOT NULL) AND (team_id IS NULL)) OR ((kind = 'team'::text) AND (owner_user_id IS NULL) AND (team_id IS NOT NULL)))),
    CONSTRAINT creation_scope_kind_check CHECK ((kind = ANY (ARRAY['personal'::text, 'team'::text]))),
    CONSTRAINT creation_scope_revision_check CHECK ((revision > 0))
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
    owner_id uuid,
    initial_canvas_id uuid,
    style_key text,
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
    owner_scope_id uuid,
    CONSTRAINT ck_project_style CHECK (((style_type = 'stylized'::text) = (style_subtype IS NOT NULL))),
    CONSTRAINT project_aspect_ratio_check CHECK ((aspect_ratio = ANY (ARRAY['9:16'::text, '16:9'::text, '1:1'::text]))),
    CONSTRAINT project_cover_nonzero CHECK (((cover_asset_id IS NULL) OR (cover_asset_id <> '00000000-0000-0000-0000-000000000000'::uuid))),
    CONSTRAINT project_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 50))),
    CONSTRAINT project_resolution_check CHECK ((resolution = '1080p'::text)),
    CONSTRAINT project_revision_check CHECK ((revision > 0)),
    CONSTRAINT project_status_check CHECK ((status = ANY (ARRAY['active'::text, 'archived'::text, 'copying'::text]))),
    CONSTRAINT project_style_key_check CHECK (((style_key IS NULL) OR ((char_length(btrim(style_key)) >= 1) AND (char_length(btrim(style_key)) <= 50)))),
    CONSTRAINT project_style_subtype_check CHECK ((style_subtype = ANY (ARRAY['anime_jp'::text, 'guofeng_xianxia'::text, 'cartoon_3d'::text, 'manhwa'::text]))),
    CONSTRAINT project_style_type_check CHECK ((style_type = ANY (ARRAY['realistic'::text, 'stylized'::text])))
);

CREATE TABLE workspace.project_activity (
    project_id uuid NOT NULL,
    modified_at timestamp with time zone NOT NULL,
    basis text NOT NULL,
    CONSTRAINT project_activity_basis_check CHECK ((basis = ANY (ARRAY['created'::text, 'metadata'::text, 'content'::text, 'legacy_metadata'::text])))
);

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
    owner_scope_id uuid,
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

CREATE TABLE workspace.project_creation_receipt (
    actor_id uuid NOT NULL,
    idem_key uuid NOT NULL,
    request_hash text NOT NULL,
    project_id uuid NOT NULL,
    response_body jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT project_creation_receipt_request_hash_check CHECK ((request_hash ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT project_creation_receipt_response_body_check CHECK ((jsonb_typeof(response_body) = 'object'::text))
);

CREATE TABLE workspace.project_folder (
    owner_scope_id uuid,
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
    owner_scope_id uuid,
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
    owner_scope_id uuid,
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

CREATE TABLE workspace.prompt_draft (
    id uuid NOT NULL,
    project_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    revision bigint NOT NULL,
    draft jsonb NOT NULL,
    draft_hash text NOT NULL,
    input_hash text NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT prompt_draft_draft_check CHECK (((jsonb_typeof(draft) = 'object'::text) AND (octet_length((draft)::text) <= 1048576))),
    CONSTRAINT prompt_draft_draft_hash_check CHECK ((draft_hash ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT prompt_draft_input_hash_check CHECK ((input_hash ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT prompt_draft_revision_check CHECK (((revision >= 1) AND (revision <= 2147483647)))
);

CREATE TABLE workspace.prompt_draft_receipt (
    actor_id uuid NOT NULL,
    request_key uuid NOT NULL,
    project_id uuid NOT NULL,
    request_hash text NOT NULL,
    response jsonb NOT NULL
);

CREATE TABLE workspace.style_preset (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid,
    owner_user_id uuid,
    library_category text CHECK (library_category IN ('realistic','2d','3d')),
    revision bigint DEFAULT 1 NOT NULL CHECK (revision BETWEEN 1 AND 2147483647),
    name text NOT NULL,
    style_type text NOT NULL,
    style_subtype text,
    prompt_fragment text DEFAULT ''::text NOT NULL,
    negative_prompt text DEFAULT ''::text NOT NULL,
    reference_asset_ids uuid[] DEFAULT '{}'::uuid[] NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    is_delete boolean DEFAULT false NOT NULL,
    owner_scope_id uuid,
    CONSTRAINT style_preset_style_subtype_check CHECK ((style_subtype = ANY (ARRAY['anime_jp'::text, 'guofeng_xianxia'::text, 'cartoon_3d'::text, 'manhwa'::text]))),
    CONSTRAINT style_preset_style_type_check CHECK ((style_type = ANY (ARRAY['realistic'::text, 'stylized'::text])))
);

CREATE TABLE workspace.style_preset_command (
    actor_id uuid NOT NULL,
    idem_key uuid NOT NULL,
    org_id uuid NOT NULL,
    owner_scope_id uuid NOT NULL,
    preset_id uuid NOT NULL,
    action text NOT NULL CHECK (action IN ('create','patch','delete')),
    request_sha256 text NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
    response_body jsonb NOT NULL CHECK (jsonb_typeof(response_body)='object'),
    create_time timestamptz DEFAULT now() NOT NULL,
    PRIMARY KEY (actor_id,idem_key)
);

-- Keep every adopted image pinned after edits and soft deletion.
CREATE TABLE workspace.style_preset_reference (
    preset_id uuid NOT NULL,
    asset_id uuid NOT NULL,
    PRIMARY KEY(preset_id,asset_id)
);
CREATE INDEX style_preset_library_idx ON workspace.style_preset(owner_scope_id,owner_user_id,name,id) WHERE NOT is_delete AND project_id IS NULL;
CREATE INDEX style_preset_reference_asset_idx ON workspace.style_preset_reference(asset_id);
CREATE INDEX style_preset_media_refs_idx ON workspace.style_preset USING gin(reference_asset_ids);


CREATE TABLE workspace.team (
    id uuid NOT NULL,
    name text NOT NULL,
    avatar_asset_id uuid,
    created_by uuid NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    resume_status text,
    governance_blocked boolean DEFAULT false NOT NULL,
    auth_epoch bigint DEFAULT 1 NOT NULL,
    invitation_enabled boolean DEFAULT true NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT team_auth_epoch_check CHECK ((auth_epoch > 0)),
    CONSTRAINT team_check CHECK (((status = 'suspended'::text) = (resume_status IS NOT NULL))),
    CONSTRAINT team_name_check CHECK (((char_length(name) >= 2) AND (char_length(name) <= 50))),
    CONSTRAINT team_resume_status_check CHECK ((resume_status = ANY (ARRAY['active'::text, 'archived'::text]))),
    CONSTRAINT team_revision_check CHECK ((revision > 0)),
    CONSTRAINT team_status_check CHECK ((status = ANY (ARRAY['active'::text, 'archived'::text, 'suspended'::text])))
);

CREATE TABLE workspace.team_member (
    team_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role text NOT NULL,
    status text NOT NULL,
    auth_epoch bigint DEFAULT 1 NOT NULL,
    joined_at timestamp with time zone NOT NULL,
    joined_by uuid,
    source text NOT NULL,
    last_team_activity_at timestamp with time zone,
    revision bigint DEFAULT 1 NOT NULL,
    create_time timestamp with time zone DEFAULT now() NOT NULL,
    update_time timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT team_member_auth_epoch_check CHECK ((auth_epoch > 0)),
    CONSTRAINT team_member_revision_check CHECK ((revision > 0)),
    CONSTRAINT team_member_role_check CHECK ((role = ANY (ARRAY['team_admin'::text, 'member'::text]))),
    CONSTRAINT team_member_source_check CHECK ((source = ANY (ARRAY['created'::text, 'invited'::text, 'migrated'::text, 'transferred'::text]))),
    CONSTRAINT team_member_status_check CHECK ((status = ANY (ARRAY['active'::text, 'suspended'::text, 'removed'::text])))
);

ALTER TABLE ONLY audit.audit_log
    ADD CONSTRAINT audit_log_pkey PRIMARY KEY (service_owner, id, create_time);

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
    ADD CONSTRAINT document_id_project_unique UNIQUE (id, project_id);

ALTER TABLE ONLY canvas.document
    ADD CONSTRAINT document_pkey PRIMARY KEY (id);

ALTER TABLE ONLY canvas.edge
    ADD CONSTRAINT edge_pkey PRIMARY KEY (id);

ALTER TABLE ONLY canvas.node
    ADD CONSTRAINT node_document_id_id_key UNIQUE (document_id, id);

ALTER TABLE ONLY canvas.node_input
    ADD CONSTRAINT node_input_pkey PRIMARY KEY (node_id, input_revision);

ALTER TABLE ONLY canvas.node
    ADD CONSTRAINT node_pkey PRIMARY KEY (id);

ALTER TABLE ONLY canvas.project_copy_receipt
    ADD CONSTRAINT project_copy_receipt_pkey PRIMARY KEY (snapshot_id);

ALTER TABLE ONLY canvas.project_copy_snapshot
    ADD CONSTRAINT project_copy_snapshot_job_id_key UNIQUE (job_id);

ALTER TABLE ONLY canvas.project_copy_snapshot
    ADD CONSTRAINT project_copy_snapshot_pkey PRIMARY KEY (id);

ALTER TABLE ONLY canvas.save_receipt
    ADD CONSTRAINT save_receipt_pkey PRIMARY KEY (actor_id, idem_key);

ALTER TABLE ONLY canvas.command_log
    ADD CONSTRAINT uq_canvas_command_rev UNIQUE (document_id, revision);

ALTER TABLE ONLY catalog.capability_default
    ADD CONSTRAINT capability_default_pkey PRIMARY KEY (capability);

ALTER TABLE ONLY catalog.capability
    ADD CONSTRAINT capability_key_key UNIQUE (key);

ALTER TABLE ONLY catalog.capability
    ADD CONSTRAINT capability_pkey PRIMARY KEY (id);

ALTER TABLE ONLY catalog.config_request
    ADD CONSTRAINT config_request_pkey PRIMARY KEY (actor_id, action, target_id, request_id);

ALTER TABLE ONLY catalog.model_candidate
    ADD CONSTRAINT model_candidate_check_id_provider_model_id_capability_key UNIQUE (check_id, provider_model_id, capability);

ALTER TABLE ONLY catalog.model_candidate
    ADD CONSTRAINT model_candidate_pkey PRIMARY KEY (id);

ALTER TABLE ONLY catalog.model_profile
    ADD CONSTRAINT model_profile_model_key_key UNIQUE (model_key);

ALTER TABLE ONLY catalog.model_profile
    ADD CONSTRAINT model_profile_pkey PRIMARY KEY (id);

ALTER TABLE ONLY catalog.model_profile_version
    ADD CONSTRAINT model_profile_version_pkey PRIMARY KEY (id);

ALTER TABLE ONLY catalog.model_release
    ADD CONSTRAINT model_release_model_id_id_key UNIQUE (model_id, id);

ALTER TABLE ONLY catalog.model_release
    ADD CONSTRAINT model_release_pkey PRIMARY KEY (id);

ALTER TABLE ONLY catalog.model_validation
    ADD CONSTRAINT model_validation_operation_id_key UNIQUE (operation_id);

ALTER TABLE ONLY catalog.model_validation
    ADD CONSTRAINT model_validation_pkey PRIMARY KEY (id);

ALTER TABLE ONLY catalog.price_rule_version
    ADD CONSTRAINT price_rule_version_pkey PRIMARY KEY (id);

ALTER TABLE ONLY catalog.provider_check
    ADD CONSTRAINT provider_check_pkey PRIMARY KEY (id);

ALTER TABLE ONLY catalog.provider_config_version
    ADD CONSTRAINT provider_config_version_pkey PRIMARY KEY (id);

ALTER TABLE ONLY catalog.provider_config_version
    ADD CONSTRAINT provider_config_version_provider_id_id_key UNIQUE (provider_id, id);

ALTER TABLE ONLY catalog.provider_config_version
    ADD CONSTRAINT provider_config_version_provider_id_version_no_key UNIQUE (provider_id, version_no);

ALTER TABLE ONLY catalog.provider_credential
    ADD CONSTRAINT provider_credential_pkey PRIMARY KEY (id);

ALTER TABLE ONLY catalog.provider
    ADD CONSTRAINT provider_key_key UNIQUE (key);

ALTER TABLE ONLY catalog.provider
    ADD CONSTRAINT provider_pkey PRIMARY KEY (id);

ALTER TABLE ONLY catalog.provider_preset
    ADD CONSTRAINT provider_preset_pkey PRIMARY KEY (id);

ALTER TABLE ONLY catalog.provider_preset
    ADD CONSTRAINT provider_preset_slug_key UNIQUE (slug);

ALTER TABLE ONLY catalog.provider_preset_version
    ADD CONSTRAINT provider_preset_version_pkey PRIMARY KEY (id);

ALTER TABLE ONLY catalog.provider_preset_version
    ADD CONSTRAINT provider_preset_version_preset_id_version_no_key UNIQUE (preset_id, version_no);

ALTER TABLE ONLY catalog.model_profile_version
    ADD CONSTRAINT uq_model_profile_version UNIQUE (model_profile_id, version_no);

ALTER TABLE ONLY catalog.model_profile_version
    ADD CONSTRAINT uq_model_profile_version_owner UNIQUE (model_profile_id, id);

ALTER TABLE ONLY catalog.price_rule_version
    ADD CONSTRAINT uq_price_rule_version UNIQUE (model_profile_id, version_no);

ALTER TABLE ONLY identity.auth_challenge
    ADD CONSTRAINT auth_challenge_pkey PRIMARY KEY (id);

ALTER TABLE ONLY identity.auth_event
    ADD CONSTRAINT auth_event_pkey PRIMARY KEY (id);

ALTER TABLE ONLY identity.authority_proposal
    ADD CONSTRAINT authority_proposal_pkey PRIMARY KEY (id);

ALTER TABLE ONLY identity.directory_export
    ADD CONSTRAINT directory_export_actor_id_scope_id_request_key_key UNIQUE (actor_id, scope_id, request_key);

ALTER TABLE ONLY identity.directory_export
    ADD CONSTRAINT directory_export_pkey PRIMARY KEY (id);

ALTER TABLE ONLY identity.email_budget
    ADD CONSTRAINT email_budget_pkey PRIMARY KEY (bucket_key);

ALTER TABLE ONLY identity.invitation
    ADD CONSTRAINT invitation_pkey PRIMARY KEY (id);

ALTER TABLE ONLY identity.invitation
    ADD CONSTRAINT invitation_token_hash_key UNIQUE (token_hash);

ALTER TABLE ONLY identity.login_guard
    ADD CONSTRAINT login_guard_pkey PRIMARY KEY (login_key);

ALTER TABLE ONLY identity.mail_delivery
    ADD CONSTRAINT mail_delivery_pkey PRIMARY KEY (event_id);

ALTER TABLE ONLY identity.platform_settings
    ADD CONSTRAINT platform_settings_pkey PRIMARY KEY (singleton);

ALTER TABLE ONLY identity.security_event
    ADD CONSTRAINT security_event_pkey PRIMARY KEY (id);

ALTER TABLE ONLY identity.session_refresh_token
    ADD CONSTRAINT session_refresh_token_pkey PRIMARY KEY (id);

ALTER TABLE ONLY identity.session_refresh_token
    ADD CONSTRAINT session_refresh_token_session_id_generation_key UNIQUE (session_id, generation);

ALTER TABLE ONLY identity.session_refresh_token
    ADD CONSTRAINT session_refresh_token_token_digest_key UNIQUE (token_digest);

ALTER TABLE ONLY identity."user"
    ADD CONSTRAINT user_org_id_id_unique UNIQUE (org_id, id);

ALTER TABLE ONLY identity."user"
    ADD CONSTRAINT user_pkey PRIMARY KEY (id);

ALTER TABLE ONLY identity.user_session
    ADD CONSTRAINT user_session_pkey PRIMARY KEY (id);

ALTER TABLE ONLY identity.user_session
    ADD CONSTRAINT user_session_token_hash_key UNIQUE (token_hash);

ALTER TABLE ONLY infra.idempotency_record
    ADD CONSTRAINT idempotency_record_pkey PRIMARY KEY (service_owner, id);

ALTER TABLE ONLY infra.outbox
    ADD CONSTRAINT outbox_pkey PRIMARY KEY (service_owner, id, create_time);

ALTER TABLE ONLY infra.processed_event
    ADD CONSTRAINT processed_event_pkey PRIMARY KEY (service_owner, id);

ALTER TABLE ONLY infra.idempotency_record
    ADD CONSTRAINT uq_idempotency_record_natural UNIQUE (service_owner, actor_id, idem_key);

ALTER TABLE ONLY infra.processed_event
    ADD CONSTRAINT uq_processed_event_natural UNIQUE (service_owner, consumer, event_id);

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
    ADD CONSTRAINT media_asset_document_facts_check CHECK (((kind <> 'document'::text) OR ((origin = ANY (ARRAY['upload'::text, 'system'::text])) AND ((byte_size >= 1) AND (byte_size <= 20971520)) AND (sha256 IS NOT NULL) AND (sha256 ~ '^[0-9a-f]{64}$'::text) AND (codec IS NOT NULL) AND (width IS NULL) AND (height IS NULL) AND (duration_ms IS NULL) AND (fps IS NULL) AND (audio_channels IS NULL) AND (file_name IS NOT NULL) AND ((octet_length(file_name) >= 1) AND (octet_length(file_name) <= 255)) AND (file_name = btrim(file_name)) AND (file_name !~ '[[:cntrl:]/\\]'::text) AND (((mime_type = 'text/plain'::text) AND (codec = 'txt'::text) AND (lower(file_name) ~~ '%.txt'::text) AND (object_key ~~ '%.txt'::text)) OR ((mime_type = 'text/markdown'::text) AND (codec = 'md'::text) AND (lower(file_name) ~~ '%.md'::text) AND (object_key ~~ '%.md'::text)) OR ((mime_type = 'application/vnd.openxmlformats-officedocument.wordprocessingml.document'::text) AND (codec = 'docx'::text) AND (lower(file_name) ~~ '%.docx'::text) AND (object_key ~~ '%.docx'::text)))))) NOT VALID;

ALTER TABLE ONLY media.media_asset
    ADD CONSTRAINT media_asset_object_key_key UNIQUE (object_key);

ALTER TABLE ONLY media.media_asset
    ADD CONSTRAINT media_asset_pkey PRIMARY KEY (id);

ALTER TABLE ONLY media.object_location
    ADD CONSTRAINT object_location_id_object_key_key UNIQUE (id, object_key);

ALTER TABLE ONLY media.object_location
    ADD CONSTRAINT object_location_pkey PRIMARY KEY (id);

ALTER TABLE ONLY media.object_location
    ADD CONSTRAINT object_location_profile_id_bucket_object_key_key UNIQUE (profile_id, bucket, object_key);

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

ALTER TABLE ONLY media.storage_bucket_evidence
    ADD CONSTRAINT storage_bucket_evidence_pkey PRIMARY KEY (id);

ALTER TABLE ONLY media.storage_config_revision
    ADD CONSTRAINT storage_config_revision_config_sha256_key UNIQUE (config_sha256);

ALTER TABLE ONLY media.storage_config_revision
    ADD CONSTRAINT storage_config_revision_pkey PRIMARY KEY (revision);

ALTER TABLE ONLY media.storage_probe_evidence
    ADD CONSTRAINT storage_probe_evidence_pkey PRIMARY KEY (id);

ALTER TABLE ONLY media.storage_probe_intent
    ADD CONSTRAINT storage_probe_intent_pkey PRIMARY KEY (id);

ALTER TABLE ONLY media.storage_profile
    ADD CONSTRAINT storage_profile_id_bucket_key UNIQUE (id, bucket);

ALTER TABLE ONLY media.storage_profile
    ADD CONSTRAINT storage_profile_pkey PRIMARY KEY (id);

ALTER TABLE ONLY media.storage_profile
    ADD CONSTRAINT storage_profile_resource_identity_key UNIQUE (resource_identity);

ALTER TABLE ONLY media.storage_profile
    ADD CONSTRAINT storage_profile_resource_pair_key UNIQUE (id, resource_identity);

ALTER TABLE ONLY media.storage_route
    ADD CONSTRAINT storage_route_pkey PRIMARY KEY (singleton);

ALTER TABLE ONLY media.storage_route_lock
    ADD CONSTRAINT storage_route_lock_pkey PRIMARY KEY (singleton);

ALTER TABLE ONLY media.storage_work_intent
    ADD CONSTRAINT storage_work_intent_id_profile_id_bucket_key UNIQUE (id, profile_id, bucket);

ALTER TABLE ONLY media.storage_work_intent
    ADD CONSTRAINT storage_work_intent_kind_owner_id_work_id_key UNIQUE (kind, owner_id, work_id);

ALTER TABLE ONLY media.storage_work_intent
    ADD CONSTRAINT storage_work_intent_pkey PRIMARY KEY (id);

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
    ADD CONSTRAINT transfer_object_pkey PRIMARY KEY (job_id, item_index, output_attempt, rendition_kind);

ALTER TABLE ONLY media.transfer_object
    ADD CONSTRAINT transfer_object_target_object_key_key UNIQUE (target_object_key);

ALTER TABLE ONLY media.upload_admission
    ADD CONSTRAINT upload_admission_id_profile_id_bucket_key UNIQUE (id, profile_id, bucket);

ALTER TABLE ONLY media.upload_admission
    ADD CONSTRAINT upload_admission_org_id_actor_id_scope_id_request_key_key UNIQUE (org_id, actor_id, scope_id, request_key);

ALTER TABLE ONLY media.upload_admission
    ADD CONSTRAINT upload_admission_pkey PRIMARY KEY (id);

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

ALTER TABLE ONLY operation.prompt_resolution_receipt
    ADD CONSTRAINT prompt_resolution_receipt_pkey PRIMARY KEY (actor_id, request_key);

ALTER TABLE ONLY operation.prompt_submission
    ADD CONSTRAINT prompt_submission_actor_id_project_id_owner_key_epoch_key UNIQUE (actor_id, project_id, owner_key, epoch);

ALTER TABLE ONLY operation.prompt_submission
    ADD CONSTRAINT prompt_submission_confirm_key_key UNIQUE (confirm_key);

ALTER TABLE ONLY operation.prompt_submission
    ADD CONSTRAINT prompt_submission_operation_id_key UNIQUE (operation_id);

ALTER TABLE ONLY operation.prompt_submission
    ADD CONSTRAINT prompt_submission_pkey PRIMARY KEY (id);

ALTER TABLE ONLY operation.prompt_submission_slot
    ADD CONSTRAINT prompt_submission_slot_pkey PRIMARY KEY (actor_id, project_id, owner_key);

ALTER TABLE ONLY operation.provider_binding
    ADD CONSTRAINT provider_binding_pkey PRIMARY KEY (operation_id);

ALTER TABLE ONLY operation.provider_call
    ADD CONSTRAINT provider_call_pkey PRIMARY KEY (id, create_time);

ALTER TABLE ONLY operation.provider_credential_binding
    ADD CONSTRAINT provider_credential_binding_pkey PRIMARY KEY (operation_id);

ALTER TABLE ONLY operation.provider_result
    ADD CONSTRAINT provider_result_pkey PRIMARY KEY (operation_id);

ALTER TABLE ONLY operation.provider_trial
    ADD CONSTRAINT provider_trial_pkey PRIMARY KEY (operation_id);

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

ALTER TABLE ONLY workspace.creation_scope
    ADD CONSTRAINT creation_scope_physical_unique UNIQUE (id, physical_scope_id);

ALTER TABLE ONLY workspace.creation_scope
    ADD CONSTRAINT creation_scope_pkey PRIMARY KEY (id);

ALTER TABLE ONLY workspace.organization
    ADD CONSTRAINT organization_pkey PRIMARY KEY (id);

ALTER TABLE ONLY workspace.project_activity
    ADD CONSTRAINT project_activity_pkey PRIMARY KEY (project_id);

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

ALTER TABLE ONLY workspace.project_creation_receipt
    ADD CONSTRAINT project_creation_receipt_pkey PRIMARY KEY (actor_id, idem_key);

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

ALTER TABLE ONLY workspace.prompt_draft
    ADD CONSTRAINT prompt_draft_pkey PRIMARY KEY (id);

ALTER TABLE ONLY workspace.prompt_draft_receipt
    ADD CONSTRAINT prompt_draft_receipt_pkey PRIMARY KEY (actor_id, request_key);

ALTER TABLE ONLY workspace.style_preset
    ADD CONSTRAINT style_preset_pkey PRIMARY KEY (id);

ALTER TABLE ONLY workspace.team_member
    ADD CONSTRAINT team_member_pkey PRIMARY KEY (team_id, user_id);

ALTER TABLE ONLY workspace.team
    ADD CONSTRAINT team_pkey PRIMARY KEY (id);

ALTER TABLE ONLY workspace.prompt_customization
    ADD CONSTRAINT uq_prompt_customization_owner_operation UNIQUE (org_id, owner_id, operation);

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
CREATE INDEX ix_canvas_document_asset_kind ON canvas.document(kind,update_time DESC,id DESC) WHERE NOT is_delete;

CREATE INDEX ix_canvas_node_doc ON canvas.node USING btree (document_id);

CREATE INDEX ix_model_profile_provider_not_deleted ON catalog.model_profile USING btree (provider_id) WHERE (NOT is_delete);

CREATE INDEX ix_price_rule_effective ON catalog.price_rule_version USING btree (model_profile_id, effective_from DESC);

CREATE UNIQUE INDEX uq_provider_credential_active ON catalog.provider_credential USING btree (provider_id) WHERE ((status = 'active'::text) AND (NOT is_delete));

CREATE INDEX auth_challenge_email_idx ON identity.auth_challenge USING btree (email_key, purpose, created_at DESC);

CREATE INDEX auth_challenge_expiry_idx ON identity.auth_challenge USING btree (expires_at);

CREATE INDEX auth_challenge_operation_idx ON identity.auth_challenge USING btree (operation_id, purpose);

CREATE UNIQUE INDEX authority_proposal_platform_pending_idx ON identity.authority_proposal USING btree (target_user_id) WHERE ((kind = 'platform_grant'::text) AND (status = 'pending'::text));

CREATE INDEX authority_proposal_target_idx ON identity.authority_proposal USING btree (target_user_id, status, expires_at);

CREATE INDEX authority_proposal_team_idx ON identity.authority_proposal USING btree (team_id, status);

CREATE UNIQUE INDEX authority_proposal_team_pending_idx ON identity.authority_proposal USING btree (team_id) WHERE ((kind = ANY (ARRAY['team_transfer'::text, 'platform_takeover'::text])) AND (status = 'pending'::text));

CREATE INDEX directory_export_actor_idx ON identity.directory_export USING btree (actor_id, created_at DESC, id);

CREATE INDEX directory_export_work_idx ON identity.directory_export USING btree (status, expires_at, created_at, id);

CREATE INDEX directory_export_storage_idx ON identity.directory_export USING btree (storage_profile_id, status, expires_at) WHERE (private_object_id IS NOT NULL);

CREATE INDEX idx_user_directory ON identity."user" USING btree (org_id, create_time DESC, id DESC) WHERE (NOT is_delete);

CREATE UNIQUE INDEX invitation_pending_account_idx ON identity.invitation USING btree (email_key) WHERE ((kind = 'account'::text) AND (status = 'pending'::text));

CREATE UNIQUE INDEX invitation_pending_team_idx ON identity.invitation USING btree (team_id, email_key) WHERE ((kind = 'team'::text) AND (status = 'pending'::text));

CREATE INDEX mail_delivery_pending_idx ON identity.mail_delivery USING btree (status, next_attempt_at, event_id);

CREATE INDEX security_event_team_idx ON identity.security_event USING btree (team_id, created_at DESC, id);

CREATE INDEX security_event_time_idx ON identity.security_event USING btree (created_at DESC, id);

CREATE INDEX security_event_user_idx ON identity.security_event USING btree (target_user_id, created_at DESC, id);

CREATE UNIQUE INDEX session_refresh_one_active_idx ON identity.session_refresh_token USING btree (session_id) WHERE ((consumed_at IS NULL) AND (revoked_at IS NULL));

CREATE INDEX session_refresh_replay_cleanup_idx ON identity.session_refresh_token USING btree (replay_until, id) WHERE (receipt_ciphertext IS NOT NULL);

CREATE UNIQUE INDEX uq_user_login_global ON identity."user" USING btree (lower(btrim((login_name)::text))) WHERE (NOT is_delete);

CREATE UNIQUE INDEX uq_user_login_name ON identity."user" USING btree (org_id, login_name) WHERE (NOT is_delete);

CREATE UNIQUE INDEX user_email_key_unique ON identity."user" USING btree (email_key) WHERE (email_key IS NOT NULL);

CREATE INDEX user_platform_directory_idx ON identity."user" USING btree (create_time DESC, id DESC);

CREATE INDEX user_platform_role_idx ON identity."user" USING btree (platform_role, status);

CREATE INDEX user_platform_status_idx ON identity."user" USING btree (status, create_time DESC, id DESC);

CREATE INDEX user_session_account_idx ON identity.user_session USING btree (account_id);

CREATE INDEX user_session_activity_idx ON identity.user_session USING btree (account_id, last_active_at DESC, id);

CREATE INDEX user_session_live_idx ON identity.user_session USING btree (account_id, id) WHERE (revoked_at IS NULL);

CREATE INDEX ix_outbox_pending ON ONLY infra.outbox USING btree (next_attempt_at, create_time, id)
    WHERE (published_at IS NULL AND dead_lettered_at IS NULL);

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

CREATE INDEX object_location_cleanup_idx ON media.object_location USING btree (state, updated_at) WHERE (state = ANY (ARRAY['reserved'::text, 'present'::text, 'delete_pending'::text, 'unknown'::text]));

CREATE INDEX package_actor_jobs ON media.package_job USING btree (org_id, actor_id, created_at DESC, id);

CREATE UNIQUE INDEX package_library_active ON media.package_job USING btree (library_id) WHERE (status = 'needs_reconciliation'::text);

CREATE INDEX package_project_work ON media.package_job USING btree (project_id, status);

CREATE INDEX purge_actor_jobs ON media.purge_job USING btree (org_id, actor_id, created_at DESC, id);

CREATE INDEX purge_project_work ON media.purge_job USING btree (project_id, status);

CREATE UNIQUE INDEX purge_reserved_item ON media.purge_item USING btree (item_id) WHERE (status = ANY (ARRAY['queued'::text, 'running'::text, 'needs_reconciliation'::text, 'succeeded'::text]));

CREATE INDEX storage_work_open_profile_idx ON media.storage_work_intent USING btree (profile_id, state);

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

CREATE INDEX ix_operation_canvas_billing_target ON operation.operation (project_id, (source_context->>'canvas_id'), (source_context->>'node_id'), (CASE WHEN source_context ? 'shot_id' THEN 'shot/' || (source_context->>'shot_id') ELSE COALESCE(source_context->>'row_id','') END)) WHERE NOT is_delete AND source_context IS NOT NULL;

CREATE INDEX ix_operation_target ON operation.operation USING btree (target_type, target_id, create_time DESC);

CREATE INDEX ix_provider_call_operation ON ONLY operation.provider_call USING btree (operation_id, create_time);

CREATE UNIQUE INDEX script_episode_active_seq ON script.episode USING btree (split_set_id, seq_no) WHERE (NOT is_delete);

CREATE INDEX script_source_project ON script.script_source USING btree (org_id, project_id, created_at, id);

CREATE INDEX script_version_content ON script.script_version USING btree (project_id, content_hash);

CREATE INDEX script_version_document ON script.script_version USING btree (project_id, document_sha256);

CREATE UNIQUE INDEX creation_scope_personal_unique ON workspace.creation_scope USING btree (owner_user_id) WHERE (owner_user_id IS NOT NULL);

CREATE UNIQUE INDEX creation_scope_team_unique ON workspace.creation_scope USING btree (team_id) WHERE (team_id IS NOT NULL);

CREATE INDEX ix_project_copy_org_recent ON workspace.project_copy_job USING btree (org_id, create_time DESC, id DESC);

CREATE INDEX ix_project_copy_source_active ON workspace.project_copy_job USING btree (source_project_id, status) WHERE (status <> ALL (ARRAY['succeeded'::text, 'cancelled'::text]));

CREATE INDEX ix_project_folder_actor_recent ON workspace.project_folder USING btree (org_id, actor_id, update_time DESC, id DESC) WHERE (NOT is_delete);

CREATE INDEX ix_project_folder_members ON workspace.project_folder_placement USING btree (org_id, actor_id, folder_id, project_id);

CREATE INDEX ix_project_org_status ON workspace.project USING btree (org_id, status, update_time DESC) WHERE (NOT is_delete);

CREATE INDEX project_activity_recent_idx ON workspace.project_activity USING btree (modified_at DESC, project_id DESC);

CREATE INDEX project_copy_scope_idx ON workspace.project_copy_job USING btree (owner_scope_id, source_project_id, create_time DESC, id DESC);

CREATE INDEX project_folder_scope_actor_idx ON workspace.project_folder USING btree (owner_scope_id, actor_id, update_time DESC, id DESC);

CREATE INDEX project_owner_scope_idx ON workspace.project USING btree (owner_scope_id, is_delete, update_time DESC, id DESC);

CREATE INDEX project_owner_updated ON workspace.project USING btree (org_id, owner_id, update_time DESC, id DESC) WHERE (NOT is_delete);

CREATE INDEX prompt_draft_owner ON workspace.prompt_draft USING btree (project_id, actor_id, id);

CREATE INDEX style_preset_owner_scope_idx ON workspace.style_preset USING btree (owner_scope_id, is_delete);

CREATE INDEX team_creator_idx ON workspace.team USING btree (created_by, status);

CREATE INDEX team_member_admin_idx ON workspace.team_member USING btree (team_id, role, status);

CREATE INDEX team_member_directory_idx ON workspace.team_member USING btree (team_id, status, joined_at DESC, user_id DESC);

CREATE INDEX team_member_user_idx ON workspace.team_member USING btree (user_id, status, team_id);

CREATE INDEX team_status_idx ON workspace.team USING btree (status, create_time DESC, id DESC);

CREATE TRIGGER audit_log_append_only BEFORE DELETE OR UPDATE ON audit.audit_log FOR EACH ROW EXECUTE FUNCTION audit.reject_audit_mutation();

CREATE TRIGGER trg_budget_update_time BEFORE UPDATE ON billing.budget FOR EACH ROW EXECUTE FUNCTION billing.set_budget_update_time();

CREATE TRIGGER trg_ledger_entry_append_only BEFORE DELETE OR UPDATE ON billing.ledger_entry FOR EACH ROW EXECUTE FUNCTION billing.keep_ledger_entry_append_only();

CREATE TRIGGER keep_object_location_id BEFORE UPDATE ON media.media_asset FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('object_location_id');

CREATE TRIGGER keep_object_location_id BEFORE UPDATE ON media.package_object FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('object_location_id');

CREATE TRIGGER keep_object_location_id BEFORE UPDATE ON media.purge_object FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('object_location_id');

CREATE TRIGGER keep_object_location_id BEFORE UPDATE ON media.rendition FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('object_location_id');

CREATE TRIGGER keep_source_location_id BEFORE UPDATE ON media.project_copy_object FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('source_location_id');

CREATE TRIGGER keep_source_location_id BEFORE UPDATE ON media.transfer_object FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('source_location_id');


CREATE TRIGGER keep_storage_evidence BEFORE DELETE OR UPDATE ON media.storage_bucket_evidence FOR EACH ROW EXECUTE FUNCTION media.keep_storage_evidence();

CREATE TRIGGER keep_storage_evidence BEFORE DELETE OR UPDATE ON media.storage_config_revision FOR EACH ROW EXECUTE FUNCTION media.keep_storage_evidence();


CREATE TRIGGER keep_storage_evidence BEFORE DELETE OR UPDATE ON media.storage_probe_evidence FOR EACH ROW EXECUTE FUNCTION media.keep_storage_evidence();

CREATE TRIGGER keep_storage_identity BEFORE UPDATE ON media.object_location FOR EACH ROW EXECUTE FUNCTION media.keep_storage_identity();


CREATE TRIGGER keep_storage_identity BEFORE DELETE OR UPDATE ON media.storage_profile FOR EACH ROW EXECUTE FUNCTION media.keep_storage_identity();

CREATE TRIGGER keep_storage_probe_intent BEFORE UPDATE ON media.storage_probe_intent FOR EACH ROW EXECUTE FUNCTION media.keep_storage_probe_intent();

CREATE TRIGGER keep_storage_work BEFORE UPDATE ON media.object_location FOR EACH ROW EXECUTE FUNCTION media.keep_storage_work();

CREATE TRIGGER keep_storage_work BEFORE UPDATE ON media.storage_work_intent FOR EACH ROW EXECUTE FUNCTION media.keep_storage_work();

CREATE TRIGGER keep_storage_write_deadline BEFORE UPDATE ON media.object_location FOR EACH ROW EXECUTE FUNCTION media.keep_storage_write_deadline();

CREATE TRIGGER keep_target_location_id BEFORE UPDATE ON media.project_copy_object FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('target_location_id');

CREATE TRIGGER keep_target_location_id BEFORE UPDATE ON media.transfer_object FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('target_location_id');

CREATE TRIGGER keep_upload_admission BEFORE UPDATE ON media.upload_admission FOR EACH ROW EXECUTE FUNCTION media.keep_upload_admission();

CREATE TRIGGER library_command_immutable BEFORE DELETE OR UPDATE ON media.library_command FOR EACH ROW EXECUTE FUNCTION media.reject_library_command_mutation();

CREATE TRIGGER library_item_scope BEFORE INSERT OR UPDATE ON media.library_item FOR EACH ROW EXECUTE FUNCTION media.check_library_asset_scope();

CREATE TRIGGER package_command_immutable BEFORE DELETE OR UPDATE ON media.package_command FOR EACH ROW EXECUTE FUNCTION media.reject_package_command_mutation();

CREATE TRIGGER package_frozen_immutable BEFORE UPDATE ON media.package_job FOR EACH ROW EXECUTE FUNCTION media.reject_package_frozen_mutation();

CREATE TRIGGER package_scope BEFORE INSERT ON media.package_job FOR EACH ROW EXECUTE FUNCTION media.check_package_scope();

CREATE TRIGGER purge_command_immutable BEFORE DELETE OR UPDATE ON media.purge_command FOR EACH ROW EXECUTE FUNCTION media.reject_purge_command_mutation();

CREATE TRIGGER purge_scope BEFORE INSERT ON media.purge_job FOR EACH ROW EXECUTE FUNCTION media.check_purge_scope();

CREATE TRIGGER transfer_command_immutable BEFORE DELETE OR UPDATE ON media.transfer_command FOR EACH ROW EXECUTE FUNCTION media.reject_transfer_command_mutation();

CREATE TRIGGER transfer_ownership_immutable BEFORE UPDATE ON media.transfer_job FOR EACH ROW EXECUTE FUNCTION media.protect_transfer_ownership();

CREATE TRIGGER transfer_scope BEFORE INSERT ON media.transfer_job FOR EACH ROW EXECUTE FUNCTION media.check_transfer_scope();

CREATE TRIGGER upload_request_scope_guard BEFORE INSERT ON media.upload_request FOR EACH ROW EXECUTE FUNCTION media.check_upload_receipt_scope();

CREATE TRIGGER keep_object_location_id BEFORE UPDATE ON mediatool.depth_object FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('object_location_id');

CREATE TRIGGER keep_object_location_id BEFORE UPDATE ON script.object_intent FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('object_location_id');

CREATE TRIGGER keep_object_location_id BEFORE UPDATE ON script.object_state FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('object_location_id');

CREATE TRIGGER project_change_command_immutable BEFORE DELETE OR UPDATE ON workspace.project_change_command FOR EACH ROW EXECUTE FUNCTION workspace.reject_project_change_command_mutation();

CREATE TRIGGER project_copy_scope_binding BEFORE INSERT OR UPDATE ON workspace.project_copy_job FOR EACH ROW EXECUTE FUNCTION workspace.validate_secondary_scope();

CREATE TRIGGER project_folder_command_immutable BEFORE DELETE OR UPDATE ON workspace.project_folder_command FOR EACH ROW EXECUTE FUNCTION workspace.reject_project_folder_command_mutation();

CREATE TRIGGER project_folder_placement_scope_binding BEFORE INSERT OR UPDATE ON workspace.project_folder_placement FOR EACH ROW EXECUTE FUNCTION workspace.validate_secondary_scope();

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

ALTER TABLE ONLY canvas.node_input
    ADD CONSTRAINT node_input_created_by_fkey FOREIGN KEY (created_by) REFERENCES identity."user"(id);

ALTER TABLE ONLY canvas.node
    ADD CONSTRAINT node_input_head_fk FOREIGN KEY (id, input_revision) REFERENCES canvas.node_input(node_id, input_revision) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY canvas.node_input
    ADD CONSTRAINT node_input_node_id_fkey FOREIGN KEY (node_id) REFERENCES canvas.node(id) ON DELETE CASCADE;

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

ALTER TABLE ONLY canvas.save_receipt
    ADD CONSTRAINT save_receipt_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY canvas.save_receipt
    ADD CONSTRAINT save_receipt_document_id_fkey FOREIGN KEY (document_id) REFERENCES canvas.document(id) ON DELETE CASCADE;

ALTER TABLE ONLY catalog.capability_default
    ADD CONSTRAINT capability_default_capability_fkey FOREIGN KEY (capability) REFERENCES catalog.capability(key);

ALTER TABLE ONLY catalog.capability_default
    ADD CONSTRAINT capability_default_release_id_fkey FOREIGN KEY (release_id) REFERENCES catalog.model_release(id);

ALTER TABLE ONLY catalog.model_profile
    ADD CONSTRAINT fk_model_profile_current_version FOREIGN KEY (id, current_version_id) REFERENCES catalog.model_profile_version(model_profile_id, id);

ALTER TABLE ONLY catalog.model_profile
    ADD CONSTRAINT model_active_release_fk FOREIGN KEY (id, active_release_id) REFERENCES catalog.model_release(model_id, id);

ALTER TABLE ONLY catalog.model_candidate
    ADD CONSTRAINT model_candidate_check_id_fkey FOREIGN KEY (check_id) REFERENCES catalog.provider_check(id);

ALTER TABLE ONLY catalog.model_profile
    ADD CONSTRAINT model_profile_capability_fkey FOREIGN KEY (capability) REFERENCES catalog.capability(key);

ALTER TABLE ONLY catalog.model_profile
    ADD CONSTRAINT model_profile_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES catalog.provider(id);

ALTER TABLE ONLY catalog.model_profile_version
    ADD CONSTRAINT model_profile_version_model_profile_id_fkey FOREIGN KEY (model_profile_id) REFERENCES catalog.model_profile(id);

ALTER TABLE ONLY catalog.model_release
    ADD CONSTRAINT model_release_config_version_id_fkey FOREIGN KEY (config_version_id) REFERENCES catalog.provider_config_version(id);

ALTER TABLE ONLY catalog.model_release
    ADD CONSTRAINT model_release_model_id_fkey FOREIGN KEY (model_id) REFERENCES catalog.model_profile(id);

ALTER TABLE ONLY catalog.model_release
    ADD CONSTRAINT model_release_model_version_id_fkey FOREIGN KEY (model_version_id) REFERENCES catalog.model_profile_version(id);

ALTER TABLE ONLY catalog.model_release
    ADD CONSTRAINT model_release_price_version_id_fkey FOREIGN KEY (price_version_id) REFERENCES catalog.price_rule_version(id);

ALTER TABLE ONLY catalog.model_validation
    ADD CONSTRAINT model_validation_credential_id_fkey FOREIGN KEY (credential_id) REFERENCES catalog.provider_credential(id);

ALTER TABLE ONLY catalog.model_validation
    ADD CONSTRAINT model_validation_release_id_fkey FOREIGN KEY (release_id) REFERENCES catalog.model_release(id);

ALTER TABLE ONLY catalog.price_rule_version
    ADD CONSTRAINT price_rule_version_model_profile_id_fkey FOREIGN KEY (model_profile_id) REFERENCES catalog.model_profile(id);

ALTER TABLE ONLY catalog.provider_check
    ADD CONSTRAINT provider_check_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY catalog.provider_check
    ADD CONSTRAINT provider_check_config_version_id_fkey FOREIGN KEY (config_version_id) REFERENCES catalog.provider_config_version(id);

ALTER TABLE ONLY catalog.provider_check
    ADD CONSTRAINT provider_check_credential_id_fkey FOREIGN KEY (credential_id) REFERENCES catalog.provider_credential(id);

ALTER TABLE ONLY catalog.provider_check
    ADD CONSTRAINT provider_check_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES catalog.provider(id);

ALTER TABLE ONLY catalog.provider_config_version
    ADD CONSTRAINT provider_config_version_preset_version_id_fkey FOREIGN KEY (preset_version_id) REFERENCES catalog.provider_preset_version(id);

ALTER TABLE ONLY catalog.provider_config_version
    ADD CONSTRAINT provider_config_version_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES catalog.provider(id);

ALTER TABLE ONLY catalog.provider_credential
    ADD CONSTRAINT provider_credential_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES catalog.provider(id);

ALTER TABLE ONLY catalog.provider
    ADD CONSTRAINT provider_current_config_fk FOREIGN KEY (id, current_config_version_id) REFERENCES catalog.provider_config_version(provider_id, id);

ALTER TABLE ONLY catalog.provider_preset
    ADD CONSTRAINT provider_preset_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES catalog.provider_preset(id);

ALTER TABLE ONLY catalog.provider_preset_version
    ADD CONSTRAINT provider_preset_version_preset_id_fkey FOREIGN KEY (preset_id) REFERENCES catalog.provider_preset(id);

ALTER TABLE ONLY identity.auth_challenge
    ADD CONSTRAINT auth_challenge_user_id_fkey FOREIGN KEY (user_id) REFERENCES identity."user"(id) ON DELETE RESTRICT;

ALTER TABLE ONLY identity.auth_event
    ADD CONSTRAINT auth_event_account_id_fkey FOREIGN KEY (account_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY identity.authority_proposal
    ADD CONSTRAINT authority_proposal_source_user_id_fkey FOREIGN KEY (source_user_id) REFERENCES identity."user"(id) ON DELETE RESTRICT;

ALTER TABLE ONLY identity.authority_proposal
    ADD CONSTRAINT authority_proposal_target_user_id_fkey FOREIGN KEY (target_user_id) REFERENCES identity."user"(id) ON DELETE RESTRICT;

ALTER TABLE ONLY identity.authority_proposal
    ADD CONSTRAINT authority_proposal_team_id_fkey FOREIGN KEY (team_id) REFERENCES workspace.team(id) ON DELETE RESTRICT;

ALTER TABLE ONLY identity.directory_export
    ADD CONSTRAINT directory_export_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES identity."user"(id) ON DELETE RESTRICT;

ALTER TABLE ONLY identity.directory_export
    ADD CONSTRAINT directory_export_team_id_fkey FOREIGN KEY (team_id) REFERENCES workspace.team(id) ON DELETE RESTRICT;

ALTER TABLE ONLY identity.directory_export
    ADD CONSTRAINT directory_export_storage_location_id_fkey FOREIGN KEY (storage_location_id) REFERENCES media.object_location(id) ON DELETE RESTRICT;

ALTER TABLE ONLY identity.directory_export
    ADD CONSTRAINT directory_export_storage_intent_id_fkey FOREIGN KEY (storage_intent_id) REFERENCES media.storage_work_intent(id) ON DELETE RESTRICT;

ALTER TABLE ONLY identity.invitation
    ADD CONSTRAINT invitation_accepted_user_id_fkey FOREIGN KEY (accepted_user_id) REFERENCES identity."user"(id) ON DELETE RESTRICT;

ALTER TABLE ONLY identity.invitation
    ADD CONSTRAINT invitation_inviter_id_fkey FOREIGN KEY (inviter_id) REFERENCES identity."user"(id) ON DELETE RESTRICT;

ALTER TABLE ONLY identity.invitation
    ADD CONSTRAINT invitation_team_id_fkey FOREIGN KEY (team_id) REFERENCES workspace.team(id) ON DELETE RESTRICT;

ALTER TABLE ONLY identity.mail_delivery
    ADD CONSTRAINT mail_delivery_challenge_id_fkey FOREIGN KEY (challenge_id) REFERENCES identity.auth_challenge(id) ON DELETE RESTRICT;

ALTER TABLE ONLY identity.mail_delivery
    ADD CONSTRAINT mail_delivery_invitation_id_fkey FOREIGN KEY (invitation_id) REFERENCES identity.invitation(id) ON DELETE RESTRICT;

ALTER TABLE ONLY identity.platform_settings
    ADD CONSTRAINT platform_settings_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES identity."user"(id) ON DELETE RESTRICT;

ALTER TABLE ONLY identity.security_event
    ADD CONSTRAINT security_event_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES identity."user"(id) ON DELETE RESTRICT;

ALTER TABLE ONLY identity.security_event
    ADD CONSTRAINT security_event_target_user_id_fkey FOREIGN KEY (target_user_id) REFERENCES identity."user"(id) ON DELETE RESTRICT;

ALTER TABLE ONLY identity.security_event
    ADD CONSTRAINT security_event_team_id_fkey FOREIGN KEY (team_id) REFERENCES workspace.team(id) ON DELETE RESTRICT;

ALTER TABLE ONLY identity.session_refresh_token
    ADD CONSTRAINT session_refresh_token_session_id_fkey FOREIGN KEY (session_id) REFERENCES identity.user_session(id) ON DELETE RESTRICT;

ALTER TABLE ONLY identity."user"
    ADD CONSTRAINT user_avatar_asset_id_fkey FOREIGN KEY (avatar_asset_id) REFERENCES media.media_asset(id) ON DELETE RESTRICT;

ALTER TABLE ONLY identity.user_session
    ADD CONSTRAINT user_session_account_id_fkey FOREIGN KEY (account_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY media.media_asset
    ADD CONSTRAINT fk_media_source_operation_project FOREIGN KEY (project_id, source_operation_id) REFERENCES operation.operation(project_id, id);

ALTER TABLE ONLY media.library_command
    ADD CONSTRAINT library_command_actor_scope_user_fk FOREIGN KEY (actor_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY media.library_command
    ADD CONSTRAINT library_command_library_id_fkey FOREIGN KEY (library_id) REFERENCES media.library(id);

ALTER TABLE ONLY media.library_command
    ADD CONSTRAINT library_command_physical_org_fk FOREIGN KEY (org_id) REFERENCES workspace.organization(id);

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
    ADD CONSTRAINT media_asset_object_location_id_fkey FOREIGN KEY (object_location_id, object_key) REFERENCES media.object_location(id, object_key);

ALTER TABLE ONLY media.media_asset
    ADD CONSTRAINT media_asset_personal_owner_fk FOREIGN KEY (personal_org_id, personal_actor_id) REFERENCES identity."user"(org_id, id);

ALTER TABLE ONLY media.media_asset
    ADD CONSTRAINT media_asset_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY media.object_location
    ADD CONSTRAINT object_location_profile_id_bucket_fkey FOREIGN KEY (profile_id, bucket) REFERENCES media.storage_profile(id, bucket);

ALTER TABLE ONLY media.object_location
    ADD CONSTRAINT object_location_upload_owner_fkey FOREIGN KEY (owner_session_id, profile_id, bucket) REFERENCES media.upload_admission(id, profile_id, bucket);

ALTER TABLE ONLY media.object_location
    ADD CONSTRAINT object_location_work_owner_fkey FOREIGN KEY (owner_intent_id, profile_id, bucket) REFERENCES media.storage_work_intent(id, profile_id, bucket);

ALTER TABLE ONLY media.package_command
    ADD CONSTRAINT package_command_actor_scope_user_fk FOREIGN KEY (actor_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY media.package_command
    ADD CONSTRAINT package_command_job_id_fkey FOREIGN KEY (job_id) REFERENCES media.package_job(id);

ALTER TABLE ONLY media.package_command
    ADD CONSTRAINT package_command_physical_org_fk FOREIGN KEY (org_id) REFERENCES workspace.organization(id);

ALTER TABLE ONLY media.package_job
    ADD CONSTRAINT package_job_actor_scope_user_fk FOREIGN KEY (actor_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY media.package_job
    ADD CONSTRAINT package_job_library_id_fkey FOREIGN KEY (library_id) REFERENCES media.library(id);

ALTER TABLE ONLY media.package_job
    ADD CONSTRAINT package_job_physical_org_fk FOREIGN KEY (org_id) REFERENCES workspace.organization(id);

ALTER TABLE ONLY media.package_job
    ADD CONSTRAINT package_job_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY media.package_object
    ADD CONSTRAINT package_object_job_id_fkey FOREIGN KEY (job_id) REFERENCES media.package_job(id);

ALTER TABLE ONLY media.package_object
    ADD CONSTRAINT package_object_object_location_id_fkey FOREIGN KEY (object_location_id, object_key) REFERENCES media.object_location(id, object_key);

ALTER TABLE ONLY media.project_copy_object
    ADD CONSTRAINT project_copy_object_snapshot_id_fkey FOREIGN KEY (snapshot_id) REFERENCES media.project_copy_snapshot(id);

ALTER TABLE ONLY media.project_copy_object
    ADD CONSTRAINT project_copy_object_source_location_id_fkey FOREIGN KEY (source_location_id, source_object_key) REFERENCES media.object_location(id, object_key);

ALTER TABLE ONLY media.project_copy_object
    ADD CONSTRAINT project_copy_object_target_location_id_fkey FOREIGN KEY (target_location_id, target_object_key) REFERENCES media.object_location(id, object_key);

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
    ADD CONSTRAINT purge_command_actor_scope_user_fk FOREIGN KEY (actor_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY media.purge_command
    ADD CONSTRAINT purge_command_job_id_fkey FOREIGN KEY (job_id) REFERENCES media.purge_job(id);

ALTER TABLE ONLY media.purge_command
    ADD CONSTRAINT purge_command_physical_org_fk FOREIGN KEY (org_id) REFERENCES workspace.organization(id);

ALTER TABLE ONLY media.purge_item
    ADD CONSTRAINT purge_item_asset_id_fkey FOREIGN KEY (asset_id) REFERENCES media.media_asset(id);

ALTER TABLE ONLY media.purge_item
    ADD CONSTRAINT purge_item_item_id_fkey FOREIGN KEY (item_id) REFERENCES media.library_item(id);

ALTER TABLE ONLY media.purge_item
    ADD CONSTRAINT purge_item_job_id_fkey FOREIGN KEY (job_id) REFERENCES media.purge_job(id);

ALTER TABLE ONLY media.purge_job
    ADD CONSTRAINT purge_job_actor_scope_user_fk FOREIGN KEY (actor_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY media.purge_job
    ADD CONSTRAINT purge_job_library_id_fkey FOREIGN KEY (library_id) REFERENCES media.library(id);

ALTER TABLE ONLY media.purge_job
    ADD CONSTRAINT purge_job_physical_org_fk FOREIGN KEY (org_id) REFERENCES workspace.organization(id);

ALTER TABLE ONLY media.purge_job
    ADD CONSTRAINT purge_job_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY media.purge_object
    ADD CONSTRAINT purge_object_job_id_item_index_fkey FOREIGN KEY (job_id, item_index) REFERENCES media.purge_item(job_id, item_index);

ALTER TABLE ONLY media.purge_object
    ADD CONSTRAINT purge_object_object_location_id_fkey FOREIGN KEY (object_location_id, object_key) REFERENCES media.object_location(id, object_key);

ALTER TABLE ONLY media.rendition
    ADD CONSTRAINT rendition_media_asset_id_fkey FOREIGN KEY (media_asset_id) REFERENCES media.media_asset(id);

ALTER TABLE ONLY media.rendition
    ADD CONSTRAINT rendition_object_location_id_fkey FOREIGN KEY (object_location_id, object_key) REFERENCES media.object_location(id, object_key);

ALTER TABLE ONLY media.storage_bucket_evidence
    ADD CONSTRAINT storage_bucket_evidence_profile_id_resource_identity_fkey FOREIGN KEY (profile_id, resource_identity) REFERENCES media.storage_profile(id, resource_identity);

ALTER TABLE ONLY media.storage_config_revision
    ADD CONSTRAINT storage_config_revision_profile_id_fkey FOREIGN KEY (profile_id) REFERENCES media.storage_profile(id);

ALTER TABLE ONLY media.storage_probe_evidence
    ADD CONSTRAINT storage_probe_evidence_profile_id_fkey FOREIGN KEY (profile_id) REFERENCES media.storage_profile(id);

ALTER TABLE ONLY media.storage_probe_intent
    ADD CONSTRAINT storage_probe_intent_profile_id_bucket_fkey FOREIGN KEY (profile_id, bucket) REFERENCES media.storage_profile(id, bucket);

ALTER TABLE ONLY media.storage_route
    ADD CONSTRAINT storage_route_profile_id_fkey FOREIGN KEY (profile_id) REFERENCES media.storage_profile(id);

ALTER TABLE ONLY media.storage_work_intent
    ADD CONSTRAINT storage_work_intent_profile_id_bucket_fkey FOREIGN KEY (profile_id, bucket) REFERENCES media.storage_profile(id, bucket);

ALTER TABLE ONLY media.transfer_command
    ADD CONSTRAINT transfer_command_actor_scope_user_fk FOREIGN KEY (actor_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY media.transfer_command
    ADD CONSTRAINT transfer_command_job_id_fkey FOREIGN KEY (job_id) REFERENCES media.transfer_job(id);

ALTER TABLE ONLY media.transfer_command
    ADD CONSTRAINT transfer_command_physical_org_fk FOREIGN KEY (org_id) REFERENCES workspace.organization(id);

ALTER TABLE ONLY media.transfer_item
    ADD CONSTRAINT transfer_item_job_id_fkey FOREIGN KEY (job_id) REFERENCES media.transfer_job(id);

ALTER TABLE ONLY media.transfer_item
    ADD CONSTRAINT transfer_item_source_asset_id_fkey FOREIGN KEY (source_asset_id) REFERENCES media.media_asset(id);

ALTER TABLE ONLY media.transfer_job
    ADD CONSTRAINT transfer_job_actor_scope_user_fk FOREIGN KEY (actor_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY media.transfer_job
    ADD CONSTRAINT transfer_job_physical_org_fk FOREIGN KEY (org_id) REFERENCES workspace.organization(id);

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

ALTER TABLE ONLY media.transfer_object
    ADD CONSTRAINT transfer_object_source_location_id_fkey FOREIGN KEY (source_location_id, source_object_key) REFERENCES media.object_location(id, object_key);

ALTER TABLE ONLY media.transfer_object
    ADD CONSTRAINT transfer_object_target_location_id_fkey FOREIGN KEY (target_location_id, target_object_key) REFERENCES media.object_location(id, object_key);

ALTER TABLE ONLY media.transfer_job
    ADD CONSTRAINT transfer_source_org_fkey FOREIGN KEY (source_org_id) REFERENCES workspace.organization(id);

ALTER TABLE ONLY media.transfer_job
    ADD CONSTRAINT transfer_source_scope_fkey FOREIGN KEY (source_scope_id) REFERENCES workspace.creation_scope(id);

ALTER TABLE ONLY media.transfer_job
    ADD CONSTRAINT transfer_target_org_fkey FOREIGN KEY (target_org_id) REFERENCES workspace.organization(id);

ALTER TABLE ONLY media.transfer_job
    ADD CONSTRAINT transfer_target_scope_fkey FOREIGN KEY (target_scope_id) REFERENCES workspace.creation_scope(id);

ALTER TABLE ONLY media.upload_admission
    ADD CONSTRAINT upload_admission_accepted_asset_id_fkey FOREIGN KEY (accepted_asset_id) REFERENCES media.media_asset(id);

ALTER TABLE ONLY media.upload_admission
    ADD CONSTRAINT upload_admission_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY media.upload_admission
    ADD CONSTRAINT upload_admission_org_id_fkey FOREIGN KEY (org_id) REFERENCES workspace.organization(id);

ALTER TABLE ONLY media.upload_admission
    ADD CONSTRAINT upload_admission_profile_id_bucket_fkey FOREIGN KEY (profile_id, bucket) REFERENCES media.storage_profile(id, bucket);

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

ALTER TABLE ONLY mediatool.depth_object
    ADD CONSTRAINT depth_object_object_location_id_fkey FOREIGN KEY (object_location_id, object_key) REFERENCES media.object_location(id, object_key);

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

ALTER TABLE ONLY operation.prompt_submission
    ADD CONSTRAINT prompt_submission_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY operation.prompt_submission
    ADD CONSTRAINT prompt_submission_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES operation.operation(id);

ALTER TABLE ONLY operation.prompt_submission
    ADD CONSTRAINT prompt_submission_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY operation.prompt_submission_slot
    ADD CONSTRAINT prompt_submission_slot_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY operation.prompt_submission_slot
    ADD CONSTRAINT prompt_submission_slot_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY operation.provider_binding
    ADD CONSTRAINT provider_binding_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES operation.operation(id);

ALTER TABLE ONLY operation.provider_binding
    ADD CONSTRAINT provider_binding_release_id_fkey FOREIGN KEY (release_id) REFERENCES catalog.model_release(id);

ALTER TABLE ONLY operation.provider_credential_binding
    ADD CONSTRAINT provider_credential_binding_credential_id_fkey FOREIGN KEY (credential_id) REFERENCES catalog.provider_credential(id);

ALTER TABLE ONLY operation.provider_credential_binding
    ADD CONSTRAINT provider_credential_binding_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES operation.provider_binding(operation_id);

ALTER TABLE ONLY operation.provider_result
    ADD CONSTRAINT provider_result_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES operation.provider_credential_binding(operation_id);

ALTER TABLE ONLY operation.provider_trial
    ADD CONSTRAINT provider_trial_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY operation.provider_trial
    ADD CONSTRAINT provider_trial_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES operation.provider_binding(operation_id);

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

ALTER TABLE ONLY script.object_intent
    ADD CONSTRAINT object_intent_object_location_id_fkey FOREIGN KEY (object_location_id, object_key) REFERENCES media.object_location(id, object_key);

ALTER TABLE ONLY script.object_state
    ADD CONSTRAINT object_state_object_key_fkey FOREIGN KEY (object_key) REFERENCES script.object_intent(object_key);

ALTER TABLE ONLY script.object_state
    ADD CONSTRAINT object_state_object_location_id_fkey FOREIGN KEY (object_location_id, object_key) REFERENCES media.object_location(id, object_key);

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

-- Personal account existence is accepted by Identity's immutable registration
-- command; Creation commits the exact source receipt with this scope. The account
-- becomes usable only after Identity has both resource-owner receipts.

ALTER TABLE ONLY workspace.creation_scope
    ADD CONSTRAINT creation_scope_physical_scope_id_fkey FOREIGN KEY (physical_scope_id) REFERENCES workspace.organization(id) ON DELETE RESTRICT;

ALTER TABLE ONLY workspace.creation_scope
    ADD CONSTRAINT creation_scope_team_id_fkey FOREIGN KEY (team_id) REFERENCES workspace.team(id) ON DELETE RESTRICT;

ALTER TABLE ONLY workspace.style_preset
    ADD CONSTRAINT fk_style_preset_project FOREIGN KEY (project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY workspace.project_activity
    ADD CONSTRAINT project_activity_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id) ON DELETE CASCADE;

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
    ADD CONSTRAINT project_copy_job_scope_fk FOREIGN KEY (owner_scope_id, org_id) REFERENCES workspace.creation_scope(id, physical_scope_id);

ALTER TABLE ONLY workspace.project_copy_job
    ADD CONSTRAINT project_copy_job_source_project_id_fkey FOREIGN KEY (source_project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY workspace.project_copy_job
    ADD CONSTRAINT project_copy_job_target_project_id_fkey FOREIGN KEY (target_project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY workspace.project
    ADD CONSTRAINT project_cover_asset_id_fkey FOREIGN KEY (cover_asset_id) REFERENCES media.media_asset(id);

ALTER TABLE ONLY workspace.project_creation_receipt
    ADD CONSTRAINT project_creation_receipt_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY workspace.project_creation_receipt
    ADD CONSTRAINT project_creation_receipt_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id) ON DELETE CASCADE;

ALTER TABLE ONLY workspace.project_folder
    ADD CONSTRAINT project_folder_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY workspace.project_folder_command
    ADD CONSTRAINT project_folder_command_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY workspace.project_folder_command
    ADD CONSTRAINT project_folder_command_org_id_fkey FOREIGN KEY (org_id) REFERENCES workspace.organization(id);

ALTER TABLE ONLY workspace.project_folder_command
    ADD CONSTRAINT project_folder_command_scope_fk FOREIGN KEY (owner_scope_id, org_id) REFERENCES workspace.creation_scope(id, physical_scope_id);

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

ALTER TABLE ONLY workspace.project_folder_placement
    ADD CONSTRAINT project_folder_placement_scope_fk FOREIGN KEY (owner_scope_id, org_id) REFERENCES workspace.creation_scope(id, physical_scope_id);

ALTER TABLE ONLY workspace.project_folder
    ADD CONSTRAINT project_folder_scope_fk FOREIGN KEY (owner_scope_id, org_id) REFERENCES workspace.creation_scope(id, physical_scope_id);

ALTER TABLE ONLY workspace.project
    ADD CONSTRAINT project_initial_canvas_fk FOREIGN KEY (initial_canvas_id, id) REFERENCES canvas.document(id, project_id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY workspace.project
    ADD CONSTRAINT project_org_id_fkey FOREIGN KEY (org_id) REFERENCES workspace.organization(id);

ALTER TABLE ONLY workspace.project
    ADD CONSTRAINT project_owner_fk FOREIGN KEY (owner_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY workspace.project
    ADD CONSTRAINT project_owner_scope_fk FOREIGN KEY (owner_scope_id, org_id) REFERENCES workspace.creation_scope(id, physical_scope_id) ON DELETE RESTRICT;

ALTER TABLE ONLY workspace.project
    ADD CONSTRAINT project_style_preset_id_fkey FOREIGN KEY (style_preset_id) REFERENCES workspace.style_preset(id);

ALTER TABLE ONLY workspace.prompt_customization
    ADD CONSTRAINT prompt_customization_org_id_fkey FOREIGN KEY (org_id) REFERENCES workspace.organization(id);

ALTER TABLE ONLY workspace.prompt_customization
    ADD CONSTRAINT prompt_customization_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY workspace.prompt_draft
    ADD CONSTRAINT prompt_draft_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY workspace.prompt_draft
    ADD CONSTRAINT prompt_draft_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY workspace.prompt_draft_receipt
    ADD CONSTRAINT prompt_draft_receipt_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES identity."user"(id);

ALTER TABLE ONLY workspace.prompt_draft_receipt
    ADD CONSTRAINT prompt_draft_receipt_project_id_fkey FOREIGN KEY (project_id) REFERENCES workspace.project(id);

ALTER TABLE ONLY workspace.style_preset
    ADD CONSTRAINT style_preset_org_id_fkey FOREIGN KEY (org_id) REFERENCES workspace.organization(id);

ALTER TABLE ONLY workspace.style_preset
    ADD CONSTRAINT style_preset_owner_scope_fk FOREIGN KEY (owner_scope_id, org_id) REFERENCES workspace.creation_scope(id, physical_scope_id) ON DELETE RESTRICT;

ALTER TABLE ONLY workspace.team
    ADD CONSTRAINT team_avatar_asset_id_fkey FOREIGN KEY (avatar_asset_id) REFERENCES media.media_asset(id) ON DELETE RESTRICT;

ALTER TABLE ONLY workspace.team
    ADD CONSTRAINT team_created_by_fkey FOREIGN KEY (created_by) REFERENCES identity."user"(id) ON DELETE RESTRICT;

ALTER TABLE ONLY workspace.team_member
    ADD CONSTRAINT team_member_joined_by_fkey FOREIGN KEY (joined_by) REFERENCES identity."user"(id) ON DELETE RESTRICT;

ALTER TABLE ONLY workspace.team_member
    ADD CONSTRAINT team_member_team_id_fkey FOREIGN KEY (team_id) REFERENCES workspace.team(id) ON DELETE RESTRICT;

ALTER TABLE ONLY workspace.team_member
    ADD CONSTRAINT team_member_user_id_fkey FOREIGN KEY (user_id) REFERENCES identity."user"(id) ON DELETE RESTRICT;

GRANT USAGE ON SCHEMA audit TO glowloom_app;

GRANT USAGE ON SCHEMA bible TO glowloom_app;

GRANT USAGE ON SCHEMA billing TO glowloom_app;

GRANT USAGE ON SCHEMA canvas TO glowloom_app;

GRANT USAGE ON SCHEMA catalog TO glowloom_app;

GRANT USAGE ON SCHEMA identity TO glowloom_app;

GRANT USAGE ON SCHEMA infra TO glowloom_app;

GRANT USAGE ON SCHEMA media TO glowloom_app;

GRANT USAGE ON SCHEMA mediatool TO glowloom_app;

GRANT USAGE ON SCHEMA operation TO glowloom_app;

GRANT USAGE ON SCHEMA script TO glowloom_app;

GRANT USAGE ON SCHEMA workspace TO glowloom_app;

GRANT SELECT,INSERT ON TABLE audit.audit_log TO glowloom_app;

GRANT SELECT,INSERT ON TABLE bible."character" TO glowloom_app;

GRANT UPDATE(revision) ON TABLE bible."character" TO glowloom_app;

GRANT UPDATE(current_version_id) ON TABLE bible."character" TO glowloom_app;

GRANT UPDATE(confirmed_version_id) ON TABLE bible."character" TO glowloom_app;

GRANT UPDATE(redirect_id) ON TABLE bible."character" TO glowloom_app;

GRANT UPDATE(is_delete) ON TABLE bible."character" TO glowloom_app;

GRANT UPDATE(updated_at) ON TABLE bible."character" TO glowloom_app;

GRANT SELECT,INSERT ON TABLE bible.character_confirmation TO glowloom_app;

GRANT SELECT,INSERT ON TABLE bible.character_redirect TO glowloom_app;

GRANT SELECT,INSERT ON TABLE bible.character_split TO glowloom_app;

GRANT SELECT,INSERT ON TABLE bible.character_version TO glowloom_app;

GRANT SELECT,INSERT ON TABLE bible.command TO glowloom_app;

GRANT SELECT,INSERT ON TABLE bible.copy_cleanup_receipt TO glowloom_app;

GRANT SELECT,INSERT ON TABLE bible.copy_receipt TO glowloom_app;

GRANT SELECT,INSERT ON TABLE bible.copy_snapshot TO glowloom_app;

GRANT SELECT,INSERT ON TABLE bible.copy_transfer_receipt TO glowloom_app;

GRANT SELECT,INSERT ON TABLE bible.location TO glowloom_app;

GRANT UPDATE(revision) ON TABLE bible.location TO glowloom_app;

GRANT UPDATE(current_version_id) ON TABLE bible.location TO glowloom_app;

GRANT UPDATE(confirmed_version_id) ON TABLE bible.location TO glowloom_app;

GRANT UPDATE(is_delete) ON TABLE bible.location TO glowloom_app;

GRANT UPDATE(updated_at) ON TABLE bible.location TO glowloom_app;

GRANT SELECT,INSERT ON TABLE bible.location_confirmation TO glowloom_app;

GRANT SELECT,INSERT ON TABLE bible.location_version TO glowloom_app;

GRANT SELECT,INSERT ON TABLE bible.look TO glowloom_app;

GRANT SELECT,INSERT ON TABLE bible.look_version TO glowloom_app;

GRANT SELECT,INSERT ON TABLE bible.prop TO glowloom_app;

GRANT UPDATE(revision) ON TABLE bible.prop TO glowloom_app;

GRANT UPDATE(current_version_id) ON TABLE bible.prop TO glowloom_app;

GRANT UPDATE(confirmed_version_id) ON TABLE bible.prop TO glowloom_app;

GRANT UPDATE(is_delete) ON TABLE bible.prop TO glowloom_app;

GRANT UPDATE(updated_at) ON TABLE bible.prop TO glowloom_app;

GRANT SELECT,INSERT ON TABLE bible.prop_confirmation TO glowloom_app;

GRANT SELECT,INSERT ON TABLE bible.prop_version TO glowloom_app;

GRANT SELECT,INSERT ON TABLE bible.reference_version TO glowloom_app;

GRANT SELECT,INSERT ON TABLE bible.voice_version TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE billing.budget TO glowloom_app;

GRANT SELECT,INSERT ON TABLE billing.ledger_entry TO glowloom_app;

GRANT UPDATE(update_time) ON TABLE billing.ledger_entry TO glowloom_app;

GRANT UPDATE(is_delete) ON TABLE billing.ledger_entry TO glowloom_app;

GRANT SELECT,INSERT ON TABLE billing.reservation TO glowloom_app;

GRANT UPDATE(status) ON TABLE billing.reservation TO glowloom_app;

GRANT UPDATE(closed_at) ON TABLE billing.reservation TO glowloom_app;

GRANT UPDATE(update_time) ON TABLE billing.reservation TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE canvas.command_log TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE canvas.document TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE canvas.edge TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE canvas.node TO glowloom_app;

GRANT SELECT,INSERT ON TABLE canvas.node_input TO glowloom_app;

GRANT SELECT,INSERT ON TABLE canvas.project_copy_receipt TO glowloom_app;

GRANT SELECT,INSERT ON TABLE canvas.project_copy_snapshot TO glowloom_app;

GRANT SELECT,INSERT ON TABLE canvas.save_receipt TO glowloom_app;

GRANT SELECT,INSERT ON TABLE catalog.capability TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE catalog.capability_default TO glowloom_app;

GRANT SELECT,INSERT ON TABLE catalog.config_request TO glowloom_app;

GRANT SELECT,INSERT ON TABLE catalog.model_candidate TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE catalog.model_profile TO glowloom_app;

GRANT SELECT,INSERT ON TABLE catalog.model_profile_version TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE catalog.model_release TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE catalog.model_validation TO glowloom_app;

GRANT SELECT,INSERT ON TABLE catalog.price_rule_version TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE catalog.provider TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE catalog.provider_check TO glowloom_app;

GRANT SELECT,INSERT ON TABLE catalog.provider_config_version TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE catalog.provider_credential TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE catalog.provider_preset TO glowloom_app;

GRANT SELECT,INSERT ON TABLE catalog.provider_preset_version TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE identity.auth_challenge TO glowloom_app;

GRANT SELECT,INSERT ON TABLE identity.auth_event TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE identity.authority_proposal TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE identity.directory_export TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE identity.email_budget TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE identity.invitation TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE identity.login_guard TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE identity.mail_delivery TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE identity.platform_settings TO glowloom_app;

GRANT SELECT,INSERT ON TABLE identity.security_event TO glowloom_app;

GRANT SELECT,INSERT,DELETE ON TABLE identity.session_refresh_token TO glowloom_app;

GRANT UPDATE(consumed_at) ON TABLE identity.session_refresh_token TO glowloom_app;

GRANT UPDATE(revoked_at) ON TABLE identity.session_refresh_token TO glowloom_app;

GRANT UPDATE(rotation_request_id) ON TABLE identity.session_refresh_token TO glowloom_app;

GRANT UPDATE(replay_until) ON TABLE identity.session_refresh_token TO glowloom_app;

GRANT UPDATE(receipt_ciphertext) ON TABLE identity.session_refresh_token TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE identity."user" TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE identity.user_session TO glowloom_app;

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE infra.idempotency_record TO glowloom_app;

GRANT SELECT,INSERT,DELETE ON TABLE infra.outbox TO glowloom_app;

GRANT UPDATE(published_at) ON TABLE infra.outbox TO glowloom_app;

GRANT UPDATE(published_at, attempt_count, next_attempt_at, claim_token, lease_until, last_error_class, dead_lettered_at, update_time) ON TABLE infra.outbox TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE infra.outbox_dead_letter TO glowloom_app;
GRANT SELECT,INSERT ON TABLE infra.outbox_dead_letter_audit TO glowloom_app;

GRANT SELECT,INSERT,DELETE ON TABLE infra.processed_event TO glowloom_app;

GRANT UPDATE(update_time) ON TABLE infra.processed_event TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.library TO glowloom_app;

GRANT UPDATE(revision) ON TABLE media.library TO glowloom_app;

GRANT UPDATE(update_time) ON TABLE media.library TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.library_command TO glowloom_app;

GRANT SELECT,INSERT,DELETE ON TABLE media.library_folder TO glowloom_app;

GRANT UPDATE(parent_id) ON TABLE media.library_folder TO glowloom_app;

GRANT UPDATE(name) ON TABLE media.library_folder TO glowloom_app;

GRANT UPDATE(name_key) ON TABLE media.library_folder TO glowloom_app;

GRANT UPDATE("position") ON TABLE media.library_folder TO glowloom_app;

GRANT UPDATE(style) ON TABLE media.library_folder TO glowloom_app;

GRANT UPDATE(theme) ON TABLE media.library_folder TO glowloom_app;

GRANT UPDATE(revision) ON TABLE media.library_folder TO glowloom_app;

GRANT UPDATE(update_time) ON TABLE media.library_folder TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.library_item TO glowloom_app;

GRANT UPDATE(plain_text) ON TABLE media.library_item TO glowloom_app;

GRANT UPDATE(folder_id) ON TABLE media.library_item TO glowloom_app;

GRANT UPDATE(title) ON TABLE media.library_item TO glowloom_app;

GRANT UPDATE(category) ON TABLE media.library_item TO glowloom_app;

GRANT UPDATE(tags) ON TABLE media.library_item TO glowloom_app;

GRANT UPDATE(source_label) ON TABLE media.library_item TO glowloom_app;

GRANT UPDATE(note) ON TABLE media.library_item TO glowloom_app;

GRANT UPDATE(favorite) ON TABLE media.library_item TO glowloom_app;

GRANT UPDATE(catalog_state) ON TABLE media.library_item TO glowloom_app;

GRANT UPDATE(trashed_at) ON TABLE media.library_item TO glowloom_app;

GRANT UPDATE("position") ON TABLE media.library_item TO glowloom_app;

GRANT UPDATE(revision) ON TABLE media.library_item TO glowloom_app;

GRANT UPDATE(update_time) ON TABLE media.library_item TO glowloom_app;

GRANT UPDATE(purged_at) ON TABLE media.library_item TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.media_asset TO glowloom_app;

GRANT UPDATE(status) ON TABLE media.media_asset TO glowloom_app;

GRANT UPDATE(mime_type) ON TABLE media.media_asset TO glowloom_app;

GRANT UPDATE(byte_size) ON TABLE media.media_asset TO glowloom_app;

GRANT UPDATE(sha256) ON TABLE media.media_asset TO glowloom_app;

GRANT UPDATE(width) ON TABLE media.media_asset TO glowloom_app;

GRANT UPDATE(height) ON TABLE media.media_asset TO glowloom_app;

GRANT UPDATE(duration_ms) ON TABLE media.media_asset TO glowloom_app;

GRANT UPDATE(fps) ON TABLE media.media_asset TO glowloom_app;

GRANT UPDATE(audio_channels) ON TABLE media.media_asset TO glowloom_app;

GRANT UPDATE(codec) ON TABLE media.media_asset TO glowloom_app;

GRANT UPDATE(moderation_status) ON TABLE media.media_asset TO glowloom_app;

GRANT UPDATE(moderation_detail) ON TABLE media.media_asset TO glowloom_app;

GRANT UPDATE(failure_reason) ON TABLE media.media_asset TO glowloom_app;

GRANT UPDATE(delete_time) ON TABLE media.media_asset TO glowloom_app;

GRANT UPDATE(purge_after) ON TABLE media.media_asset TO glowloom_app;

GRANT UPDATE(revision) ON TABLE media.media_asset TO glowloom_app;

GRANT UPDATE(update_time) ON TABLE media.media_asset TO glowloom_app;

GRANT UPDATE(is_delete) ON TABLE media.media_asset TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE media.object_location TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.package_command TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.package_job TO glowloom_app;

GRANT UPDATE(status) ON TABLE media.package_job TO glowloom_app;

GRANT UPDATE(revision) ON TABLE media.package_job TO glowloom_app;

GRANT UPDATE(library_revision) ON TABLE media.package_job TO glowloom_app;

GRANT UPDATE(project_revision) ON TABLE media.package_job TO glowloom_app;

GRANT UPDATE(result) ON TABLE media.package_job TO glowloom_app;

GRANT UPDATE(updated_at) ON TABLE media.package_job TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.package_object TO glowloom_app;

GRANT UPDATE(status) ON TABLE media.package_object TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.project_copy_object TO glowloom_app;

GRANT UPDATE(byte_size) ON TABLE media.project_copy_object TO glowloom_app;

GRANT UPDATE(source_verified) ON TABLE media.project_copy_object TO glowloom_app;

GRANT UPDATE(write_started) ON TABLE media.project_copy_object TO glowloom_app;

GRANT UPDATE(sha256) ON TABLE media.project_copy_object TO glowloom_app;

GRANT UPDATE(status) ON TABLE media.project_copy_object TO glowloom_app;

GRANT UPDATE(update_time) ON TABLE media.project_copy_object TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.project_copy_receipt TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.project_copy_snapshot TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.purge_command TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.purge_item TO glowloom_app;

GRANT UPDATE(status) ON TABLE media.purge_item TO glowloom_app;

GRANT UPDATE(failure_code) ON TABLE media.purge_item TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.purge_job TO glowloom_app;

GRANT UPDATE(status) ON TABLE media.purge_job TO glowloom_app;

GRANT UPDATE(stage) ON TABLE media.purge_job TO glowloom_app;

GRANT UPDATE(attempt) ON TABLE media.purge_job TO glowloom_app;

GRANT UPDATE(revision) ON TABLE media.purge_job TO glowloom_app;

GRANT UPDATE(cancellation_requested) ON TABLE media.purge_job TO glowloom_app;

GRANT UPDATE(needs_reconciliation) ON TABLE media.purge_job TO glowloom_app;

GRANT UPDATE(execution_unconfirmed) ON TABLE media.purge_job TO glowloom_app;

GRANT UPDATE(execution_id) ON TABLE media.purge_job TO glowloom_app;

GRANT UPDATE(worker_fence) ON TABLE media.purge_job TO glowloom_app;

GRANT UPDATE(lease_until) ON TABLE media.purge_job TO glowloom_app;

GRANT UPDATE(process_ended) ON TABLE media.purge_job TO glowloom_app;

GRANT UPDATE(updated_at) ON TABLE media.purge_job TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.purge_object TO glowloom_app;

GRANT UPDATE(byte_size) ON TABLE media.purge_object TO glowloom_app;

GRANT UPDATE(sha256) ON TABLE media.purge_object TO glowloom_app;

GRANT UPDATE(verified) ON TABLE media.purge_object TO glowloom_app;

GRANT UPDATE(removal_started) ON TABLE media.purge_object TO glowloom_app;

GRANT UPDATE(removed) ON TABLE media.purge_object TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.rendition TO glowloom_app;

GRANT UPDATE(update_time) ON TABLE media.rendition TO glowloom_app;

GRANT UPDATE(is_delete) ON TABLE media.rendition TO glowloom_app;



GRANT SELECT,INSERT ON TABLE media.storage_bucket_evidence TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.storage_config_revision TO glowloom_app;


GRANT SELECT,INSERT ON TABLE media.storage_probe_evidence TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE media.storage_probe_intent TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.storage_profile TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE media.storage_route TO glowloom_app;

GRANT SELECT, UPDATE(singleton) ON TABLE media.storage_route_lock TO glowloom_app;


GRANT SELECT,INSERT,UPDATE ON TABLE media.storage_work_intent TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.transfer_command TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.transfer_item TO glowloom_app;

GRANT UPDATE(status) ON TABLE media.transfer_item TO glowloom_app;

GRANT UPDATE(failure_code) ON TABLE media.transfer_item TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.transfer_job TO glowloom_app;

GRANT UPDATE(status) ON TABLE media.transfer_job TO glowloom_app;

GRANT UPDATE(stage) ON TABLE media.transfer_job TO glowloom_app;

GRANT UPDATE(attempt) ON TABLE media.transfer_job TO glowloom_app;

GRANT UPDATE(revision) ON TABLE media.transfer_job TO glowloom_app;

GRANT UPDATE(needs_reconciliation) ON TABLE media.transfer_job TO glowloom_app;

GRANT UPDATE(cancellation_requested) ON TABLE media.transfer_job TO glowloom_app;

GRANT UPDATE(execution_unconfirmed) ON TABLE media.transfer_job TO glowloom_app;

GRANT UPDATE(execution_id) ON TABLE media.transfer_job TO glowloom_app;

GRANT UPDATE(worker_fence) ON TABLE media.transfer_job TO glowloom_app;

GRANT UPDATE(lease_until) ON TABLE media.transfer_job TO glowloom_app;

GRANT UPDATE(process_ended) ON TABLE media.transfer_job TO glowloom_app;

GRANT UPDATE(updated_at) ON TABLE media.transfer_job TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.transfer_object TO glowloom_app;

GRANT UPDATE(byte_size) ON TABLE media.transfer_object TO glowloom_app;

GRANT UPDATE(sha256) ON TABLE media.transfer_object TO glowloom_app;

GRANT UPDATE(source_verified) ON TABLE media.transfer_object TO glowloom_app;

GRANT UPDATE(write_started) ON TABLE media.transfer_object TO glowloom_app;

GRANT UPDATE(status) ON TABLE media.transfer_object TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE media.upload_admission TO glowloom_app;

GRANT SELECT,INSERT ON TABLE media.upload_request TO glowloom_app;

GRANT SELECT,INSERT ON TABLE mediatool.depth_command TO glowloom_app;

GRANT SELECT,INSERT ON TABLE mediatool.depth_job TO glowloom_app;

GRANT UPDATE(status) ON TABLE mediatool.depth_job TO glowloom_app;

GRANT UPDATE(stage) ON TABLE mediatool.depth_job TO glowloom_app;

GRANT UPDATE(attempt) ON TABLE mediatool.depth_job TO glowloom_app;

GRANT UPDATE(revision) ON TABLE mediatool.depth_job TO glowloom_app;

GRANT UPDATE(process_state) ON TABLE mediatool.depth_job TO glowloom_app;

GRANT UPDATE(active_worker) ON TABLE mediatool.depth_job TO glowloom_app;

GRANT UPDATE(failure_code) ON TABLE mediatool.depth_job TO glowloom_app;

GRANT UPDATE(retryable) ON TABLE mediatool.depth_job TO glowloom_app;

GRANT UPDATE(needs_reconciliation) ON TABLE mediatool.depth_job TO glowloom_app;

GRANT UPDATE(execution_unconfirmed) ON TABLE mediatool.depth_job TO glowloom_app;

GRANT UPDATE(cancellation_requested) ON TABLE mediatool.depth_job TO glowloom_app;

GRANT UPDATE(reconciliation_requested) ON TABLE mediatool.depth_job TO glowloom_app;

GRANT UPDATE(updated_at) ON TABLE mediatool.depth_job TO glowloom_app;

GRANT SELECT,INSERT ON TABLE mediatool.depth_object TO glowloom_app;

GRANT UPDATE(write_started) ON TABLE mediatool.depth_object TO glowloom_app;

GRANT UPDATE(delete_started) ON TABLE mediatool.depth_object TO glowloom_app;

GRANT UPDATE(status) ON TABLE mediatool.depth_object TO glowloom_app;

GRANT UPDATE(updated_at) ON TABLE mediatool.depth_object TO glowloom_app;

GRANT SELECT,INSERT ON TABLE mediatool.depth_result_intent TO glowloom_app;

GRANT SELECT,INSERT ON TABLE mediatool.export_command TO glowloom_app;

GRANT SELECT,INSERT ON TABLE mediatool.export_job TO glowloom_app;

GRANT UPDATE(actor_id) ON TABLE mediatool.export_job TO glowloom_app;

GRANT UPDATE(actor_role) ON TABLE mediatool.export_job TO glowloom_app;

GRANT UPDATE(status) ON TABLE mediatool.export_job TO glowloom_app;

GRANT UPDATE(stage) ON TABLE mediatool.export_job TO glowloom_app;

GRANT UPDATE(progress) ON TABLE mediatool.export_job TO glowloom_app;

GRANT UPDATE(attempt) ON TABLE mediatool.export_job TO glowloom_app;

GRANT UPDATE(revision) ON TABLE mediatool.export_job TO glowloom_app;

GRANT UPDATE(asset_id) ON TABLE mediatool.export_job TO glowloom_app;

GRANT UPDATE(sha256) ON TABLE mediatool.export_job TO glowloom_app;

GRANT UPDATE(failure_code) ON TABLE mediatool.export_job TO glowloom_app;

GRANT UPDATE(active_worker) ON TABLE mediatool.export_job TO glowloom_app;

GRANT UPDATE(updated_at) ON TABLE mediatool.export_job TO glowloom_app;

GRANT SELECT,INSERT ON TABLE mediatool.transcription_command TO glowloom_app;

GRANT SELECT,INSERT ON TABLE mediatool.transcription_job TO glowloom_app;

GRANT UPDATE(actor_id) ON TABLE mediatool.transcription_job TO glowloom_app;

GRANT UPDATE(actor_role) ON TABLE mediatool.transcription_job TO glowloom_app;

GRANT UPDATE(status) ON TABLE mediatool.transcription_job TO glowloom_app;

GRANT UPDATE(stage) ON TABLE mediatool.transcription_job TO glowloom_app;

GRANT UPDATE(progress) ON TABLE mediatool.transcription_job TO glowloom_app;

GRANT UPDATE(attempt) ON TABLE mediatool.transcription_job TO glowloom_app;

GRANT UPDATE(revision) ON TABLE mediatool.transcription_job TO glowloom_app;

GRANT UPDATE(inference_state) ON TABLE mediatool.transcription_job TO glowloom_app;

GRANT UPDATE(result) ON TABLE mediatool.transcription_job TO glowloom_app;

GRANT UPDATE(result_sha256) ON TABLE mediatool.transcription_job TO glowloom_app;

GRANT UPDATE(failure_code) ON TABLE mediatool.transcription_job TO glowloom_app;

GRANT UPDATE(active_worker) ON TABLE mediatool.transcription_job TO glowloom_app;

GRANT UPDATE(updated_at) ON TABLE mediatool.transcription_job TO glowloom_app;

GRANT SELECT,INSERT ON TABLE operation.batch TO glowloom_app;

GRANT UPDATE(status) ON TABLE operation.batch TO glowloom_app;

GRANT UPDATE(paused_reason) ON TABLE operation.batch TO glowloom_app;

GRANT UPDATE(total_count) ON TABLE operation.batch TO glowloom_app;

GRANT UPDATE(succeeded_count) ON TABLE operation.batch TO glowloom_app;

GRANT UPDATE(failed_count) ON TABLE operation.batch TO glowloom_app;

GRANT UPDATE(unknown_count) ON TABLE operation.batch TO glowloom_app;

GRANT UPDATE(quote_total_micros) ON TABLE operation.batch TO glowloom_app;

GRANT UPDATE(workflow_id) ON TABLE operation.batch TO glowloom_app;

GRANT UPDATE(update_time) ON TABLE operation.batch TO glowloom_app;

GRANT UPDATE(cancel_requested_at) ON TABLE operation.batch TO glowloom_app;

GRANT SELECT,INSERT ON TABLE operation.batch_launch TO glowloom_app;

GRANT UPDATE(state) ON TABLE operation.batch_launch TO glowloom_app;

GRANT UPDATE(released_at) ON TABLE operation.batch_launch TO glowloom_app;

GRANT SELECT,INSERT ON TABLE operation.confirmation_request TO glowloom_app;

GRANT SELECT,INSERT ON TABLE operation.operation TO glowloom_app;

GRANT UPDATE(status) ON TABLE operation.operation TO glowloom_app;

GRANT UPDATE(failure_code) ON TABLE operation.operation TO glowloom_app;

GRANT UPDATE(failure_message) ON TABLE operation.operation TO glowloom_app;

GRANT UPDATE(retryable) ON TABLE operation.operation TO glowloom_app;

GRANT UPDATE(reservation_id) ON TABLE operation.operation TO glowloom_app;

GRANT UPDATE(provider_request_key) ON TABLE operation.operation TO glowloom_app;

GRANT UPDATE(workflow_id) ON TABLE operation.operation TO glowloom_app;

GRANT UPDATE(confirmed_at) ON TABLE operation.operation TO glowloom_app;

GRANT UPDATE(confirmed_by) ON TABLE operation.operation TO glowloom_app;

GRANT UPDATE(started_at) ON TABLE operation.operation TO glowloom_app;

GRANT UPDATE(finished_at) ON TABLE operation.operation TO glowloom_app;

GRANT UPDATE(settled_micros) ON TABLE operation.operation TO glowloom_app;

GRANT UPDATE(cost_estimated) ON TABLE operation.operation TO glowloom_app;

GRANT UPDATE(update_time) ON TABLE operation.operation TO glowloom_app;

GRANT SELECT,INSERT ON TABLE operation.operation_event TO glowloom_app;

GRANT SELECT,INSERT ON TABLE operation.operation_input TO glowloom_app;

GRANT SELECT,INSERT ON TABLE operation.operation_output TO glowloom_app;

GRANT UPDATE(moderation_status) ON TABLE operation.operation_output TO glowloom_app;

GRANT UPDATE(moderation_reason) ON TABLE operation.operation_output TO glowloom_app;

GRANT UPDATE(update_time) ON TABLE operation.operation_output TO glowloom_app;

GRANT SELECT,INSERT ON TABLE operation.prompt_resolution_receipt TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE operation.prompt_submission TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE operation.prompt_submission_slot TO glowloom_app;

GRANT SELECT,INSERT ON TABLE operation.provider_binding TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE operation.provider_call TO glowloom_app;

GRANT SELECT,INSERT ON TABLE operation.provider_credential_binding TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE operation.provider_result TO glowloom_app;

GRANT SELECT,INSERT ON TABLE operation.provider_trial TO glowloom_app;

GRANT SELECT,INSERT ON TABLE operation.quote_request TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.action_line TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.command TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.command_result TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.command_state TO glowloom_app;

GRANT UPDATE(revision) ON TABLE script.command_state TO glowloom_app;

GRANT UPDATE(status) ON TABLE script.command_state TO glowloom_app;

GRANT UPDATE(failure_code) ON TABLE script.command_state TO glowloom_app;

GRANT UPDATE(cancellation_requested) ON TABLE script.command_state TO glowloom_app;

GRANT UPDATE(needs_reconciliation) ON TABLE script.command_state TO glowloom_app;

GRANT UPDATE(io_owner_id) ON TABLE script.command_state TO glowloom_app;

GRANT UPDATE(io_state) ON TABLE script.command_state TO glowloom_app;

GRANT UPDATE(updated_at) ON TABLE script.command_state TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.copy_object_intent TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.copy_object_state TO glowloom_app;

GRANT UPDATE(put_started) ON TABLE script.copy_object_state TO glowloom_app;

GRANT UPDATE(confirmed) ON TABLE script.copy_object_state TO glowloom_app;

GRANT UPDATE(removed) ON TABLE script.copy_object_state TO glowloom_app;

GRANT UPDATE(updated_at) ON TABLE script.copy_object_state TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.copy_receipt TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.copy_snapshot TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.dialogue_line TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.episode TO glowloom_app;

GRANT UPDATE(title) ON TABLE script.episode TO glowloom_app;

GRANT UPDATE(revision) ON TABLE script.episode TO glowloom_app;

GRANT UPDATE(current_structure_id) ON TABLE script.episode TO glowloom_app;

GRANT UPDATE(confirmed_structure_id) ON TABLE script.episode TO glowloom_app;

GRANT UPDATE(previous_episode_id) ON TABLE script.episode TO glowloom_app;

GRANT UPDATE(inherit_status) ON TABLE script.episode TO glowloom_app;

GRANT UPDATE(is_delete) ON TABLE script.episode TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.episode_structure TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.import_attempt TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.import_command TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.import_file TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.import_file_result TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.import_job TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.import_publication TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.import_state TO glowloom_app;

GRANT UPDATE(revision) ON TABLE script.import_state TO glowloom_app;

GRANT UPDATE(attempt) ON TABLE script.import_state TO glowloom_app;

GRANT UPDATE(status) ON TABLE script.import_state TO glowloom_app;

GRANT UPDATE(stage) ON TABLE script.import_state TO glowloom_app;

GRANT UPDATE(latest_script_revision) ON TABLE script.import_state TO glowloom_app;

GRANT UPDATE(latest_version_id) ON TABLE script.import_state TO glowloom_app;

GRANT UPDATE(cancellation_requested) ON TABLE script.import_state TO glowloom_app;

GRANT UPDATE(reconciliation_requested) ON TABLE script.import_state TO glowloom_app;

GRANT UPDATE(needs_reconciliation) ON TABLE script.import_state TO glowloom_app;

GRANT UPDATE(failure_code) ON TABLE script.import_state TO glowloom_app;

GRANT UPDATE(io_owner_id) ON TABLE script.import_state TO glowloom_app;

GRANT UPDATE(io_state) ON TABLE script.import_state TO glowloom_app;

GRANT UPDATE(updated_at) ON TABLE script.import_state TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.object_intent TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.object_state TO glowloom_app;

GRANT UPDATE(put_started) ON TABLE script.object_state TO glowloom_app;

GRANT UPDATE(confirmed) ON TABLE script.object_state TO glowloom_app;

GRANT UPDATE(removed) ON TABLE script.object_state TO glowloom_app;

GRANT UPDATE(updated_at) ON TABLE script.object_state TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.project_state TO glowloom_app;

GRANT UPDATE(revision) ON TABLE script.project_state TO glowloom_app;

GRANT UPDATE(draft_version_id) ON TABLE script.project_state TO glowloom_app;

GRANT UPDATE(adopted_version_id) ON TABLE script.project_state TO glowloom_app;

GRANT UPDATE(updated_at) ON TABLE script.project_state TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.request TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.review_command TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.scene TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.script_source TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.script_version TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.source_control TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.split_confirmation TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.split_set TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.version_head TO glowloom_app;

GRANT UPDATE(split_revision) ON TABLE script.version_head TO glowloom_app;

GRANT UPDATE(candidate_split_set_id) ON TABLE script.version_head TO glowloom_app;

GRANT UPDATE(confirmed_split_set_id) ON TABLE script.version_head TO glowloom_app;

GRANT SELECT,INSERT ON TABLE script.version_source TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE workspace.creation_scope TO glowloom_app;

GRANT SELECT,INSERT ON TABLE workspace.organization TO glowloom_app;

GRANT UPDATE(update_time) ON TABLE workspace.organization TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE workspace.project TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE workspace.project_activity TO glowloom_app;

GRANT SELECT,INSERT ON TABLE workspace.project_change_command TO glowloom_app;

GRANT SELECT,INSERT ON TABLE workspace.project_copy_command TO glowloom_app;

GRANT SELECT,INSERT ON TABLE workspace.project_copy_job TO glowloom_app;

GRANT UPDATE(status) ON TABLE workspace.project_copy_job TO glowloom_app;

GRANT UPDATE(stage) ON TABLE workspace.project_copy_job TO glowloom_app;

GRANT UPDATE(revision) ON TABLE workspace.project_copy_job TO glowloom_app;

GRANT UPDATE(attempt) ON TABLE workspace.project_copy_job TO glowloom_app;

GRANT UPDATE(worker_id) ON TABLE workspace.project_copy_job TO glowloom_app;

GRANT UPDATE(started_at) ON TABLE workspace.project_copy_job TO glowloom_app;

GRANT UPDATE(media_receipt) ON TABLE workspace.project_copy_job TO glowloom_app;

GRANT UPDATE(canvas_receipt) ON TABLE workspace.project_copy_job TO glowloom_app;

GRANT UPDATE(failure_code) ON TABLE workspace.project_copy_job TO glowloom_app;

GRANT UPDATE(retryable) ON TABLE workspace.project_copy_job TO glowloom_app;

GRANT UPDATE(needs_reconciliation) ON TABLE workspace.project_copy_job TO glowloom_app;

GRANT UPDATE(cancellation_requested) ON TABLE workspace.project_copy_job TO glowloom_app;

GRANT UPDATE(reconciliation_requested) ON TABLE workspace.project_copy_job TO glowloom_app;

GRANT UPDATE(execution_unconfirmed) ON TABLE workspace.project_copy_job TO glowloom_app;

GRANT UPDATE(update_time) ON TABLE workspace.project_copy_job TO glowloom_app;

GRANT UPDATE(script_receipt) ON TABLE workspace.project_copy_job TO glowloom_app;

GRANT UPDATE(bible_receipt) ON TABLE workspace.project_copy_job TO glowloom_app;

GRANT UPDATE(execution_actor_id) ON TABLE workspace.project_copy_job TO glowloom_app;

GRANT SELECT,INSERT ON TABLE workspace.project_creation_receipt TO glowloom_app;

GRANT SELECT,INSERT ON TABLE workspace.project_folder TO glowloom_app;

GRANT UPDATE(name) ON TABLE workspace.project_folder TO glowloom_app;

GRANT UPDATE(cover_project_id) ON TABLE workspace.project_folder TO glowloom_app;

GRANT UPDATE(cover_asset_id) ON TABLE workspace.project_folder TO glowloom_app;

GRANT UPDATE(revision) ON TABLE workspace.project_folder TO glowloom_app;

GRANT UPDATE(is_delete) ON TABLE workspace.project_folder TO glowloom_app;

GRANT UPDATE(delete_time) ON TABLE workspace.project_folder TO glowloom_app;

GRANT UPDATE(update_time) ON TABLE workspace.project_folder TO glowloom_app;

GRANT SELECT,INSERT ON TABLE workspace.project_folder_command TO glowloom_app;

GRANT SELECT,INSERT ON TABLE workspace.project_folder_placement TO glowloom_app;

GRANT UPDATE(folder_id) ON TABLE workspace.project_folder_placement TO glowloom_app;

GRANT UPDATE(revision) ON TABLE workspace.project_folder_placement TO glowloom_app;

GRANT UPDATE(update_time) ON TABLE workspace.project_folder_placement TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE workspace.prompt_customization TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE workspace.prompt_draft TO glowloom_app;

GRANT SELECT,INSERT ON TABLE workspace.prompt_draft_receipt TO glowloom_app;

ALTER TABLE ONLY workspace.style_preset ADD CONSTRAINT style_preset_owner_user_fk FOREIGN KEY(owner_user_id) REFERENCES identity."user"(id) ON DELETE RESTRICT;
ALTER TABLE ONLY workspace.style_preset_command ADD CONSTRAINT style_preset_command_actor_fk FOREIGN KEY(actor_id) REFERENCES identity."user"(id) ON DELETE RESTRICT;
ALTER TABLE ONLY workspace.style_preset_command ADD CONSTRAINT style_preset_command_preset_fk FOREIGN KEY(preset_id) REFERENCES workspace.style_preset(id) ON DELETE RESTRICT;
ALTER TABLE ONLY workspace.style_preset_command ADD CONSTRAINT style_preset_command_scope_fk FOREIGN KEY(owner_scope_id,org_id) REFERENCES workspace.creation_scope(id,physical_scope_id) ON DELETE RESTRICT;
ALTER TABLE ONLY workspace.style_preset_reference ADD CONSTRAINT style_preset_reference_preset_fk FOREIGN KEY(preset_id) REFERENCES workspace.style_preset(id) ON DELETE RESTRICT;
ALTER TABLE ONLY workspace.style_preset_reference ADD CONSTRAINT style_preset_reference_asset_fk FOREIGN KEY(asset_id) REFERENCES media.media_asset(id) ON DELETE RESTRICT;

GRANT SELECT,INSERT ON TABLE workspace.style_preset TO glowloom_app;
GRANT UPDATE(name,library_category,prompt_fragment,reference_asset_ids,is_delete,revision) ON TABLE workspace.style_preset TO glowloom_app;
GRANT SELECT,INSERT ON TABLE workspace.style_preset_command TO glowloom_app;
GRANT SELECT,INSERT ON TABLE workspace.style_preset_reference TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE workspace.team TO glowloom_app;

GRANT SELECT,INSERT,UPDATE ON TABLE workspace.team_member TO glowloom_app;

-- 分区按初始化时的当前 UTC 月及未来三个月生成，并保留默认分区。
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

INSERT INTO identity.platform_settings(singleton) VALUES(true);



-- Versioned cloud provider expansion; existing connection snapshots stay pinned.

-- Project recycle retention worker (metadata only until explicitly scheduled).
CREATE TABLE workspace.project_retention_job (
 project_id uuid NOT NULL REFERENCES workspace.project(id),
 org_id uuid NOT NULL,
 owner_scope_id uuid NOT NULL,
 deleted_revision bigint NOT NULL CHECK(deleted_revision>0),
 purge_after timestamptz NOT NULL,
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','completed','skipped')),
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts>=0),
 last_error text,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 completed_at timestamptz,
 PRIMARY KEY(project_id,deleted_revision)
);
CREATE INDEX project_retention_pending ON workspace.project_retention_job(purge_after,project_id) WHERE status='pending';
CREATE INDEX project_recycle_expiry ON workspace.project(purge_after,id) WHERE is_delete;
GRANT SELECT,INSERT,UPDATE ON workspace.project_retention_job TO glowloom_app;
-- Only the internal retention transaction uses these; media entities and blobs are retained.
GRANT DELETE ON canvas.edge,canvas.node TO glowloom_app;
GRANT UPDATE(response_body) ON canvas.save_receipt TO glowloom_app;
GRANT DELETE ON workspace.prompt_draft TO glowloom_app;
GRANT UPDATE ON workspace.prompt_draft_receipt TO glowloom_app;

-- Durable Yjs collaboration and legacy-writer fence.
-- Full binary checkpoints are shared by the head and immutable history.
CREATE TABLE canvas.collab_checkpoint (
 id uuid PRIMARY KEY,
 document_id uuid NOT NULL REFERENCES canvas.document(id),
 state bytea NOT NULL CHECK(octet_length(state)<=8388608),
 UNIQUE(document_id,id)
);
GRANT SELECT,INSERT,DELETE ON canvas.collab_checkpoint TO glowloom_app;

CREATE TABLE canvas.collab_document (
 document_id uuid PRIMARY KEY REFERENCES canvas.document(id),
 epoch uuid NOT NULL,
 writer_term bigint NOT NULL DEFAULT 1 CHECK(writer_term>0),
 commit_seq bigint NOT NULL DEFAULT 0 CHECK(commit_seq>=0),
 content_revision bigint NOT NULL CHECK(content_revision>0),
 state_id uuid NOT NULL,
 graph_hash text NOT NULL CHECK(length(graph_hash)=64),
 model_version integer NOT NULL DEFAULT 1 CHECK(model_version=1),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(document_id,state_id) REFERENCES canvas.collab_checkpoint(document_id,id)
);
CREATE TABLE canvas.collab_update (
 document_id uuid NOT NULL REFERENCES canvas.collab_document(document_id),
 epoch uuid NOT NULL,
 commit_seq bigint NOT NULL CHECK(commit_seq>0),
 content_revision bigint NOT NULL,
 actor_id uuid NOT NULL,
 session_id uuid NOT NULL,
 update_id uuid NOT NULL,
 payload_hash text NOT NULL CHECK(length(payload_hash)=64),
 update_bytes bytea NOT NULL CHECK(octet_length(update_bytes)<=786432),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(document_id,epoch,commit_seq)
);
CREATE TABLE canvas.collab_receipt (
 document_id uuid NOT NULL REFERENCES canvas.document(id),
 epoch uuid NOT NULL,
 actor_id uuid NOT NULL,
 session_id uuid NOT NULL,
 update_id uuid NOT NULL,
 request_hash text NOT NULL CHECK(length(request_hash)=64),
 response_body jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(document_id,epoch,actor_id,session_id,update_id)
);
GRANT SELECT,INSERT,UPDATE ON canvas.collab_document TO glowloom_app;
GRANT SELECT,INSERT ON canvas.collab_update,canvas.collab_receipt TO glowloom_app;

CREATE FUNCTION canvas.guard_collab_writer() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target uuid;
BEGIN
 IF TG_TABLE_NAME='document' THEN target:=OLD.id;
 ELSIF TG_OP='DELETE' THEN target:=OLD.document_id;
 ELSE target:=NEW.document_id;
 END IF;
 -- Serialize migration with every old graph writer, including direct node jobs.
 PERFORM 1 FROM canvas.document WHERE id=target FOR UPDATE;
 IF EXISTS(SELECT 1 FROM canvas.collab_document WHERE document_id=target)
    AND current_setting('glowloom.yjs_document_id',true) IS DISTINCT FROM target::text THEN
  RAISE EXCEPTION 'canvas requires the Yjs acceptance path' USING ERRCODE='42501';
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER canvas_collab_document_writer BEFORE UPDATE OR DELETE ON canvas.document FOR EACH ROW EXECUTE FUNCTION canvas.guard_collab_writer();
CREATE TRIGGER canvas_collab_node_writer BEFORE INSERT OR UPDATE OR DELETE ON canvas.node FOR EACH ROW EXECUTE FUNCTION canvas.guard_collab_writer();
CREATE TRIGGER canvas_collab_edge_writer BEFORE INSERT OR UPDATE OR DELETE ON canvas.edge FOR EACH ROW EXECUTE FUNCTION canvas.guard_collab_writer();

CREATE TABLE canvas.collab_version (
 id uuid PRIMARY KEY, document_id uuid NOT NULL REFERENCES canvas.document(id),
 epoch uuid NOT NULL, revision bigint NOT NULL, document_json jsonb NOT NULL,
 writer_term bigint NOT NULL, commit_seq bigint NOT NULL, state_id uuid NOT NULL, graph_hash text NOT NULL,
 actor_id uuid NOT NULL, reason text NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(document_id,revision), CHECK(octet_length(document_json::text)<=16777216),
 FOREIGN KEY(document_id,state_id) REFERENCES canvas.collab_checkpoint(document_id,id)
);
CREATE INDEX collab_version_state_idx ON canvas.collab_version(document_id,state_id);
CREATE TABLE canvas.collab_control_receipt (
 document_id uuid NOT NULL REFERENCES canvas.document(id), actor_id uuid NOT NULL,
 session_id uuid NOT NULL, command_id uuid NOT NULL, request_hash text NOT NULL,
 response_body jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(document_id,actor_id,session_id,command_id)
);
GRANT SELECT,INSERT ON canvas.collab_version,canvas.collab_control_receipt TO glowloom_app;

-- Capability only: preview and explicitly select each historical job via the
-- project-copy-primary operator command. Never infer primary IDs in bulk SQL.
CREATE TABLE workspace.project_copy_primary_repair (
 job_id uuid PRIMARY KEY REFERENCES workspace.project_copy_job(id),
 org_id uuid NOT NULL REFERENCES workspace.organization(id),
 target_project_id uuid NOT NULL REFERENCES workspace.project(id),
 source_document_id uuid NOT NULL,
 target_document_id uuid NOT NULL,
 review_hash text NOT NULL CHECK(review_hash ~ '^[0-9a-f]{64}$'),
 before_revision bigint NOT NULL CHECK(before_revision > 0),
 after_revision bigint NOT NULL CHECK(after_revision = before_revision + 1),
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 500),
 operator_name text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
REVOKE ALL ON workspace.project_copy_primary_repair FROM PUBLIC, glowloom_app;

ALTER TABLE catalog.model_release ADD CONSTRAINT model_release_onboarding_check_fkey FOREIGN KEY(onboarding_check_id) REFERENCES catalog.provider_check(id);
ALTER TABLE catalog.provider_check ADD CONSTRAINT provider_check_model_fkey FOREIGN KEY(model_id) REFERENCES catalog.model_profile(id);
ALTER TABLE catalog.provider_check ADD CONSTRAINT provider_check_model_version_fkey FOREIGN KEY(model_version_id) REFERENCES catalog.model_profile_version(id);

-- Space-owned subjects are independent of the project Bible. Append-only
-- versions and bindings retain source-media evidence after product deletion.
CREATE SCHEMA subject;
CREATE TABLE subject.subject_head (
 id uuid PRIMARY KEY,
 owner_scope_id uuid NOT NULL REFERENCES workspace.creation_scope(id),
 physical_scope_id uuid NOT NULL REFERENCES workspace.organization(id),
 revision bigint NOT NULL CHECK(revision BETWEEN 1 AND 2147483647),
 current_version_id uuid NOT NULL,
 created_by uuid NOT NULL,
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 deleted_at timestamptz,
 UNIQUE(id,owner_scope_id,physical_scope_id),
 CHECK(updated_at>=created_at)
);
CREATE TABLE subject.subject_version (
 id uuid PRIMARY KEY,
 subject_id uuid NOT NULL,
 owner_scope_id uuid NOT NULL,
 physical_scope_id uuid NOT NULL,
 version_no bigint NOT NULL CHECK(version_no BETWEEN 1 AND 2147483647),
 previous_id uuid REFERENCES subject.subject_version(id),
 created_by uuid NOT NULL,
 created_at timestamptz NOT NULL,
 content jsonb NOT NULL CHECK(jsonb_typeof(content)='object' AND octet_length(content::text)<=1048576),
 content_sha256 text NOT NULL CHECK(content_sha256 ~ '^[a-f0-9]{64}$'),
 UNIQUE(subject_id,version_no),
 UNIQUE(id,subject_id,owner_scope_id,physical_scope_id),
 FOREIGN KEY(subject_id,owner_scope_id,physical_scope_id) REFERENCES subject.subject_head(id,owner_scope_id,physical_scope_id) DEFERRABLE INITIALLY DEFERRED,
 CHECK((version_no=1 AND previous_id IS NULL) OR (version_no>1 AND previous_id IS NOT NULL))
);
ALTER TABLE subject.subject_head ADD CONSTRAINT subject_current_version_fk
 FOREIGN KEY(current_version_id,id,owner_scope_id,physical_scope_id)
 REFERENCES subject.subject_version(id,subject_id,owner_scope_id,physical_scope_id)
 DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE subject.media_binding (
 binding_id uuid PRIMARY KEY,
 subject_id uuid NOT NULL,
 version_id uuid NOT NULL,
 owner_scope_id uuid NOT NULL,
 physical_scope_id uuid NOT NULL,
 position integer NOT NULL CHECK(position BETWEEN 0 AND 119),
 role text NOT NULL CHECK(role IN ('primary','reference','voice')),
 asset_id uuid NOT NULL REFERENCES media.media_asset(id),
 asset_revision bigint NOT NULL CHECK(asset_revision>0),
 kind text NOT NULL CHECK(kind IN ('image','video','audio')),
 sha256 text NOT NULL CHECK(sha256 ~ '^[a-f0-9]{64}$'),
 binding_sha256 text NOT NULL CHECK(binding_sha256 ~ '^[a-f0-9]{64}$'),
 fact jsonb NOT NULL CHECK(jsonb_typeof(fact)='object' AND octet_length(fact::text)<=65536),
 UNIQUE(version_id,role,position),
 FOREIGN KEY(version_id,subject_id,owner_scope_id,physical_scope_id)
 REFERENCES subject.subject_version(id,subject_id,owner_scope_id,physical_scope_id),
 CHECK((role='primary' AND position=0 AND kind IN ('image','video')) OR (role='reference' AND kind='image') OR (role='voice' AND position=0 AND kind='audio')),
 CHECK((fact->>'asset_id')::uuid=asset_id AND (fact->>'asset_revision')::bigint=asset_revision AND fact->>'kind'=kind AND fact->>'sha256'=sha256)
);
CREATE TABLE subject.favorite (
 actor_id uuid NOT NULL,
 subject_id uuid NOT NULL,
 owner_scope_id uuid NOT NULL,
 physical_scope_id uuid NOT NULL,
 favorite boolean NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(actor_id,subject_id),
 FOREIGN KEY(subject_id,owner_scope_id,physical_scope_id) REFERENCES subject.subject_head(id,owner_scope_id,physical_scope_id)
);
CREATE TABLE subject.command (
 actor_id uuid NOT NULL,
 idem_key uuid NOT NULL,
 owner_scope_id uuid NOT NULL REFERENCES workspace.creation_scope(id),
 physical_scope_id uuid NOT NULL REFERENCES workspace.organization(id),
 request_sha256 text NOT NULL CHECK(request_sha256 ~ '^[a-f0-9]{64}$'),
 response_body jsonb NOT NULL CHECK(jsonb_typeof(response_body)='object' AND octet_length(response_body::text)<=1048576),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(actor_id,idem_key)
);
CREATE INDEX subject_head_updated ON subject.subject_head(owner_scope_id,physical_scope_id,updated_at,id) WHERE deleted_at IS NULL;
CREATE INDEX subject_head_created ON subject.subject_head(owner_scope_id,physical_scope_id,created_at,id) WHERE deleted_at IS NULL;
CREATE INDEX subject_binding_retained_asset ON subject.media_binding(asset_id);
CREATE INDEX subject_binding_version ON subject.media_binding(owner_scope_id,physical_scope_id,subject_id,version_id);
CREATE INDEX subject_favorite_actor ON subject.favorite(actor_id,owner_scope_id,subject_id) WHERE favorite;
GRANT USAGE ON SCHEMA subject TO glowloom_app;
GRANT SELECT,INSERT,UPDATE ON subject.subject_head,subject.favorite TO glowloom_app;
GRANT SELECT,INSERT ON subject.subject_version,subject.media_binding,subject.command TO glowloom_app;
REVOKE DELETE ON subject.subject_head,subject.subject_version,subject.media_binding,subject.command FROM glowloom_app;

ALTER TABLE subject.subject_head ADD CONSTRAINT subject_scope_physical_fk FOREIGN KEY(owner_scope_id,physical_scope_id) REFERENCES workspace.creation_scope(id,physical_scope_id);
ALTER TABLE subject.command ADD CONSTRAINT subject_command_scope_physical_fk FOREIGN KEY(owner_scope_id,physical_scope_id) REFERENCES workspace.creation_scope(id,physical_scope_id);


-- Canvas asset collections and preferences.
ALTER TABLE canvas.document
 ADD COLUMN asset_collection text NOT NULL DEFAULT 'current';
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='canvas.document'::regclass AND conname='document_asset_collection_check') THEN
  ALTER TABLE canvas.document ADD CONSTRAINT document_asset_collection_check CHECK(asset_collection IN ('current','legacy'));
 END IF;
END $$;
CREATE TABLE canvas.asset_preference (
 actor_id uuid NOT NULL REFERENCES identity."user"(id),
 scope_id uuid NOT NULL REFERENCES workspace.creation_scope(id),
 document_id uuid NOT NULL REFERENCES canvas.document(id),
 favorite boolean NOT NULL DEFAULT false,
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(actor_id,scope_id,document_id)
);
GRANT SELECT,INSERT,UPDATE ON canvas.asset_preference TO glowloom_app;
CREATE INDEX ix_canvas_asset_favorites ON canvas.asset_preference(actor_id,scope_id,document_id) WHERE favorite;


-- Personal views never alter operation execution or shared media ownership.
CREATE TABLE operation.personal_history_record (
 actor_id uuid NOT NULL,
 org_id uuid NOT NULL,
 scope_id uuid NOT NULL,
 operation_id uuid NOT NULL REFERENCES operation.operation(id),
 revision bigint NOT NULL DEFAULT 0 CHECK(revision>=0),
 deleted_at timestamptz,
 PRIMARY KEY(actor_id,org_id,scope_id,operation_id)
);
CREATE TABLE operation.personal_history_output (
 actor_id uuid NOT NULL,
 org_id uuid NOT NULL,
 scope_id uuid NOT NULL,
 operation_id uuid NOT NULL REFERENCES operation.operation(id),
 output_id uuid NOT NULL REFERENCES operation.operation_output(id),
 revision bigint NOT NULL DEFAULT 0 CHECK(revision>=0),
 favorite boolean NOT NULL DEFAULT false,
 deleted_at timestamptz,
 PRIMARY KEY(actor_id,org_id,scope_id,operation_id,output_id)
);
CREATE INDEX personal_history_output_favorite_idx
 ON operation.personal_history_output(actor_id,org_id,scope_id,operation_id) WHERE favorite AND deleted_at IS NULL;
CREATE TABLE operation.personal_history_command_receipt (
 actor_id uuid NOT NULL,
 org_id uuid NOT NULL,
 scope_id uuid NOT NULL,
 request_id uuid NOT NULL,
 request_hash text NOT NULL,
 response jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(actor_id,org_id,scope_id,request_id)
);
GRANT SELECT,INSERT,UPDATE ON operation.personal_history_record,operation.personal_history_output TO glowloom_app;
GRANT SELECT,INSERT ON operation.personal_history_command_receipt TO glowloom_app;


-- Explicitly selected public works; project and sibling results stay private.
CREATE TABLE operation.published_work (
 id uuid PRIMARY KEY,
 actor_id uuid NOT NULL,
 org_id uuid NOT NULL,
 scope_id uuid NOT NULL,
 project_id uuid NOT NULL REFERENCES workspace.project(id),
 operation_id uuid NOT NULL REFERENCES operation.operation(id),
 output_id uuid NOT NULL REFERENCES operation.operation_output(id),
 media_asset_id uuid NOT NULL REFERENCES media.media_asset(id),
 title text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 120),
 description text NOT NULL DEFAULT '' CHECK (char_length(description)<=2000),
 prompt text,
 created_at timestamptz NOT NULL DEFAULT now(),
 withdrawn_at timestamptz,
 UNIQUE(actor_id,output_id)
);
CREATE TABLE operation.history_publication_receipt (
 actor_id uuid NOT NULL,
 org_id uuid NOT NULL,
 scope_id uuid NOT NULL,
 request_key uuid NOT NULL,
 request_hash text NOT NULL,
 response jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(actor_id,org_id,scope_id,request_key)
);
GRANT SELECT,INSERT ON operation.published_work,operation.history_publication_receipt TO glowloom_app;

-- SRC-AI-28: separately authenticated Doubao speech service.

-- Subject-owned Doubao speech sample jobs; no synthetic project identities.
CREATE TABLE subject.voice_job (
 id uuid PRIMARY KEY,
 actor_id uuid NOT NULL REFERENCES identity."user"(id),
 owner_scope_id uuid NOT NULL,
 physical_scope_id uuid NOT NULL,
 personal_scope_id uuid NOT NULL,
 personal_org_id uuid NOT NULL,
 draft_intent_id uuid NOT NULL,
 idem_key uuid NOT NULL,
 request_sha256 text NOT NULL CHECK(request_sha256 ~ '^[a-f0-9]{64}$'),
 name text NOT NULL CHECK(char_length(name) BETWEEN 1 AND 20 AND name=btrim(name)),
 text text NOT NULL CHECK(char_length(text) BETWEEN 1 AND 500),
 release_id uuid NOT NULL REFERENCES catalog.model_release(id),
 provider_id uuid NOT NULL REFERENCES catalog.provider(id),
 config_version_id uuid NOT NULL REFERENCES catalog.provider_config_version(id),
 credential_id uuid NOT NULL REFERENCES catalog.provider_credential(id),
 model_version_id uuid NOT NULL REFERENCES catalog.model_profile_version(id),
 price_version_id uuid NOT NULL REFERENCES catalog.price_rule_version(id),
 resource_id text NOT NULL CHECK(resource_id='seed-tts-2.0'),
 voice_key text NOT NULL CHECK(char_length(voice_key) BETWEEN 1 AND 512),
 speech_rate integer NOT NULL DEFAULT 0 CHECK(speech_rate BETWEEN -50 AND 100),
 upstream_cost jsonb NOT NULL CHECK(jsonb_typeof(upstream_cost)='object'),
 moderation text NOT NULL CHECK(moderation='provider'),
 event_id uuid NOT NULL UNIQUE,
 asset_id uuid NOT NULL UNIQUE,
 output_asset_id uuid REFERENCES media.media_asset(id),
 storage_intent_id uuid,
 status text NOT NULL DEFAULT 'queued' CHECK(status IN ('queued','dispatching','ingesting','succeeded','failed','unknown')),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 dispatch_started_at timestamptz,
 execution_id uuid,
 provider_log_id text NOT NULL DEFAULT '' CHECK(char_length(provider_log_id)<=256),
 error_code text NOT NULL DEFAULT '' CHECK(char_length(error_code)<=100),
 audio_location_id uuid,
 manifest_location_id uuid,
 audio_sha256 text CHECK(audio_sha256 IS NULL OR audio_sha256 ~ '^[a-f0-9]{64}$'),
 audio_byte_size bigint CHECK(audio_byte_size IS NULL OR audio_byte_size BETWEEN 1 AND 104857600),
 usage jsonb CHECK(usage IS NULL OR jsonb_typeof(usage)='object'),
 result jsonb CHECK(result IS NULL OR jsonb_typeof(result)='object'),
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 completed_at timestamptz,
 UNIQUE(actor_id,idem_key),
 FOREIGN KEY(owner_scope_id,physical_scope_id) REFERENCES workspace.creation_scope(id,physical_scope_id),
 FOREIGN KEY(personal_scope_id,personal_org_id) REFERENCES workspace.creation_scope(id,physical_scope_id),
 CHECK((dispatch_started_at IS NULL)=(execution_id IS NULL)),
 CHECK(status NOT IN ('dispatching','ingesting','succeeded','unknown') OR dispatch_started_at IS NOT NULL),
 CHECK((status='succeeded')=(result IS NOT NULL)),
 CHECK((output_asset_id IS NULL) OR output_asset_id=asset_id),
 CHECK(status<>'succeeded' OR (output_asset_id IS NOT NULL AND audio_sha256 IS NOT NULL AND audio_byte_size IS NOT NULL AND completed_at IS NOT NULL))
);
CREATE INDEX voice_job_owner_idx ON subject.voice_job(owner_scope_id,actor_id,created_at,id);
CREATE INDEX voice_job_provider_dispatch_idx ON subject.voice_job(provider_id,dispatch_started_at) WHERE dispatch_started_at IS NOT NULL;
REVOKE UPDATE,DELETE ON subject.voice_job FROM glowloom_app;
GRANT SELECT,INSERT ON subject.voice_job TO glowloom_app;
GRANT UPDATE(status,revision,dispatch_started_at,execution_id,provider_log_id,error_code,audio_location_id,manifest_location_id,audio_sha256,audio_byte_size,usage,result,output_asset_id,updated_at,completed_at) ON subject.voice_job TO glowloom_app;
-- No DELETE or mutation of original actor/scope/intent/key/catalog/voice/cost.


-- Asset-entry creation receipt.
CREATE TABLE canvas.asset_creation_receipt (
 actor_id uuid NOT NULL REFERENCES identity."user"(id),
 scope_id uuid NOT NULL REFERENCES workspace.creation_scope(id),
 idem_key uuid NOT NULL,
 request_hash text NOT NULL CHECK(length(request_hash)=64),
 project_id uuid NOT NULL REFERENCES workspace.project(id),
 document_id uuid NOT NULL REFERENCES canvas.document(id),
 response_body jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(actor_id,idem_key)
);
GRANT SELECT,INSERT ON canvas.asset_creation_receipt TO glowloom_app;

-- Physical identities for script sources, versions and copied objects.
ALTER TABLE script.script_source ADD COLUMN original_location_id uuid NOT NULL;
ALTER TABLE script.script_source ADD CONSTRAINT script_source_original_location_id_fkey FOREIGN KEY(original_location_id,original_key) REFERENCES media.object_location(id,object_key);
CREATE TRIGGER keep_original_location_id BEFORE UPDATE ON script.script_source FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('original_location_id');

ALTER TABLE script.script_source ADD COLUMN rich_location_id uuid NOT NULL;
ALTER TABLE script.script_source ADD CONSTRAINT script_source_rich_location_id_fkey FOREIGN KEY(rich_location_id,rich_key) REFERENCES media.object_location(id,object_key);
CREATE TRIGGER keep_rich_location_id BEFORE UPDATE ON script.script_source FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('rich_location_id');

ALTER TABLE script.script_version ADD COLUMN text_location_id uuid NOT NULL;
ALTER TABLE script.script_version ADD CONSTRAINT script_version_text_location_id_fkey FOREIGN KEY(text_location_id,text_key) REFERENCES media.object_location(id,object_key);
CREATE TRIGGER keep_text_location_id BEFORE UPDATE ON script.script_version FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('text_location_id');

ALTER TABLE script.script_version ADD COLUMN rich_location_id uuid NOT NULL;
ALTER TABLE script.script_version ADD CONSTRAINT script_version_rich_location_id_fkey FOREIGN KEY(rich_location_id,rich_key) REFERENCES media.object_location(id,object_key);
CREATE TRIGGER keep_rich_location_id BEFORE UPDATE ON script.script_version FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('rich_location_id');

ALTER TABLE script.copy_object_intent ADD COLUMN source_location_id uuid NOT NULL;
ALTER TABLE script.copy_object_intent ADD CONSTRAINT copy_object_intent_source_location_id_fkey FOREIGN KEY(source_location_id,source_key) REFERENCES media.object_location(id,object_key);
CREATE TRIGGER keep_source_location_id BEFORE UPDATE ON script.copy_object_intent FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('source_location_id');

ALTER TABLE script.copy_object_intent ADD COLUMN target_location_id uuid NOT NULL;
ALTER TABLE script.copy_object_intent ADD CONSTRAINT copy_object_intent_target_location_id_fkey FOREIGN KEY(target_location_id,target_key) REFERENCES media.object_location(id,object_key);
CREATE TRIGGER keep_target_location_id BEFORE UPDATE ON script.copy_object_intent FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('target_location_id');

ALTER TABLE script.copy_object_state ADD COLUMN target_location_id uuid NOT NULL;
ALTER TABLE script.copy_object_state ADD CONSTRAINT copy_object_state_target_location_id_fkey FOREIGN KEY(target_location_id,target_key) REFERENCES media.object_location(id,object_key);
CREATE TRIGGER keep_target_location_id BEFORE UPDATE ON script.copy_object_state FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('target_location_id');

-- Storage schema version checked at runtime.
CREATE TABLE media.storage_schema_version (
 version integer PRIMARY KEY,
 applied_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
INSERT INTO media.storage_schema_version(version) VALUES (2) ON CONFLICT (version) DO NOTHING;
GRANT SELECT ON TABLE media.storage_schema_version TO glowloom_app;

CREATE TABLE media.storage_idempotency_lock (
 namespace text NOT NULL CHECK (namespace IN ('purge','package','transfer','upload','personal_upload','library')),
 actor_id uuid NOT NULL,
 request_key uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY (namespace, actor_id, request_key)
);
GRANT SELECT, INSERT, UPDATE ON TABLE media.storage_idempotency_lock TO glowloom_app;

-- Measured empty deployment proof (no invented legacy profile).
-- Empty deployment proof contains a measured closed inventory, not a provider claim.
-- Execute explicitly in one transaction. It does not prove a live cloud bucket.
CREATE TABLE media.storage_empty_deployment_evidence (
 config_sha256 text PRIMARY KEY CHECK(config_sha256 ~ '^[a-f0-9]{64}$'),
 evidence_sha256 text NOT NULL CHECK(evidence_sha256 ~ '^[a-f0-9]{64}$'),
 inventory jsonb NOT NULL CHECK(jsonb_typeof(inventory)='object'),
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TRIGGER keep_storage_evidence BEFORE UPDATE OR DELETE ON media.storage_empty_deployment_evidence FOR EACH ROW EXECUTE FUNCTION media.keep_storage_evidence();
GRANT SELECT ON media.storage_empty_deployment_evidence TO glowloom_app;

-- Script copy admission freezes its own output work in the caller transaction.
ALTER TABLE script.copy_snapshot ADD COLUMN storage_intent_id uuid REFERENCES media.storage_work_intent(id);

-- Private canvas assistant history; generation drafts and tasks keep their owners.
CREATE TABLE canvas.assistant_session (
 id uuid PRIMARY KEY,
 canvas_id uuid NOT NULL REFERENCES canvas.document(id),
 project_id uuid NOT NULL REFERENCES workspace.project(id),
 actor_id uuid NOT NULL REFERENCES identity."user"(id),
 title text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 128),
 revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
 turns jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(turns)='array' AND jsonb_array_length(turns)<=64 AND octet_length(turns::text)<=65536),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX assistant_session_owner ON canvas.assistant_session(canvas_id,actor_id,updated_at DESC,id);
GRANT SELECT,INSERT,UPDATE ON canvas.assistant_session TO glowloom_app;

-- Portable timeline artifacts, including native draft ZIPs.
CREATE TABLE mediatool.export_artifact (
 id uuid PRIMARY KEY,
 job_id uuid NOT NULL REFERENCES mediatool.export_job(id),
 project_id uuid NOT NULL,
 attempt integer NOT NULL CHECK(attempt BETWEEN 1 AND 100),
 format text NOT NULL CHECK(format IN ('media_bundle','fcpxml','xmeml','jianying')),
 format_version text NOT NULL,
 file_name text NOT NULL CHECK(file_name ~ '^timeline-[a-f0-9-]{36}\.zip$'),
 mime_type text NOT NULL CHECK(mime_type='application/zip'),
 byte_size bigint NOT NULL CHECK(byte_size BETWEEN 1 AND 524288000),
 sha256 text NOT NULL CHECK(sha256 ~ '^[a-f0-9]{64}$'),
 object_key text NOT NULL,
 object_location_id uuid NOT NULL REFERENCES media.object_location(id),
 created_at timestamptz NOT NULL,
 UNIQUE(id,job_id,project_id,attempt),
 UNIQUE(job_id,attempt),
 CHECK((format='media_bundle' AND format_version='1') OR (format='fcpxml' AND format_version='1.10') OR (format='xmeml' AND format_version='5') OR (format='jianying' AND format_version='134.0.0')),
 CHECK(object_key ~ '^projects/[a-f0-9-]{36}/exports/[0-9]{4}/[0-9]{2}/[a-f0-9-]{36}\.zip$')
);
ALTER TABLE mediatool.export_job ADD CONSTRAINT export_job_artifact_identity_fkey FOREIGN KEY(artifact_id,id,project_id,attempt) REFERENCES mediatool.export_artifact(id,job_id,project_id,attempt);
GRANT SELECT,INSERT ON mediatool.export_artifact TO glowloom_app;
GRANT UPDATE(artifact_id) ON mediatool.export_job TO glowloom_app;

-- Volcengine creation presets.

-- Private director recovery does not alter the authoritative Yjs document.
CREATE SEQUENCE canvas.director_recovery_revision AS integer NO CYCLE;
GRANT USAGE ON SEQUENCE canvas.director_recovery_revision TO glowloom_app;
CREATE TABLE canvas.director_recovery (
 actor_id uuid NOT NULL,
 document_id uuid NOT NULL REFERENCES canvas.document(id) ON DELETE CASCADE,
 node_id uuid NOT NULL,
 revision bigint NOT NULL CHECK(revision BETWEEN 1 AND 2147483647),
 epoch uuid NOT NULL,
 base_fingerprint text NOT NULL CHECK(base_fingerprint ~ '^[0-9a-f]{64}$'),
 director jsonb NOT NULL CHECK(octet_length(director::text)<=600000),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(actor_id,document_id,node_id)
);
GRANT SELECT,INSERT,UPDATE,DELETE ON canvas.director_recovery TO glowloom_app;

-- A new immutable preset version; existing connection snapshots stay unchanged.

-- Private reference voices; supplier training is a separate, explicit operation.
CREATE TABLE media.voice_profile (
 id uuid PRIMARY KEY,
 project_id uuid NOT NULL REFERENCES workspace.project(id) ON DELETE CASCADE,
 actor_id uuid NOT NULL REFERENCES identity."user"(id) ON DELETE CASCADE,
 request_id uuid NOT NULL,
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 60),
 request_name text NOT NULL CHECK(length(request_name) BETWEEN 1 AND 60),
 asset_id uuid NOT NULL REFERENCES media.media_asset(id) ON DELETE CASCADE,
 asset_revision bigint NOT NULL CHECK(asset_revision>0),
 consent_at timestamptz NOT NULL,
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 favorite boolean NOT NULL DEFAULT false,
 is_delete boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(project_id,actor_id,request_id)
);
CREATE INDEX voice_profile_owner ON media.voice_profile(project_id,actor_id,id) WHERE NOT is_delete;
GRANT SELECT,INSERT,UPDATE ON media.voice_profile TO glowloom_app;

CREATE TABLE media.voice_favorite (
 project_id uuid NOT NULL REFERENCES workspace.project(id) ON DELETE CASCADE,
 actor_id uuid NOT NULL REFERENCES identity."user"(id) ON DELETE CASCADE,
 speaker text NOT NULL CHECK(length(speaker) BETWEEN 1 AND 128),
 favorite boolean NOT NULL,
 PRIMARY KEY(project_id,actor_id,speaker)
);
GRANT SELECT,INSERT,UPDATE ON media.voice_favorite TO glowloom_app;

ALTER TABLE media.voice_profile ADD COLUMN gender text NOT NULL DEFAULT '未标注' CHECK(gender IN ('未标注','男声','女声'));
ALTER TABLE media.voice_profile ADD COLUMN age text NOT NULL DEFAULT '未标注' CHECK(age IN ('未标注','儿童','青年','中年','老年'));
ALTER TABLE media.voice_profile ADD COLUMN language text NOT NULL DEFAULT '未标注' CHECK(language IN ('未标注','中文','英文','日文','其他'));
ALTER TABLE media.voice_profile ADD COLUMN style text NOT NULL DEFAULT '未标注' CHECK(style IN ('未标注','自然','温柔','活泼','沉稳','旁白'));

-- Native-app capability: private signed envelope, fixed expiry and no re-signing.
CREATE TABLE mediatool.export_launch (
 token_hash text PRIMARY KEY CHECK(token_hash ~ '^[a-f0-9]{64}$'),
 job_id uuid NOT NULL REFERENCES mediatool.export_job(id) ON DELETE CASCADE,
 artifact_id uuid NOT NULL REFERENCES mediatool.export_artifact(id) ON DELETE CASCADE,
 project_id uuid NOT NULL, revision bigint NOT NULL CHECK(revision>0),
 actor jsonb NOT NULL, envelope jsonb NOT NULL CHECK(octet_length(envelope::text)<=16384),
 expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(expires_at>created_at AND expires_at<=created_at+interval '10 minutes')
);
CREATE INDEX export_launch_expiry_idx ON mediatool.export_launch(expires_at);
GRANT SELECT,INSERT,DELETE ON mediatool.export_launch TO glowloom_app;


-- Agent runtime storage (Mastra static tables + Kysely service control tables).
-- @mastra/pg runs with disableInit: true; all DDL lives exclusively in schema.sql.
CREATE SCHEMA agent;

CREATE TABLE agent.mastra_threads (
    id text PRIMARY KEY,
    "resourceId" text NOT NULL,
    title text NOT NULL,
    metadata jsonb,
    "createdAt" timestamp NOT NULL DEFAULT now(),
    "updatedAt" timestamp NOT NULL DEFAULT now()
);
CREATE INDEX mastra_threads_resource_idx ON agent.mastra_threads("resourceId", "updatedAt" DESC);

CREATE TABLE agent.mastra_messages (
    id text PRIMARY KEY,
    thread_id text NOT NULL,
    content text NOT NULL,
    role text NOT NULL,
    type text NOT NULL,
    "createdAt" timestamp NOT NULL DEFAULT now(),
    "resourceId" text
);
CREATE INDEX mastra_messages_thread_idx ON agent.mastra_messages(thread_id, "createdAt" ASC);

CREATE TABLE agent.mastra_resources (
    id text PRIMARY KEY,
    "workingMemory" text,
    metadata jsonb,
    "createdAt" timestamp NOT NULL DEFAULT now(),
    "updatedAt" timestamp NOT NULL DEFAULT now()
);

-- Workflow storage exported by the locked @mastra/pg WorkflowsPG adapter.
CREATE TABLE agent.mastra_workflow_snapshot (
    workflow_name text NOT NULL,
    run_id text NOT NULL,
    "resourceId" text,
    snapshot jsonb NOT NULL,
    "createdAt" timestamp NOT NULL DEFAULT now(),
    "updatedAt" timestamp NOT NULL DEFAULT now(),
    "createdAtZ" timestamptz DEFAULT now(),
    "updatedAtZ" timestamptz DEFAULT now(),
    PRIMARY KEY (workflow_name, run_id)
);
CREATE INDEX mastra_workflow_snapshot_resource_idx ON agent.mastra_workflow_snapshot("resourceId", "updatedAt" DESC);

CREATE TABLE agent.mastra_workflow_runs (
    workflow_name text NOT NULL,
    run_id text NOT NULL,
    "resourceId" text,
    "threadId" text,
    status text NOT NULL,
    header jsonb NOT NULL,
    "lastSeq" integer NOT NULL,
    "startStateSeq" integer NOT NULL,
    "stateShared" boolean NOT NULL,
    "stateSeq" integer NOT NULL,
    "statePatchCount" integer NOT NULL,
    "statePatchBytes" integer NOT NULL,
    "stateBytes" integer,
    "runToken" text NOT NULL,
    "createdAt" timestamp NOT NULL,
    "updatedAt" timestamp NOT NULL,
    "createdAtZ" timestamptz DEFAULT now(),
    "updatedAtZ" timestamptz DEFAULT now(),
    PRIMARY KEY (workflow_name, run_id)
);
CREATE INDEX mastra_workflow_runs_status_idx ON agent.mastra_workflow_runs(workflow_name, status, "createdAt" DESC);
CREATE INDEX mastra_workflow_runs_thread_idx ON agent.mastra_workflow_runs("threadId");

CREATE TABLE agent.mastra_workflow_run_entries (
    workflow_name text NOT NULL,
    run_id text NOT NULL,
    seq integer NOT NULL,
    kind text NOT NULL,
    step_id text,
    payload jsonb,
    first_seq integer,
    PRIMARY KEY (workflow_name, run_id, seq)
);
CREATE INDEX mastra_workflow_run_entries_step_idx ON agent.mastra_workflow_run_entries(workflow_name, run_id, kind, step_id, seq);
CREATE INDEX mastra_workflow_run_entries_state_base_idx ON agent.mastra_workflow_run_entries((payload ->> 'workflowName'), (payload ->> 'runId')) WHERE kind = 'state-base';

CREATE TABLE agent.mastra_workflow_run_owners (
    "runId" text PRIMARY KEY,
    generation bigint NOT NULL,
    "ownerId" text NOT NULL,
    "leaseExpiresAt" bigint,
    "updatedAt" bigint NOT NULL
);

REVOKE ALL ON agent.mastra_threads, agent.mastra_messages, agent.mastra_resources, agent.mastra_workflow_snapshot,
    agent.mastra_workflow_runs, agent.mastra_workflow_run_entries, agent.mastra_workflow_run_owners FROM PUBLIC;
GRANT USAGE ON SCHEMA agent TO glowloom_agent;
GRANT SELECT, INSERT, UPDATE, DELETE ON agent.mastra_threads, agent.mastra_messages, agent.mastra_resources,
    agent.mastra_workflow_snapshot, agent.mastra_workflow_runs, agent.mastra_workflow_run_entries,
    agent.mastra_workflow_run_owners TO glowloom_agent;

-- Agent service control metadata (managed by Kysely).
CREATE TABLE agent.service_sessions (
    id uuid PRIMARY KEY,
    actor_id uuid NOT NULL,
    org_id uuid NOT NULL,
    scope_id uuid NOT NULL,
    project_id uuid NOT NULL,
    canvas_id uuid,
    title text NOT NULL,
    request_id uuid NOT NULL,
    content_hash text NOT NULL,
    execution_revision bigint NOT NULL DEFAULT 0 CHECK (execution_revision >= 0),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    last_input_at timestamptz,
    pinned boolean NOT NULL DEFAULT false,
    archived_at timestamptz,
    UNIQUE (actor_id, project_id, request_id)
);
CREATE INDEX service_sessions_owner ON agent.service_sessions(actor_id, project_id, created_at DESC, id);
CREATE INDEX service_sessions_input ON agent.service_sessions(actor_id, project_id, last_input_at DESC, id);

CREATE TABLE agent.skill_revisions (
    digest text PRIMARY KEY CHECK (digest ~ '^[a-f0-9]{64}$'),
    bundle jsonb NOT NULL
);
REVOKE ALL ON agent.skill_revisions FROM PUBLIC;
GRANT SELECT, INSERT ON agent.skill_revisions TO glowloom_agent;

CREATE TABLE agent.service_runs (
    id uuid PRIMARY KEY,
    session_id uuid NOT NULL REFERENCES agent.service_sessions(id),
    request_id uuid NOT NULL,
    content_hash text NOT NULL,
    goal text NOT NULL,
    manifest jsonb NOT NULL,
    skill_revisions jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(skill_revisions) = 'array'),
    prompt_revisions jsonb NOT NULL CHECK (jsonb_typeof(prompt_revisions) = 'object'),
    submission_id uuid NOT NULL,
    accepted_revision bigint NOT NULL,
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'waiting_approval', 'completed', 'failed', 'cancelled')),
    lease_owner text,
    lease_until timestamptz,
    workflow_run_id text,
    error_code text,
    max_model_requests integer NOT NULL DEFAULT 24 CHECK (max_model_requests > 0),
    used_model_requests integer NOT NULL DEFAULT 0 CHECK (used_model_requests >= 0),
    max_tool_steps integer NOT NULL DEFAULT 32 CHECK (max_tool_steps > 0),
    used_tool_steps integer NOT NULL DEFAULT 0 CHECK (used_tool_steps >= 0),
    input jsonb,
    retry_of_run_id uuid REFERENCES agent.service_runs(id),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    completed_at timestamptz,
    UNIQUE (session_id, request_id)
);
CREATE INDEX service_runs_history ON agent.service_runs(session_id, accepted_revision DESC, id);
CREATE INDEX service_runs_active_lease ON agent.service_runs(status, lease_until) WHERE status IN ('queued', 'running');

CREATE TABLE agent.service_run_events (
    run_id uuid NOT NULL REFERENCES agent.service_runs(id) ON DELETE CASCADE,
    seq bigint NOT NULL CHECK (seq > 0),
    event_type text NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (run_id, seq)
);

CREATE TABLE agent.service_public_messages (
    run_id uuid NOT NULL REFERENCES agent.service_runs(id) ON DELETE CASCADE,
    seq bigint NOT NULL CHECK (seq > 0),
    part_index integer NOT NULL DEFAULT 0,
    message_id uuid NOT NULL,
    kind text NOT NULL,
    content text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (run_id, seq, part_index)
);
CREATE INDEX service_public_messages_history ON agent.service_public_messages(run_id, seq DESC, part_index DESC);
CREATE INDEX service_public_messages_identity ON agent.service_public_messages(run_id, message_id, part_index);

CREATE TABLE agent.service_control_inbox (
    decision_id uuid PRIMARY KEY,
    root_run_id uuid NOT NULL REFERENCES agent.service_runs(id) ON DELETE CASCADE,
    authorization_revision bigint NOT NULL,
    content_hash text NOT NULL,
    input jsonb NOT NULL,
    receipt jsonb,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (root_run_id, authorization_revision)
);
CREATE INDEX service_control_pending ON agent.service_control_inbox(root_run_id, authorization_revision) WHERE receipt IS NULL;

REVOKE ALL ON agent.service_sessions, agent.service_runs, agent.service_run_events, agent.service_public_messages, agent.service_control_inbox FROM PUBLIC;
GRANT SELECT, INSERT, UPDATE ON agent.service_sessions, agent.service_runs, agent.service_control_inbox TO glowloom_agent;
GRANT SELECT, INSERT ON agent.service_run_events, agent.service_public_messages TO glowloom_agent;

-- Agent business authorization.
CREATE SCHEMA agent_business;
CREATE TABLE agent_business.run_authorization (
 root_run_id uuid PRIMARY KEY, session_id uuid NOT NULL, submission_id uuid NOT NULL,
 request_id uuid NOT NULL, content_hash text NOT NULL, scope jsonb NOT NULL,
 budget_micros bigint NOT NULL CHECK(budget_micros>=0), reserved_micros bigint NOT NULL DEFAULT 0 CHECK(reserved_micros>=0),
 authorization_revision bigint NOT NULL DEFAULT 1 CHECK(authorization_revision>0),owner_epoch bigint NOT NULL DEFAULT 0,
 barrier boolean NOT NULL DEFAULT false, terminal boolean NOT NULL DEFAULT false,
 current_decision_id uuid, current_proposal_id uuid, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(session_id,request_id)
);
CREATE TABLE agent_business.execution_grant (
 grant_id uuid PRIMARY KEY,root_run_id uuid NOT NULL REFERENCES agent_business.run_authorization(root_run_id),
 submission_id uuid NOT NULL,owner_epoch bigint NOT NULL,authorization_revision bigint NOT NULL,expires_at timestamptz NOT NULL,
 UNIQUE(root_run_id,submission_id,owner_epoch,authorization_revision)
);
CREATE TABLE agent_business.control_decision (
 decision_id uuid PRIMARY KEY, root_run_id uuid NOT NULL REFERENCES agent_business.run_authorization(root_run_id),
 request_id uuid NOT NULL,input_hash text NOT NULL,authorization_revision bigint NOT NULL,
 action text NOT NULL CHECK(action IN ('approve','reject','pause','resume','stop')),body jsonb NOT NULL,
 delivery_state text NOT NULL DEFAULT 'pending' CHECK(delivery_state IN ('pending','leased','applied','superseded')),
 attempts integer NOT NULL DEFAULT 0,next_attempt_at timestamptz NOT NULL DEFAULT now(),lease_until timestamptz,
 applied_receipt jsonb,UNIQUE(root_run_id,request_id),UNIQUE(root_run_id,authorization_revision)
);
CREATE INDEX agent_business_decision_delivery ON agent_business.control_decision(next_attempt_at) WHERE delivery_state IN ('pending','leased');
CREATE TABLE agent_business.quiesced_receipt (
 decision_id uuid PRIMARY KEY REFERENCES agent_business.control_decision(decision_id),receipt_id uuid NOT NULL UNIQUE,
 root_run_id uuid NOT NULL REFERENCES agent_business.run_authorization(root_run_id),continuation_id uuid NOT NULL,
 owner_epoch bigint NOT NULL,authorization_revision bigint NOT NULL,grant_id uuid NOT NULL REFERENCES agent_business.execution_grant(grant_id),
 UNIQUE(root_run_id,continuation_id)
);
CREATE TABLE agent_business.proposal (
 proposal_id uuid PRIMARY KEY,root_run_id uuid NOT NULL REFERENCES agent_business.run_authorization(root_run_id),
 request_id uuid NOT NULL,input_hash text NOT NULL,candidate_hash text NOT NULL,body jsonb NOT NULL,
 authorization_revision bigint NOT NULL, UNIQUE(root_run_id,request_id)
);
CREATE TABLE agent_business.call_receipt (
 receipt_id uuid PRIMARY KEY,root_run_id uuid NOT NULL REFERENCES agent_business.run_authorization(root_run_id),submission_id uuid NOT NULL,
 task_id text NOT NULL,call_id text NOT NULL,checkpoint_id text NOT NULL,attempt bigint NOT NULL CHECK(attempt>0),contract_version text NOT NULL,
 input_hash text NOT NULL,kind text NOT NULL,name text NOT NULL,max_micros bigint NOT NULL CHECK(max_micros>=0),
 status text NOT NULL CHECK(status IN ('in_flight','completed','failed','unknown')),result jsonb NOT NULL DEFAULT '{}',cost_micros bigint NOT NULL DEFAULT 0,
 created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(root_run_id,submission_id,task_id,call_id,checkpoint_id,attempt,contract_version)
);
-- A source publication retains its original call/grant identity across I/O and restarts.
CREATE TABLE agent_business.source_authorization (
 receipt_id uuid PRIMARY KEY REFERENCES agent_business.call_receipt(receipt_id) ON DELETE CASCADE,
 grant_id uuid NOT NULL REFERENCES agent_business.execution_grant(grant_id),
 source_hash text NOT NULL CHECK(source_hash ~ '^[a-f0-9]{64}$')
);
CREATE TABLE agent_business.skill_version (
 skill_id uuid NOT NULL,version text NOT NULL,resource_hash text NOT NULL,owner_actor_id uuid NOT NULL,org_id uuid NOT NULL,scope_id uuid NOT NULL,
 name text NOT NULL,description text NOT NULL,body text NOT NULL,resources jsonb NOT NULL DEFAULT '{}',enabled boolean NOT NULL DEFAULT true,
 PRIMARY KEY(skill_id,version),CHECK(length(body)<=262144)
);
REVOKE ALL ON SCHEMA agent_business FROM PUBLIC;
REVOKE ALL ON ALL TABLES IN SCHEMA agent_business FROM PUBLIC;
-- Application roles must already exist; this fragment never creates login credentials.
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='glowloom_app') THEN
  GRANT USAGE ON SCHEMA agent_business TO glowloom_app;
  GRANT SELECT,INSERT,UPDATE ON ALL TABLES IN SCHEMA agent_business TO glowloom_app;
 END IF;
 IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='glowloom_agent') THEN
  REVOKE ALL ON SCHEMA agent_business FROM glowloom_agent;
  REVOKE ALL ON ALL TABLES IN SCHEMA agent_business FROM glowloom_agent;
 END IF;
END $$;
-- Durable import identity is separate from immutable resource identity.
ALTER TABLE agent_business.skill_version ADD COLUMN request_id uuid;
CREATE UNIQUE INDEX agent_business_skill_import_request ON agent_business.skill_version(owner_actor_id,org_id,scope_id,request_id);
-- Immutable authorized Skill pins are business scope, not Harness execution state.
ALTER TABLE agent_business.run_authorization ADD COLUMN skill_pins jsonb NOT NULL DEFAULT '[]';
-- Go freezes the admitted model selection independently of Node's manifest.
ALTER TABLE agent_business.run_authorization ADD COLUMN model_selection jsonb NOT NULL DEFAULT '{}';

-- Durable business tasks.
CREATE TABLE infra.task_work (
    service_owner text NOT NULL DEFAULT infra.runtime_owner() CHECK (service_owner IN ('legacy','identity','creation','media','execution')),
    kind text NOT NULL,
    business_id text NOT NULL,
    next_run_at timestamptz NOT NULL,
    owner text,
    epoch bigint NOT NULL DEFAULT 0,
    lease_until timestamptz,
    attempt integer NOT NULL DEFAULT 0,
    reference jsonb NOT NULL DEFAULT 'null',
    failures integer NOT NULL DEFAULT 0,
    failed_at timestamptz,
    stop_requested_at timestamptz,
    completed_at timestamptz,
    PRIMARY KEY (service_owner, kind, business_id),
    CHECK (kind <> '' AND business_id <> ''),
    CHECK ((owner IS NULL) = (lease_until IS NULL))
);
CREATE INDEX task_work_due ON infra.task_work (kind, next_run_at) WHERE completed_at IS NULL AND failed_at IS NULL;

-- Exactly one cursor for each periodic business sweep. Restarting a process
-- never resets its due time. The worker checks existing expiries on first claim.
INSERT INTO infra.task_work (kind,business_id,next_run_at) VALUES
 ('maintenance.outbox','periodic',now()),
 ('maintenance.inbox','periodic',now()),
 ('catalog.price_sync','periodic',now()),
 -- Content deletion remains disabled until an operator explicitly enables it.
 ('workspace.retention','periodic','infinity'),
 ('operation.quote_expiry','periodic',now());

-- Administrator trials previously kept their owner-scoped state in external
-- execution history. This is the business record for new local trial requests.
CREATE TABLE operation.development_trial (
 id text PRIMARY KEY,
 input jsonb NOT NULL,
 state jsonb NOT NULL,
 phase text NOT NULL CHECK(phase IN ('ready','dispatching','poll','done')),
 generation bigint NOT NULL DEFAULT 1,
 polls integer NOT NULL DEFAULT 0,
 updated_at timestamptz NOT NULL DEFAULT now()
);

GRANT SELECT, INSERT, UPDATE ON infra.task_work, operation.development_trial TO glowloom_app;

-- A resumed batch acknowledges failures observed before the operator reopened admission.
ALTER TABLE operation.batch ADD COLUMN resumed_at timestamptz;
GRANT SELECT(resumed_at), UPDATE(resumed_at) ON operation.batch TO glowloom_app;

-- Agent conversation history columns & service_public_messages are defined inline in CREATE SCHEMA agent above.

-- Transfer output attempt facts.
CREATE TABLE media.transfer_output_attempt (
  job_id uuid NOT NULL,
  item_index integer NOT NULL,
  output_attempt integer NOT NULL CHECK (output_attempt > 0),
  work_intent_id uuid UNIQUE REFERENCES media.storage_work_intent(id),
  frozen bytea NOT NULL CHECK (octet_length(frozen) BETWEEN 2 AND 1048576),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (job_id, item_index, output_attempt),
  FOREIGN KEY (job_id, item_index) REFERENCES media.transfer_item(job_id, item_index)
);
ALTER TABLE media.transfer_object ADD CONSTRAINT transfer_object_output_attempt_fkey
  FOREIGN KEY (job_id, item_index, output_attempt) REFERENCES media.transfer_output_attempt(job_id, item_index, output_attempt);
CREATE FUNCTION media.reject_transfer_output_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'transfer physical output identity is append-only' USING ERRCODE = '42501';
END $$;

CREATE TRIGGER transfer_output_attempt_immutable BEFORE UPDATE OR DELETE ON media.transfer_output_attempt
FOR EACH ROW EXECUTE FUNCTION media.reject_transfer_output_mutation();

CREATE FUNCTION media.protect_transfer_object_attempt() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF ROW(NEW.job_id, NEW.item_index, NEW.output_attempt, NEW.rendition_kind, NEW.source_location_id,
         NEW.source_object_key, NEW.target_location_id, NEW.target_object_key, NEW.content_type)
     IS DISTINCT FROM
     ROW(OLD.job_id, OLD.item_index, OLD.output_attempt, OLD.rendition_kind, OLD.source_location_id,
         OLD.source_object_key, OLD.target_location_id, OLD.target_object_key, OLD.content_type)
     OR (OLD.byte_size IS NOT NULL AND NEW.byte_size IS DISTINCT FROM OLD.byte_size)
     OR (OLD.sha256 IS NOT NULL AND NEW.sha256 IS DISTINCT FROM OLD.sha256)
     OR (OLD.source_verified AND NOT NEW.source_verified)
     OR (OLD.write_started AND NOT NEW.write_started)
     OR (OLD.status = 'removed' AND NEW.status <> 'removed')
     OR (OLD.status = 'verified' AND NEW.status NOT IN ('verified','removed')) THEN
    RAISE EXCEPTION 'transfer object identity and receipts cannot regress' USING ERRCODE = '23514';
  END IF;
  RETURN NEW;
END $$;

CREATE TRIGGER transfer_object_attempt_immutable BEFORE UPDATE ON media.transfer_object
FOR EACH ROW EXECUTE FUNCTION media.protect_transfer_object_attempt();

GRANT SELECT, INSERT ON media.transfer_output_attempt TO glowloom_app;
-- No UPDATE is granted on output_attempt, frozen physical identity or work IDs.
ALTER TABLE billing.reservation ADD COLUMN cap_at_quote boolean NOT NULL DEFAULT false;
ALTER TABLE billing.reservation ADD COLUMN whole_credits boolean NOT NULL DEFAULT false;
ALTER TABLE operation.provider_call ADD COLUMN procurement_cost_micros bigint CHECK (procurement_cost_micros >= 0);
GRANT UPDATE (procurement_cost_micros) ON operation.provider_call TO glowloom_app;

-- Execution receives the accepted source registration and exact Creation ready
-- receipt before preparing this local wallet. It never looks up Creation tables.
CREATE TABLE billing.wallet_account (
    scope_id uuid PRIMARY KEY,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE FUNCTION billing.ensure_wallet_account(scope uuid) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM workspace.creation_scope WHERE id=scope) THEN
        RAISE EXCEPTION 'creation scope required' USING ERRCODE='23503';
    END IF;
    INSERT INTO billing.wallet_account(scope_id) VALUES (scope) ON CONFLICT (scope_id) DO NOTHING;
    RETURN scope;
END
$$;
REVOKE ALL ON FUNCTION billing.ensure_wallet_account(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION billing.ensure_wallet_account(uuid) TO glowloom_app;

CREATE TABLE billing.project_wallet (
    project_id uuid PRIMARY KEY REFERENCES workspace.project(id),
    scope_id uuid NOT NULL REFERENCES billing.wallet_account(scope_id)
);

CREATE TABLE billing.wallet_lot (
    id uuid PRIMARY KEY,
    scope_id uuid NOT NULL REFERENCES billing.wallet_account(scope_id),
    source_kind text NOT NULL CHECK (source_kind IN ('promotional', 'paid', 'subscription', 'expiry_return')),
    source_id uuid NOT NULL,
    issued_micros bigint NOT NULL CHECK (issued_micros > 0),
    available_micros bigint NOT NULL CHECK (available_micros BETWEEN 0 AND issued_micros),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    expires_at timestamptz,
    model_key text CHECK (length(model_key) BETWEEN 1 AND 256),
    root_lot_id uuid REFERENCES billing.wallet_lot(id),
    reason text NOT NULL CHECK (length(reason) BETWEEN 1 AND 500),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source_kind, source_id, root_lot_id),
    UNIQUE (scope_id, id),
    CHECK ((source_kind = 'expiry_return') = (root_lot_id IS NOT NULL)),
    CHECK ((source_kind = 'paid') = (expires_at IS NULL))
);

CREATE UNIQUE INDEX wallet_lot_source_root ON billing.wallet_lot(source_kind, source_id) WHERE root_lot_id IS NULL;
CREATE INDEX wallet_lot_scope_available ON billing.wallet_lot(scope_id, expires_at, id) WHERE available_micros > 0;

CREATE TABLE billing.wallet_hold (
    reservation_id uuid PRIMARY KEY REFERENCES billing.reservation(id),
    scope_id uuid NOT NULL REFERENCES billing.wallet_account(scope_id),
    UNIQUE (reservation_id, scope_id)
);

CREATE TABLE billing.wallet_allocation (
    reservation_id uuid NOT NULL,
    scope_id uuid NOT NULL,
    lot_id uuid NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal > 0),
    amount_micros bigint NOT NULL CHECK (amount_micros > 0),
    PRIMARY KEY (reservation_id, lot_id),
    UNIQUE (reservation_id, ordinal),
    FOREIGN KEY (reservation_id, scope_id) REFERENCES billing.wallet_hold(reservation_id, scope_id),
    FOREIGN KEY (scope_id, lot_id) REFERENCES billing.wallet_lot(scope_id, id)
);

CREATE TABLE billing.credit_command (
    id uuid PRIMARY KEY,
    scope_id uuid NOT NULL REFERENCES billing.wallet_account(scope_id),
    caller_id text NOT NULL CHECK (length(caller_id) BETWEEN 1 AND 200),
    idempotency_key_hash character(64) NOT NULL CHECK (idempotency_key_hash ~ '^[0-9a-f]{64}$'),
    request_hash character(64) NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    action text NOT NULL CHECK (action IN ('grant', 'debit', 'freeze', 'commit', 'release')),
    source_id uuid NOT NULL,
    reference_id uuid,
    amount_micros bigint NOT NULL CHECK (amount_micros > 0),
    expires_at timestamptz,
    model_key text CHECK (length(model_key) BETWEEN 1 AND 256),
    reason text NOT NULL CHECK (length(reason) BETWEEN 1 AND 500),
    status text NOT NULL CHECK (status IN ('accepted', 'succeeded', 'insufficient', 'failed', 'reconcile_required')),
    result jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (caller_id, idempotency_key_hash),
    CHECK ((action IN ('commit', 'release')) = (reference_id IS NOT NULL)),
    CHECK ((action = 'grant') = (expires_at IS NOT NULL))
);

CREATE UNIQUE INDEX credit_command_grant_source ON billing.credit_command(scope_id, source_id) WHERE action='grant';
CREATE UNIQUE INDEX credit_command_source_action ON billing.credit_command(scope_id, action, source_id)
    WHERE action IN ('debit', 'freeze');
CREATE UNIQUE INDEX credit_command_hold_terminal ON billing.credit_command(scope_id, action, reference_id)
    WHERE action IN ('commit', 'release') AND status IN ('accepted', 'succeeded');
CREATE INDEX credit_command_scope_time ON billing.credit_command(scope_id, created_at DESC, id DESC);

CREATE TABLE billing.credit_hold (
    id uuid PRIMARY KEY,
    scope_id uuid NOT NULL REFERENCES billing.wallet_account(scope_id),
    source_id uuid NOT NULL,
    command_id uuid NOT NULL UNIQUE REFERENCES billing.credit_command(id),
    amount_micros bigint NOT NULL CHECK (amount_micros > 0),
    status text NOT NULL CHECK (status IN ('held', 'committed', 'released')),
    created_at timestamptz NOT NULL,
    closed_at timestamptz,
    UNIQUE (scope_id, source_id),
    UNIQUE (scope_id, id),
    CHECK ((status = 'held') = (closed_at IS NULL))
);

CREATE TABLE billing.credit_hold_allocation (
    hold_id uuid NOT NULL,
    scope_id uuid NOT NULL,
    lot_id uuid NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal > 0),
    amount_micros bigint NOT NULL CHECK (amount_micros > 0),
    PRIMARY KEY (hold_id, lot_id),
    UNIQUE (hold_id, ordinal),
    FOREIGN KEY (scope_id, hold_id) REFERENCES billing.credit_hold(scope_id, id),
    FOREIGN KEY (scope_id, lot_id) REFERENCES billing.wallet_lot(scope_id, id)
);

CREATE TABLE billing.credit_saga (
    hold_id uuid PRIMARY KEY,
    scope_id uuid NOT NULL,
    outcome text NOT NULL CHECK (outcome IN ('unknown', 'succeeded', 'failed')),
    amount_micros bigint NOT NULL CHECK (amount_micros >= 0),
    status text NOT NULL CHECK (status IN ('unknown', 'command_accepted', 'committed', 'released', 'reconcile_required')),
    command_id uuid UNIQUE REFERENCES billing.credit_command(id),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    FOREIGN KEY (scope_id, hold_id) REFERENCES billing.credit_hold(scope_id, id),
    CHECK ((outcome='unknown' AND status='unknown' AND amount_micros=0 AND command_id IS NULL) OR
        (outcome IN ('succeeded','failed') AND status<>'unknown' AND command_id IS NOT NULL)),
    CHECK (outcome<>'failed' OR amount_micros=0)
);

CREATE TABLE billing.credit_saga_event (
    event_id uuid PRIMARY KEY,
    event_type text NOT NULL CHECK (event_type IN ('settlement_outcome', 'command_result')),
    scope_id uuid NOT NULL,
    hold_id uuid NOT NULL,
    command_id uuid REFERENCES billing.credit_command(id),
    request_hash character(64) NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL,
    FOREIGN KEY (scope_id, hold_id) REFERENCES billing.credit_hold(scope_id, id),
    CHECK ((event_type='settlement_outcome' AND command_id IS NULL) OR
        (event_type='command_result' AND command_id IS NOT NULL))
);
CREATE INDEX credit_saga_scope_time ON billing.credit_saga(scope_id, updated_at DESC, hold_id);

CREATE TABLE billing.wallet_entry (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    scope_id uuid NOT NULL,
    lot_id uuid NOT NULL,
    reservation_id uuid REFERENCES billing.reservation(id),
    command_id uuid REFERENCES billing.credit_command(id),
    hold_id uuid REFERENCES billing.credit_hold(id),
    kind text NOT NULL CHECK (kind IN ('issue', 'hold', 'redeem', 'release', 'expire', 'restore', 'compensate')),
    amount_micros bigint NOT NULL CHECK (amount_micros > 0),
    available_delta_micros bigint NOT NULL DEFAULT 0,
    held_delta_micros bigint NOT NULL DEFAULT 0,
    redeemed_delta_micros bigint NOT NULL DEFAULT 0,
    expired_delta_micros bigint NOT NULL DEFAULT 0,
    funding_delta_micros bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (created_at, id),
    FOREIGN KEY (scope_id, lot_id) REFERENCES billing.wallet_lot(scope_id, id),
    CHECK (reservation_id IS NULL OR command_id IS NULL),
    CHECK (available_delta_micros::numeric + held_delta_micros::numeric + redeemed_delta_micros::numeric +
        expired_delta_micros::numeric + funding_delta_micros::numeric = 0)
) PARTITION BY RANGE (created_at);

CREATE TABLE billing.wallet_entry_default PARTITION OF billing.wallet_entry DEFAULT;
CREATE INDEX wallet_entry_scope_time ON billing.wallet_entry(scope_id, created_at DESC, id DESC);

CREATE FUNCTION billing.set_wallet_entry_deltas() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE lot_expiry timestamptz;
BEGIN
    NEW.available_delta_micros := 0;
    NEW.held_delta_micros := 0;
    NEW.redeemed_delta_micros := 0;
    NEW.expired_delta_micros := 0;
    NEW.funding_delta_micros := 0;
    CASE NEW.kind
      WHEN 'issue' THEN
        NEW.available_delta_micros := NEW.amount_micros;
        NEW.funding_delta_micros := -NEW.amount_micros;
      WHEN 'hold' THEN
        NEW.available_delta_micros := -NEW.amount_micros;
        NEW.held_delta_micros := NEW.amount_micros;
      WHEN 'redeem' THEN
        IF NEW.hold_id IS NOT NULL OR NEW.reservation_id IS NOT NULL THEN
          NEW.held_delta_micros := -NEW.amount_micros;
        ELSE
          NEW.available_delta_micros := -NEW.amount_micros;
        END IF;
        NEW.redeemed_delta_micros := NEW.amount_micros;
      WHEN 'release' THEN
        NEW.available_delta_micros := NEW.amount_micros;
        NEW.held_delta_micros := -NEW.amount_micros;
      WHEN 'expire' THEN
        NEW.held_delta_micros := -NEW.amount_micros;
        NEW.expired_delta_micros := NEW.amount_micros;
      WHEN 'restore' THEN
        NEW.available_delta_micros := NEW.amount_micros;
        NEW.expired_delta_micros := -NEW.amount_micros;
      WHEN 'compensate' THEN
        NEW.redeemed_delta_micros := -NEW.amount_micros;
        SELECT expires_at INTO lot_expiry FROM billing.wallet_lot WHERE id=NEW.lot_id;
        IF lot_expiry IS NULL OR lot_expiry>NEW.created_at THEN
          NEW.available_delta_micros := NEW.amount_micros;
        ELSE
          NEW.expired_delta_micros := NEW.amount_micros;
        END IF;
      ELSE
        RAISE EXCEPTION 'unsupported wallet entry kind %', NEW.kind USING ERRCODE='23514';
    END CASE;
    RETURN NEW;
END
$$;
REVOKE ALL ON FUNCTION billing.set_wallet_entry_deltas() FROM PUBLIC;
CREATE TRIGGER wallet_entry_set_deltas BEFORE INSERT ON billing.wallet_entry
FOR EACH ROW EXECUTE FUNCTION billing.set_wallet_entry_deltas();

-- Global identity stays unpartitioned because PostgreSQL cannot enforce a
-- unique wallet entry ID or business source key across monthly partitions.
CREATE TABLE billing.wallet_entry_identity (
    id uuid PRIMARY KEY,
    identity_key text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL
);

CREATE FUNCTION billing.register_wallet_entry_identity() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog AS $$
DECLARE identity text;
BEGIN
    IF NEW.command_id IS NOT NULL THEN
        identity := 'command:' || NEW.command_id::text || ':' || NEW.lot_id::text || ':' || NEW.kind;
    ELSIF NEW.reservation_id IS NOT NULL THEN
        identity := 'reservation:' || NEW.reservation_id::text || ':' || NEW.lot_id::text || ':' || NEW.kind;
    ELSIF NEW.kind = 'issue' THEN
        identity := 'issue:' || NEW.lot_id::text;
    ELSIF NEW.kind = 'restore' THEN
        identity := 'restore:' || NEW.lot_id::text;
    ELSE
        identity := 'entry:' || NEW.id::text;
    END IF;
    INSERT INTO billing.wallet_entry_identity(id, identity_key, created_at)
    VALUES (NEW.id, identity, NEW.created_at);
    RETURN NEW;
END
$$;
REVOKE ALL ON FUNCTION billing.register_wallet_entry_identity() FROM PUBLIC;
CREATE TRIGGER wallet_entry_identity_before_insert BEFORE INSERT ON billing.wallet_entry
FOR EACH ROW EXECUTE FUNCTION billing.register_wallet_entry_identity();

CREATE FUNCTION billing.reject_wallet_entry_mutation() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    IF current_user = pg_get_userbyid((SELECT relowner FROM pg_class WHERE oid='billing.wallet_entry'::regclass)) THEN
        IF TG_OP='DELETE' THEN RETURN OLD; END IF;
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'wallet entries are append only';
END
$$;
CREATE TRIGGER wallet_entry_append_only BEFORE UPDATE OR DELETE ON billing.wallet_entry
FOR EACH ROW EXECUTE FUNCTION billing.reject_wallet_entry_mutation();

CREATE TABLE billing.wallet_expiry_grace (
    root_lot_id uuid PRIMARY KEY REFERENCES billing.wallet_lot(id),
    expires_at timestamptz NOT NULL
);

-- Only the schema owner/finance role may bind projects or issue initial lots.
GRANT SELECT ON billing.wallet_account, billing.project_wallet TO glowloom_app;
-- PostgreSQL requires an UPDATE privilege to take a row lock.
GRANT UPDATE (created_at, version) ON billing.wallet_account TO glowloom_app;
GRANT SELECT, INSERT ON billing.wallet_hold, billing.wallet_allocation, billing.wallet_entry, billing.wallet_expiry_grace TO glowloom_app;
GRANT SELECT ON billing.wallet_lot TO glowloom_app;
GRANT UPDATE (available_micros, version) ON billing.wallet_lot TO glowloom_app;
GRANT SELECT, INSERT ON billing.credit_command, billing.credit_hold, billing.credit_hold_allocation TO glowloom_app;
GRANT UPDATE (status, result, updated_at) ON billing.credit_command TO glowloom_app;
GRANT UPDATE (status, closed_at) ON billing.credit_hold TO glowloom_app;
GRANT SELECT, INSERT ON billing.credit_saga_event TO glowloom_app;
GRANT SELECT, INSERT ON billing.credit_saga TO glowloom_app;
GRANT UPDATE (outcome, amount_micros, status, command_id, updated_at) ON billing.credit_saga TO glowloom_app;

CREATE FUNCTION billing.insert_credit_return_lot(
    lot uuid, scope uuid, hold uuid, amount bigint, deadline timestamptz, root uuid, model text, at_time timestamptz
) RETURNS uuid
LANGUAGE sql SECURITY DEFINER SET search_path = pg_catalog
AS $$
    INSERT INTO billing.wallet_lot
        (id,scope_id,source_kind,source_id,issued_micros,available_micros,version,expires_at,model_key,root_lot_id,reason,created_at)
    SELECT lot,scope,'expiry_return',hold,amount,amount,1,deadline,source.model_key,root,'expired hold return',at_time
    FROM billing.wallet_lot source
    JOIN billing.wallet_expiry_grace grace ON grace.root_lot_id=root AND grace.expires_at=deadline
    JOIN billing.credit_hold h ON h.id=hold AND h.scope_id=scope
    JOIN billing.credit_hold_allocation allocation ON allocation.hold_id=hold AND allocation.scope_id=scope
    WHERE source.id=root AND source.scope_id=scope AND source.root_lot_id IS NULL AND deadline>at_time
      AND amount<=allocation.amount_micros
      AND amount<=COALESCE((SELECT sum(e.amount_micros) FROM billing.wallet_entry e
        JOIN billing.wallet_lot original ON original.id=e.lot_id
        WHERE e.hold_id=hold AND e.kind IN ('expire','compensate')
          AND COALESCE(original.root_lot_id,original.id)=root),0)
    RETURNING id
$$;
REVOKE ALL ON FUNCTION billing.insert_credit_return_lot(uuid,uuid,uuid,bigint,timestamptz,uuid,text,timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION billing.insert_credit_return_lot(uuid,uuid,uuid,bigint,timestamptz,uuid,text,timestamptz) TO glowloom_app;

CREATE FUNCTION billing.insert_credit_grant_lot(lot uuid, command uuid, at_time timestamptz) RETURNS uuid
LANGUAGE sql SECURITY DEFINER SET search_path = pg_catalog
AS $$
    INSERT INTO billing.wallet_lot
        (id,scope_id,source_kind,source_id,issued_micros,available_micros,version,expires_at,model_key,reason,created_at)
    SELECT lot,c.scope_id,'promotional',c.source_id,c.amount_micros,c.amount_micros,1,c.expires_at,c.model_key,c.reason,at_time
    FROM billing.credit_command c
    WHERE c.id=command AND c.action='grant' AND c.status='accepted' AND c.expires_at>at_time
    ON CONFLICT DO NOTHING
    RETURNING id
$$;
REVOKE ALL ON FUNCTION billing.insert_credit_grant_lot(uuid,uuid,timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION billing.insert_credit_grant_lot(uuid,uuid,timestamptz) TO glowloom_app;

-- Only a return lot may be inserted by the runtime; root issuance is separately controlled.
CREATE FUNCTION billing.insert_wallet_return_lot(
    lot uuid, scope uuid, reservation uuid, amount bigint, deadline timestamptz, root uuid, at_time timestamptz
) RETURNS uuid
LANGUAGE sql SECURITY DEFINER SET search_path = pg_catalog
AS $$
    INSERT INTO billing.wallet_lot
        (id, scope_id, source_kind, source_id, issued_micros, available_micros, expires_at, model_key, root_lot_id, reason, created_at)
    SELECT lot, scope, 'expiry_return', reservation, amount, amount, deadline, l.model_key, root, 'expired hold return', at_time
    FROM billing.wallet_lot l
    JOIN billing.wallet_expiry_grace g ON g.root_lot_id=l.id AND g.expires_at=deadline
    JOIN billing.wallet_hold h ON h.reservation_id=reservation AND h.scope_id=scope
    WHERE l.id=root AND l.scope_id=scope AND l.root_lot_id IS NULL AND deadline>at_time
      AND amount <= COALESCE((SELECT sum(e.amount_micros) FROM billing.wallet_entry e
        JOIN billing.wallet_lot original ON original.id=e.lot_id
        WHERE e.reservation_id=reservation AND e.kind IN ('expire','compensate')
          AND coalesce(original.root_lot_id,original.id)=root), 0)
    RETURNING id
$$;
REVOKE ALL ON FUNCTION billing.insert_wallet_return_lot(uuid,uuid,uuid,bigint,timestamptz,uuid,timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION billing.insert_wallet_return_lot(uuid,uuid,uuid,bigint,timestamptz,uuid,timestamptz) TO glowloom_app;

CREATE FUNCTION billing.bind_project_wallet(project uuid) RETURNS uuid
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE payer uuid;
BEGIN
    SELECT owner_scope_id INTO STRICT payer FROM workspace.project
        WHERE id=$1 AND status='active' AND NOT is_delete FOR SHARE;
    PERFORM id FROM billing.budget WHERE project_id=$1 AND NOT is_delete FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION 'active project budget required'; END IF;
    INSERT INTO billing.wallet_account(scope_id) VALUES (payer) ON CONFLICT DO NOTHING;
    INSERT INTO billing.project_wallet(project_id,scope_id) VALUES ($1,payer)
        ON CONFLICT (project_id) DO UPDATE SET scope_id=excluded.scope_id;
    RETURN payer;
END
$$;
REVOKE ALL ON FUNCTION billing.bind_project_wallet(uuid) FROM PUBLIC;

CREATE FUNCTION billing.issue_promotional_credit(
    scope uuid, source uuid, amount bigint, deadline timestamptz, model text, justification text
) RETURNS uuid
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE lot billing.wallet_lot%ROWTYPE;
DECLARE account_version bigint;
DECLARE event_id uuid;
DECLARE current_timepoint timestamptz := clock_timestamp();
BEGIN
    SELECT version INTO account_version FROM billing.wallet_account WHERE scope_id=scope;
    IF NOT FOUND THEN RAISE EXCEPTION 'wallet account required'; END IF;
    SELECT * INTO lot FROM billing.wallet_lot
        WHERE source_kind='promotional' AND source_id=source AND root_lot_id IS NULL;
    IF FOUND THEN
        IF lot.scope_id IS DISTINCT FROM scope OR lot.issued_micros IS DISTINCT FROM amount OR lot.expires_at IS DISTINCT FROM deadline
          OR lot.model_key IS DISTINCT FROM model OR lot.reason IS DISTINCT FROM justification THEN
            RAISE EXCEPTION 'credit issuance identity conflict';
        END IF;
        RETURN lot.id;
    END IF;
    IF amount IS NULL OR amount<=0 OR deadline IS NULL OR deadline<=current_timepoint THEN
        RAISE EXCEPTION 'positive credit and future expiry required';
    END IF;
    UPDATE billing.wallet_account SET version=version+1
        WHERE scope_id=scope AND version=account_version RETURNING version INTO account_version;
    IF NOT FOUND THEN RAISE EXCEPTION 'wallet account version conflict'; END IF;
    INSERT INTO billing.wallet_lot
        (id,scope_id,source_kind,source_id,issued_micros,available_micros,version,expires_at,model_key,reason,created_at)
    VALUES (gen_random_uuid(),scope,'promotional',source,amount,amount,1,deadline,model,justification,current_timepoint)
    RETURNING * INTO lot;
    INSERT INTO billing.wallet_entry(scope_id,lot_id,kind,amount_micros,created_at)
        VALUES (scope,lot.id,'issue',amount,current_timepoint);
    event_id := gen_random_uuid();
    INSERT INTO infra.outbox(id,topic,partition_key,payload)
        VALUES (event_id,'glowloom.billing.wallet_balance_changed.v1',scope,
            jsonb_build_object('event_id',event_id,'event_type','glowloom.billing.wallet_balance_changed.v1',
                'scope_id',scope,'account_version',account_version,'occurred_at',current_timepoint));
    RETURN lot.id;
END
$$;
REVOKE ALL ON FUNCTION billing.issue_promotional_credit(uuid,uuid,bigint,timestamptz,text,text) FROM PUBLIC;

CREATE TABLE billing.wallet_compensation (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    reservation_id uuid NOT NULL UNIQUE REFERENCES billing.wallet_hold(reservation_id),
    reason text NOT NULL CHECK (length(reason) BETWEEN 1 AND 500),
    reviewed_by text NOT NULL,
    created_at timestamptz NOT NULL
);

-- Finance approves compensation using the original reservation. It leaves
-- project consumption and supplier liabilities available for reconciliation.
CREATE FUNCTION billing.compensate_wallet_consumption(reservation uuid, justification text) RETURNS uuid
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE payer uuid;
DECLARE correction billing.wallet_compensation%ROWTYPE;
DECLARE allocation record;
DECLARE root uuid;
DECLARE returned_lot uuid;
DECLARE deadline timestamptz;
DECLARE account_version bigint;
DECLARE new_available bigint;
DECLARE lot_version bigint;
DECLARE event_id uuid;
DECLARE current_timepoint timestamptz := clock_timestamp();
BEGIN
    SELECT h.scope_id INTO STRICT payer
        FROM billing.reservation r JOIN billing.wallet_hold h ON h.reservation_id=r.id
        WHERE r.id=$1 AND r.status='settled' AND NOT r.is_delete FOR UPDATE OF r;
    SELECT version INTO account_version FROM billing.wallet_account WHERE scope_id=payer;
    SELECT * INTO correction FROM billing.wallet_compensation WHERE reservation_id=$1;
    IF FOUND THEN
        IF correction.reason IS DISTINCT FROM justification THEN
            RAISE EXCEPTION 'credit compensation identity conflict';
        END IF;
        RETURN correction.id;
    END IF;
    UPDATE billing.wallet_account SET version=version+1
        WHERE scope_id=payer AND version=account_version RETURNING version INTO account_version;
    IF NOT FOUND THEN RAISE EXCEPTION 'wallet account version conflict'; END IF;
    INSERT INTO billing.wallet_compensation(reservation_id,reason,reviewed_by,created_at)
        VALUES ($1,justification,current_user,current_timepoint) RETURNING * INTO correction;
    FOR allocation IN
        SELECT l.id,l.expires_at,l.root_lot_id,l.model_key,e.amount_micros
        FROM billing.wallet_entry e JOIN billing.wallet_lot l ON l.id=e.lot_id
        WHERE e.reservation_id=$1 AND e.scope_id=payer AND e.kind='redeem'
        ORDER BY l.id
    LOOP
        INSERT INTO billing.wallet_entry(scope_id,lot_id,reservation_id,kind,amount_micros,created_at)
            VALUES (payer,allocation.id,$1,'compensate',allocation.amount_micros,current_timepoint);
        IF allocation.expires_at IS NULL OR allocation.expires_at>current_timepoint THEN
            SELECT available_micros,version INTO new_available,lot_version FROM billing.wallet_lot WHERE id=allocation.id;
            IF new_available > 9223372036854775807-allocation.amount_micros THEN
                RAISE EXCEPTION 'credit compensation balance overflow';
            END IF;
            UPDATE billing.wallet_lot SET available_micros=new_available+allocation.amount_micros,version=lot_version+1
                WHERE id=allocation.id AND version=lot_version;
            IF NOT FOUND THEN RAISE EXCEPTION 'wallet lot version conflict'; END IF;
        ELSE
            root := coalesce(allocation.root_lot_id,allocation.id);
            INSERT INTO billing.wallet_expiry_grace(root_lot_id,expires_at)
                VALUES (root,current_timepoint+interval '168 hours') ON CONFLICT DO NOTHING;
            SELECT expires_at INTO STRICT deadline FROM billing.wallet_expiry_grace WHERE root_lot_id=root;
            IF deadline<=current_timepoint THEN
                RAISE EXCEPTION 'credit compensation requires expiry review';
            END IF;
            INSERT INTO billing.wallet_lot
                (id,scope_id,source_kind,source_id,issued_micros,available_micros,version,expires_at,model_key,root_lot_id,reason,created_at)
            VALUES (gen_random_uuid(),payer,'expiry_return',gen_random_uuid(),allocation.amount_micros,
                allocation.amount_micros,1,deadline,allocation.model_key,root,'reviewed compensation',current_timepoint)
            RETURNING id INTO returned_lot;
            INSERT INTO billing.wallet_entry(scope_id,lot_id,reservation_id,kind,amount_micros,created_at)
                VALUES (payer,returned_lot,$1,'restore',allocation.amount_micros,current_timepoint);
        END IF;
    END LOOP;
    IF NOT FOUND THEN RAISE EXCEPTION 'wallet redemption evidence required'; END IF;
    event_id := gen_random_uuid();
    INSERT INTO infra.outbox(id,topic,partition_key,payload)
        VALUES (event_id,'glowloom.billing.wallet_balance_changed.v1',payer,
            jsonb_build_object('event_id',event_id,'event_type','glowloom.billing.wallet_balance_changed.v1',
                'scope_id',payer,'account_version',account_version,'occurred_at',current_timepoint));
    RETURN correction.id;
END
$$;
REVOKE ALL ON FUNCTION billing.compensate_wallet_consumption(uuid,text) FROM PUBLIC;

-- Require funding for every new capped reservation and any wallet-bound project.
CREATE FUNCTION billing.check_wallet_reservation() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    IF TG_OP='INSERT' AND NEW.amount_micros>0 AND
       (NEW.cap_at_quote OR EXISTS(SELECT 1 FROM billing.project_wallet WHERE project_id=NEW.project_id)) THEN
        -- SAGA confirmation records a pending freeze instead of a synchronous
        -- wallet hold. Only the exact internal operation command plus its
        -- durable dispatch event may defer funding; workers still await success.
        IF NEW.cap_at_quote AND NEW.status='held' AND
           NOT EXISTS(SELECT 1 FROM billing.wallet_hold WHERE reservation_id=NEW.id) AND
           EXISTS (
             SELECT 1 FROM operation.operation o
             JOIN workspace.project p ON p.id=o.project_id AND p.status='active' AND NOT p.is_delete
             LEFT JOIN billing.project_wallet w ON w.project_id=p.id
             JOIN billing.credit_command c ON c.source_id=o.id
             JOIN infra.outbox event ON event.topic='glowloom.billing.command.v1' AND NOT event.is_delete
               AND event.partition_key=c.scope_id::text
               AND event.payload->>'event_id'=event.id::text
               AND event.payload->>'event_type'=event.topic
               AND event.payload->>'command_id'=c.id::text
               AND event.payload->>'scope_id'=c.scope_id::text
             WHERE o.id=NEW.operation_id AND o.project_id=NEW.project_id AND o.reservation_id=NEW.id
               AND NOT o.is_delete AND o.status IN ('confirmed','cancelling') AND o.quote_micros=NEW.amount_micros
               AND c.scope_id=p.owner_scope_id AND COALESCE(w.scope_id,p.owner_scope_id)=p.owner_scope_id
               AND c.caller_id='system:billing-operation' AND c.action='freeze' AND c.status='accepted'
               AND c.amount_micros=NEW.amount_micros
               AND c.idempotency_key_hash=encode(sha256(convert_to('operation/'||o.id::text||'/freeze','UTF8')),'hex')
           ) THEN
            RETURN NEW;
        END IF;
        IF NOT NEW.cap_at_quote OR
           NOT EXISTS(SELECT 1 FROM billing.wallet_hold WHERE reservation_id=NEW.id) OR
           coalesce((SELECT sum(amount_micros) FROM billing.wallet_allocation WHERE reservation_id=NEW.id),0)<>NEW.amount_micros THEN
            RAISE EXCEPTION 'wallet-backed reservation requires complete funding' USING ERRCODE='23514';
        END IF;
    ELSIF TG_OP='UPDATE' AND OLD.status='held' AND NEW.status IN ('settled','released') AND
          EXISTS(SELECT 1 FROM billing.wallet_hold WHERE reservation_id=NEW.id) THEN
        IF coalesce((SELECT sum(amount_micros) FROM billing.wallet_entry
          WHERE reservation_id=NEW.id AND kind IN ('redeem','release','expire')),0)<>NEW.amount_micros THEN
            RAISE EXCEPTION 'wallet-backed reservation requires complete settlement' USING ERRCODE='23514';
        END IF;
    END IF;
    RETURN NEW;
END
$$;
CREATE CONSTRAINT TRIGGER wallet_reservation_consistency
AFTER INSERT OR UPDATE ON billing.reservation DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION billing.check_wallet_reservation();

-- Independent creation keeps project-scoped foreign keys without exposing a
-- business project. Old projects retain the existing visibility and identity.
ALTER TABLE workspace.project ADD COLUMN independent_canvas boolean NOT NULL DEFAULT false;
ALTER TABLE workspace.project ADD COLUMN independent_media boolean NOT NULL DEFAULT false;
ALTER TABLE workspace.project ADD CONSTRAINT independent_project_kind CHECK (NOT (independent_canvas AND independent_media));
CREATE UNIQUE INDEX project_independent_media_scope ON workspace.project(owner_scope_id)
    WHERE independent_media;

CREATE FUNCTION billing.ensure_creation_wallet(scope uuid) RETURNS void
LANGUAGE sql SECURITY DEFINER SET search_path = pg_catalog AS $$
    INSERT INTO billing.wallet_account(scope_id) VALUES (scope) ON CONFLICT DO NOTHING
$$;
REVOKE ALL ON FUNCTION billing.ensure_creation_wallet(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION billing.ensure_creation_wallet(uuid) TO glowloom_app;

-- Public supplier rate references. Executable price versions remain reviewed per model.
CREATE TABLE catalog.model_price_preset (
    adapter_key text NOT NULL,
    provider_model_id text NOT NULL,
    currency text NOT NULL CHECK (currency IN ('CNY', 'USD')),
    source_url text NOT NULL CHECK (source_url LIKE 'https://%'),
    reference_lines jsonb NOT NULL CHECK (jsonb_typeof(reference_lines) = 'array' AND jsonb_array_length(reference_lines) > 0),
    official_price jsonb CHECK (official_price IS NULL OR jsonb_typeof(official_price) = 'object'),
    official_price_observed_on date,
    gateway_text_tariff jsonb CHECK (gateway_text_tariff IS NULL OR jsonb_typeof(gateway_text_tariff) = 'object'),
    gateway_media_tariff jsonb CHECK (gateway_media_tariff IS NULL OR jsonb_typeof(gateway_media_tariff) = 'object'),
    unit text,
    rule jsonb,
    executable_until date,
    quick_onboarding jsonb CHECK (quick_onboarding IS NULL OR (currency = 'CNY' AND jsonb_typeof(quick_onboarding) = 'object')),
    video_estimate jsonb CHECK (video_estimate IS NULL OR (currency = 'CNY' AND jsonb_typeof(video_estimate) = 'object')),
    PRIMARY KEY (adapter_key, provider_model_id),
    CHECK ((official_price IS NULL) = (official_price_observed_on IS NULL)),
    CHECK (executable_until IS NULL OR unit IS NOT NULL),
    CHECK ((unit IS NULL AND rule IS NULL) OR
           (unit IN ('per_image', 'per_1k_tokens', 'per_1k_chars', 'per_second') AND unit IS NOT NULL AND rule IS NOT NULL AND jsonb_typeof(rule) = 'object'))
);
GRANT SELECT ON catalog.model_price_preset TO glowloom_app;

-- Public-source pricing snapshots are proposals; only reviewed price_rule_version rows bill.
CREATE TABLE catalog.model_price_candidate (
    id uuid PRIMARY KEY,
    adapter_key text NOT NULL CHECK (adapter_key ~ '^[a-z0-9_-]{1,64}$'),
    provider_model_id text NOT NULL CHECK (btrim(provider_model_id) <> '' AND length(provider_model_id) <= 256),
    source_url text NOT NULL CHECK (source_url LIKE 'https://%' AND length(source_url) <= 2048),
    source_hash text NOT NULL CHECK (source_hash ~ '^[0-9a-f]{64}$'),
    parser_version text NOT NULL CHECK (btrim(parser_version) <> '' AND length(parser_version) <= 64),
    source_fetched_at timestamptz NOT NULL,
    currency character(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    unit text,
    rule jsonb,
    source_pricing jsonb NOT NULL CHECK (jsonb_typeof(source_pricing) = 'object'),
    status text NOT NULL CHECK (status IN ('pending_review', 'needs_review', 'published', 'rejected')),
    published_price_version_id uuid REFERENCES catalog.price_rule_version(id),
    reviewed_by uuid REFERENCES identity."user"(id),
    reviewed_at timestamptz,
    create_time timestamptz NOT NULL DEFAULT now(),
    UNIQUE (adapter_key, provider_model_id, source_hash, parser_version),
    CHECK ((unit IS NULL AND rule IS NULL) OR
           (unit IS NOT NULL AND unit IN ('per_image', 'per_second', 'per_request', 'per_1k_tokens', 'per_1k_chars') AND
            (rule IS NULL OR jsonb_typeof(rule) = 'object'))),
    CHECK (
      (status = 'pending_review' AND unit IS NOT NULL AND rule IS NOT NULL AND published_price_version_id IS NULL AND reviewed_by IS NULL AND reviewed_at IS NULL) OR
      (status = 'needs_review' AND published_price_version_id IS NULL AND reviewed_by IS NULL AND reviewed_at IS NULL) OR
      (status = 'published' AND unit IS NOT NULL AND rule IS NOT NULL AND published_price_version_id IS NOT NULL AND reviewed_by IS NOT NULL AND reviewed_at IS NOT NULL) OR
      (status = 'rejected' AND published_price_version_id IS NULL AND reviewed_by IS NOT NULL AND reviewed_at IS NOT NULL)
    )
);
CREATE INDEX model_price_candidate_model_recent
    ON catalog.model_price_candidate (adapter_key, provider_model_id, source_fetched_at DESC, id DESC);
CREATE INDEX model_price_candidate_review_queue
    ON catalog.model_price_candidate (source_fetched_at DESC, id DESC)
    WHERE status IN ('pending_review', 'needs_review');
GRANT SELECT, INSERT, UPDATE ON catalog.model_price_candidate TO glowloom_app;

-- Public model candidates are immutable snapshots of a provider preset version.
CREATE TABLE catalog.model_preset (
    preset_version_id uuid NOT NULL REFERENCES catalog.provider_preset_version(id),
    model_key text NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal >= 0),
    payload jsonb NOT NULL,
    content_hash text NOT NULL,
    PRIMARY KEY (preset_version_id, model_key),
    UNIQUE (preset_version_id, ordinal),
    CHECK (jsonb_typeof(payload)='object' AND octet_length(payload::text)<=65536),
    CHECK (payload->>'key'=model_key AND length(model_key) BETWEEN 1 AND 64),
    CHECK (content_hash=encode(sha256(convert_to(payload::text,'UTF8')),'hex'))
);
CREATE FUNCTION catalog.guard_model_preset() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='UPDATE' AND NEW IS NOT DISTINCT FROM OLD THEN RETURN NEW; END IF;
    RAISE EXCEPTION 'model preset snapshots are immutable' USING ERRCODE='23514';
END $$;
CREATE TRIGGER model_preset_immutable BEFORE UPDATE OR DELETE ON catalog.model_preset
FOR EACH ROW EXECUTE FUNCTION catalog.guard_model_preset();
GRANT SELECT,INSERT ON catalog.model_preset TO glowloom_app;

-- One original Agent receipt owns one operation commitment. The receipt is
-- inserted after the atomic command callback, within this same transaction.
CREATE TABLE agent_business.operation_commitment (
 receipt_id uuid PRIMARY KEY REFERENCES agent_business.call_receipt(receipt_id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
 operation_id uuid NOT NULL UNIQUE REFERENCES operation.operation(id)
);
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='glowloom_app') THEN
  GRANT SELECT,INSERT ON agent_business.operation_commitment TO glowloom_app;
 END IF;
END $$;

-- Recovery diagnostics/backoff belongs to the original Agent run/control, not a new executor.
CREATE TABLE agent.recovery_backoff (
 kind text NOT NULL CHECK(kind IN ('run','control')),
 subject_id uuid NOT NULL,
 root_run_id uuid NOT NULL REFERENCES agent.service_runs(id) ON DELETE CASCADE,
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts>=0),
 code text,
 next_attempt_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 parked_at timestamptz,
 PRIMARY KEY(kind,subject_id)
);
CREATE INDEX recovery_backoff_run ON agent.recovery_backoff(root_run_id);
REVOKE ALL ON agent.recovery_backoff FROM PUBLIC;
GRANT SELECT,INSERT,UPDATE ON agent.recovery_backoff TO glowloom_agent;

-- Frozen business goal; empty production plans retain their delivery obligation.
ALTER TABLE agent_business.run_authorization ADD COLUMN goal_contract jsonb NOT NULL DEFAULT '{"kind":"chat","required_deliveries":0}' CHECK (jsonb_typeof(goal_contract) = 'object');

-- Route changes publish immutable membership. A quote keeps the version it saw.
CREATE TABLE catalog.route_pool_version (
    id uuid PRIMARY KEY,
    primary_release_id uuid NOT NULL REFERENCES catalog.model_release(id),
    revision integer NOT NULL CHECK (revision > 0),
    created_by uuid NOT NULL REFERENCES identity."user"(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(primary_release_id,revision)
);
CREATE TABLE catalog.route_pool_member (
    version_id uuid NOT NULL REFERENCES catalog.route_pool_version(id),
    release_id uuid NOT NULL REFERENCES catalog.model_release(id),
    priority integer NOT NULL CHECK(priority BETWEEN 0 AND 15),
    PRIMARY KEY(version_id,release_id), UNIQUE(version_id,priority)
);
CREATE TABLE operation.route_pool_binding (
    operation_id uuid PRIMARY KEY REFERENCES operation.provider_binding(operation_id),
    version_id uuid NOT NULL REFERENCES catalog.route_pool_version(id)
);
CREATE TABLE operation.route_delivery (
    operation_id uuid PRIMARY KEY REFERENCES operation.route_pool_binding(operation_id),
    release_id uuid NOT NULL REFERENCES catalog.model_release(id),
    procurement_price_id uuid NOT NULL REFERENCES catalog.price_rule_version(id),
    created_at timestamptz NOT NULL DEFAULT now()
);
GRANT SELECT, INSERT ON catalog.route_pool_version,catalog.route_pool_member,operation.route_pool_binding,operation.route_delivery TO glowloom_app;

-- An unexpired quote can keep its original sale when only the retail
-- price changed. A changed execution contract or purchase tariff closes it.
CREATE VIEW catalog.executable_release AS
SELECT old.* FROM catalog.model_release old
JOIN catalog.model_profile m ON m.id=old.model_id AND m.status='active' AND NOT m.is_delete
JOIN catalog.model_release live ON live.id=m.active_release_id AND live.status='active'
JOIN catalog.price_rule_version sale ON sale.id=old.price_version_id AND NOT sale.is_delete
JOIN catalog.price_rule_version current_sale ON current_sale.id=live.price_version_id AND NOT current_sale.is_delete
WHERE old.id=live.id OR (old.status='retired' AND old.model_version_id=live.model_version_id
 AND old.config_version_id=live.config_version_id AND old.upstream_cost=live.upstream_cost
 AND EXISTS(SELECT 1 FROM catalog.price_rule_version prior JOIN catalog.price_rule_version current_purchase
 ON current_purchase.id::text=current_sale.rule->>'procurement_price_version_id'
 WHERE prior.id::text=sale.rule->>'procurement_price_version_id' AND NOT prior.is_delete AND NOT current_purchase.is_delete
 AND prior.model_profile_id=current_purchase.model_profile_id AND prior.unit=current_purchase.unit AND prior.rule=current_purchase.rule
 AND prior.currency=current_purchase.currency AND prior.fx_rate_to_cny IS NOT DISTINCT FROM current_purchase.fx_rate_to_cny));
GRANT SELECT ON catalog.executable_release TO glowloom_app;

-- Equivalence requires a matching published contract. The administrator separately
-- attests that the upstream SKU, data handling and storage location are equivalent.
CREATE VIEW catalog.route_pool_candidate AS
SELECT base.id AS primary_release_id, alt.id AS release_id, alt.model_version_id,
       alt.config_version_id,alt.price_version_id,alt.model_id,ap.id AS provider_id,
       ap.name AS provider_name,av.provider_model_id,am.display_name,
       ap.adapter_key,tariff.id AS procurement_price_id
FROM catalog.executable_release base
JOIN catalog.model_profile bm ON bm.id=base.model_id AND bm.status='active' AND NOT bm.is_delete
JOIN catalog.provider bp ON bp.id=bm.provider_id
JOIN catalog.model_profile_version bv ON bv.id=base.model_version_id AND NOT bv.is_delete
JOIN catalog.price_rule_version sale ON sale.id=base.price_version_id AND NOT sale.is_delete
JOIN catalog.model_profile am ON am.capability=bm.capability AND am.status='active' AND NOT am.is_delete
JOIN catalog.executable_release alt ON alt.model_id=am.id
JOIN catalog.model_profile_version av ON av.id=alt.model_version_id AND NOT av.is_delete
JOIN catalog.provider ap ON ap.id=am.provider_id AND ap.region=bp.region AND NOT ap.is_delete
JOIN catalog.price_rule_version rate ON rate.id=alt.price_version_id AND NOT rate.is_delete AND rate.unit=sale.unit
JOIN catalog.price_rule_version tariff ON tariff.id::text=rate.rule->>'procurement_price_version_id' AND tariff.model_profile_id=am.id AND NOT tariff.is_delete
WHERE av.modes @> bv.modes AND av.param_schema=bv.param_schema
AND (av.limits-'provider_endpoint')=(bv.limits-'provider_endpoint') AND av.queue=bv.queue AND av.moderation=bv.moderation
AND av.supports_query=bv.supports_query AND av.supports_cancel=bv.supports_cancel AND av.supports_callback=bv.supports_callback
AND av.expected_max_ms<=bv.expected_max_ms
AND (rate.rule->>'pricing_strategy') IS NOT DISTINCT FROM (sale.rule->>'pricing_strategy')
AND (rate.rule->>'reference_scope') IS NOT DISTINCT FROM (sale.rule->>'reference_scope')
AND (rate.rule->>'time_pricing') IS NOT DISTINCT FROM (sale.rule->>'time_pricing')
AND (rate.rule ? 'cached_input_micros_per_1k')=(sale.rule ? 'cached_input_micros_per_1k')
AND ap.current_config_version_id=alt.config_version_id
AND (alt.id=base.id OR (alt.upstream_cost->>'currency'=base.upstream_cost->>'currency'
AND (alt.upstream_cost->>'max_micros')::bigint BETWEEN 0 AND (base.upstream_cost->>'max_micros')::bigint
AND (ap.adapter_key<>'openrouter' OR COALESCE(av.limits->>'provider_endpoint','')<>'')));
GRANT SELECT ON catalog.route_pool_candidate TO glowloom_app;

-- Rotation keeps its historical draining semantics. Explicit admission lets an
-- administrator reuse a previously verified credential for new pooled calls.
ALTER TABLE catalog.provider_credential ADD COLUMN pool_enabled boolean NOT NULL DEFAULT false;


-- Current execution evidence belongs to one key, mode and immutable release.
CREATE TABLE catalog.route_connection_cooldown (
 provider_id uuid PRIMARY KEY REFERENCES catalog.provider(id),
 until_at timestamptz NOT NULL
);
GRANT SELECT,INSERT,UPDATE ON catalog.route_connection_cooldown TO glowloom_app;

CREATE VIEW catalog.verified_route_key AS
SELECT DISTINCT r.id AS release_id, x.mode, k.id AS credential_id
FROM catalog.executable_release r
JOIN catalog.model_validation x ON x.release_id=r.id AND x.status='succeeded'
 AND x.adapter_version='ai-provider-v1' AND x.evidence->>'kind'='real_provider'
JOIN catalog.model_profile m ON m.id=r.model_id AND NOT m.is_delete AND m.status='active'
JOIN catalog.provider p ON p.id=m.provider_id AND p.status='active' AND NOT p.is_delete AND p.current_config_version_id=r.config_version_id
 AND NOT EXISTS(SELECT 1 FROM catalog.route_connection_cooldown down WHERE down.provider_id=p.id AND down.until_at>statement_timestamp())
JOIN catalog.provider_config_version cfg ON cfg.id=r.config_version_id
JOIN catalog.provider_preset_version pv ON pv.id=cfg.preset_version_id
JOIN catalog.provider_preset preset ON preset.id=pv.preset_id AND NOT preset.emergency_disabled AND NOT preset.archived
JOIN catalog.provider_credential k ON k.id=x.credential_id AND k.provider_id=p.id AND NOT k.is_delete
 AND (k.status='active' OR (k.status='draining' AND k.pool_enabled)) AND k.last_test_result IS DISTINCT FROM 'auth_failed'
JOIN operation.operation op ON op.id=x.operation_id AND op.status='completed' AND NOT op.is_delete AND op.mode=x.mode
 AND op.model_profile_version_id=r.model_version_id AND op.price_rule_version_id=r.price_version_id
JOIN operation.provider_binding b ON b.operation_id=op.id AND b.release_id=r.id
JOIN operation.provider_credential_binding used ON used.operation_id=op.id AND used.credential_id=k.id
;
GRANT SELECT ON catalog.verified_route_key TO glowloom_app;

CREATE VIEW catalog.route_pool_ready AS
SELECT v.id AS version_id,v.primary_release_id,c.release_id,c.model_version_id,c.provider_id,c.model_id,
       c.config_version_id,c.procurement_price_id,m.priority,k.mode
FROM catalog.route_pool_version v
JOIN catalog.route_pool_member m ON m.version_id=v.id
JOIN catalog.route_pool_candidate c ON c.primary_release_id=v.primary_release_id AND c.release_id=m.release_id
JOIN (SELECT DISTINCT release_id,mode FROM catalog.verified_route_key) k ON k.release_id=m.release_id;
GRANT SELECT ON catalog.route_pool_ready TO glowloom_app;

-- A second send may follow only a transport-proven failure before connection.
CREATE TABLE operation.route_retry (
 operation_id uuid PRIMARY KEY REFERENCES operation.route_delivery(operation_id),
 release_id uuid NOT NULL REFERENCES catalog.model_release(id),
 credential_id uuid NOT NULL REFERENCES catalog.provider_credential(id),
 procurement_price_id uuid NOT NULL REFERENCES catalog.price_rule_version(id),
 reason text NOT NULL CHECK(reason='dial_not_sent'),
 created_at timestamptz NOT NULL DEFAULT now()
);
GRANT SELECT,INSERT ON operation.route_retry TO glowloom_app;

ALTER TABLE catalog.provider ADD COLUMN quota_group text NOT NULL DEFAULT '' CHECK(octet_length(quota_group)<=80);
CREATE INDEX provider_quota_group ON catalog.provider(quota_group) WHERE quota_group<>'';

-- The smallest declared member limit protects a shared upstream account.
CREATE VIEW catalog.effective_provider_limit AS
SELECT p.id, min(peer.concurrency_limit) AS concurrency_limit,min(peer.rate_limit_per_min) AS rate_limit_per_min
FROM catalog.provider p JOIN catalog.provider peer ON peer.id=p.id OR (p.quota_group<>'' AND peer.quota_group=p.quota_group AND NOT peer.is_delete)
GROUP BY p.id;
GRANT SELECT ON catalog.effective_provider_limit TO glowloom_app;

-- Native image throughput from the Ark model list. Key by immutable SKU.
CREATE TABLE catalog.model_image_limit (
 adapter_key text NOT NULL,provider_model_id text NOT NULL,
 images_per_minute integer NOT NULL CHECK(images_per_minute>0),
 PRIMARY KEY(adapter_key,provider_model_id)
);
GRANT SELECT ON catalog.model_image_limit TO glowloom_app;

ALTER TABLE catalog.provider ADD CONSTRAINT provider_logo_asset_fk FOREIGN KEY (logo_asset_id) REFERENCES media.media_asset(id);

-- Developer Workspace runtime; business workspace schema remains unchanged.
-- NOLOGIN capability role. Administrators provision a dedicated LOGIN and grant this role;
-- never grant it glowloom_app or business-schema privileges.
DO $$
BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='glowloom_workspace') THEN
  CREATE ROLE glowloom_workspace NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION;
 ELSIF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='glowloom_workspace' AND (rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication)) THEN
  RAISE EXCEPTION 'glowloom_workspace must be a restricted role';
 END IF;
END $$;
CREATE SCHEMA dev_workspace;
REVOKE ALL ON SCHEMA dev_workspace FROM PUBLIC;
GRANT USAGE ON SCHEMA dev_workspace TO glowloom_workspace;
CREATE TABLE dev_workspace.content_proposal (
 id uuid PRIMARY KEY,
 actor text NOT NULL CHECK(length(actor) BETWEEN 1 AND 128),
 idempotency_key text NOT NULL CHECK(length(idempotency_key) BETWEEN 1 AND 160),
 input_digest text NOT NULL CHECK(length(input_digest)=64),
 record jsonb NOT NULL CHECK(jsonb_typeof(record)='object' AND octet_length(record::text)<=3000000),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(actor,idempotency_key)
);
CREATE TABLE dev_workspace.workflow_run (
 id uuid PRIMARY KEY,
 actor text NOT NULL CHECK(length(actor) BETWEEN 1 AND 128),
 idempotency_key text NOT NULL CHECK(length(idempotency_key) BETWEEN 1 AND 160),
 input_digest text NOT NULL CHECK(length(input_digest)=64),
 record jsonb NOT NULL CHECK(jsonb_typeof(record)='object' AND octet_length(record::text)<=3000000),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(actor,idempotency_key)
);
CREATE INDEX dev_workspace_run_actor_created ON dev_workspace.workflow_run(actor,created_at DESC);
CREATE TABLE dev_workspace.run_event (
 run_id uuid NOT NULL REFERENCES dev_workspace.workflow_run(id),
 seq bigint NOT NULL CHECK(seq>0),
 event_key text NOT NULL CHECK(length(event_key) BETWEEN 1 AND 160),
 kind text NOT NULL CHECK(length(kind) BETWEEN 1 AND 64),
 payload jsonb NOT NULL CHECK(jsonb_typeof(payload)='object' AND octet_length(payload::text)<=1000000),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(run_id,seq), UNIQUE(run_id,event_key)
);
CREATE TABLE dev_workspace.artifact_ref (
 id uuid PRIMARY KEY,
 run_id uuid REFERENCES dev_workspace.workflow_run(id), actor text NOT NULL,
 kind text NOT NULL, source_revision text NOT NULL,
 checksum text NOT NULL CHECK(length(checksum)=64),
 manifest jsonb NOT NULL CHECK(jsonb_typeof(manifest)='object' AND octet_length(manifest::text)<=1000000),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE dev_workspace.executor_binding (
 run_id uuid NOT NULL REFERENCES dev_workspace.workflow_run(id), attempt integer NOT NULL CHECK(attempt>0),
 executor text NOT NULL, external_handle text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(run_id,attempt)
);
REVOKE ALL ON ALL TABLES IN SCHEMA dev_workspace FROM PUBLIC;
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA dev_workspace TO glowloom_workspace;

-- Writer-local final-commit barriers. These are technical records, not an
-- alternative source of account, session or project policy. Only authenticated
-- issuer control adapters may install decisions; effects/receipts share the
-- writer's existing local business transaction. No TTL deletes responsibility.
CREATE TABLE infra.writer_fence (
    service_owner text NOT NULL DEFAULT infra.runtime_owner() CHECK (service_owner IN ('legacy','identity','creation','media','execution')),
    issuer text NOT NULL,
    authority_kind text NOT NULL,
    authority_id uuid NOT NULL,
    epoch bigint NOT NULL CHECK (epoch > 0),
    blocked boolean NOT NULL,
    coordination_id uuid NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    PRIMARY KEY (service_owner, issuer, authority_kind, authority_id),
    CHECK ((issuer='identity' AND authority_kind IN ('account','session')) OR
           (issuer='creation' AND authority_kind IN ('scope','project','agent_grant')))
);

CREATE TABLE infra.writer_command (
    service_owner text NOT NULL DEFAULT infra.runtime_owner() CHECK (service_owner IN ('legacy','identity','creation','media','execution')),
    id uuid NOT NULL,
    request_sha256 text NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
    action varchar(128) NOT NULL CHECK (action ~ '^[a-z0-9._-]+$'),
    authority_set_sha256 text NOT NULL CHECK (authority_set_sha256 ~ '^[0-9a-f]{64}$'),
    authorities jsonb NOT NULL CHECK (jsonb_typeof(authorities)='array' AND jsonb_array_length(authorities) BETWEEN 1 AND 16),
    state text NOT NULL CHECK (state IN ('admitted','committed','frozen')),
    receipt jsonb,
    created_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    PRIMARY KEY (service_owner, id),
    CHECK ((state='committed') = (receipt IS NOT NULL)),
    -- The adapter bounds canonical JSON to 1MiB. PostgreSQL JSONB display adds
    -- separators/spaces, so this defensive storage cap allows that expansion.
    CHECK (receipt IS NULL OR (jsonb_typeof(receipt)='object' AND octet_length(receipt::text)<=2097152))
);
GRANT SELECT,INSERT,UPDATE,DELETE ON infra.writer_fence,infra.writer_command TO glowloom_app;

-- Issuer-local durable coordination. Pending open AND block controls close
-- admission locally; only all four exact writer ACKs permit completion. Policy
-- checks remain in the issuer's existing business transaction, not these tables.
CREATE TABLE infra.authority_head (
    service_owner text NOT NULL DEFAULT infra.runtime_owner() CHECK (service_owner IN ('identity','creation')),
    issuer text NOT NULL CHECK (issuer = service_owner),
    authority_kind text NOT NULL,
    authority_id uuid NOT NULL,
    epoch bigint NOT NULL CHECK (epoch > 0),
    coordination_id uuid NOT NULL,
    blocked boolean NOT NULL DEFAULT true,
    complete boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    PRIMARY KEY (service_owner, issuer, authority_kind, authority_id),
    CHECK (complete OR blocked),
    CHECK ((issuer='identity' AND authority_kind IN ('account','session')) OR
           (issuer='creation' AND authority_kind IN ('scope','project','agent_grant')))
);

CREATE TABLE infra.authority_coordination (
    service_owner text NOT NULL DEFAULT infra.runtime_owner() CHECK (service_owner IN ('identity','creation')),
    coordination_id uuid NOT NULL,
    issuer text NOT NULL CHECK (issuer = service_owner),
    authority_kind text NOT NULL,
    authority_id uuid NOT NULL,
    expected_epoch bigint NOT NULL CHECK (expected_epoch >= 0 AND expected_epoch < 9223372036854775807),
    epoch bigint NOT NULL CHECK (epoch = expected_epoch + 1),
    blocked boolean NOT NULL,
    created_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    PRIMARY KEY (service_owner, coordination_id),
    UNIQUE (service_owner, issuer, authority_kind, authority_id, epoch),
    FOREIGN KEY (service_owner, issuer, authority_kind, authority_id)
        REFERENCES infra.authority_head(service_owner, issuer, authority_kind, authority_id)
);

CREATE TABLE infra.authority_acknowledgement (
    service_owner text NOT NULL DEFAULT infra.runtime_owner() CHECK (service_owner IN ('identity','creation')),
    coordination_id uuid NOT NULL,
    writer text NOT NULL CHECK (writer IN ('identity','creation','media','execution')),
    created_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    PRIMARY KEY (service_owner, coordination_id, writer),
    FOREIGN KEY (service_owner, coordination_id)
        REFERENCES infra.authority_coordination(service_owner, coordination_id)
);

CREATE TABLE infra.authority_admission (
    service_owner text NOT NULL DEFAULT infra.runtime_owner() CHECK (service_owner IN ('identity','creation')),
    id uuid NOT NULL,
    request_sha256 text NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
    action varchar(128) NOT NULL CHECK (action ~ '^[a-z0-9._-]+$'),
    authority_set_sha256 text NOT NULL CHECK (authority_set_sha256 ~ '^[0-9a-f]{64}$'),
    authorities jsonb NOT NULL CHECK (jsonb_typeof(authorities)='array' AND jsonb_array_length(authorities) BETWEEN 1 AND 16),
    target_writer text NOT NULL CHECK (target_writer IN ('identity','creation','media','execution')),
    created_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    PRIMARY KEY (service_owner, id)
);
-- History, ACKs and admissions are append-only under every runtime role. No
-- UPDATE/DELETE privilege can clear unknown responsibility or reset an old ID.
GRANT SELECT,INSERT ON infra.authority_head,infra.authority_coordination,
    infra.authority_acknowledgement,infra.authority_admission TO glowloom_app;
GRANT UPDATE(epoch,coordination_id,blocked,complete,updated_at)
    ON infra.authority_head TO glowloom_app;

-- Stage B: database-enforced technical-record ownership. The old runtime is
-- still the sole writer of unmigrated business paths, but only of legacy rows.
-- Applying this policy to existing children also protects accidental direct
-- grants. Future offline partitions receive no runtime ACL/default privileges.
DO $$
DECLARE
  target regclass;
  column_names text;
BEGIN
  FOR target IN
    SELECT oid FROM pg_class WHERE oid IN (
      'audit.audit_log'::regclass,'infra.idempotency_record'::regclass,
      'infra.outbox'::regclass,'infra.outbox_dead_letter'::regclass,
      'infra.outbox_dead_letter_audit'::regclass,'infra.processed_event'::regclass,
      'infra.task_work'::regclass,'infra.writer_fence'::regclass,'infra.writer_command'::regclass,
      'infra.authority_head'::regclass,'infra.authority_coordination'::regclass,
      'infra.authority_acknowledgement'::regclass,'infra.authority_admission'::regclass
    ) OR oid IN (
      SELECT inhrelid FROM pg_inherits WHERE inhparent IN ('audit.audit_log'::regclass,'infra.outbox'::regclass)
    )
  LOOP
    EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', target);
    EXECUTE format('ALTER TABLE %s FORCE ROW LEVEL SECURITY', target);
    EXECUTE format('CREATE POLICY owner_isolation ON %s USING (service_owner = infra.runtime_owner()) WITH CHECK (service_owner = infra.runtime_owner())', target);
    -- Preserve the original mutable columns, never grant UPDATE(service_owner).
    IF has_table_privilege('glowloom_app',target,'UPDATE') THEN
      SELECT string_agg(quote_ident(attname),',' ORDER BY attnum) INTO column_names
        FROM pg_attribute WHERE attrelid=target AND attnum>0 AND NOT attisdropped AND attname<>'service_owner';
      EXECUTE format('REVOKE UPDATE ON %s FROM glowloom_app',target);
      EXECUTE format('GRANT UPDATE(%s) ON %s TO glowloom_app',column_names,target);
    END IF;
  END LOOP;
END $$;

-- Reuse the final legacy ACL as the mutation contract, including column-level
-- UPDATE restrictions. This is initialization-time GRANT only, not runtime DDL
-- or role inheritance. No schema-wide business grants and no child-table grants.
DO $$
DECLARE
  owned_table record;
  table_privilege text;
  column_grant record;
  runtime_role text;
BEGIN
  FOR owned_table IN
    SELECT c.oid,n.nspname,c.relname,
      CASE WHEN n.nspname='identity' THEN 'identity'
           WHEN n.nspname IN ('workspace','canvas','script','bible','subject') THEN 'creation'
           WHEN n.nspname IN ('media','mediatool') THEN 'media'
           WHEN n.nspname IN ('operation','catalog','billing') THEN 'execution'
           WHEN n.nspname='agent_business' AND c.relname='operation_commitment' THEN 'execution'
           WHEN n.nspname='agent_business' THEN 'creation'
           ELSE NULL END AS domain_owner
    FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
    WHERE n.nspname IN ('identity','workspace','canvas','script','bible','subject','media','mediatool','operation','catalog','billing','agent_business','infra','audit')
      AND c.relkind IN ('r','p','v') AND NOT c.relispartition
  LOOP
    FOREACH runtime_role IN ARRAY CASE WHEN owned_table.domain_owner IS NULL
      THEN ARRAY['glowloom_identity','glowloom_creation','glowloom_media','glowloom_execution']
      ELSE ARRAY['glowloom_' || owned_table.domain_owner] END
    LOOP
      EXECUTE format('GRANT USAGE ON SCHEMA %I TO %I',owned_table.nspname,runtime_role);
      FOREACH table_privilege IN ARRAY ARRAY['SELECT','INSERT','UPDATE','DELETE'] LOOP
        IF has_table_privilege('glowloom_app',owned_table.oid,table_privilege) THEN
          EXECUTE format('GRANT %s ON %s TO %I',table_privilege,owned_table.oid::regclass,runtime_role);
        END IF;
      END LOOP;
      FOR column_grant IN
        SELECT a.attname,acl.privilege_type
        FROM pg_attribute a CROSS JOIN LATERAL aclexplode(a.attacl) acl
        WHERE a.attrelid=owned_table.oid AND a.attnum>0 AND NOT a.attisdropped
          AND acl.grantee='glowloom_app'::regrole
          AND acl.privilege_type IN ('SELECT','INSERT','UPDATE')
      LOOP
        EXECUTE format('GRANT %s(%I) ON %s TO %I',column_grant.privilege_type,column_grant.attname,owned_table.oid::regclass,runtime_role);
      END LOOP;
    END LOOP;
  END LOOP;
END $$;

-- Functions are not a cross-owner back door. Keep legacy's existing EXECUTE
-- rights, remove PUBLIC access, and expose only same-domain SECURITY INVOKER
-- functions to the new roles. Existing cross-domain triggers/FKs are deliberately
-- not declared usable: their durable handoff belongs to later stages.
DO $$
DECLARE
  owned_function record;
  legacy_execute boolean;
BEGIN
  FOR owned_function IN
    SELECT p.oid,p.prosecdef,n.nspname,
      CASE WHEN n.nspname='identity' THEN 'glowloom_identity'
           WHEN n.nspname IN ('workspace','canvas','script','bible','subject','agent_business') THEN 'glowloom_creation'
           WHEN n.nspname IN ('media','mediatool') THEN 'glowloom_media'
           WHEN n.nspname IN ('operation','catalog','billing') THEN 'glowloom_execution'
           ELSE NULL END AS runtime_role
    FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
    WHERE n.nspname IN ('identity','workspace','canvas','script','bible','subject','agent_business','media','mediatool','operation','catalog','billing','audit')
  LOOP
    legacy_execute := has_function_privilege('glowloom_app',owned_function.oid,'EXECUTE');
    EXECUTE format('REVOKE ALL ON FUNCTION %s FROM PUBLIC',owned_function.oid::regprocedure);
    IF legacy_execute THEN
      EXECUTE format('GRANT EXECUTE ON FUNCTION %s TO glowloom_app',owned_function.oid::regprocedure);
      IF owned_function.runtime_role IS NOT NULL AND NOT owned_function.prosecdef THEN
        EXECUTE format('GRANT EXECUTE ON FUNCTION %s TO %I',owned_function.oid::regprocedure,owned_function.runtime_role);
      END IF;
    END IF;
  END LOOP;
END $$;

-- Identity's journal is not shared owner infrastructure: actual role ACLs, not
-- multi-owner RLS, isolate it. Keep all binding fields immutable and retain every
-- original command. Legacy receives the same minimum for offline compatibility.
-- Grant after the legacy ACL-copy loop; other business owners receive nothing.
GRANT SELECT,INSERT ON TABLE identity.session_revocation TO glowloom_app,glowloom_identity;
GRANT UPDATE(completed_at) ON TABLE identity.session_revocation TO glowloom_app,glowloom_identity;
GRANT SELECT,INSERT ON TABLE identity.session_revocation_request TO glowloom_app,glowloom_identity;
GRANT UPDATE(completed_at) ON TABLE identity.session_revocation_request TO glowloom_app,glowloom_identity;
GRANT SELECT,INSERT ON TABLE identity.session_revocation_request_target TO glowloom_app,glowloom_identity;
GRANT SELECT,INSERT ON TABLE identity.session_revocation_request_proof TO glowloom_app,glowloom_identity;

-- Media-local retention ledger. Other owner identities remain opaque values;
-- no cross-domain foreign keys, legacy grants or physical routing keys.
CREATE TABLE media.reference_pin_gate (
    asset_id uuid PRIMARY KEY REFERENCES media.media_asset(id) ON DELETE RESTRICT,
    epoch bigint NOT NULL DEFAULT 1 CHECK (epoch > 0),
    closed boolean NOT NULL DEFAULT false,
    close_command_id uuid,
    CHECK (closed = (close_command_id IS NOT NULL))
);
CREATE TABLE media.reference_pin (
    pin_id uuid PRIMARY KEY,
    consumer text NOT NULL CHECK (consumer IN ('identity','creation','execution')),
    consumer_command_id uuid NOT NULL,
    consumer_command_sha256 text NOT NULL CHECK (consumer_command_sha256 ~ '^[0-9a-f]{64}$'),
    consumer_resource_id uuid NOT NULL,
    scope_id uuid NOT NULL,
    project_id uuid NOT NULL,
    asset_id uuid NOT NULL REFERENCES media.reference_pin_gate(asset_id) ON DELETE RESTRICT,
    location_id uuid NOT NULL REFERENCES media.object_location(id) ON DELETE RESTRICT,
    byte_size bigint NOT NULL CHECK (byte_size >= 0),
    sha256 text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    asset_revision bigint NOT NULL CHECK (asset_revision > 0),
    gate_epoch bigint NOT NULL CHECK (gate_epoch > 0),
    acquire_command_id uuid NOT NULL UNIQUE,
    facts jsonb NOT NULL CHECK (jsonb_typeof(facts)='object' AND octet_length(facts::text)<=2097152),
    state text NOT NULL CHECK (state IN ('held','bound','released')),
    adoption_receipt_sha256 text NOT NULL DEFAULT '' CHECK (adoption_receipt_sha256='' OR adoption_receipt_sha256 ~ '^[0-9a-f]{64}$'),
    release_receipt_sha256 text NOT NULL DEFAULT '' CHECK (release_receipt_sha256='' OR release_receipt_sha256 ~ '^[0-9a-f]{64}$'),
    release_outcome text NOT NULL DEFAULT '' CHECK (release_outcome IN ('','not_adopted_frozen','removed')),
    CHECK (state<>'held' OR adoption_receipt_sha256=''),
    CHECK (state<>'bound' OR adoption_receipt_sha256<>''),
    CHECK ((state='released') = (release_receipt_sha256<>'' AND release_outcome<>'')),
    CHECK (state='released' OR (release_receipt_sha256='' AND release_outcome='')),
    CHECK (adoption_receipt_sha256='' OR release_outcome<>'not_adopted_frozen')
);
CREATE INDEX reference_pin_asset_responsibility ON media.reference_pin(asset_id,state,pin_id);

-- Column ACLs protect identity. This local lifecycle guard also prevents a
-- runtime SQL error from reopening a gate or resurrecting released responsibility.
CREATE FUNCTION media.guard_reference_pin_lifecycle() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_TABLE_NAME='reference_pin_gate' THEN
        IF OLD.closed OR NOT NEW.closed OR NEW.epoch<>OLD.epoch+1 OR NEW.close_command_id IS NULL THEN
            RAISE EXCEPTION 'reference acquisition closure is permanent' USING ERRCODE='23514';
        END IF;
    ELSE
        IF (OLD.state='released' AND NEW IS DISTINCT FROM OLD)
           OR (OLD.state='bound' AND NEW.state='held')
           OR (OLD.adoption_receipt_sha256<>'' AND NEW.adoption_receipt_sha256<>OLD.adoption_receipt_sha256) THEN
            RAISE EXCEPTION 'reference pin responsibility is monotonic' USING ERRCODE='23514';
        END IF;
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER reference_pin_gate_lifecycle BEFORE UPDATE ON media.reference_pin_gate
FOR EACH ROW EXECUTE FUNCTION media.guard_reference_pin_lifecycle();
CREATE TRIGGER reference_pin_lifecycle BEFORE UPDATE ON media.reference_pin
FOR EACH ROW EXECUTE FUNCTION media.guard_reference_pin_lifecycle();
REVOKE ALL ON FUNCTION media.guard_reference_pin_lifecycle() FROM PUBLIC;
REVOKE ALL ON media.reference_pin_gate,media.reference_pin FROM PUBLIC,glowloom_app,glowloom_identity,glowloom_creation,glowloom_execution;
GRANT SELECT,INSERT ON media.reference_pin_gate,media.reference_pin TO glowloom_media;
GRANT UPDATE(epoch,closed,close_command_id) ON media.reference_pin_gate TO glowloom_media;
GRANT UPDATE(state,adoption_receipt_sha256,release_receipt_sha256,release_outcome) ON media.reference_pin TO glowloom_media;

-- Creation freezes only locally locked source facts. Actor/session/consumer IDs
-- are opaque identities, never cross-owner foreign keys or authority projections.
CREATE TABLE workspace.frozen_input_handoff (
    handoff_id uuid PRIMARY KEY,
    request_id uuid NOT NULL,
    command_id uuid NOT NULL UNIQUE,
    command_sha256 text NOT NULL CHECK (command_sha256 ~ '^[0-9a-f]{64}$'),
    request_sha256 text NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
    actor_id uuid NOT NULL,
    session_id uuid NOT NULL,
    scope_id uuid NOT NULL,
    project_id uuid NOT NULL,
    source_key text NOT NULL,
    source_generation bigint NOT NULL CHECK (source_generation>0),
    draft_revision bigint NOT NULL CHECK (draft_revision>0),
    snapshot jsonb NOT NULL CHECK (jsonb_typeof(snapshot)='object' AND octet_length(snapshot::text)<=2097152),
    snapshot_sha256 text NOT NULL CHECK (snapshot_sha256 ~ '^[0-9a-f]{64}$'),
    adoption_blocked boolean NOT NULL DEFAULT false,
    cancel_command jsonb,
    cancel_receipt jsonb,
    created_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    UNIQUE(actor_id,project_id,source_key,source_generation),
    UNIQUE(actor_id,request_id),
    CHECK (adoption_blocked = (cancel_command IS NOT NULL)),
    CHECK (cancel_command IS NULL OR jsonb_typeof(cancel_command)='object'),
    CHECK (cancel_receipt IS NULL OR (adoption_blocked AND jsonb_typeof(cancel_receipt)='object'))
);
CREATE FUNCTION workspace.guard_frozen_input_lifecycle() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF (OLD.adoption_blocked AND (NOT NEW.adoption_blocked OR NEW.cancel_command IS DISTINCT FROM OLD.cancel_command))
       OR (OLD.cancel_receipt IS NOT NULL AND NEW.cancel_receipt IS DISTINCT FROM OLD.cancel_receipt) THEN
        RAISE EXCEPTION 'frozen input cancellation is permanent' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER frozen_input_lifecycle BEFORE UPDATE ON workspace.frozen_input_handoff
FOR EACH ROW EXECUTE FUNCTION workspace.guard_frozen_input_lifecycle();
REVOKE ALL ON FUNCTION workspace.guard_frozen_input_lifecycle() FROM PUBLIC;
REVOKE ALL ON workspace.frozen_input_handoff FROM PUBLIC,glowloom_app,glowloom_identity,glowloom_media,glowloom_execution;
GRANT SELECT,INSERT ON workspace.frozen_input_handoff TO glowloom_creation;
GRANT UPDATE(adoption_blocked,cancel_command,cancel_receipt) ON workspace.frozen_input_handoff TO glowloom_creation;

-- Execution acceptance is an immutable local input identity. It does not create
-- an operation, quote, reserve funds or replace the existing confirmation path.
CREATE TABLE operation.frozen_input_acceptance (
    handoff_id uuid PRIMARY KEY,
    command_id uuid NOT NULL UNIQUE,
    command_sha256 text NOT NULL CHECK (command_sha256 ~ '^[0-9a-f]{64}$'),
    snapshot_sha256 text NOT NULL CHECK (snapshot_sha256 ~ '^[0-9a-f]{64}$'),
    snapshot jsonb NOT NULL CHECK (jsonb_typeof(snapshot)='object' AND octet_length(snapshot::text)<=2097152),
    operation_id uuid NOT NULL UNIQUE,
    submission_id uuid NOT NULL UNIQUE,
    receipt jsonb NOT NULL CHECK (jsonb_typeof(receipt)='object'),
    created_at timestamptz NOT NULL DEFAULT statement_timestamp()
);
CREATE TABLE operation.frozen_input_cancellation (
    handoff_id uuid PRIMARY KEY,
    cancel_command_id uuid NOT NULL UNIQUE,
    command_id uuid NOT NULL UNIQUE,
    snapshot_sha256 text NOT NULL CHECK (snapshot_sha256 ~ '^[0-9a-f]{64}$'),
    proof jsonb NOT NULL CHECK (jsonb_typeof(proof)='object'),
    created_at timestamptz NOT NULL DEFAULT statement_timestamp()
);
REVOKE ALL ON operation.frozen_input_acceptance,operation.frozen_input_cancellation FROM PUBLIC,glowloom_app,glowloom_identity,glowloom_creation,glowloom_media;
GRANT SELECT,INSERT ON operation.frozen_input_acceptance,operation.frozen_input_cancellation TO glowloom_execution;

-- Accepted registration remains private until both resource owners acknowledge
-- the exact original command. No user, credential or public preparing state is
-- exposed by these internal responsibility records.
CREATE TABLE identity.registration_preparation (
    operation_id uuid PRIMARY KEY,
    account_id uuid NOT NULL UNIQUE,
    organization_id uuid NOT NULL UNIQUE,
    scope_id uuid NOT NULL UNIQUE,
    email_key text NOT NULL UNIQUE CHECK (email_key=lower(btrim(email_key)) AND octet_length(email_key) BETWEEN 3 AND 254),
    input_digest bytea NOT NULL CHECK (octet_length(input_digest)=32),
    user_snapshot jsonb NOT NULL CHECK (jsonb_typeof(user_snapshot)='object' AND octet_length(user_snapshot::text)<=16384),
    creation_ready boolean NOT NULL DEFAULT false,
    wallet_ready boolean NOT NULL DEFAULT false,
    accepted_at timestamptz NOT NULL,
    activated_at timestamptz,
    CHECK (activated_at IS NULL OR (creation_ready AND wallet_ready AND activated_at>=accepted_at))
);
REVOKE ALL ON identity.registration_preparation FROM PUBLIC,glowloom_app,glowloom_creation,glowloom_media,glowloom_execution;
GRANT SELECT,INSERT ON identity.registration_preparation TO glowloom_identity;
GRANT UPDATE(creation_ready,wallet_ready,activated_at) ON identity.registration_preparation TO glowloom_identity;

-- Creation's original project-create responsibility. A copying project and its
-- initial canvas stay private until Execution preparation and the original
-- zero-epoch project control are both durable. No TTL drops unknown work.
CREATE TABLE workspace.project_preparation (
    preparation_id uuid PRIMARY KEY,
    source_command_id uuid NOT NULL UNIQUE,
    source_sha256 text NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
    input_sha256 text NOT NULL CHECK (input_sha256 ~ '^[0-9a-f]{64}$'),
    command jsonb NOT NULL CHECK (jsonb_typeof(command)='object' AND octet_length(command::text)<=32768),
    actor_id uuid NOT NULL,
    session_id uuid NOT NULL,
    idempotency_key uuid NOT NULL,
    request_id uuid NOT NULL,
    scope_id uuid NOT NULL REFERENCES workspace.creation_scope(id),
    organization_id uuid NOT NULL REFERENCES workspace.organization(id),
    scope_revision bigint NOT NULL CHECK (scope_revision>0),
    project_id uuid NOT NULL UNIQUE,
    canvas_id uuid NOT NULL UNIQUE,
    project_snapshot jsonb NOT NULL CHECK (jsonb_typeof(project_snapshot)='object' AND octet_length(project_snapshot::text)<=1048576),
    initial_coordination_id uuid NOT NULL UNIQUE,
    wallet_receipt jsonb CHECK (wallet_receipt IS NULL OR (jsonb_typeof(wallet_receipt)='object' AND octet_length(wallet_receipt::text)<=16384)),
    activation_receipt jsonb CHECK (activation_receipt IS NULL OR (jsonb_typeof(activation_receipt)='object' AND octet_length(activation_receipt::text)<=1048576)),
    prepared_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    activated_at timestamptz,
    UNIQUE(actor_id,idempotency_key),
    CHECK ((activated_at IS NOT NULL)=(activation_receipt IS NOT NULL)),
    CHECK (activated_at IS NULL OR (wallet_receipt IS NOT NULL AND activated_at>=prepared_at))
);
REVOKE ALL ON workspace.project_preparation FROM PUBLIC,glowloom_app,glowloom_identity,glowloom_media,glowloom_execution;
GRANT SELECT,INSERT ON workspace.project_preparation TO glowloom_creation;
GRANT UPDATE(wallet_receipt,activation_receipt,activated_at) ON workspace.project_preparation TO glowloom_creation;
CREATE FUNCTION workspace.keep_project_preparation() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    IF TG_OP='DELETE' THEN
        RAISE EXCEPTION 'project preparation responsibility is retained' USING ERRCODE='23514';
    END IF;
    IF (to_jsonb(NEW)-ARRAY['wallet_receipt','activation_receipt','activated_at']) IS DISTINCT FROM
       (to_jsonb(OLD)-ARRAY['wallet_receipt','activation_receipt','activated_at']) OR
       (OLD.wallet_receipt IS NOT NULL AND NEW.wallet_receipt IS DISTINCT FROM OLD.wallet_receipt) OR
       (OLD.activation_receipt IS NOT NULL AND NEW.activation_receipt IS DISTINCT FROM OLD.activation_receipt) OR
       (OLD.activated_at IS NOT NULL AND NEW.activated_at IS DISTINCT FROM OLD.activated_at) THEN
        RAISE EXCEPTION 'project preparation identity and decisions are immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END
$$;
CREATE TRIGGER project_preparation_immutable BEFORE UPDATE OR DELETE ON workspace.project_preparation
FOR EACH ROW EXECUTE FUNCTION workspace.keep_project_preparation();

-- Project lifecycle gates retain original responsibility independently of the
-- project's public status and writer generation. Network probes never hold SQL
-- locks; busy work reopens this gate without advancing project authority.
CREATE TABLE workspace.project_closure (
    id uuid PRIMARY KEY,
    actor_id uuid NOT NULL,
    session_id uuid NOT NULL,
    scope_id uuid NOT NULL,
    org_id uuid NOT NULL,
    project_id uuid NOT NULL,
    idempotency_key uuid NOT NULL,
    source_command jsonb NOT NULL CHECK (jsonb_typeof(source_command)='object'),
    source_sha256 text NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
    spec jsonb NOT NULL CHECK (jsonb_typeof(spec)='object'),
    cycle bigint NOT NULL CHECK (cycle>0),
    phase text NOT NULL CHECK (phase IN ('draining','busy','closing','closed','aborting','aborted')),
    ledger_sha256 text NOT NULL CHECK (ledger_sha256 ~ '^[0-9a-f]{64}$'),
    result jsonb CHECK (result IS NULL OR jsonb_typeof(result)='object'),
    accepted_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    completed_at timestamptz,
    UNIQUE(actor_id,idempotency_key),
    CHECK ((phase='closed')=(result IS NOT NULL)),
    CHECK ((phase IN ('closed','aborted'))=(completed_at IS NOT NULL)),
    CHECK (completed_at IS NULL OR completed_at>=accepted_at)
);
REVOKE ALL ON workspace.project_closure FROM PUBLIC,glowloom_app,glowloom_identity,glowloom_media,glowloom_execution;
GRANT SELECT,INSERT ON workspace.project_closure TO glowloom_creation;
GRANT UPDATE(cycle,phase,ledger_sha256,result,completed_at) ON workspace.project_closure TO glowloom_creation;
CREATE FUNCTION workspace.keep_project_closure() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    IF TG_OP='DELETE' THEN
        RAISE EXCEPTION 'project closure responsibility is retained' USING ERRCODE='23514';
    END IF;
    IF (to_jsonb(NEW)-ARRAY['cycle','phase','ledger_sha256','result','completed_at']) IS DISTINCT FROM
       (to_jsonb(OLD)-ARRAY['cycle','phase','ledger_sha256','result','completed_at']) OR
       (OLD.phase IN ('closed','aborted') AND NEW IS DISTINCT FROM OLD) OR
       (NEW.cycle<>OLD.cycle AND NOT (OLD.phase='busy' AND NEW.phase='draining' AND NEW.cycle=OLD.cycle+1)) OR
       (NEW.ledger_sha256<>OLD.ledger_sha256 AND NOT (OLD.phase='busy' AND NEW.phase='draining' AND NEW.cycle=OLD.cycle+1)) OR
       (NEW.phase<>OLD.phase AND NOT (
          (OLD.phase='draining' AND NEW.phase IN ('busy','closing','aborting')) OR
          (OLD.phase='busy' AND NEW.phase='draining') OR
          (OLD.phase='closing' AND NEW.phase IN ('closed','aborting')) OR
          (OLD.phase='aborting' AND NEW.phase='aborted'))) THEN
        RAISE EXCEPTION 'project closure identity and original decisions are immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END
$$;
CREATE TRIGGER project_closure_immutable BEFORE UPDATE OR DELETE ON workspace.project_closure
FOR EACH ROW EXECUTE FUNCTION workspace.keep_project_closure();

CREATE TABLE workspace.project_admission_gate (
    project_id uuid PRIMARY KEY,
    close_id uuid NOT NULL REFERENCES workspace.project_closure(id),
    cycle bigint NOT NULL CHECK (cycle>0),
    blocked boolean NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT statement_timestamp()
);
REVOKE ALL ON workspace.project_admission_gate FROM PUBLIC,glowloom_app,glowloom_identity,glowloom_media,glowloom_execution;
GRANT SELECT,INSERT ON workspace.project_admission_gate TO glowloom_creation;
GRANT UPDATE(close_id,cycle,blocked,updated_at) ON workspace.project_admission_gate TO glowloom_creation;
CREATE FUNCTION workspace.keep_project_admission_gate() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    IF TG_OP='DELETE' THEN
        RAISE EXCEPTION 'project admission gate is retained' USING ERRCODE='23514';
    END IF;
    IF NEW.project_id<>OLD.project_id OR
       (OLD.blocked AND (NEW.close_id<>OLD.close_id OR NEW.cycle<>OLD.cycle)) THEN
        RAISE EXCEPTION 'an unresolved project gate keeps its original close identity' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END
$$;
CREATE TRIGGER project_admission_gate_identity BEFORE UPDATE OR DELETE ON workspace.project_admission_gate
FOR EACH ROW EXECUTE FUNCTION workspace.keep_project_admission_gate();

CREATE TABLE workspace.personal_scope_preparation (
    operation_id uuid PRIMARY KEY,
    account_id uuid NOT NULL UNIQUE,
    organization_id uuid NOT NULL UNIQUE REFERENCES workspace.organization(id),
    scope_id uuid NOT NULL UNIQUE REFERENCES workspace.creation_scope(id),
    input_sha256 text NOT NULL CHECK (input_sha256 ~ '^[0-9a-f]{64}$'),
    prepared_at timestamptz NOT NULL DEFAULT statement_timestamp()
);
REVOKE ALL ON workspace.personal_scope_preparation FROM PUBLIC,glowloom_app,glowloom_identity,glowloom_media,glowloom_execution;
GRANT SELECT,INSERT ON workspace.personal_scope_preparation TO glowloom_creation;

CREATE TABLE billing.registration_wallet_preparation (
    operation_id uuid PRIMARY KEY,
    account_id uuid NOT NULL UNIQUE,
    organization_id uuid NOT NULL UNIQUE,
    scope_id uuid NOT NULL UNIQUE REFERENCES billing.wallet_account(scope_id),
    input_sha256 text NOT NULL CHECK (input_sha256 ~ '^[0-9a-f]{64}$'),
    creation_receipt_sha256 text NOT NULL CHECK (creation_receipt_sha256 ~ '^[0-9a-f]{64}$'),
    prepared_at timestamptz NOT NULL DEFAULT statement_timestamp()
);
REVOKE ALL ON billing.registration_wallet_preparation FROM PUBLIC,glowloom_app,glowloom_identity,glowloom_creation,glowloom_media;
GRANT SELECT,INSERT ON billing.registration_wallet_preparation TO glowloom_execution;
GRANT INSERT(scope_id) ON billing.wallet_account TO glowloom_execution;
GRANT INSERT(project_id,scope_id) ON billing.project_wallet TO glowloom_execution;

-- Identity binds the original credential generation to the exact admitted writer.
-- Historical admissions are immutable; missing bindings cannot be backfilled by
-- a read or inferred from authority epochs.
CREATE TABLE identity.session_command_admission (
    command_id uuid NOT NULL CHECK (command_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    target_writer text NOT NULL CHECK (target_writer IN ('identity','creation','media','execution')),
    account_id uuid NOT NULL CHECK (account_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    session_id uuid NOT NULL CHECK (session_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    credential_epoch bigint NOT NULL CHECK (credential_epoch>0),
    request_sha256 text NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
    authority_set_sha256 text NOT NULL CHECK (authority_set_sha256 ~ '^[0-9a-f]{64}$'),
    PRIMARY KEY(command_id,target_writer)
);
REVOKE ALL ON identity.session_command_admission FROM PUBLIC,glowloom_app,glowloom_creation,glowloom_media,glowloom_execution;
GRANT SELECT,INSERT ON identity.session_command_admission TO glowloom_identity;

-- Media retains the exact original generation command before streaming object I/O.
-- Operation/project/scope IDs are opaque: this ledger has no foreign-owner FK.
CREATE TABLE media.operation_output_receipt (
    command_id uuid PRIMARY KEY,
    operation_id uuid NOT NULL,
    seq_no integer NOT NULL CHECK (seq_no>=0),
    request_sha256 text NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
    payload_sha256 text NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
    payload jsonb NOT NULL CHECK (jsonb_typeof(payload)='object' AND octet_length(payload::text)<=65536),
    asset_id uuid NOT NULL UNIQUE,
    state text NOT NULL CHECK (state IN ('prepared','published','rejected')),
    receipt jsonb CHECK (receipt IS NULL OR (jsonb_typeof(receipt)='object' AND octet_length(receipt::text)<=65536)),
    create_time timestamptz NOT NULL DEFAULT statement_timestamp(),
    update_time timestamptz NOT NULL DEFAULT statement_timestamp(),
    UNIQUE(operation_id,seq_no),
    CHECK ((state='prepared')=(receipt IS NULL))
);
CREATE FUNCTION media.guard_operation_output_receipt() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.state<>'prepared' AND NEW IS DISTINCT FROM OLD THEN
        RAISE EXCEPTION 'published output receipt is immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER operation_output_receipt_lifecycle BEFORE UPDATE ON media.operation_output_receipt
FOR EACH ROW EXECUTE FUNCTION media.guard_operation_output_receipt();
REVOKE ALL ON FUNCTION media.guard_operation_output_receipt() FROM PUBLIC;
REVOKE ALL ON media.operation_output_receipt FROM PUBLIC,glowloom_app,glowloom_identity,glowloom_creation,glowloom_execution;
GRANT SELECT,INSERT ON media.operation_output_receipt TO glowloom_media;
GRANT UPDATE(state,receipt,update_time) ON media.operation_output_receipt TO glowloom_media;

-- Execution persists its original output command before any Media request.
-- Lost ACKs recover this identity; attempts are permanent and never infer absence.
CREATE TABLE operation.output_delivery (
    command_id uuid PRIMARY KEY,
    operation_id uuid NOT NULL REFERENCES operation.operation(id) ON DELETE RESTRICT,
    seq_no integer NOT NULL CHECK (seq_no>=0),
    request_sha256 text NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
    payload_sha256 text NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
    payload jsonb NOT NULL CHECK (jsonb_typeof(payload)='object' AND octet_length(payload::text)<=65536),
    attempted boolean NOT NULL DEFAULT false,
    state text NOT NULL CHECK (state IN ('prepared','adopted','rejected')),
    media_receipt jsonb CHECK (media_receipt IS NULL OR (jsonb_typeof(media_receipt)='object' AND octet_length(media_receipt::text)<=65536)),
    create_time timestamptz NOT NULL DEFAULT statement_timestamp(),
    update_time timestamptz NOT NULL DEFAULT statement_timestamp(),
    UNIQUE(operation_id,seq_no),
    CHECK ((state='prepared')=(media_receipt IS NULL)),
    CHECK (state='prepared' OR attempted)
);
CREATE FUNCTION operation.guard_output_delivery() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF (OLD.attempted AND NOT NEW.attempted) OR (OLD.state<>'prepared' AND NEW IS DISTINCT FROM OLD) THEN
        RAISE EXCEPTION 'output delivery responsibility is monotonic' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER output_delivery_lifecycle BEFORE UPDATE ON operation.output_delivery
FOR EACH ROW EXECUTE FUNCTION operation.guard_output_delivery();
REVOKE ALL ON FUNCTION operation.guard_output_delivery() FROM PUBLIC;
REVOKE ALL ON operation.output_delivery FROM PUBLIC,glowloom_app,glowloom_identity,glowloom_creation,glowloom_media;
GRANT SELECT,INSERT ON operation.output_delivery TO glowloom_execution;
GRANT UPDATE(attempted,state,media_receipt,update_time) ON operation.output_delivery TO glowloom_execution;

-- A successful password check creates a private session responsibility first.
-- Credentials are released only after its exact initial source control has four
-- durable writer ACKs. Original responses remain encrypted and immutable.
CREATE TABLE identity.session_activation (
    request_id uuid PRIMARY KEY CHECK (request_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    account_id uuid NOT NULL CHECK (account_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    session_id uuid NOT NULL UNIQUE CHECK (session_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    coordination_id uuid NOT NULL UNIQUE CHECK (coordination_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    input_digest bytea NOT NULL CHECK (octet_length(input_digest)=32),
    response_key_id text NOT NULL CHECK (octet_length(response_key_id) BETWEEN 1 AND 128 AND response_key_id=btrim(response_key_id)),
    response_ciphertext bytea NOT NULL CHECK (octet_length(response_ciphertext) BETWEEN 1 AND 65536),
    accepted_at timestamptz NOT NULL,
    completed_at timestamptz CHECK (completed_at IS NULL OR completed_at>=accepted_at)
);
REVOKE ALL ON identity.session_activation FROM PUBLIC,glowloom_app,glowloom_creation,glowloom_media,glowloom_execution;
GRANT SELECT,INSERT ON identity.session_activation TO glowloom_identity;
GRANT UPDATE(completed_at) ON identity.session_activation TO glowloom_identity;

-- Execution's project bootstrap participant retains the original Creation proof.
-- Preparation creates no credit lot, balance grant, reservation or spending permit.
-- Opaque Creation IDs have no foreign-owner FK; the private source proof is required.
CREATE TABLE billing.project_budget_preparation (
    execution_command_id uuid PRIMARY KEY CHECK (execution_command_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    preparation_id uuid NOT NULL UNIQUE CHECK (preparation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    source_command_id uuid NOT NULL UNIQUE CHECK (source_command_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    source_sha256 text NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
    input_sha256 text NOT NULL CHECK (input_sha256 ~ '^[0-9a-f]{64}$'),
    request_sha256 text NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
    project_id uuid NOT NULL UNIQUE CHECK (project_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    organization_id uuid NOT NULL CHECK (organization_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    scope_id uuid NOT NULL REFERENCES billing.wallet_account(scope_id) ON DELETE RESTRICT,
    budget_id uuid NOT NULL UNIQUE REFERENCES billing.budget(id) ON DELETE RESTRICT,
    receipt jsonb NOT NULL CHECK (jsonb_typeof(receipt)='object' AND octet_length(receipt::text)<=4096),
    prepared_at timestamptz NOT NULL DEFAULT statement_timestamp()
);
REVOKE ALL ON billing.project_budget_preparation FROM PUBLIC,glowloom_app,glowloom_identity,glowloom_creation,glowloom_media;
GRANT SELECT,INSERT ON billing.project_budget_preparation TO glowloom_execution;

-- Original account qualification responsibilities are private to Identity.
-- Source closure and this immutable binding commit together; later phase writes
-- cannot replace the actor, accepted candidate, input proof or original control.
CREATE TABLE identity.account_qualification_change (
    request_id uuid PRIMARY KEY CHECK (request_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    actor_id uuid NOT NULL CHECK (actor_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    actor_session_id uuid CHECK (actor_session_id IS NULL OR actor_session_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    proof_operation_id uuid CHECK (proof_operation_id IS NULL OR proof_operation_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    target_id uuid NOT NULL CHECK (target_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    action text NOT NULL CHECK (action IN ('password_change','password_reset','email_change','disable','enable','delete','self_delete','revoke_admin','revoke_sessions','grant_admin')),
    input_digest bytea NOT NULL CHECK (octet_length(input_digest)=32),
    proof_digest bytea NOT NULL CHECK (octet_length(proof_digest)=32),
    candidate jsonb NOT NULL CHECK (jsonb_typeof(candidate)='object' AND octet_length(candidate::text)<=65536),
    close_coordination_id uuid NOT NULL UNIQUE CHECK (close_coordination_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    close_expected_epoch bigint NOT NULL CHECK (close_expected_epoch>0),
    mutation_applied_at timestamptz CHECK (mutation_applied_at IS NULL OR mutation_applied_at>=accepted_at),
    creation_receipt jsonb CHECK (creation_receipt IS NULL OR (jsonb_typeof(creation_receipt)='object' AND octet_length(creation_receipt::text)<=1048576)),
    reopen_coordination_id uuid UNIQUE CHECK (reopen_coordination_id IS NULL OR (reopen_coordination_id<>'00000000-0000-0000-0000-000000000000'::uuid AND reopen_coordination_id<>close_coordination_id)),
    reopen_expected_epoch bigint CHECK (reopen_expected_epoch IS NULL OR reopen_expected_epoch=close_expected_epoch+1),
    reopen_purpose text CHECK (reopen_purpose IS NULL OR reopen_purpose IN ('credential_replacement','restore_rejection','qualification_restore')),
    replacement_session_id uuid UNIQUE CHECK (replacement_session_id IS NULL OR replacement_session_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    replacement_coordination_id uuid UNIQUE CHECK (replacement_coordination_id IS NULL OR replacement_coordination_id<>'00000000-0000-0000-0000-000000000000'::uuid),
    response_key_id text CHECK (response_key_id IS NULL OR (octet_length(response_key_id) BETWEEN 1 AND 128 AND response_key_id=btrim(response_key_id))),
    response_ciphertext bytea CHECK (response_ciphertext IS NULL OR octet_length(response_ciphertext) BETWEEN 1 AND 65536),
    outcome text NOT NULL DEFAULT 'pending' CHECK (outcome IN ('pending','accepted','rejected_duties')),
    accepted_at timestamptz NOT NULL,
    completed_at timestamptz,
    CHECK ((action='password_reset' AND actor_session_id IS NULL AND proof_operation_id IS NOT NULL AND actor_id=target_id) OR
           (action='self_delete' AND actor_session_id IS NOT NULL AND proof_operation_id IS NOT NULL AND actor_id=target_id) OR
           (action NOT IN ('password_reset','self_delete') AND actor_session_id IS NOT NULL AND proof_operation_id IS NULL)),
    CHECK (outcome<>'accepted' OR action NOT IN ('password_reset','self_delete') OR mutation_applied_at IS NOT NULL),
    CHECK ((reopen_coordination_id IS NULL)=(reopen_expected_epoch IS NULL) AND (reopen_coordination_id IS NULL)=(reopen_purpose IS NULL)),
    CHECK ((replacement_session_id IS NULL)=(replacement_coordination_id IS NULL)),
    CHECK ((response_key_id IS NULL)=(response_ciphertext IS NULL) AND (response_key_id IS NULL)=(replacement_session_id IS NULL)),
    CHECK ((outcome='pending' AND completed_at IS NULL) OR (outcome<>'pending' AND completed_at IS NOT NULL AND completed_at>=accepted_at)),
    CHECK (outcome<>'rejected_duties' OR (reopen_purpose IS NOT NULL AND reopen_purpose='restore_rejection' AND response_ciphertext IS NULL)),
    CHECK (reopen_purpose IS DISTINCT FROM 'restore_rejection' OR outcome<>'accepted'),
    CHECK (reopen_purpose IS DISTINCT FROM 'credential_replacement' OR action IN ('password_change','password_reset','email_change')),
    CHECK (outcome<>'accepted' OR action NOT IN ('disable','enable','delete','self_delete') OR creation_receipt IS NOT NULL)
);
CREATE INDEX account_qualification_pending_target ON identity.account_qualification_change(target_id) WHERE outcome='pending';
CREATE FUNCTION identity.guard_account_qualification_change() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF (OLD.completed_at IS NOT NULL AND NEW IS DISTINCT FROM OLD)
       OR (OLD.mutation_applied_at IS NOT NULL AND NEW.mutation_applied_at IS DISTINCT FROM OLD.mutation_applied_at)
       OR (OLD.creation_receipt IS NOT NULL AND NEW.creation_receipt IS DISTINCT FROM OLD.creation_receipt)
       OR (OLD.reopen_coordination_id IS NOT NULL AND
           (NEW.reopen_coordination_id IS DISTINCT FROM OLD.reopen_coordination_id OR NEW.reopen_expected_epoch IS DISTINCT FROM OLD.reopen_expected_epoch OR NEW.reopen_purpose IS DISTINCT FROM OLD.reopen_purpose))
       OR (OLD.response_ciphertext IS NOT NULL AND
           (NEW.response_ciphertext IS DISTINCT FROM OLD.response_ciphertext OR NEW.response_key_id IS DISTINCT FROM OLD.response_key_id OR NEW.replacement_session_id IS DISTINCT FROM OLD.replacement_session_id OR NEW.replacement_coordination_id IS DISTINCT FROM OLD.replacement_coordination_id)) THEN
        RAISE EXCEPTION 'account qualification responsibility is immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER account_qualification_lifecycle BEFORE UPDATE ON identity.account_qualification_change
FOR EACH ROW EXECUTE FUNCTION identity.guard_account_qualification_change();
REVOKE ALL ON FUNCTION identity.guard_account_qualification_change() FROM PUBLIC;
REVOKE ALL ON identity.account_qualification_change FROM PUBLIC,glowloom_app,glowloom_creation,glowloom_media,glowloom_execution;
GRANT SELECT,INSERT ON identity.account_qualification_change TO glowloom_identity;
GRANT UPDATE(mutation_applied_at,creation_receipt,reopen_coordination_id,reopen_expected_epoch,reopen_purpose,replacement_session_id,replacement_coordination_id,response_key_id,response_ciphertext,outcome,completed_at) ON identity.account_qualification_change TO glowloom_identity;

-- LastTeamAdministrator is an account-qualification purpose, never a session or
-- general write permit. Creation owns the original team/member snapshot. Explicit
-- successor reads keep old proofs/commands immutable after a safety revocation.
CREATE TABLE identity.team_administrator_qualification (
    id uuid PRIMARY KEY CHECK (id <> '00000000-0000-0000-0000-000000000000'::uuid),
    request_id uuid NOT NULL REFERENCES identity.account_qualification_change(request_id),
    team_id uuid NOT NULL CHECK (team_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    candidate_id uuid NOT NULL CHECK (candidate_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    team_snapshot_sha256 text NOT NULL CHECK (team_snapshot_sha256 ~ '^[0-9a-f]{64}$'),
    qualification_epoch bigint NOT NULL CHECK (qualification_epoch > 0),
    source_coordination_id uuid NOT NULL CHECK (source_coordination_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    target_writer text NOT NULL DEFAULT 'creation' CHECK (target_writer='creation'),
    eligible boolean NOT NULL,
    command_sha256 text NOT NULL CHECK (command_sha256 ~ '^[0-9a-f]{64}$'),
    authority_set_sha256 text NOT NULL CHECK (authority_set_sha256 ~ '^[0-9a-f]{64}$'),
    previous_proof_id uuid REFERENCES identity.team_administrator_qualification(id),
    created_at timestamptz NOT NULL,
    UNIQUE(request_id,team_id,candidate_id,team_snapshot_sha256,qualification_epoch),
    CHECK (previous_proof_id IS NULL OR previous_proof_id<>id)
);
REVOKE ALL ON identity.team_administrator_qualification FROM PUBLIC,glowloom_app,glowloom_creation,glowloom_media,glowloom_execution;
GRANT SELECT,INSERT ON identity.team_administrator_qualification TO glowloom_identity;

-- A durable governance latch serializes only team/member management writes. It
-- never advances a shared scope epoch for preparation or a duties rejection.
-- Only an actual permanent freeze records a scope-close control in phase.
CREATE TABLE workspace.account_governance_preparation (
    request_id uuid PRIMARY KEY CHECK (request_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    account_id uuid NOT NULL CHECK (account_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    action text NOT NULL CHECK (action IN ('disable','enable','delete')),
    input_sha256 text NOT NULL CHECK (input_sha256 ~ '^[0-9a-f]{64}$'),
    identity_close jsonb NOT NULL CHECK (jsonb_typeof(identity_close)='object' AND octet_length(identity_close::text)<=16384),
    snapshot jsonb NOT NULL CHECK (jsonb_typeof(snapshot)='object' AND octet_length(snapshot::text)<=1048576),
    phase jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(phase)='object' AND octet_length(phase::text)<=1048576),
    receipt jsonb CHECK (receipt IS NULL OR (jsonb_typeof(receipt)='object' AND octet_length(receipt::text)<=65536)),
    verdict text NOT NULL DEFAULT 'pending' CHECK (verdict IN ('pending','ready','rejected_duties')),
    accepted_at timestamptz NOT NULL,
    effects_applied_at timestamptz CHECK (effects_applied_at>=accepted_at),
    completed_at timestamptz CHECK (completed_at>=accepted_at AND (effects_applied_at IS NULL OR completed_at>=effects_applied_at)),
    CHECK ((verdict='pending' AND receipt IS NULL AND completed_at IS NULL) OR
           (verdict='ready' AND receipt IS NOT NULL AND effects_applied_at IS NOT NULL AND completed_at IS NOT NULL) OR
           (verdict='rejected_duties' AND action='delete' AND receipt IS NOT NULL AND effects_applied_at IS NULL AND completed_at IS NOT NULL)),
    CHECK (verdict<>'rejected_duties' OR NOT (phase ? 'freeze_controls'))
);
CREATE INDEX account_governance_pending ON workspace.account_governance_preparation(account_id) WHERE verdict='pending';
CREATE FUNCTION workspace.guard_account_governance_preparation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF (OLD.completed_at IS NOT NULL AND NEW IS DISTINCT FROM OLD)
       OR (NEW.request_id IS DISTINCT FROM OLD.request_id OR NEW.account_id IS DISTINCT FROM OLD.account_id OR
           NEW.action IS DISTINCT FROM OLD.action OR NEW.input_sha256 IS DISTINCT FROM OLD.input_sha256 OR
           NEW.identity_close IS DISTINCT FROM OLD.identity_close OR NEW.snapshot IS DISTINCT FROM OLD.snapshot OR
           NEW.accepted_at IS DISTINCT FROM OLD.accepted_at)
       OR (OLD.effects_applied_at IS NOT NULL AND NEW.effects_applied_at IS DISTINCT FROM OLD.effects_applied_at)
       OR NOT (NEW.phase @> OLD.phase) THEN
        RAISE EXCEPTION 'account governance responsibility is immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER account_governance_lifecycle BEFORE UPDATE ON workspace.account_governance_preparation
FOR EACH ROW EXECUTE FUNCTION workspace.guard_account_governance_preparation();
REVOKE ALL ON FUNCTION workspace.guard_account_governance_preparation() FROM PUBLIC;
REVOKE ALL ON workspace.account_governance_preparation FROM PUBLIC,glowloom_app,glowloom_identity,glowloom_media,glowloom_execution;
GRANT SELECT,INSERT ON workspace.account_governance_preparation TO glowloom_creation;
GRANT UPDATE(phase,receipt,verdict,effects_applied_at,completed_at) ON workspace.account_governance_preparation TO glowloom_creation;

-- Same-owner management latch. No table privilege is exported to legacy callers.
-- Statement locking precedes management row locks; the VOLATILE row check uses
-- a fresh snapshot after any wait, including a statement begun before Prepare.
CREATE FUNCTION workspace.lock_team_governance_management() RETURNS trigger
LANGUAGE plpgsql VOLATILE SET search_path=pg_catalog AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(hashtextextended('creation.team_management',0));
    RETURN NULL;
END $$;
REVOKE ALL ON FUNCTION workspace.lock_team_governance_management() FROM PUBLIC;
CREATE FUNCTION workspace.guard_team_governance_management() RETURNS trigger
LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,workspace AS $$
DECLARE
    team_key uuid;
    account_key uuid;
    previous_account_key uuid;
    marker text;
    creator_capability boolean;
    pending record;
    snapshot_team jsonb;
    own_effect boolean;
BEGIN
    IF TG_TABLE_NAME='team' THEN
        IF TG_OP='DELETE' THEN
            team_key:=OLD.id;
            account_key:=OLD.created_by;
        ELSE
            team_key:=NEW.id;
            account_key:=NEW.created_by;
            IF TG_OP='UPDATE' THEN previous_account_key:=OLD.created_by; END IF;
        END IF;
    ELSE
        IF TG_OP='DELETE' THEN
            team_key:=OLD.team_id;
            account_key:=OLD.user_id;
        ELSE
            team_key:=NEW.team_id;
            account_key:=NEW.user_id;
            IF TG_OP='UPDATE' THEN previous_account_key:=OLD.user_id; END IF;
        END IF;
    END IF;
    marker:=current_setting('glowloom.creation_governance_request',true);
    creator_capability:=pg_has_role(session_user,'glowloom_creation','MEMBER') AND
        NOT pg_has_role(session_user,'glowloom_identity','MEMBER') AND
        NOT pg_has_role(session_user,'glowloom_media','MEMBER') AND
        NOT pg_has_role(session_user,'glowloom_execution','MEMBER');
    FOR pending IN
        SELECT p.request_id,p.account_id,p.action,p.snapshot
        FROM workspace.account_governance_preparation p
        WHERE p.verdict='pending' AND
          (p.account_id=account_key OR p.account_id=previous_account_key OR
           EXISTS(SELECT 1 FROM jsonb_array_elements(p.snapshot->'Teams') t WHERE t->>'ID'=team_key::text))
        ORDER BY p.request_id
    LOOP
        SELECT t INTO snapshot_team FROM jsonb_array_elements(pending.snapshot->'Teams') t WHERE t->>'ID'=team_key::text;
        own_effect:=false;
        IF creator_capability AND marker=pending.request_id::text AND snapshot_team IS NOT NULL AND TG_OP='UPDATE' THEN
            IF TG_TABLE_NAME='team' THEN
                own_effect:=NEW.id=OLD.id AND NEW.created_by=OLD.created_by AND
                    OLD.revision=(snapshot_team->>'Revision')::bigint AND
                    OLD.auth_epoch=(snapshot_team->>'AuthEpoch')::bigint;
            ELSE
                own_effect:=pending.action='delete' AND NEW.team_id=OLD.team_id AND
                    NEW.user_id=OLD.user_id AND NEW.user_id=pending.account_id AND NEW.status='removed' AND
                    OLD.revision=(snapshot_team->'Target'->>'Revision')::bigint AND
                    OLD.auth_epoch=(snapshot_team->'Target'->>'AuthEpoch')::bigint;
            END IF;
        END IF;
        IF NOT COALESCE(own_effect,false) THEN
            RAISE EXCEPTION 'team governance responsibility pending' USING ERRCODE='55P03';
        END IF;
        snapshot_team:=NULL;
    END LOOP;
    IF TG_OP='DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION workspace.guard_team_governance_management() FROM PUBLIC;
CREATE TRIGGER team_governance_management_lock BEFORE INSERT OR UPDATE OR DELETE ON workspace.team
FOR EACH STATEMENT EXECUTE FUNCTION workspace.lock_team_governance_management();
CREATE TRIGGER team_governance_management_guard BEFORE INSERT OR UPDATE OR DELETE ON workspace.team
FOR EACH ROW EXECUTE FUNCTION workspace.guard_team_governance_management();
CREATE TRIGGER team_member_governance_management_lock BEFORE INSERT OR UPDATE OR DELETE ON workspace.team_member
FOR EACH STATEMENT EXECUTE FUNCTION workspace.lock_team_governance_management();
CREATE TRIGGER team_member_governance_management_guard BEFORE INSERT OR UPDATE OR DELETE ON workspace.team_member
FOR EACH ROW EXECUTE FUNCTION workspace.guard_team_governance_management();

-- Original provider-result handoff. No object profile, bucket, key or credential
-- belongs to Execution. Source bytes are bounded immutable transfer chunks only;
-- they are deleted solely after the exact durable Media present ACK is adopted.
CREATE TABLE operation.provider_artifact_delivery (
    delivery_id uuid PRIMARY KEY,
    media_command_id uuid NOT NULL UNIQUE,
    provider_call_id uuid NOT NULL,
    provider_call_created_at timestamptz NOT NULL,
    operation_id uuid NOT NULL REFERENCES operation.operation(id) ON DELETE RESTRICT,
    project_id uuid NOT NULL,
    call_action text NOT NULL CHECK (call_action IN ('submit','query')),
    call_attempt integer NOT NULL CHECK (call_attempt>0),
    request_key text NOT NULL CHECK (octet_length(request_key) BETWEEN 1 AND 256),
    artifact_seq integer NOT NULL CHECK (artifact_seq>=0),
    byte_size bigint NOT NULL CHECK (byte_size BETWEEN 1 AND 536870912),
    sha256 text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    manifest_sha256 text NOT NULL CHECK (manifest_sha256 ~ '^[0-9a-f]{64}$'),
    request_sha256 text NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
    request jsonb NOT NULL CHECK (jsonb_typeof(request)='object' AND octet_length(request::text)<=65536),
    chunk_count integer NOT NULL CHECK (chunk_count BETWEEN 1 AND 2048 AND chunk_count=(byte_size+262143)/262144),
    state text NOT NULL DEFAULT 'collecting' CHECK (state IN ('collecting','prepared','present')),
    attempted boolean NOT NULL DEFAULT false,
    sealed_at timestamptz,
    media_receipt jsonb CHECK (media_receipt IS NULL OR (jsonb_typeof(media_receipt)='object' AND octet_length(media_receipt::text)<=65536)),
    create_time timestamptz NOT NULL DEFAULT statement_timestamp(),
    update_time timestamptz NOT NULL DEFAULT statement_timestamp(),
    FOREIGN KEY(provider_call_id,provider_call_created_at) REFERENCES operation.provider_call(id,create_time) ON DELETE RESTRICT,
    UNIQUE(provider_call_id,provider_call_created_at,artifact_seq),
    CHECK ((state='collecting')=(sealed_at IS NULL)),
    CHECK ((state='present')=(media_receipt IS NOT NULL)),
    CHECK (NOT attempted OR state<>'collecting')
);
CREATE FUNCTION operation.guard_provider_artifact_delivery() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.state='present' AND NEW IS DISTINCT FROM OLD
       OR OLD.attempted AND NOT NEW.attempted
       OR OLD.state<>'collecting' AND NEW.state='collecting'
       OR OLD.sealed_at IS NOT NULL AND NEW.sealed_at IS DISTINCT FROM OLD.sealed_at THEN
        RAISE EXCEPTION 'provider artifact responsibility is monotonic' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER provider_artifact_delivery_lifecycle BEFORE UPDATE ON operation.provider_artifact_delivery
FOR EACH ROW EXECUTE FUNCTION operation.guard_provider_artifact_delivery();
REVOKE ALL ON FUNCTION operation.guard_provider_artifact_delivery() FROM PUBLIC;

CREATE TABLE operation.provider_artifact_chunk (
    delivery_id uuid NOT NULL REFERENCES operation.provider_artifact_delivery(delivery_id) ON DELETE RESTRICT,
    seq_no integer NOT NULL CHECK (seq_no>=0),
    data bytea NOT NULL CHECK (octet_length(data) BETWEEN 1 AND 262144),
    PRIMARY KEY(delivery_id,seq_no)
);
CREATE FUNCTION operation.guard_provider_artifact_chunk() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE parent operation.provider_artifact_delivery; expected_size bigint;
BEGIN
    IF TG_OP='DELETE' THEN
        SELECT * INTO parent FROM operation.provider_artifact_delivery WHERE delivery_id=OLD.delivery_id FOR SHARE;
        IF parent.state IS DISTINCT FROM 'present' OR parent.media_receipt IS NULL THEN
            RAISE EXCEPTION 'durable Media ACK required before source chunk removal' USING ERRCODE='23514';
        END IF;
        RETURN OLD;
    END IF;
    SELECT * INTO parent FROM operation.provider_artifact_delivery WHERE delivery_id=NEW.delivery_id FOR SHARE;
    IF parent.state IS DISTINCT FROM 'collecting' OR NEW.seq_no>=parent.chunk_count THEN
        RAISE EXCEPTION 'artifact source is sealed or chunk is outside original manifest' USING ERRCODE='23514';
    END IF;
    expected_size:=least(262144::bigint,parent.byte_size-NEW.seq_no::bigint*262144);
    IF octet_length(NEW.data)<>expected_size THEN
        RAISE EXCEPTION 'artifact source chunk length mismatch' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER provider_artifact_chunk_insert BEFORE INSERT ON operation.provider_artifact_chunk
FOR EACH ROW EXECUTE FUNCTION operation.guard_provider_artifact_chunk();
CREATE TRIGGER provider_artifact_chunk_delete BEFORE DELETE ON operation.provider_artifact_chunk
FOR EACH ROW EXECUTE FUNCTION operation.guard_provider_artifact_chunk();
REVOKE ALL ON FUNCTION operation.guard_provider_artifact_chunk() FROM PUBLIC;
REVOKE ALL ON operation.provider_artifact_delivery,operation.provider_artifact_chunk FROM PUBLIC,glowloom_app,glowloom_identity,glowloom_creation,glowloom_media;
GRANT SELECT,INSERT ON operation.provider_artifact_delivery,operation.provider_artifact_chunk TO glowloom_execution;
GRANT UPDATE(state,attempted,sealed_at,media_receipt,update_time) ON operation.provider_artifact_delivery TO glowloom_execution;
GRANT DELETE ON operation.provider_artifact_chunk TO glowloom_execution;

-- Media's opaque receipt owns the physical staged artifact. The original provider
-- call identity is opaque foreign-owner evidence, never a cross-owner foreign key.
CREATE TABLE media.provider_artifact_receipt (
    command_id uuid PRIMARY KEY,
    delivery_id uuid NOT NULL UNIQUE,
    provider_call_id uuid NOT NULL,
    provider_call_created_at timestamptz NOT NULL,
    operation_id uuid NOT NULL,
    project_id uuid NOT NULL,
    call_action text NOT NULL CHECK (call_action IN ('submit','query')),
    call_attempt integer NOT NULL CHECK (call_attempt>0),
    request_key text NOT NULL CHECK (octet_length(request_key) BETWEEN 1 AND 256),
    artifact_seq integer NOT NULL CHECK (artifact_seq>=0),
    byte_size bigint NOT NULL CHECK (byte_size BETWEEN 1 AND 536870912),
    sha256 text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    manifest_sha256 text NOT NULL CHECK (manifest_sha256 ~ '^[0-9a-f]{64}$'),
    request_sha256 text NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
    request jsonb NOT NULL CHECK (jsonb_typeof(request)='object' AND octet_length(request::text)<=65536),
    storage_work_intent_id uuid NOT NULL REFERENCES media.storage_work_intent(id) ON DELETE RESTRICT,
    location_id uuid REFERENCES media.object_location(id) ON DELETE RESTRICT,
    state text NOT NULL DEFAULT 'prepared' CHECK (state IN ('prepared','present')),
    receipt jsonb CHECK (receipt IS NULL OR (jsonb_typeof(receipt)='object' AND octet_length(receipt::text)<=65536)),
    create_time timestamptz NOT NULL DEFAULT statement_timestamp(),
    update_time timestamptz NOT NULL DEFAULT statement_timestamp(),
    UNIQUE(provider_call_id,provider_call_created_at,artifact_seq),
    CHECK ((state='present')=(receipt IS NOT NULL)),
    CHECK (state<>'present' OR location_id IS NOT NULL)
);
CREATE FUNCTION media.guard_provider_artifact_receipt() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.state='present' AND NEW IS DISTINCT FROM OLD
       OR OLD.location_id IS NOT NULL AND NEW.location_id IS DISTINCT FROM OLD.location_id THEN
        RAISE EXCEPTION 'provider artifact receipt is immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER provider_artifact_receipt_lifecycle BEFORE UPDATE ON media.provider_artifact_receipt
FOR EACH ROW EXECUTE FUNCTION media.guard_provider_artifact_receipt();
REVOKE ALL ON FUNCTION media.guard_provider_artifact_receipt() FROM PUBLIC;
REVOKE ALL ON media.provider_artifact_receipt FROM PUBLIC,glowloom_app,glowloom_identity,glowloom_creation,glowloom_execution;
GRANT SELECT,INSERT ON media.provider_artifact_receipt TO glowloom_media;
GRANT UPDATE(state,location_id,receipt,update_time) ON media.provider_artifact_receipt TO glowloom_media;

-- Identity owns announcement revisions, immutable inbox snapshots and display
-- receipts. Applications never create these structures at startup.
CREATE TABLE identity.announcement (
    id uuid PRIMARY KEY,
    version bigint NOT NULL CHECK (version > 0),
    draft jsonb NOT NULL CHECK (jsonb_typeof(draft) = 'object'),
    published_revision bigint,
    enabled boolean NOT NULL DEFAULT false,
    deleted_at timestamptz,
    created_by uuid NOT NULL REFERENCES identity."user"(id),
    updated_by uuid NOT NULL REFERENCES identity."user"(id),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CHECK (published_revision IS NULL OR published_revision > 0),
    CHECK (deleted_at IS NULL OR (published_revision IS NULL AND NOT enabled))
);
CREATE TABLE identity.announcement_revision (
    announcement_id uuid NOT NULL REFERENCES identity.announcement(id),
    revision bigint NOT NULL CHECK (revision > 0),
    content jsonb NOT NULL CHECK (jsonb_typeof(content) = 'object'),
    published_at timestamptz NOT NULL,
    PRIMARY KEY (announcement_id, revision)
);
ALTER TABLE identity.announcement ADD CONSTRAINT announcement_published_revision_fk
    FOREIGN KEY (id,published_revision) REFERENCES identity.announcement_revision(announcement_id,revision);
CREATE INDEX announcement_updated_idx ON identity.announcement(updated_at DESC,id DESC);
CREATE TABLE identity.notification_mailbox (
    account_id uuid PRIMARY KEY REFERENCES identity."user"(id),
    data_epoch uuid NOT NULL,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    last_seq bigint NOT NULL DEFAULT 0 CHECK (last_seq >= 0)
);
CREATE TABLE identity.notification_inbox (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES identity.notification_mailbox(account_id),
    inbox_seq bigint CHECK (inbox_seq > 0),
    announcement_id uuid,
    revision bigint,
    source_kind text NOT NULL DEFAULT 'announcement' CHECK (source_kind IN ('announcement','operation')),
    source_id uuid,
    source_event_id uuid,
    category text NOT NULL CHECK (category IN ('system','task','collaboration','account')),
    content jsonb NOT NULL CHECK (jsonb_typeof(content) = 'object'),
    occurred_at timestamptz NOT NULL,
    read_at timestamptz,
    cancelled_at timestamptz,
    FOREIGN KEY (announcement_id,revision) REFERENCES identity.announcement_revision(announcement_id,revision),
    UNIQUE (account_id,inbox_seq),
    UNIQUE (account_id,announcement_id,revision),
    CHECK ((source_kind='announcement' AND announcement_id IS NOT NULL AND revision IS NOT NULL AND source_id IS NULL AND source_event_id IS NULL)
        OR (source_kind='operation' AND category='task' AND inbox_seq IS NOT NULL AND announcement_id IS NULL AND revision IS NULL AND source_id IS NOT NULL AND source_event_id IS NOT NULL)),
    CHECK (inbox_seq IS NOT NULL OR (source_kind='announcement' AND read_at IS NULL)),
    CHECK (cancelled_at IS NULL OR (source_kind='announcement' AND inbox_seq IS NULL))
);
CREATE UNIQUE INDEX notification_inbox_announcement_delivery_idx ON identity.notification_inbox(account_id,announcement_id) WHERE announcement_id IS NOT NULL AND inbox_seq IS NOT NULL;
CREATE INDEX notification_inbox_pending_idx ON identity.notification_inbox(account_id,occurred_at,id) WHERE inbox_seq IS NULL AND cancelled_at IS NULL;
CREATE UNIQUE INDEX notification_inbox_source_event_idx ON identity.notification_inbox(account_id,source_event_id) WHERE source_event_id IS NOT NULL;
CREATE INDEX notification_inbox_unread_idx ON identity.notification_inbox(account_id,inbox_seq DESC) WHERE read_at IS NULL;
CREATE INDEX notification_inbox_category_idx ON identity.notification_inbox(account_id,category,inbox_seq DESC);
CREATE TABLE identity.notification_display_receipt (
    account_id uuid NOT NULL REFERENCES identity."user"(id),
    announcement_id uuid NOT NULL,
    revision bigint NOT NULL,
    scope_key text NOT NULL CHECK (scope_key = 'once' OR scope_key ~ '^daily:[0-9]{4}-[0-9]{2}-[0-9]{2}$' OR scope_key ~ '^login:[0-9a-f-]{36}$'),
    shown_at timestamptz,
    dismissed_at timestamptz,
    PRIMARY KEY (account_id,announcement_id,scope_key),
    FOREIGN KEY (announcement_id,revision) REFERENCES identity.announcement_revision(announcement_id,revision)
);
CREATE TABLE identity.notification_command (
    actor_id uuid NOT NULL REFERENCES identity."user"(id),
    command_id uuid NOT NULL,
    fingerprint text NOT NULL CHECK (length(fingerprint)=64),
    response jsonb NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY(actor_id,command_id)
);
CREATE TABLE identity.notification_audit (
    id uuid PRIMARY KEY,
    announcement_id uuid NOT NULL REFERENCES identity.announcement(id),
    actor_id uuid NOT NULL REFERENCES identity."user"(id),
    command_id uuid NOT NULL,
    action text NOT NULL CHECK (action IN ('created','saved','published','enabled','disabled','deleted')),
    version bigint NOT NULL CHECK (version>0),
    created_at timestamptz NOT NULL,
    UNIQUE(actor_id,command_id)
);
CREATE INDEX notification_audit_announcement_idx ON identity.notification_audit(announcement_id,created_at DESC,id DESC);
REVOKE ALL ON identity.announcement,identity.announcement_revision,identity.notification_mailbox,identity.notification_inbox,identity.notification_display_receipt,identity.notification_command,identity.notification_audit FROM PUBLIC,glowloom_creation,glowloom_media,glowloom_execution;
-- The existing combined runtime retains its explicit legacy authority. This is
-- not a grant to another owner or a claim that the team audience is remote.
GRANT SELECT,INSERT,UPDATE ON identity.announcement,identity.notification_mailbox,identity.notification_inbox,identity.notification_display_receipt TO glowloom_identity,glowloom_app;
GRANT SELECT,INSERT ON identity.announcement_revision,identity.notification_command,identity.notification_audit TO glowloom_identity,glowloom_app;

-- Rebuilt AI business boundary. These facts do not depend on operation/billing.
CREATE SCHEMA ai;
REVOKE ALL ON SCHEMA ai FROM PUBLIC;
CREATE TABLE ai.account (
 scope_id uuid PRIMARY KEY REFERENCES workspace.creation_scope(id),
 available bigint NOT NULL DEFAULT 0 CHECK(available>=0),
 held bigint NOT NULL DEFAULT 0 CHECK(held>=0),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0)
);
CREATE TABLE ai.model (
 id uuid PRIMARY KEY,name text NOT NULL CHECK(length(name) BETWEEN 1 AND 160),
 document jsonb NOT NULL CHECK(jsonb_typeof(document)='object'),
 enabled boolean NOT NULL DEFAULT false,draft boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),CHECK(NOT (enabled AND draft))
);
CREATE UNIQUE INDEX ai_model_published_name ON ai.model(name,(document->>'protocol')) WHERE enabled;
CREATE TABLE ai.model_sale (
 model_id uuid NOT NULL REFERENCES ai.model(id),version bigint NOT NULL CHECK(version>0),
 price jsonb NOT NULL CHECK(jsonb_typeof(price)='object'),actor_id uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(model_id,version)
);
CREATE VIEW ai.model_current AS
SELECT m.id,m.name,m.enabled,m.draft,
       CASE WHEN sale.version IS NULL THEN m.document
            ELSE m.document || jsonb_build_object('price',sale.price,'sale_version',sale.version)
       END AS document
FROM ai.model m
LEFT JOIN LATERAL (SELECT version,price FROM ai.model_sale WHERE model_id=m.id ORDER BY version DESC LIMIT 1) sale ON true;
GRANT SELECT,INSERT ON ai.model_sale TO glowloom_app,glowloom_execution;
GRANT SELECT ON ai.model_current TO glowloom_app,glowloom_execution;

CREATE TABLE ai.quota (
 name text PRIMARY KEY CHECK(length(name) BETWEEN 1 AND 160),
 concurrency integer NOT NULL CHECK(concurrency BETWEEN 1 AND 1000),
 requests_per_minute integer NOT NULL CHECK(requests_per_minute>0),
 cooldown_until timestamptz
);
CREATE TABLE ai.credential (
 id uuid PRIMARY KEY,supplier_id uuid NOT NULL,key_id text NOT NULL,
 ciphertext bytea NOT NULL CHECK(octet_length(ciphertext) BETWEEN 1 AND 16384),
 state text NOT NULL DEFAULT 'disabled' CHECK(state IN ('active','draining','disabled','revoked'))
);
CREATE TABLE ai.route (
 id uuid PRIMARY KEY,model_id uuid NOT NULL REFERENCES ai.model(id),
 credential_id uuid NOT NULL REFERENCES ai.credential(id),
 quota_group text NOT NULL REFERENCES ai.quota(name),priority integer NOT NULL CHECK(priority>=0),
 document jsonb NOT NULL CHECK(jsonb_typeof(document)='object'),enabled boolean NOT NULL DEFAULT false,
 deleted_at timestamptz,deleted_by uuid,
 CHECK ((deleted_at IS NULL)=(deleted_by IS NULL)),CHECK (deleted_at IS NULL OR NOT enabled)
);
CREATE INDEX ai_route_model ON ai.route(model_id,priority);
CREATE TABLE ai.route_purchase (
 route_id uuid NOT NULL REFERENCES ai.route(id),version bigint NOT NULL CHECK(version>0),
 price jsonb NOT NULL CHECK(jsonb_typeof(price)='object'),actor_id uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(route_id,version)
);
CREATE VIEW ai.route_current AS
SELECT r.id,r.model_id,r.credential_id,r.quota_group,r.priority,r.enabled,r.deleted_at,
       CASE WHEN purchase.version IS NULL THEN r.document
            ELSE r.document || jsonb_build_object('purchase',purchase.price,'purchase_version',purchase.version)
       END AS document
FROM ai.route r
LEFT JOIN LATERAL (SELECT version,price FROM ai.route_purchase WHERE route_id=r.id ORDER BY version DESC LIMIT 1) purchase ON true;
GRANT SELECT,INSERT ON ai.route_purchase TO glowloom_app,glowloom_execution;
GRANT SELECT ON ai.route_current TO glowloom_app,glowloom_execution;


-- Project consumers discover only published cloud products with a usable route.
CREATE VIEW ai.available_model AS
SELECT m.id,m.name,m.document,route.id AS route_id,route.credential_id,
       CASE WHEN m.document->>'protocol' IN ('media','ark_embedding') THEN m.document->>'capability'
            ELSE 'text.generate' END AS capability
FROM ai.model_current m
JOIN LATERAL (SELECT r.id,r.credential_id FROM ai.route r JOIN ai.credential k ON k.id=r.credential_id
              WHERE r.model_id=m.id AND r.enabled AND k.state='active'
              ORDER BY r.priority,r.id LIMIT 1) route ON true
WHERE m.enabled AND m.document->>'protocol'<>'local';
GRANT SELECT ON ai.available_model TO glowloom_app,glowloom_execution;

CREATE TABLE ai.request (
 id uuid PRIMARY KEY,scope_id uuid NOT NULL REFERENCES ai.account(scope_id),actor_id uuid NOT NULL,
 project_id uuid,run_id uuid,origin text NOT NULL,
 idempotency_key text NOT NULL CHECK(length(idempotency_key) BETWEEN 1 AND 160),
 input_hash text NOT NULL CHECK(length(input_hash)=64),model_id uuid NOT NULL REFERENCES ai.model(id),
 contract jsonb NOT NULL,stream boolean NOT NULL,deadline timestamptz NOT NULL,
 state text NOT NULL CHECK(state IN ('sending','pending','unknown','received','settled','released')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(scope_id,idempotency_key),UNIQUE(project_id,id)
);
CREATE TABLE ai.hold (
 request_id uuid PRIMARY KEY REFERENCES ai.request(id),amount bigint NOT NULL CHECK(amount>=0),
 charged bigint NOT NULL DEFAULT 0 CHECK(charged>=0),released bigint NOT NULL DEFAULT 0 CHECK(released>=0),
 closed boolean NOT NULL DEFAULT false,
 CHECK(charged+released<=amount),CHECK(NOT closed OR charged+released=amount)
);
CREATE TABLE ai.attempt (
 id uuid PRIMARY KEY,request_id uuid NOT NULL REFERENCES ai.request(id),number integer NOT NULL CHECK(number BETWEEN 1 AND 2),
 route_id uuid NOT NULL REFERENCES ai.route(id),credential_id uuid NOT NULL REFERENCES ai.credential(id),
 quota_group text NOT NULL REFERENCES ai.quota(name),contract jsonb NOT NULL,
 state text NOT NULL CHECK(state IN ('ready','sending','pending','unknown','not_sent','received','reconciled')),
 receipt jsonb,digest text,output_duration_evidence jsonb CHECK (output_duration_evidence IS NULL OR jsonb_typeof(output_duration_evidence)='object'),started_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(request_id,number),CHECK((receipt IS NULL)=(digest IS NULL))
);
CREATE INDEX ai_attempt_capacity ON ai.attempt(quota_group,state,started_at);
CREATE TABLE ai.money_event (
 id uuid PRIMARY KEY,scope_id uuid NOT NULL REFERENCES ai.account(scope_id),
 actor_id uuid NOT NULL, source_reference text NOT NULL DEFAULT '',
 request_id uuid REFERENCES ai.request(id),event_key text NOT NULL,kind text NOT NULL CHECK(kind IN ('grant','freeze','settle','release')),
 available_delta bigint NOT NULL,held_delta bigint NOT NULL,spent_delta bigint NOT NULL CHECK(spent_delta>=0),
 source_hash text NOT NULL CHECK(length(source_hash)=64),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(scope_id,event_key),CHECK(request_id IS NOT NULL OR kind='grant')
);
CREATE INDEX ai_money_request ON ai.money_event(request_id);
CREATE FUNCTION ai.immutable_fact() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'AI immutable fact cannot be rewritten'; END $$;
CREATE TRIGGER ai_money_immutable BEFORE UPDATE OR DELETE ON ai.money_event FOR EACH ROW EXECUTE FUNCTION ai.immutable_fact();
CREATE TABLE ai.credit_grant_order (
 id uuid PRIMARY KEY,source_key text NOT NULL UNIQUE CHECK(length(source_key) BETWEEN 1 AND 120),
 scope_id uuid NOT NULL REFERENCES workspace.creation_scope(id),proposer_id uuid NOT NULL,
 credits bigint NOT NULL CHECK(credits BETWEEN 1 AND 1000000000),reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 500),
 recipient_kind text NOT NULL CHECK(recipient_kind IN ('personal','team')),recipient_label text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(id,proposer_id)
);
CREATE INDEX ai_credit_grant_order_recent ON ai.credit_grant_order(created_at DESC,id DESC);
CREATE TABLE ai.credit_grant_review (
 order_id uuid PRIMARY KEY,proposer_id uuid NOT NULL,reviewer_id uuid NOT NULL CHECK (reviewer_id<>proposer_id),
 approved boolean NOT NULL,event_id uuid UNIQUE REFERENCES ai.money_event(id),
 reviewed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(order_id,proposer_id) REFERENCES ai.credit_grant_order(id,proposer_id),
 CHECK (approved=(event_id IS NOT NULL))
);
CREATE TRIGGER ai_credit_grant_order_immutable BEFORE UPDATE OR DELETE ON ai.credit_grant_order FOR EACH ROW EXECUTE FUNCTION ai.immutable_fact();
CREATE TRIGGER ai_credit_grant_review_immutable BEFORE UPDATE OR DELETE ON ai.credit_grant_review FOR EACH ROW EXECUTE FUNCTION ai.immutable_fact();
GRANT SELECT,INSERT ON ai.credit_grant_order,ai.credit_grant_review TO glowloom_app;

CREATE FUNCTION ai.immutable_document() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.document IS DISTINCT FROM NEW.document THEN RAISE EXCEPTION 'AI published document is immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER ai_model_document BEFORE UPDATE ON ai.model FOR EACH ROW EXECUTE FUNCTION ai.immutable_document();
CREATE TRIGGER ai_route_document BEFORE UPDATE ON ai.route FOR EACH ROW EXECUTE FUNCTION ai.immutable_document();
GRANT USAGE ON SCHEMA ai TO glowloom_app,glowloom_execution;
GRANT SELECT,INSERT,UPDATE ON ai.account,ai.model,ai.quota,ai.credential,ai.route,ai.request,ai.hold,ai.attempt TO glowloom_app,glowloom_execution;
GRANT SELECT,INSERT ON ai.money_event TO glowloom_app,glowloom_execution;
CREATE TABLE ai.budget (
 scope_id uuid NOT NULL REFERENCES ai.account(scope_id),id uuid NOT NULL,
 limit_credits bigint NOT NULL CHECK(limit_credits>=0),held bigint NOT NULL DEFAULT 0 CHECK(held>=0),
 spent bigint NOT NULL DEFAULT 0 CHECK(spent>=0),PRIMARY KEY(scope_id,id),CHECK(held+spent<=limit_credits)
);
GRANT SELECT,INSERT,UPDATE ON ai.budget TO glowloom_app,glowloom_execution;
-- Successful authentication evidence contains no key or upstream response.
CREATE TABLE ai.provider_key_verification (
    fingerprint text PRIMARY KEY CHECK (fingerprint ~ '^[0-9a-f]{64}$'),
    provider text NOT NULL,
    base_url text NOT NULL,
    verified_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
GRANT SELECT,INSERT ON ai.provider_key_verification TO glowloom_app,glowloom_execution;

-- A reviewed model and route may be prepared without credentials or live publication.
CREATE TABLE ai.onboarding (
 id uuid PRIMARY KEY,
 key_check_fingerprint text REFERENCES ai.provider_key_verification(fingerprint),
 model_id uuid NOT NULL REFERENCES ai.model(id),
 previous_model_id uuid REFERENCES ai.model(id),
 replaces_route_id uuid REFERENCES ai.route(id),
 document jsonb NOT NULL CHECK(jsonb_typeof(document)='object'),
 state text NOT NULL DEFAULT 'prepared' CHECK(state IN ('prepared','verifying','active','failed','stale','rolled_back')),
 actor jsonb CHECK(jsonb_typeof(actor)='object'),
 confirmed_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 deleted_at timestamptz,deleted_by uuid,
 CHECK ((deleted_at IS NULL)=(deleted_by IS NULL)),CHECK (deleted_at IS NULL OR state<>'verifying'),
 CHECK((actor IS NULL)=(confirmed_at IS NULL))
);
CREATE INDEX ai_onboarding_model ON ai.onboarding(model_id,created_at);
CREATE TRIGGER ai_onboarding_document BEFORE UPDATE ON ai.onboarding FOR EACH ROW EXECUTE FUNCTION ai.immutable_document();
GRANT SELECT,INSERT,UPDATE ON ai.onboarding TO glowloom_app,glowloom_execution;

CREATE TABLE ai.integration_grant (
 id uuid PRIMARY KEY,token_hash text NOT NULL UNIQUE CHECK(length(token_hash)=64),
 scope_id uuid NOT NULL REFERENCES ai.account(scope_id),actor jsonb NOT NULL,
 model_ids jsonb NOT NULL CHECK(jsonb_typeof(model_ids)='array'),
 expires_at timestamptz NOT NULL,revoked boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
GRANT SELECT,INSERT,UPDATE ON ai.integration_grant TO glowloom_app,glowloom_execution;
CREATE FUNCTION ai.request_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (OLD.scope_id,OLD.actor_id,OLD.project_id,OLD.run_id,OLD.origin,OLD.idempotency_key,OLD.input_hash,OLD.model_id,OLD.contract,OLD.stream,OLD.deadline)
 IS DISTINCT FROM (NEW.scope_id,NEW.actor_id,NEW.project_id,NEW.run_id,NEW.origin,NEW.idempotency_key,NEW.input_hash,NEW.model_id,NEW.contract,NEW.stream,NEW.deadline)
 OR (OLD.state IN ('settled','released') AND NEW.state<>OLD.state)
 THEN RAISE EXCEPTION 'AI request identity is immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER ai_request_identity BEFORE UPDATE ON ai.request FOR EACH ROW EXECUTE FUNCTION ai.request_identity();
CREATE FUNCTION ai.attempt_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (OLD.request_id,OLD.number,OLD.route_id,OLD.credential_id,OLD.quota_group,OLD.contract,OLD.started_at)
 IS DISTINCT FROM (NEW.request_id,NEW.number,NEW.route_id,NEW.credential_id,NEW.quota_group,NEW.contract,NEW.started_at)
 OR (OLD.receipt IS NOT NULL AND (OLD.receipt,OLD.digest) IS DISTINCT FROM (NEW.receipt,NEW.digest))
 THEN RAISE EXCEPTION 'AI attempt evidence is immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER ai_attempt_identity BEFORE UPDATE ON ai.attempt FOR EACH ROW EXECUTE FUNCTION ai.attempt_identity();
CREATE FUNCTION ai.hold_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.amount<>NEW.amount OR (OLD.closed AND (OLD.charged,OLD.released,OLD.closed) IS DISTINCT FROM (NEW.charged,NEW.released,NEW.closed))
 THEN RAISE EXCEPTION 'AI closed hold is immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER ai_hold_identity BEFORE UPDATE ON ai.hold FOR EACH ROW EXECUTE FUNCTION ai.hold_identity();
CREATE TABLE ai.quote (
 id uuid PRIMARY KEY,scope_id uuid NOT NULL REFERENCES workspace.creation_scope(id),actor_id uuid NOT NULL,
 idempotency_key text NOT NULL,input_hash text NOT NULL CHECK(length(input_hash)=64),
 contract jsonb NOT NULL,expires_at timestamptz NOT NULL,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(scope_id,idempotency_key)
);
CREATE TRIGGER ai_route_purchase_immutable BEFORE UPDATE OR DELETE ON ai.route_purchase FOR EACH ROW EXECUTE FUNCTION ai.immutable_fact();
CREATE TRIGGER ai_model_sale_immutable BEFORE UPDATE OR DELETE ON ai.model_sale FOR EACH ROW EXECUTE FUNCTION ai.immutable_fact();
CREATE TRIGGER ai_quote_immutable BEFORE UPDATE OR DELETE ON ai.quote FOR EACH ROW EXECUTE FUNCTION ai.immutable_fact();
GRANT SELECT,INSERT ON ai.quote TO glowloom_app,glowloom_execution;

-- Private workbench intent. No legacy operation, catalog or wallet authority.
CREATE TABLE ai.creation_slot (
 actor_id uuid NOT NULL, project_id uuid NOT NULL REFERENCES workspace.project(id), owner_key text NOT NULL,
 epoch bigint NOT NULL DEFAULT 0 CHECK(epoch>=0), creation_id uuid,
 PRIMARY KEY(actor_id,project_id,owner_key)
);
CREATE TABLE ai.creation (
 id uuid PRIMARY KEY REFERENCES ai.quote(id), scope_id uuid NOT NULL REFERENCES workspace.creation_scope(id),
 actor_id uuid NOT NULL, project_id uuid NOT NULL REFERENCES workspace.project(id),
 owner_key text, epoch bigint NOT NULL DEFAULT 0,
 source jsonb, draft jsonb, presentation jsonb NOT NULL,
 input_hash text NOT NULL, actor jsonb NOT NULL,
 state text NOT NULL DEFAULT 'quoted' CHECK(state IN ('quoted','accepted','closed')),
 confirm_key uuid NOT NULL UNIQUE, created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
GRANT SELECT,INSERT,UPDATE ON ai.creation,ai.creation_slot TO glowloom_app,glowloom_execution;

CREATE TABLE ai.reconciliation (
 request_id uuid PRIMARY KEY REFERENCES ai.request(id),attempt_id uuid NOT NULL REFERENCES ai.attempt(id),
 actor_id uuid NOT NULL,digest text NOT NULL CHECK(length(digest)=64),evidence jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TRIGGER ai_reconciliation_immutable BEFORE UPDATE OR DELETE ON ai.reconciliation FOR EACH ROW EXECUTE FUNCTION ai.immutable_fact();
GRANT SELECT,INSERT ON ai.reconciliation TO glowloom_app,glowloom_execution;

CREATE TABLE ai.pending (
 request_id uuid PRIMARY KEY REFERENCES ai.request(id),attempt_id uuid NOT NULL UNIQUE REFERENCES ai.attempt(id),
 task_id text NOT NULL CHECK(length(task_id) BETWEEN 1 AND 256), receipt jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TRIGGER ai_pending_immutable BEFORE UPDATE OR DELETE ON ai.pending FOR EACH ROW EXECUTE FUNCTION ai.immutable_fact();
GRANT SELECT,INSERT ON ai.pending TO glowloom_app,glowloom_execution;

CREATE TABLE ai.job (
 request_id uuid PRIMARY KEY REFERENCES ai.request(id), actor jsonb NOT NULL,worker_proof jsonb NOT NULL DEFAULT '{}'::jsonb,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TRIGGER ai_job_immutable BEFORE UPDATE OR DELETE ON ai.job FOR EACH ROW EXECUTE FUNCTION ai.immutable_fact();
GRANT SELECT,INSERT ON ai.job TO glowloom_app,glowloom_execution;
CREATE TABLE ai.project_command (
 actor_id uuid NOT NULL,command_key uuid NOT NULL,input_hash text NOT NULL CHECK(length(input_hash)=64),
 response jsonb NOT NULL,PRIMARY KEY(actor_id,command_key)
);
CREATE TRIGGER ai_project_command_immutable BEFORE UPDATE OR DELETE ON ai.project_command FOR EACH ROW EXECUTE FUNCTION ai.immutable_fact();
GRANT SELECT,INSERT ON ai.project_command TO glowloom_app,glowloom_execution;
CREATE TABLE ai.batch (
 id uuid PRIMARY KEY,scope_id uuid NOT NULL REFERENCES workspace.creation_scope(id),actor_id uuid NOT NULL,
 project_id uuid NOT NULL REFERENCES workspace.project(id),command_key text NOT NULL,input_hash text NOT NULL,
 quotes jsonb NOT NULL,response jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(scope_id,command_key)
);
CREATE TRIGGER ai_batch_immutable BEFORE UPDATE OR DELETE ON ai.batch FOR EACH ROW EXECUTE FUNCTION ai.immutable_fact();
GRANT SELECT,INSERT ON ai.batch TO glowloom_app,glowloom_execution;

ALTER TABLE media.media_asset ADD CONSTRAINT fk_media_source_ai_request_project
 FOREIGN KEY(project_id,source_ai_request_id) REFERENCES ai.request(project_id,id);

CREATE TABLE ai.output (
 request_id uuid PRIMARY KEY REFERENCES ai.request(id),
 asset_id uuid NOT NULL UNIQUE,
 receipt_sha256 text NOT NULL CHECK(receipt_sha256 ~ '^[a-f0-9]{64}$'),
 sha256 text NOT NULL CHECK(sha256 ~ '^[a-f0-9]{64}$'),
 manifest_sha256 text NOT NULL CHECK(manifest_sha256 ~ '^[a-f0-9]{64}$'),
 manifest jsonb NOT NULL,
 kind text NOT NULL CHECK(kind IN ('image','video','audio')),
 state text NOT NULL DEFAULT 'preparing' CHECK(state IN ('preparing','pending','ready')),
 lease_id uuid,lease_until timestamptz,
 summary jsonb,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK((lease_id IS NULL)=(lease_until IS NULL)),
 CHECK((state='preparing')=(summary IS NULL))
);
CREATE TABLE ai.voice_intent (
 request_id uuid PRIMARY KEY REFERENCES ai.quote(id),
 scope_id uuid NOT NULL REFERENCES workspace.creation_scope(id),
 actor_id uuid NOT NULL REFERENCES identity."user"(id),
 command_key uuid NOT NULL,
 draft_intent_id uuid NOT NULL,
 input_hash text NOT NULL CHECK(input_hash ~ '^[a-f0-9]{64}$'),
 input jsonb NOT NULL,
 owner jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(scope_id,actor_id,command_key)
);
GRANT SELECT,INSERT,UPDATE ON ai.output TO glowloom_app,glowloom_execution;
GRANT SELECT,INSERT ON ai.voice_intent TO glowloom_app,glowloom_execution;
CREATE TABLE ai.agent_intent (
 receipt_id uuid PRIMARY KEY REFERENCES agent_business.call_receipt(receipt_id) DEFERRABLE INITIALLY DEFERRED,
 request_id uuid NOT NULL UNIQUE REFERENCES ai.request(id),root_run_id uuid NOT NULL
);
GRANT SELECT,INSERT ON ai.agent_intent TO glowloom_app,glowloom_execution;
CREATE VIEW ai.call_cost WITH (security_barrier=true) AS
 SELECT r.id AS request_id,r.scope_id,r.actor_id,r.project_id,r.run_id,r.origin,r.state,
 h.amount*100000 AS reserved_micros,h.charged*100000 AS charged_micros,h.released*100000 AS released_micros,
 (h.amount-h.charged-h.released)*100000 AS held_micros,
 l.receipt_id,
 (SELECT e.id FROM ai.money_event e WHERE e.request_id=r.id AND e.kind='settle') AS settlement_id
 FROM ai.request r JOIN ai.hold h ON h.request_id=r.id LEFT JOIN ai.agent_intent l ON l.request_id=r.id;
GRANT USAGE ON SCHEMA ai TO glowloom_creation;
GRANT SELECT ON ai.call_cost TO glowloom_app,glowloom_execution,glowloom_creation;

-- Immutable operational identities; only explicitly mutable lifecycle fields can change.
CREATE FUNCTION ai.credential_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (OLD.id,OLD.supplier_id,OLD.key_id,OLD.ciphertext) IS DISTINCT FROM (NEW.id,NEW.supplier_id,NEW.key_id,NEW.ciphertext)
 OR (OLD.state='revoked' AND NEW.state<>'revoked') THEN RAISE EXCEPTION 'AI credential identity is immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER ai_credential_identity BEFORE UPDATE ON ai.credential FOR EACH ROW EXECUTE FUNCTION ai.credential_identity();
CREATE FUNCTION ai.creation_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (OLD.id,OLD.scope_id,OLD.actor_id,OLD.project_id,OLD.owner_key,OLD.epoch,OLD.source,OLD.draft,OLD.presentation,OLD.input_hash,OLD.actor,OLD.confirm_key,OLD.created_at)
 IS DISTINCT FROM (NEW.id,NEW.scope_id,NEW.actor_id,NEW.project_id,NEW.owner_key,NEW.epoch,NEW.source,NEW.draft,NEW.presentation,NEW.input_hash,NEW.actor,NEW.confirm_key,NEW.created_at)
 THEN RAISE EXCEPTION 'AI creation source is immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER ai_creation_identity BEFORE UPDATE ON ai.creation FOR EACH ROW EXECUTE FUNCTION ai.creation_identity();
CREATE FUNCTION ai.output_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (OLD.request_id,OLD.asset_id,OLD.receipt_sha256,OLD.sha256,OLD.manifest_sha256,OLD.manifest,OLD.kind,OLD.created_at)
 IS DISTINCT FROM (NEW.request_id,NEW.asset_id,NEW.receipt_sha256,NEW.sha256,NEW.manifest_sha256,NEW.manifest,NEW.kind,NEW.created_at)
 OR (OLD.summary IS NOT NULL AND (OLD.summary,OLD.state) IS DISTINCT FROM (NEW.summary,NEW.state))
 THEN RAISE EXCEPTION 'AI archived output identity is immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER ai_output_identity BEFORE UPDATE ON ai.output FOR EACH ROW EXECUTE FUNCTION ai.output_identity();
ALTER TABLE ai.money_event ADD CONSTRAINT ai_money_kind_conservation CHECK(
 (kind='grant' AND request_id IS NULL AND available_delta>0 AND held_delta=0 AND spent_delta=0)
 OR (kind='freeze' AND request_id IS NOT NULL AND available_delta<=0 AND held_delta=-available_delta AND spent_delta=0)
 OR (kind='settle' AND request_id IS NOT NULL AND available_delta=0 AND held_delta=-spent_delta)
 OR (kind='release' AND request_id IS NOT NULL AND available_delta>=0 AND held_delta=-available_delta AND spent_delta=0)
);
ALTER TABLE ai.account ADD CONSTRAINT ai_account_micros_bound CHECK(available<=92233720368547-held);
CREATE TRIGGER ai_agent_intent_immutable BEFORE UPDATE OR DELETE ON ai.agent_intent FOR EACH ROW EXECUTE FUNCTION ai.immutable_fact();
CREATE TABLE ai.batch_decision (
 batch_id uuid PRIMARY KEY REFERENCES ai.batch(id),actor_id uuid NOT NULL REFERENCES identity."user"(id),
 command_key text NOT NULL,input_hash text NOT NULL,response jsonb NOT NULL
);
CREATE TRIGGER ai_batch_decision_immutable BEFORE UPDATE OR DELETE ON ai.batch_decision FOR EACH ROW EXECUTE FUNCTION ai.immutable_fact();
GRANT SELECT,INSERT ON ai.batch_decision TO glowloom_app,glowloom_execution;
CREATE TABLE ai.history (
 request_id uuid NOT NULL REFERENCES ai.quote(id),actor_id uuid NOT NULL REFERENCES identity."user"(id),
 revision bigint NOT NULL DEFAULT 0 CHECK(revision>=0),favorite boolean NOT NULL DEFAULT false,deleted_at timestamptz,
 PRIMARY KEY(request_id,actor_id)
);
GRANT SELECT,INSERT,UPDATE ON ai.history TO glowloom_app,glowloom_execution,glowloom_media;
CREATE TABLE ai.work (
 id uuid PRIMARY KEY,actor_id uuid NOT NULL,scope_id uuid NOT NULL,project_id uuid NOT NULL,
 request_id uuid NOT NULL REFERENCES ai.request(id),asset_id uuid NOT NULL REFERENCES media.media_asset(id),
 title text NOT NULL,description text NOT NULL,prompt text,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(actor_id,request_id)
);
CREATE TRIGGER ai_work_immutable BEFORE UPDATE OR DELETE ON ai.work FOR EACH ROW EXECUTE FUNCTION ai.immutable_fact();
GRANT SELECT,INSERT ON ai.work TO glowloom_app,glowloom_execution;

-- Shared project decisions are append-only. The owning project lock serializes
-- revision assignment and the active decision limit for each project.
CREATE TABLE workspace.project_memory_revision (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES workspace.project(id) ON DELETE CASCADE,
    shot_id uuid CHECK (shot_id IS NULL OR shot_id <> '00000000-0000-0000-0000-000000000000'),
    key text NOT NULL CHECK (char_length(key) BETWEEN 1 AND 48),
    revision bigint NOT NULL CHECK (revision > 0),
    statement text NOT NULL CHECK (char_length(statement) <= 512),
    withdrawn boolean NOT NULL,
    confirmed_by uuid NOT NULL,
    confirmed_at timestamptz NOT NULL,
    request_id uuid NOT NULL,
    CHECK (withdrawn = (statement = '')),
    UNIQUE (project_id, confirmed_by, request_id)
);

CREATE UNIQUE INDEX project_memory_slot_revision
    ON workspace.project_memory_revision (project_id, shot_id, key, revision DESC) NULLS NOT DISTINCT;

GRANT SELECT,INSERT ON TABLE workspace.project_memory_revision TO glowloom_app;

-- Derived, versioned excerpts of verified adopted script objects.
CREATE TABLE workspace.script_evidence_index (
    project_id uuid NOT NULL REFERENCES workspace.project(id) ON DELETE CASCADE,
    version_id uuid NOT NULL,
    PRIMARY KEY (project_id,version_id)
);
CREATE TABLE workspace.script_evidence_chunk (
    project_id uuid NOT NULL,
    version_id uuid NOT NULL,
    source_id uuid NOT NULL,
    part integer NOT NULL CHECK (part BETWEEN 1 AND 10000),
    start_pos bigint NOT NULL CHECK (start_pos >= 0),
    end_pos bigint NOT NULL CHECK (end_pos > start_pos),
    body text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 1200),
    PRIMARY KEY (project_id,version_id,source_id,part),
    FOREIGN KEY (project_id,version_id) REFERENCES workspace.script_evidence_index(project_id,version_id) ON DELETE CASCADE
);
GRANT SELECT,INSERT,DELETE ON workspace.script_evidence_index TO glowloom_app;
GRANT SELECT,INSERT ON workspace.script_evidence_chunk TO glowloom_app;

-- Current published project evidence. This projection reads the authoritative
-- rows using the invoking role and requires consumers to authorize the project.
CREATE VIEW workspace.current_project_evidence WITH (security_invoker=true) AS
WITH adopted_script AS NOT MATERIALIZED (
    SELECT s.project_id,s.org_id,v.id,v.version_no,v.source_ids
    FROM script.project_state s
    JOIN script.script_version v ON v.id=s.adopted_version_id AND v.project_id=s.project_id AND v.org_id=s.org_id
), scenes AS NOT MATERIALIZED (
    SELECT s.project_id,s.org_id,s.id,s.scene_key,s.heading,s.location_text,s.time_of_day,s.span_start,s.span_end,e.id AS episode_id,h.version_no
    FROM adopted_script v
    JOIN script.version_head sh ON sh.version_id=v.id AND sh.project_id=v.project_id AND sh.org_id=v.org_id
    JOIN script.episode e ON e.script_version_id=v.id AND e.split_set_id=sh.confirmed_split_set_id AND e.project_id=v.project_id AND e.org_id=v.org_id AND NOT e.is_delete
    JOIN script.episode_structure h ON h.id=e.confirmed_structure_id AND h.episode_id=e.id AND h.project_id=e.project_id AND h.org_id=e.org_id
    JOIN script.scene s ON s.episode_structure_id=h.id AND s.project_id=h.project_id AND s.org_id=h.org_id
), source_rows(project_id, kind, id, revision, parent_id, part, title, body, aliases, start_pos, end_pos, position_unit, draft, identity_version) AS (
    SELECT p.id,'brief',p.id,p.revision::bigint,NULL::uuid,i,p.name,substring(p.description FROM ((i-1)*1000+1) FOR 1200),'',
        ((i-1)*1000)::bigint,least((i-1)*1000+1200,length(p.description))::bigint,'character',false,''
    FROM workspace.project p
    CROSS JOIN LATERAL generate_series(1,least(64,greatest(1,(greatest(0,length(p.description)-200)+999)/1000))) i
    WHERE NOT p.is_delete AND p.status IN ('active','archived')
    UNION ALL
    SELECT m.project_id,CASE WHEN m.shot_id IS NULL THEN 'decision' ELSE 'shot_decision' END,
        m.id,m.revision,m.shot_id,1,m.key,m.statement,'',NULL,NULL,'',false,m.id::text
    FROM (SELECT DISTINCT ON (project_id,shot_id,key) * FROM workspace.project_memory_revision ORDER BY project_id,shot_id,key,revision DESC) m
    WHERE NOT m.withdrawn AND (m.shot_id IS NULL OR EXISTS (
        SELECT 1 FROM canvas.node n JOIN canvas.document d ON d.id=n.document_id AND NOT d.is_delete
        WHERE d.project_id=m.project_id AND NOT n.is_delete AND n.node_type='storyboard'
          AND n.config#>'{storyboard,shots}' @> jsonb_build_array(jsonb_build_object('id',m.shot_id::text))
    ))
    UNION ALL
    SELECT d.project_id,'canvas_node',n.id,n.content_version,d.id,i,n.title,substring(coalesce(n.config->>'text','') FROM ((i-1)*1000+1) FOR 1200),
        n.node_type,((i-1)*1000)::bigint,least((i-1)*1000+1200,length(coalesce(n.config->>'text','')))::bigint,'character',false,''
    FROM canvas.node n JOIN canvas.document d ON d.id=n.document_id AND NOT d.is_delete
    CROSS JOIN LATERAL generate_series(1,least(64,greatest(1,(greatest(0,length(coalesce(n.config->>'text',''))-200)+999)/1000))) i
    WHERE NOT n.is_delete
    UNION ALL
    SELECT d.project_id,'storyboard_shot',shot.id,shot.revision,n.id,i,shot.title,
        substring(shot.body FROM ((i-1)*1000+1) FOR 1200),'',
        ((i-1)*1000)::bigint,least((i-1)*1000+1200,length(shot.body))::bigint,'character',false,d.id::text
    FROM canvas.node n JOIN canvas.document d ON d.id=n.document_id AND NOT d.is_delete
    CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(n.config#>'{storyboard,shots}')='array'
        THEN n.config#>'{storyboard,shots}' ELSE '[]'::jsonb END) AS shot_item(value)
    CROSS JOIN LATERAL (SELECT
        CASE WHEN pg_input_is_valid(shot_item.value->>'id','uuid') THEN (shot_item.value->>'id')::uuid END AS id,
        CASE WHEN pg_input_is_valid(shot_item.value->>'revision','bigint') THEN (shot_item.value->>'revision')::bigint END AS revision,
        shot_item.value->>'title' AS title,
        concat_ws(E'\n',shot_item.value->>'action',shot_item.value->>'dialogue',shot_item.value->>'camera',shot_item.value->>'sound') AS body
    ) shot
    CROSS JOIN LATERAL generate_series(1,least(12,greatest(1,(greatest(0,length(shot.body)-200)+999)/1000))) i
    WHERE NOT n.is_delete AND n.node_type='storyboard' AND shot.id IS NOT NULL AND shot.revision>0 AND shot.title IS NOT NULL
    UNION ALL
    SELECT s.project_id,'script_source',s.source_lineage_id,s.source_revision,v.id,coalesce(c.part,1),s.title,coalesce(c.body,''),s.source_kind,
        c.start_pos,c.end_pos,CASE WHEN c.part IS NULL THEN '' ELSE 'character' END,false,s.id::text
    FROM adopted_script v
    JOIN script.script_source s ON s.project_id=v.project_id AND s.org_id=v.org_id AND s.id=ANY(v.source_ids) AND s.status IN ('ready','completed')
    LEFT JOIN workspace.script_evidence_chunk c ON c.project_id=s.project_id AND c.version_id=v.id AND c.source_id=s.id
    UNION ALL
    SELECT s.project_id,'scene',s.scene_key,s.version_no,s.episode_id,1,s.heading,concat_ws(' ',s.location_text,s.time_of_day),'',
        s.span_start::bigint,s.span_end::bigint,'character',false,s.id::text FROM scenes s
    UNION ALL
    SELECT s.project_id,'script_line',l.id,s.version_no,s.scene_key,1,s.heading,left(l.content,1200),'',
        l.span_start::bigint,l.span_end::bigint,'character',false,l.id::text
    FROM scenes s JOIN script.action_line l ON l.scene_id=s.id AND l.project_id=s.project_id AND l.org_id=s.org_id
    UNION ALL
    SELECT s.project_id,'script_line',l.id,s.version_no,s.scene_key,1,concat_ws(' · ',s.heading,l.speaker_text),left(l.content,1200),l.kind,
        l.span_start::bigint,l.span_end::bigint,'character',false,l.id::text
    FROM scenes s JOIN script.dialogue_line l ON l.scene_id=s.id AND l.project_id=s.project_id AND l.org_id=s.org_id
    UNION ALL
    SELECT e.project_id,'character',e.id,v.version_no,NULL,1,coalesce(v.content->>'name',''),left(coalesce(v.content->>'description',''),1200),
        CASE WHEN jsonb_typeof(v.content->'aliases')='array' THEN v.content->>'aliases' ELSE '' END,NULL,NULL,'',false,v.id::text
    FROM bible."character" e JOIN bible.character_version v ON v.id=e.confirmed_version_id AND v.entry_id=e.id AND v.project_id=e.project_id AND v.org_id=e.org_id
    WHERE NOT e.is_delete AND e.redirect_id IS NULL
    UNION ALL
    SELECT e.project_id,'location',e.id,v.version_no,NULL,1,coalesce(v.content->>'name',''),left(concat_ws(' ',v.content->>'description',v.content->>'prompt'),1200),
        CASE WHEN jsonb_typeof(v.content->'aliases')='array' THEN v.content->>'aliases' ELSE '' END,NULL,NULL,'',false,v.id::text
    FROM bible.location e JOIN bible.location_version v ON v.id=e.confirmed_version_id AND v.entry_id=e.id AND v.project_id=e.project_id AND v.org_id=e.org_id WHERE NOT e.is_delete
    UNION ALL
    SELECT e.project_id,'prop',e.id,v.version_no,NULL,1,coalesce(v.content->>'name',''),left(concat_ws(' ',v.content->>'description',v.content->>'prompt'),1200),
        CASE WHEN jsonb_typeof(v.content->'aliases')='array' THEN v.content->>'aliases' ELSE '' END,NULL,NULL,'',false,v.id::text
    FROM bible.prop e JOIN bible.prop_version v ON v.id=e.confirmed_version_id AND v.entry_id=e.id AND v.project_id=e.project_id AND v.org_id=e.org_id WHERE NOT e.is_delete
    UNION ALL
    SELECT a.project_id,'media',a.id,a.revision::bigint,NULL,1,left(a.file_name,512),'',a.kind,NULL,NULL,'',false,coalesce(a.sha256,'')
    FROM media.media_asset a WHERE a.project_id IS NOT NULL AND a.status='ready' AND NOT a.is_delete AND a.moderation_status='passed' AND NOT a.contains_real_person AND a.consent_record_id IS NULL
    UNION ALL
    SELECT j.project_id,'transcript',j.id,j.revision,a.id,c.n::integer,left(a.file_name,512),left(c.elem->>'text',1200),a.kind,
        (c.elem->>'start_ms')::bigint,(c.elem->>'end_ms')::bigint,'millisecond',true,j.result_sha256
    FROM mediatool.transcription_job j
    JOIN canvas.document d ON d.id=j.canvas_id AND d.project_id=j.project_id AND NOT d.is_delete
    JOIN canvas.node n ON n.id=j.node_id AND n.document_id=d.id AND NOT n.is_delete
    JOIN media.media_asset a ON a.project_id=j.project_id AND a.id::text=j.frozen#>>'{input,asset_id}'
        AND a.revision::text=j.frozen#>>'{input,revision}' AND a.sha256=j.frozen#>>'{input,sha256}'
        AND d.revision=j.source_revision AND n.ref_type='media_asset' AND n.ref_id=a.id AND n.node_type=a.kind
        AND a.kind=j.frozen#>>'{input,kind}' AND a.status='ready' AND NOT a.is_delete AND a.moderation_status='passed' AND NOT a.contains_real_person AND a.consent_record_id IS NULL
    CROSS JOIN LATERAL jsonb_array_elements(j.result->'segments') WITH ORDINALITY AS c(elem,n)
    WHERE j.status='succeeded' AND j.inference_state='terminal' AND j.result_sha256 IS NOT NULL
)
SELECT project_id,kind,id,revision,parent_id,part,title,coalesce(body,'') AS excerpt,coalesce(aliases,'') AS aliases,
    start_pos,end_pos,position_unit,draft,
    encode(sha256(convert_to(jsonb_build_array(kind,id,revision,parent_id,part,title,body,aliases,start_pos,end_pos,position_unit,draft,identity_version)::text,'UTF8')),'hex') AS version
FROM source_rows;

GRANT SELECT ON workspace.current_project_evidence TO glowloom_app;

-- Policy presence authorizes a bounded, project-specific external embedding route.
CREATE TABLE workspace.evidence_semantic_policy (
    project_id uuid PRIMARY KEY REFERENCES workspace.project(id) ON DELETE CASCADE,
    budget_id uuid NOT NULL UNIQUE,
    generation uuid NOT NULL UNIQUE,
    actor_id uuid NOT NULL,
    active_release_id uuid REFERENCES ai.model(id),
    active_profile text NOT NULL DEFAULT '',
    scan_cursor text NOT NULL DEFAULT '',
    CHECK ((active_release_id IS NULL) = (active_profile = '')),
    limit_credits bigint NOT NULL CHECK (limit_credits > 0),
    enabled boolean NOT NULL DEFAULT true
);
CREATE TABLE workspace.evidence_embedding (
    project_id uuid NOT NULL REFERENCES workspace.evidence_semantic_policy(project_id) ON DELETE CASCADE,
    release_id uuid NOT NULL REFERENCES ai.model(id),
    profile text NOT NULL,
    kind text NOT NULL,
    id uuid NOT NULL,
    part integer NOT NULL CHECK (part>0),
    version text NOT NULL CHECK (length(version)=64),
    embedding public.vector(1024) NOT NULL,
    PRIMARY KEY(project_id,release_id,profile,kind,id,part,version)
);
GRANT SELECT,UPDATE(active_release_id,active_profile,scan_cursor) ON workspace.evidence_semantic_policy TO glowloom_app;
GRANT SELECT,INSERT,DELETE ON workspace.evidence_embedding TO glowloom_app;
