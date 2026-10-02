DO $$ BEGIN
  IF EXISTS(SELECT 1 FROM media.transfer_job) OR EXISTS(SELECT 1 FROM media.transfer_command) THEN
    RAISE EXCEPTION 'retained media transfer receipts or private objects block rollback' USING ERRCODE='55000';
  END IF;
END $$;
DROP TABLE media.transfer_command,media.transfer_object,media.transfer_item,media.transfer_job;
DROP FUNCTION media.reject_transfer_command_mutation();
DROP FUNCTION media.check_transfer_scope();
