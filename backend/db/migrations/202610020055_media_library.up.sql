-- Personal media has explicit current-user ownership, never a hidden project.
ALTER TABLE identity."user" ADD CONSTRAINT user_org_id_id_unique UNIQUE (org_id,id);
ALTER TABLE media.media_asset ALTER COLUMN project_id DROP NOT NULL;
ALTER TABLE media.media_asset ADD COLUMN personal_org_id uuid;
ALTER TABLE media.media_asset ADD COLUMN personal_actor_id uuid;
ALTER TABLE media.media_asset ADD CONSTRAINT media_asset_personal_owner_fk
  FOREIGN KEY (personal_org_id,personal_actor_id) REFERENCES identity."user"(org_id,id);
ALTER TABLE media.media_asset ADD CONSTRAINT media_asset_ownership_check CHECK (
  (project_id IS NOT NULL AND project_id <> '00000000-0000-0000-0000-000000000000'::uuid
    AND personal_org_id IS NULL AND personal_actor_id IS NULL)
  OR (project_id IS NULL AND personal_org_id IS NOT NULL AND personal_actor_id IS NOT NULL
    AND personal_org_id <> '00000000-0000-0000-0000-000000000000'::uuid
    AND personal_actor_id <> '00000000-0000-0000-0000-000000000000'::uuid
    AND origin IN ('upload','system') AND source_operation_id IS NULL
    AND provider_key IS NULL AND model_key IS NULL AND region IS NULL)
);
CREATE INDEX media_personal_current ON media.media_asset(personal_org_id,personal_actor_id,update_time DESC,id)
  WHERE project_id IS NULL AND NOT is_delete;

CREATE TABLE media.library (
  id uuid PRIMARY KEY,
  kind text NOT NULL CHECK(kind IN ('project','personal')),
  org_id uuid NOT NULL REFERENCES workspace.organization(id),
  project_id uuid REFERENCES workspace.project(id),
  personal_actor_id uuid,
  revision integer NOT NULL DEFAULT 0 CHECK(revision >= 0),
  create_time timestamptz NOT NULL DEFAULT now(),
  update_time timestamptz NOT NULL DEFAULT now(),
  UNIQUE(id,kind),
  FOREIGN KEY(org_id,personal_actor_id) REFERENCES identity."user"(org_id,id),
  CHECK((kind='project' AND project_id IS NOT NULL AND personal_actor_id IS NULL)
     OR (kind='personal' AND project_id IS NULL AND personal_actor_id IS NOT NULL))
);
CREATE UNIQUE INDEX library_project_scope ON media.library(project_id) WHERE kind='project';
CREATE UNIQUE INDEX library_personal_scope ON media.library(org_id,personal_actor_id) WHERE kind='personal';

CREATE TABLE media.library_folder (
  id uuid PRIMARY KEY,
  library_id uuid NOT NULL,
  library_kind text NOT NULL,
  parent_id uuid,
  name text NOT NULL CHECK(name=btrim(name) AND char_length(name) BETWEEN 1 AND 60),
  name_key text NOT NULL CHECK(name_key=lower(btrim(name))),
  position integer NOT NULL CHECK(position >= 0),
  style text NOT NULL DEFAULT '',
  theme text NOT NULL DEFAULT '',
  revision integer NOT NULL DEFAULT 1 CHECK(revision > 0),
  create_time timestamptz NOT NULL DEFAULT now(),
  update_time timestamptz NOT NULL DEFAULT now(),
  UNIQUE(library_id,id),
  UNIQUE NULLS NOT DISTINCT(library_id,parent_id,name_key),
  FOREIGN KEY(library_id,library_kind) REFERENCES media.library(id,kind),
  FOREIGN KEY(library_id,parent_id) REFERENCES media.library_folder(library_id,id),
  CHECK(parent_id IS NULL OR parent_id<>id),
  CHECK((library_kind='personal' AND parent_id IS NULL AND char_length(name)<=40 AND style='' AND theme='')
    OR (library_kind='project' AND style IN ('glass','stacked','midnight','paper','cinema','compact')
      AND theme IN ('aurora','obsidian','ember','pearl')))
);

