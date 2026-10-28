-- Additive subject storage for an existing database.
-- Do not replay schema.sql on business data. No old Bible row is rewritten.
-- Repeating this migration preserves all rows and immutable versions.
BEGIN;
SET LOCAL lock_timeout = '5s';
SELECT pg_advisory_xact_lock(63498120261028);
-- Space-owned subjects are independent of the project Bible. Append-only
-- versions and bindings retain source-media evidence after product deletion.
CREATE SCHEMA IF NOT EXISTS subject;
CREATE TABLE IF NOT EXISTS subject.subject_head (
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
CREATE TABLE IF NOT EXISTS subject.subject_version (
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
DO $$
BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='subject.subject_head'::regclass AND conname='subject_current_version_fk') THEN
  ALTER TABLE subject.subject_head ADD CONSTRAINT subject_current_version_fk
 FOREIGN KEY(current_version_id,id,owner_scope_id,physical_scope_id)
 REFERENCES subject.subject_version(id,subject_id,owner_scope_id,physical_scope_id)
 DEFERRABLE INITIALLY DEFERRED;
 END IF;
END $$;
CREATE TABLE IF NOT EXISTS subject.media_binding (
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
CREATE TABLE IF NOT EXISTS subject.favorite (
 actor_id uuid NOT NULL,
 subject_id uuid NOT NULL,
 owner_scope_id uuid NOT NULL,
 physical_scope_id uuid NOT NULL,
 favorite boolean NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(actor_id,subject_id),
 FOREIGN KEY(subject_id,owner_scope_id,physical_scope_id) REFERENCES subject.subject_head(id,owner_scope_id,physical_scope_id)
);
CREATE TABLE IF NOT EXISTS subject.command (
 actor_id uuid NOT NULL,
 idem_key uuid NOT NULL,
 owner_scope_id uuid NOT NULL REFERENCES workspace.creation_scope(id),
 physical_scope_id uuid NOT NULL REFERENCES workspace.organization(id),
 request_sha256 text NOT NULL CHECK(request_sha256 ~ '^[a-f0-9]{64}$'),
 response_body jsonb NOT NULL CHECK(jsonb_typeof(response_body)='object' AND octet_length(response_body::text)<=1048576),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(actor_id,idem_key)
);
CREATE INDEX IF NOT EXISTS subject_head_updated ON subject.subject_head(owner_scope_id,physical_scope_id,updated_at,id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS subject_head_created ON subject.subject_head(owner_scope_id,physical_scope_id,created_at,id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS subject_binding_retained_asset ON subject.media_binding(asset_id);
CREATE INDEX IF NOT EXISTS subject_binding_version ON subject.media_binding(owner_scope_id,physical_scope_id,subject_id,version_id);
CREATE INDEX IF NOT EXISTS subject_favorite_actor ON subject.favorite(actor_id,owner_scope_id,subject_id) WHERE favorite;
GRANT USAGE ON SCHEMA subject TO glowloom_app;
GRANT SELECT,INSERT,UPDATE ON subject.subject_head,subject.favorite TO glowloom_app;
GRANT SELECT,INSERT ON subject.subject_version,subject.media_binding,subject.command TO glowloom_app;
REVOKE DELETE ON subject.subject_head,subject.subject_version,subject.media_binding,subject.command FROM glowloom_app;

DO $$
BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='subject.subject_head'::regclass AND conname='subject_scope_physical_fk') THEN
  ALTER TABLE subject.subject_head ADD CONSTRAINT subject_scope_physical_fk FOREIGN KEY(owner_scope_id,physical_scope_id) REFERENCES workspace.creation_scope(id,physical_scope_id);
 END IF;
END $$;
DO $$
BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='subject.command'::regclass AND conname='subject_command_scope_physical_fk') THEN
  ALTER TABLE subject.command ADD CONSTRAINT subject_command_scope_physical_fk FOREIGN KEY(owner_scope_id,physical_scope_id) REFERENCES workspace.creation_scope(id,physical_scope_id);
 END IF;
END $$;

DO $$
BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='operation.operation_input'::regclass AND conname='operation_input_subject_binding_check') THEN
  ALTER TABLE operation.operation_input ADD CONSTRAINT operation_input_subject_binding_check CHECK (ref_type <> 'subject_media' OR (ref_id IS NOT NULL AND ref_version IS NOT NULL AND ref_version ~ '^[a-f0-9]{64}$' AND media_asset_id IS NOT NULL AND text_value IS NULL AND mask_asset_id IS NULL AND role <> 'prompt'));
 END IF;
END $$;
COMMIT;
