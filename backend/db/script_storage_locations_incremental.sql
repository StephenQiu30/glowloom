-- Script object rows require exact physical identities. Existing rows without them fail; no key-based inference is performed.

ALTER TABLE script.script_source ADD COLUMN IF NOT EXISTS original_location_id uuid NOT NULL;
ALTER TABLE script.script_source ALTER COLUMN original_location_id SET NOT NULL;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='script.script_source'::regclass AND conname='script_source_original_location_id_fkey') THEN
 ALTER TABLE script.script_source ADD CONSTRAINT script_source_original_location_id_fkey FOREIGN KEY(original_location_id,original_key) REFERENCES media.object_location(id,object_key);
 END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='script.script_source'::regclass AND tgname='keep_original_location_id' AND NOT tgisinternal) THEN
 CREATE TRIGGER keep_original_location_id BEFORE UPDATE ON script.script_source FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('original_location_id');
 END IF;
END $$;

ALTER TABLE script.script_source ADD COLUMN IF NOT EXISTS rich_location_id uuid NOT NULL;
ALTER TABLE script.script_source ALTER COLUMN rich_location_id SET NOT NULL;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='script.script_source'::regclass AND conname='script_source_rich_location_id_fkey') THEN
 ALTER TABLE script.script_source ADD CONSTRAINT script_source_rich_location_id_fkey FOREIGN KEY(rich_location_id,rich_key) REFERENCES media.object_location(id,object_key);
 END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='script.script_source'::regclass AND tgname='keep_rich_location_id' AND NOT tgisinternal) THEN
 CREATE TRIGGER keep_rich_location_id BEFORE UPDATE ON script.script_source FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('rich_location_id');
 END IF;
END $$;

ALTER TABLE script.script_version ADD COLUMN IF NOT EXISTS text_location_id uuid NOT NULL;
ALTER TABLE script.script_version ALTER COLUMN text_location_id SET NOT NULL;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='script.script_version'::regclass AND conname='script_version_text_location_id_fkey') THEN
 ALTER TABLE script.script_version ADD CONSTRAINT script_version_text_location_id_fkey FOREIGN KEY(text_location_id,text_key) REFERENCES media.object_location(id,object_key);
 END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='script.script_version'::regclass AND tgname='keep_text_location_id' AND NOT tgisinternal) THEN
 CREATE TRIGGER keep_text_location_id BEFORE UPDATE ON script.script_version FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('text_location_id');
 END IF;
END $$;

ALTER TABLE script.script_version ADD COLUMN IF NOT EXISTS rich_location_id uuid NOT NULL;
ALTER TABLE script.script_version ALTER COLUMN rich_location_id SET NOT NULL;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='script.script_version'::regclass AND conname='script_version_rich_location_id_fkey') THEN
 ALTER TABLE script.script_version ADD CONSTRAINT script_version_rich_location_id_fkey FOREIGN KEY(rich_location_id,rich_key) REFERENCES media.object_location(id,object_key);
 END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='script.script_version'::regclass AND tgname='keep_rich_location_id' AND NOT tgisinternal) THEN
 CREATE TRIGGER keep_rich_location_id BEFORE UPDATE ON script.script_version FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('rich_location_id');
 END IF;
END $$;

ALTER TABLE script.copy_object_intent ADD COLUMN IF NOT EXISTS source_location_id uuid NOT NULL;
ALTER TABLE script.copy_object_intent ALTER COLUMN source_location_id SET NOT NULL;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='script.copy_object_intent'::regclass AND conname='copy_object_intent_source_location_id_fkey') THEN
 ALTER TABLE script.copy_object_intent ADD CONSTRAINT copy_object_intent_source_location_id_fkey FOREIGN KEY(source_location_id,source_key) REFERENCES media.object_location(id,object_key);
 END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='script.copy_object_intent'::regclass AND tgname='keep_source_location_id' AND NOT tgisinternal) THEN
 CREATE TRIGGER keep_source_location_id BEFORE UPDATE ON script.copy_object_intent FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('source_location_id');
 END IF;
END $$;

ALTER TABLE script.copy_object_intent ADD COLUMN IF NOT EXISTS target_location_id uuid NOT NULL;
ALTER TABLE script.copy_object_intent ALTER COLUMN target_location_id SET NOT NULL;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='script.copy_object_intent'::regclass AND conname='copy_object_intent_target_location_id_fkey') THEN
 ALTER TABLE script.copy_object_intent ADD CONSTRAINT copy_object_intent_target_location_id_fkey FOREIGN KEY(target_location_id,target_key) REFERENCES media.object_location(id,object_key);
 END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='script.copy_object_intent'::regclass AND tgname='keep_target_location_id' AND NOT tgisinternal) THEN
 CREATE TRIGGER keep_target_location_id BEFORE UPDATE ON script.copy_object_intent FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('target_location_id');
 END IF;
END $$;

ALTER TABLE script.copy_object_state ADD COLUMN IF NOT EXISTS target_location_id uuid NOT NULL;
ALTER TABLE script.copy_object_state ALTER COLUMN target_location_id SET NOT NULL;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='script.copy_object_state'::regclass AND conname='copy_object_state_target_location_id_fkey') THEN
 ALTER TABLE script.copy_object_state ADD CONSTRAINT copy_object_state_target_location_id_fkey FOREIGN KEY(target_location_id,target_key) REFERENCES media.object_location(id,object_key);
 END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='script.copy_object_state'::regclass AND tgname='keep_target_location_id' AND NOT tgisinternal) THEN
 CREATE TRIGGER keep_target_location_id BEFORE UPDATE ON script.copy_object_state FOR EACH ROW EXECUTE FUNCTION media.keep_object_binding('target_location_id');
 END IF;
END $$;

ALTER TABLE script.copy_snapshot ADD COLUMN IF NOT EXISTS storage_intent_id uuid REFERENCES media.storage_work_intent(id);
