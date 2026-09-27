CREATE TABLE workspace.style_preset (
  id                  uuid PRIMARY KEY,
  org_id              uuid NOT NULL REFERENCES workspace.organization(id),
  project_id          uuid,
  name                text NOT NULL,
  style_type          text NOT NULL CHECK (style_type IN ('realistic', 'stylized')),
  style_subtype       text CHECK (style_subtype IN ('anime_jp', 'guofeng_xianxia', 'cartoon_3d', 'manhwa')),
  prompt_fragment     text NOT NULL DEFAULT '',
  negative_prompt     text NOT NULL DEFAULT '',
  reference_asset_ids uuid[] NOT NULL DEFAULT '{}',
  create_time         timestamptz NOT NULL DEFAULT now(),
  update_time         timestamptz NOT NULL DEFAULT now(),
  is_delete           boolean NOT NULL DEFAULT false
);

CREATE FUNCTION workspace.set_style_preset_update_time()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  NEW.update_time := now();
  RETURN NEW;
END;
$$;

CREATE TRIGGER trg_style_preset_update_time
BEFORE UPDATE ON workspace.style_preset
FOR EACH ROW EXECUTE FUNCTION workspace.set_style_preset_update_time();
