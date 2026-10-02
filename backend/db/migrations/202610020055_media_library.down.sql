DO $$ BEGIN
  IF EXISTS(SELECT 1 FROM media.library) OR EXISTS(SELECT 1 FROM media.media_asset WHERE project_id IS NULL) THEN
    RAISE EXCEPTION 'media library rollback would discard owned content' USING ERRCODE='55000';
  END IF;
END $$;
DROP TABLE media.library_command;
DROP FUNCTION media.reject_library_command_mutation();
DROP TABLE media.library_item;
DROP FUNCTION media.check_library_asset_scope();
DROP TABLE media.library_folder;
DROP TABLE media.library;
DROP INDEX media.media_personal_current;
ALTER TABLE media.media_asset DROP CONSTRAINT media_asset_ownership_check;
ALTER TABLE media.media_asset DROP CONSTRAINT media_asset_personal_owner_fk;
ALTER TABLE media.media_asset DROP COLUMN personal_actor_id;
ALTER TABLE media.media_asset DROP COLUMN personal_org_id;
ALTER TABLE media.media_asset ALTER COLUMN project_id SET NOT NULL;
ALTER TABLE identity."user" DROP CONSTRAINT user_org_id_id_unique;
