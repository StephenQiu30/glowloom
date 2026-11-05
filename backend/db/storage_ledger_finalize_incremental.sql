-- Final storage schema migration. Apply through the database migration pipeline.
-- Existing physical object references must be bound before location IDs are required.
SET LOCAL lock_timeout='5s';
SELECT pg_advisory_xact_lock(hashtextextended('media.storage_route',0));
LOCK TABLE media.media_asset,media.rendition,media.package_object,media.project_copy_object,media.transfer_object,media.purge_object,mediatool.depth_object,script.object_intent,script.object_state,script.script_source,script.script_version,script.copy_object_intent,script.copy_object_state,media.object_location,operation.operation,media.storage_work_intent,media.upload_admission,media.upload_request,media.package_job,media.transfer_job,media.purge_job,media.project_copy_snapshot,mediatool.depth_job,mediatool.export_job,mediatool.transcription_job,script.command,script.import_job,script.copy_snapshot,script.request,workspace.project_copy_job,subject.voice_job IN SHARE ROW EXCLUSIVE MODE;
DO $proof$ DECLARE r record; dirty boolean; BEGIN
 FOR r IN SELECT * FROM (VALUES
('media.media_asset','object_key','object_location_id'),
('media.rendition','object_key','object_location_id'),
('media.package_object','object_key','object_location_id'),
('media.project_copy_object','source_object_key','source_location_id'),
('media.project_copy_object','target_object_key','target_location_id'),
('media.transfer_object','source_object_key','source_location_id'),
('media.transfer_object','target_object_key','target_location_id'),
('media.purge_object','object_key','object_location_id'),
('mediatool.depth_object','object_key','object_location_id'),
('script.object_intent','object_key','object_location_id'),
('script.object_state','object_key','object_location_id'),
('script.script_source','original_key','original_location_id'),
('script.script_source','rich_key','rich_location_id'),
('script.script_version','text_key','text_location_id'),
('script.script_version','rich_key','rich_location_id'),
('script.copy_object_intent','source_key','source_location_id'),
('script.copy_object_intent','target_key','target_location_id'),
('script.copy_object_state','target_key','target_location_id')
 ) v(tbl,key_col,location_col) LOOP
  EXECUTE format('SELECT EXISTS(SELECT 1 FROM %s WHERE %I IS NULL)',r.tbl,r.location_col) INTO dirty;
  IF dirty THEN RAISE EXCEPTION 'storage finalization not ready'; END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_constraint c JOIN pg_attribute a ON a.attrelid=c.conrelid AND a.attname=r.location_col JOIN pg_attribute k ON k.attrelid=c.conrelid AND k.attname=r.key_col WHERE c.conrelid=to_regclass(r.tbl) AND c.contype='f' AND c.convalidated AND c.confrelid='media.object_location'::regclass AND c.conkey=ARRAY[a.attnum,k.attnum]::smallint[] AND c.confkey=ARRAY[(SELECT attnum FROM pg_attribute WHERE attrelid=c.confrelid AND attname='id'),(SELECT attnum FROM pg_attribute WHERE attrelid=c.confrelid AND attname='object_key')]::smallint[]) THEN RAISE EXCEPTION 'storage finalization not ready'; END IF;
 END LOOP;
END $proof$;
-- Preserve state-to-intent business relationships using exact physical identity.
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='script.object_intent'::regclass AND conname='object_intent_location_key') THEN ALTER TABLE script.object_intent ADD CONSTRAINT object_intent_location_key UNIQUE(object_location_id,object_key); END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='script.copy_object_intent'::regclass AND conname='copy_object_intent_location_key') THEN ALTER TABLE script.copy_object_intent ADD CONSTRAINT copy_object_intent_location_key UNIQUE(target_location_id,target_key); END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='script.object_state'::regclass AND conname='object_state_intent_location_fkey') THEN ALTER TABLE script.object_state ADD CONSTRAINT object_state_intent_location_fkey FOREIGN KEY(object_location_id,object_key) REFERENCES script.object_intent(object_location_id,object_key); END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='script.copy_object_state'::regclass AND conname='copy_object_state_intent_location_fkey') THEN ALTER TABLE script.copy_object_state ADD CONSTRAINT copy_object_state_intent_location_fkey FOREIGN KEY(target_location_id,target_key) REFERENCES script.copy_object_intent(target_location_id,target_key); END IF;
 ALTER TABLE script.object_state DROP CONSTRAINT IF EXISTS object_state_object_key_fkey;
 ALTER TABLE script.copy_object_state DROP CONSTRAINT IF EXISTS copy_object_state_target_key_fkey;
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='script.object_intent'::regclass AND conname='object_intent_command_key') THEN ALTER TABLE script.object_intent ADD CONSTRAINT object_intent_command_key UNIQUE(actor_id,request_id,object_key); END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='script.copy_object_intent'::regclass AND conname='copy_object_intent_snapshot_location_key') THEN ALTER TABLE script.copy_object_intent ADD CONSTRAINT copy_object_intent_snapshot_location_key UNIQUE(snapshot_id,source_location_id); END IF;
 ALTER TABLE script.copy_object_intent DROP CONSTRAINT IF EXISTS copy_object_intent_snapshot_id_source_key_key;
