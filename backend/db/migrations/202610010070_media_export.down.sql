DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM mediatool.export_job) THEN
  RAISE EXCEPTION 'refusing to remove durable media export history';
 END IF;
END $$;
DROP TABLE mediatool.export_command;
DROP TABLE mediatool.export_job;
DROP SCHEMA mediatool;
