DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM mediatool.transcription_job) THEN
  RAISE EXCEPTION 'transcription history exists; rollback requires explicit preservation';
 END IF;
END $$;
DROP TABLE mediatool.transcription_command;
DROP TABLE mediatool.transcription_job;