END $$;
DO $$ DECLARE r record; c record; BEGIN
 FOR r IN SELECT * FROM (VALUES
('media.media_asset','object_key','object_location_id'),
('media.rendition','object_key','object_location_id'),
('media.package_object','object_key','object_location_id'),
('media.project_copy_object','source_object_key','source_location_id'),
('media.project_copy_object','target_object_key','target_location_id'),
('media.transfer_object','source_object_key','source_location_id'),
('media.transfer_object','target_object_key','target_location_id'),
('media.purge_object','object_key','object_location_id'),
('mediatool.depth_object','object_key','object_location_id'),
('script.object_intent','object_key','object_location_id'),
('script.object_state','object_key','object_location_id'),
('script.script_source','original_key','original_location_id'),
('script.script_source','rich_key','rich_location_id'),
('script.script_version','text_key','text_location_id'),
('script.script_version','rich_key','rich_location_id'),
('script.copy_object_intent','source_key','source_location_id'),
('script.copy_object_intent','target_key','target_location_id'),
('script.copy_object_state','target_key','target_location_id')
 ) v(tbl,key_col,location_col) LOOP
  EXECUTE format('ALTER TABLE %s ALTER COLUMN %I SET NOT NULL',r.tbl,r.location_col);
  FOR c IN SELECT x.conname,x.contype FROM pg_constraint x JOIN pg_attribute a ON a.attrelid=x.conrelid AND a.attname=r.key_col WHERE x.conrelid=to_regclass(r.tbl) AND x.contype IN ('p','u') AND x.conkey=ARRAY[a.attnum]::smallint[] LOOP
   EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I',r.tbl,c.conname);
   IF c.contype='p' THEN EXECUTE format('ALTER TABLE %s ADD CONSTRAINT %I PRIMARY KEY(%I)',r.tbl,c.conname,r.location_col);
   ELSE EXECUTE format('ALTER TABLE %s ADD CONSTRAINT %I UNIQUE(%I)',r.tbl,c.conname||'_physical',r.location_col); END IF;
  END LOOP;
  FOR c IN SELECT i.indexrelid::regclass AS idx FROM pg_index i JOIN pg_attribute a ON a.attrelid=i.indrelid AND a.attname=r.key_col WHERE i.indrelid=to_regclass(r.tbl) AND i.indisunique AND i.indnkeyatts=1 AND i.indkey[0]=a.attnum AND NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conindid=i.indexrelid) LOOP
   EXECUTE format('DROP INDEX %s',c.idx);
   EXECUTE format('CREATE UNIQUE INDEX IF NOT EXISTS %I ON %s(%I)',replace(r.tbl,'.','_')||'_'||r.location_col||'_physical',r.tbl,r.location_col);
  END LOOP;
 END LOOP;
 FOR r IN SELECT unnest(ARRAY['media.project_copy_object','media.transfer_object','script.copy_object_intent']) AS tbl LOOP
  FOR c IN SELECT conname FROM pg_constraint WHERE conrelid=to_regclass(r.tbl) AND contype='c' AND ((pg_get_constraintdef(oid) LIKE '%source_object_key%<>%target_object_key%') OR (pg_get_constraintdef(oid) LIKE '%source_key%<>%target_key%')) LOOP EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I',r.tbl,c.conname); END LOOP;
  IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid=to_regclass(r.tbl) AND conname='distinct_physical_locations') THEN EXECUTE format('ALTER TABLE %s ADD CONSTRAINT distinct_physical_locations CHECK(source_location_id<>target_location_id)',r.tbl); END IF;
 END LOOP;
END $$;

CREATE TABLE IF NOT EXISTS media.storage_schema_version (
 version integer PRIMARY KEY,
 applied_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
INSERT INTO media.storage_schema_version(version) VALUES (1) ON CONFLICT (version) DO NOTHING;
GRANT SELECT ON TABLE media.storage_schema_version TO glowloom_app;
