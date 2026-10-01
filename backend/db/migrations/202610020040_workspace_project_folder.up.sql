CREATE TABLE workspace.project_folder (
 id uuid PRIMARY KEY,
 org_id uuid NOT NULL REFERENCES workspace.organization(id),
 actor_id uuid NOT NULL REFERENCES identity."user"(id),
 name text NOT NULL CHECK(char_length(btrim(name)) BETWEEN 1 AND 160),
 cover_project_id uuid REFERENCES workspace.project(id),
 cover_asset_id uuid REFERENCES media.media_asset(id),
 revision integer NOT NULL DEFAULT 1 CHECK(revision>0),
 is_delete boolean NOT NULL DEFAULT false,
 delete_time timestamptz,
 create_time timestamptz NOT NULL DEFAULT now(),
 update_time timestamptz NOT NULL DEFAULT now(),
 UNIQUE(id,org_id,actor_id),
 CHECK((cover_project_id IS NULL)=(cover_asset_id IS NULL)),
 CHECK(is_delete=(delete_time IS NOT NULL))
);
CREATE INDEX ix_project_folder_actor_recent ON workspace.project_folder(org_id,actor_id,update_time DESC,id DESC) WHERE NOT is_delete;

CREATE TABLE workspace.project_folder_placement (
 org_id uuid NOT NULL REFERENCES workspace.organization(id),
 actor_id uuid NOT NULL REFERENCES identity."user"(id),
 project_id uuid NOT NULL REFERENCES workspace.project(id) ON DELETE CASCADE,
 folder_id uuid,
 revision integer NOT NULL DEFAULT 1 CHECK(revision>0),
 create_time timestamptz NOT NULL DEFAULT now(),
 update_time timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(actor_id,project_id),
 FOREIGN KEY(folder_id,org_id,actor_id) REFERENCES workspace.project_folder(id,org_id,actor_id)
);
CREATE INDEX ix_project_folder_members ON workspace.project_folder_placement(org_id,actor_id,folder_id,project_id);

CREATE TABLE workspace.project_folder_command (
 id uuid PRIMARY KEY,
 org_id uuid NOT NULL REFERENCES workspace.organization(id),
 actor_id uuid NOT NULL REFERENCES identity."user"(id),
 idem_key uuid NOT NULL,
 action text NOT NULL CHECK(action IN ('create','patch','move','recycle')),
 request_sha256 text NOT NULL CHECK(request_sha256 ~ '^[0-9a-f]{64}$'),
 response_body jsonb NOT NULL CHECK(jsonb_typeof(response_body)='object'),
 create_time timestamptz NOT NULL DEFAULT now(),
 UNIQUE(actor_id,idem_key)
);
CREATE FUNCTION workspace.reject_project_folder_command_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'project folder command is immutable' USING ERRCODE='42501';
END;
$$;
CREATE TRIGGER project_folder_command_immutable BEFORE UPDATE OR DELETE ON workspace.project_folder_command FOR EACH ROW EXECUTE FUNCTION workspace.reject_project_folder_command_mutation();

GRANT SELECT,INSERT ON workspace.project_folder,workspace.project_folder_placement,workspace.project_folder_command TO lanverse_app;
GRANT UPDATE(name,cover_project_id,cover_asset_id,revision,is_delete,delete_time,update_time) ON workspace.project_folder TO lanverse_app;
GRANT UPDATE(folder_id,revision,update_time) ON workspace.project_folder_placement TO lanverse_app;
