-- Explicitly reviewed incremental storage write fencing; never run at startup.
ALTER TABLE media.object_location ADD COLUMN IF NOT EXISTS write_until timestamptz;
CREATE OR REPLACE FUNCTION media.keep_storage_write_deadline() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.write_until IS NOT NULL AND (NEW.write_until IS NULL OR NEW.write_until<OLD.write_until) THEN
  RAISE EXCEPTION 'storage write deadline cannot regress' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.object_location'::regclass AND tgname='keep_storage_write_deadline') THEN
  CREATE TRIGGER keep_storage_write_deadline BEFORE UPDATE ON media.object_location FOR EACH ROW EXECUTE FUNCTION media.keep_storage_write_deadline();
 END IF;
END $$;
