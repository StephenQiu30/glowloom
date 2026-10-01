DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM mediatool.export_job WHERE output_kind='audio') THEN
  RAISE EXCEPTION 'audio export history exists; output identity must be preserved';
 END IF;
END $$;
ALTER TABLE mediatool.export_job DROP COLUMN output_kind;
