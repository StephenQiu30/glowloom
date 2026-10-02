ALTER TABLE workspace.project ADD COLUMN cover_asset_id uuid REFERENCES media.media_asset(id);
ALTER TABLE workspace.project ADD CONSTRAINT project_cover_nonzero CHECK(cover_asset_id IS NULL OR cover_asset_id <> '00000000-0000-0000-0000-000000000000'::uuid);
COMMENT ON COLUMN workspace.project.cover_asset_id IS 'Current-project ready/passed image; eligibility and current authorization are verified by the media owner in the project transaction.';

CREATE TABLE workspace.project_change_command (
 id uuid PRIMARY KEY,
 org_id uuid NOT NULL REFERENCES workspace.organization(id),
 project_id uuid NOT NULL REFERENCES workspace.project(id),
 actor_id uuid NOT NULL REFERENCES identity."user"(id),
 idem_key uuid NOT NULL,
 action text NOT NULL CHECK(action IN ('patch','archive','unarchive','delete','restore')),
 request_sha256 text NOT NULL CHECK(request_sha256 ~ '^[0-9a-f]{64}$'),
 status_code smallint NOT NULL CHECK(status_code=200),
 response_body jsonb NOT NULL CHECK(jsonb_typeof(response_body)='object'),
 create_time timestamptz NOT NULL DEFAULT now(),
 UNIQUE(actor_id,idem_key)
);
CREATE FUNCTION workspace.reject_project_change_command_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'project change command is immutable' USING ERRCODE='42501';
END;
$$;
CREATE TRIGGER project_change_command_immutable BEFORE UPDATE OR DELETE ON workspace.project_change_command FOR EACH ROW EXECUTE FUNCTION workspace.reject_project_change_command_mutation();
GRANT SELECT,INSERT ON workspace.project_change_command TO lanverse_app;
