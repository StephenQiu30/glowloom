CREATE TABLE workspace.project (
  id                    uuid PRIMARY KEY,
  org_id                uuid NOT NULL REFERENCES workspace.organization(id),
  name                  text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 50),
  description           text NOT NULL DEFAULT '',
  aspect_ratio          text NOT NULL CHECK (aspect_ratio IN ('9:16', '16:9')),
  style_type            text NOT NULL CHECK (style_type IN ('realistic', 'stylized')),
  style_subtype         text CHECK (style_subtype IN ('anime_jp', 'guofeng_xianxia', 'cartoon_3d', 'manhwa')),
  style_preset_id       uuid REFERENCES workspace.style_preset(id),
  resolution            text NOT NULL DEFAULT '1080p' CHECK (resolution = '1080p'),
  allow_overseas_models boolean NOT NULL DEFAULT false,
  default_models        jsonb NOT NULL DEFAULT '{}',
  aigc_mark_style       jsonb NOT NULL DEFAULT '{"preset":"bottom_right"}',
  status                text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
  archived_at           timestamptz,
  delete_time           timestamptz,
  purge_after           timestamptz,
  revision              integer NOT NULL DEFAULT 1 CHECK (revision > 0),
  create_time           timestamptz NOT NULL DEFAULT now(),
  update_time           timestamptz NOT NULL DEFAULT now(),
  is_delete             boolean NOT NULL DEFAULT false,
  CONSTRAINT ck_project_style CHECK ((style_type = 'stylized') = (style_subtype IS NOT NULL))
);

CREATE INDEX ix_project_org_status
  ON workspace.project (org_id, status, update_time DESC)
  WHERE NOT is_delete;

ALTER TABLE workspace.style_preset
  ADD CONSTRAINT fk_style_preset_project
  FOREIGN KEY (project_id) REFERENCES workspace.project(id);

CREATE FUNCTION workspace.protect_project_update()
RETURNS trigger
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

CREATE TRIGGER trg_project_protect_update
BEFORE UPDATE ON workspace.project
FOR EACH ROW EXECUTE FUNCTION workspace.protect_project_update();
