DO $$ BEGIN
  IF EXISTS(SELECT 1 FROM media.package_job) THEN
    RAISE EXCEPTION 'cannot remove retained media package batches and private object intents';
  END IF;
END $$;
DROP TRIGGER package_scope ON media.package_job;
DROP FUNCTION media.check_package_scope();
DROP TRIGGER package_frozen_immutable ON media.package_job;
DROP FUNCTION media.reject_package_frozen_mutation();
DROP TRIGGER package_command_immutable ON media.package_command;
DROP FUNCTION media.reject_package_command_mutation();
DROP TABLE media.package_command;
DROP TABLE media.package_object;
DROP TABLE media.package_job;
