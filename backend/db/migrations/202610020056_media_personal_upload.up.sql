-- Retain the same permanent upload receipt owner; only its closed scope expands.
ALTER TABLE media.upload_request DROP CONSTRAINT upload_request_pkey;
ALTER TABLE media.upload_request ALTER COLUMN project_id DROP NOT NULL;
ALTER TABLE media.upload_request ADD COLUMN personal_org_id uuid;
ALTER TABLE media.upload_request ADD CONSTRAINT upload_request_project_unique UNIQUE(project_id,principal_id,request_key);
ALTER TABLE media.upload_request ADD CONSTRAINT upload_request_personal_unique UNIQUE(personal_org_id,principal_id,request_key);
ALTER TABLE media.upload_request ADD CONSTRAINT upload_request_personal_actor_fk
 FOREIGN KEY(personal_org_id,principal_id) REFERENCES identity."user"(org_id,id);
ALTER TABLE media.upload_request ADD CONSTRAINT upload_request_scope_check CHECK(
 (project_id IS NOT NULL AND project_id <> '00000000-0000-0000-0000-000000000000' AND personal_org_id IS NULL)
 OR (project_id IS NULL AND personal_org_id IS NOT NULL AND personal_org_id <> '00000000-0000-0000-0000-000000000000')
);
CREATE FUNCTION media.check_upload_receipt_scope() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM media.media_asset a WHERE a.id=NEW.asset_id AND
  ((NEW.project_id IS NOT NULL AND a.project_id=NEW.project_id AND a.personal_org_id IS NULL AND a.personal_actor_id IS NULL)
   OR (NEW.project_id IS NULL AND a.project_id IS NULL AND a.personal_org_id=NEW.personal_org_id AND a.personal_actor_id=NEW.principal_id))) THEN
  RAISE EXCEPTION 'upload receipt ownership does not match its original' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER upload_request_scope_guard BEFORE INSERT ON media.upload_request
 FOR EACH ROW EXECUTE FUNCTION media.check_upload_receipt_scope();
