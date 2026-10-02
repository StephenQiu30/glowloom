DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM media.purge_job) OR EXISTS(SELECT 1 FROM media.purge_command) OR
  EXISTS(SELECT 1 FROM media.library_item WHERE purged_at IS NOT NULL) THEN
  RAISE EXCEPTION 'retained media purge facts prevent rollback' USING ERRCODE='55000';
 END IF;
END $$;
DROP TABLE media.purge_command;
DROP TABLE media.purge_object;
DROP TABLE media.purge_item;
DROP TABLE media.purge_job;
DROP FUNCTION media.reject_purge_command_mutation();
DROP FUNCTION media.check_purge_scope();
REVOKE UPDATE(purged_at) ON media.library_item FROM lanverse_app;
ALTER TABLE media.library_item DROP CONSTRAINT library_item_purged_check;
ALTER TABLE media.library_item DROP COLUMN purged_at;