CREATE TABLE media.library_item (
  id uuid PRIMARY KEY,
  library_id uuid NOT NULL REFERENCES media.library(id),
  asset_id uuid REFERENCES media.media_asset(id),
  plain_text text CHECK(plain_text IS NULL OR octet_length(plain_text)<=65536),
  folder_id uuid,
  title text NOT NULL CHECK(char_length(title) BETWEEN 1 AND 240 AND btrim(title)<>''),
  category text NOT NULL CHECK(category IN ('character','environment','prop','material','other')),
  tags jsonb NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(tags)='array' AND jsonb_array_length(tags)<=32),
  source_label text NOT NULL DEFAULT '' CHECK(char_length(source_label)<=240),
  note text NOT NULL DEFAULT '' CHECK(char_length(note)<=8000),
  favorite boolean NOT NULL DEFAULT false,
  catalog_state text NOT NULL DEFAULT 'active' CHECK(catalog_state IN ('active','trashed','removed')),
  trashed_at timestamptz,
  position integer NOT NULL CHECK(position >= 0),
  revision integer NOT NULL DEFAULT 1 CHECK(revision > 0),
  create_time timestamptz NOT NULL DEFAULT now(),
  update_time timestamptz NOT NULL DEFAULT now(),
  UNIQUE(library_id,asset_id),
  FOREIGN KEY(library_id,folder_id) REFERENCES media.library_folder(library_id,id),
  CHECK((asset_id IS NULL) <> (plain_text IS NULL)),
  CHECK(asset_id IS NULL OR id=asset_id),
  CHECK((catalog_state='trashed')=(trashed_at IS NOT NULL))
);
CREATE INDEX library_item_scoped_order ON media.library_item(library_id,catalog_state,update_time DESC,id);
CREATE INDEX library_item_folder ON media.library_item(library_id,folder_id,catalog_state,id);
CREATE INDEX library_item_search ON media.library_item USING gin(title gin_trgm_ops);

-- Media owns both sides. This only preserves ownership; ready/review/consent
-- eligibility remains a current, locked application decision.
CREATE FUNCTION media.check_library_asset_scope() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE TRIGGER library_item_scope BEFORE INSERT OR UPDATE ON media.library_item
  FOR EACH ROW EXECUTE FUNCTION media.check_library_asset_scope();

CREATE TABLE media.library_command (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL,
  actor_id uuid NOT NULL,
  library_id uuid NOT NULL REFERENCES media.library(id),
  idem_key uuid NOT NULL,
  action text NOT NULL CHECK(action IN ('create_folder','update_folder','delete_folder','create_text','update_item','move_items','recycle_items','restore_items','remove_items')),
  request_sha256 text NOT NULL CHECK(request_sha256 ~ '^[0-9a-f]{64}$'),
  response_body bytea NOT NULL CHECK(octet_length(response_body) BETWEEN 2 AND 1048576),
  create_time timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY(org_id,actor_id) REFERENCES identity."user"(org_id,id),
  UNIQUE(actor_id,idem_key)
);
CREATE FUNCTION media.reject_library_command_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'library command is immutable' USING ERRCODE='42501'; END;
$$;
CREATE TRIGGER library_command_immutable BEFORE UPDATE OR DELETE ON media.library_command
  FOR EACH ROW EXECUTE FUNCTION media.reject_library_command_mutation();

GRANT SELECT,INSERT,UPDATE(revision,update_time) ON media.library TO lanverse_app;
GRANT SELECT,INSERT,DELETE,UPDATE(parent_id,name,name_key,position,style,theme,revision,update_time)
  ON media.library_folder TO lanverse_app;
GRANT SELECT,INSERT,UPDATE(plain_text,folder_id,title,category,tags,source_label,note,favorite,catalog_state,trashed_at,position,revision,update_time)
  ON media.library_item TO lanverse_app;
GRANT SELECT,INSERT ON media.library_command TO lanverse_app;
