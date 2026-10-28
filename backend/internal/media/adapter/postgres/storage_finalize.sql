-- Safe finalization: explicit maintenance transaction only, never at application startup.
-- Caller must set LOCAL glowloom.storage_config_sha256 to the reviewed public config digest.
-- All old physical locations must be byte-proved/backfilled; old business FKs are replaced,
-- never dropped with CASCADE. Unknown dependencies abort the whole transaction.
SET LOCAL lock_timeout='5s';
SELECT pg_advisory_xact_lock(hashtextextended('media.storage_route',0));
LOCK TABLE media.media_asset,media.rendition,media.package_object,media.project_copy_object,media.transfer_object,media.purge_object,mediatool.depth_object,script.object_intent,script.object_state,script.script_source,script.script_version,script.copy_object_intent,script.copy_object_state,media.object_location,operation.operation,media.storage_work_intent,media.upload_admission,media.upload_request,media.package_job,media.transfer_job,media.purge_job,media.project_copy_snapshot,mediatool.depth_job,mediatool.export_job,mediatool.transcription_job,script.command,script.import_job,script.copy_snapshot,script.request,workspace.project_copy_job,subject.voice_job IN SHARE ROW EXCLUSIVE MODE;
-- First DDL runs during a maintenance drain. The Go operator additionally checks
-- exact current-attempt work tuples; a standalone SQL caller must drain all producers.
DO $drain$ BEGIN
 IF EXISTS(SELECT 1 FROM operation.operation WHERE status NOT IN ('completed','failed','cancelled','expired'))
 OR EXISTS(SELECT 1 FROM media.package_job WHERE status='needs_reconciliation')
 OR EXISTS(SELECT 1 FROM media.transfer_job WHERE status IN ('queued','running','needs_reconciliation','cancel_requested') OR execution_unconfirmed OR NOT process_ended)
 OR EXISTS(SELECT 1 FROM media.purge_job WHERE status IN ('queued','running','needs_reconciliation','cancel_requested') OR execution_unconfirmed OR NOT process_ended)
 OR EXISTS(SELECT 1 FROM mediatool.export_job WHERE status IN ('queued','running','cancel_requested') OR active_worker IS NOT NULL)
 OR EXISTS(SELECT 1 FROM mediatool.depth_job WHERE status IN ('queued','running','cancel_requested') OR active_worker IS NOT NULL OR execution_unconfirmed)
 OR EXISTS(SELECT 1 FROM mediatool.transcription_job WHERE status IN ('queued','running','cancel_requested') OR active_worker IS NOT NULL)
 OR EXISTS(SELECT 1 FROM script.command_state WHERE status='pending')
 OR EXISTS(SELECT 1 FROM workspace.project_copy_job WHERE status IN ('queued','running','cancel_requested') OR execution_unconfirmed)
 OR EXISTS(SELECT 1 FROM subject.voice_job WHERE status NOT IN ('succeeded','failed'))
 THEN RAISE EXCEPTION 'storage finalization not ready'; END IF;
END $drain$;
DO $proof$ DECLARE r record; cfg text:=current_setting('glowloom.storage_config_sha256',true); dirty boolean; initial_cfg text; BEGIN
 IF cfg IS NULL OR cfg !~ '^[a-f0-9]{64}$' THEN RAISE EXCEPTION 'storage finalization not ready'; END IF;
 IF NOT EXISTS(SELECT 1 FROM media.storage_legacy_binding b JOIN media.storage_profile p ON p.id=b.profile_id AND p.bucket=b.bucket AND p.resource_identity=b.resource_identity WHERE p.provider='minio') THEN
  SELECT e.config_sha256 INTO initial_cfg FROM media.storage_empty_deployment_evidence e JOIN media.storage_activation_guard g ON g.config_sha256=e.config_sha256 JOIN media.storage_activation a ON a.revision=g.revision ORDER BY a.activated_at,a.revision LIMIT 1;
  IF initial_cfg IS NOT NULL THEN cfg:=initial_cfg; END IF;
  IF NOT EXISTS(SELECT 1 FROM media.storage_empty_deployment_evidence WHERE config_sha256=cfg AND inventory=('{"schema_version":1,"config_sha256":"' || cfg || '","empty_tables":["media.media_asset","media.object_location","media.package_job","media.package_object","media.project_copy_object","media.project_copy_snapshot","media.purge_job","media.purge_object","media.rendition","media.storage_work_intent","media.transfer_job","media.transfer_object","media.upload_admission","media.upload_request","mediatool.depth_job","mediatool.depth_object","mediatool.export_job","mediatool.transcription_job","operation.operation","script.command","script.copy_object_intent","script.copy_object_state","script.copy_snapshot","script.import_job","script.object_intent","script.object_state","script.request","script.script_source","script.script_version","subject.voice_job","workspace.project_copy_job"]}')::jsonb AND evidence_sha256=encode(sha256(convert_to(('{"schema_version":1,"config_sha256":"' || cfg || '","empty_tables":["media.media_asset","media.object_location","media.package_job","media.package_object","media.project_copy_object","media.project_copy_snapshot","media.purge_job","media.purge_object","media.rendition","media.storage_work_intent","media.transfer_job","media.transfer_object","media.upload_admission","media.upload_request","mediatool.depth_job","mediatool.depth_object","mediatool.export_job","mediatool.transcription_job","operation.operation","script.command","script.copy_object_intent","script.copy_object_state","script.copy_snapshot","script.import_job","script.object_intent","script.object_state","script.request","script.script_source","script.script_version","subject.voice_job","workspace.project_copy_job"]}'),'UTF8')),'hex')) THEN RAISE EXCEPTION 'storage finalization not ready'; END IF;
  IF initial_cfg IS NULL THEN
  FOR r IN SELECT unnest(ARRAY['media.media_asset','media.rendition','media.package_object','media.project_copy_object','media.transfer_object','media.purge_object','mediatool.depth_object','script.object_intent','script.object_state','script.script_source','script.script_version','script.copy_object_intent','script.copy_object_state','media.object_location','operation.operation','media.storage_work_intent','media.upload_admission','media.upload_request','media.package_job','media.transfer_job','media.purge_job','media.project_copy_snapshot','mediatool.depth_job','mediatool.export_job','mediatool.transcription_job','script.command','script.import_job','script.copy_snapshot','script.request','workspace.project_copy_job','subject.voice_job']) AS tbl LOOP
   EXECUTE format('SELECT EXISTS(SELECT 1 FROM %s)',r.tbl) INTO dirty;
   IF dirty THEN RAISE EXCEPTION 'storage finalization not ready'; END IF;
  END LOOP;
  IF EXISTS(SELECT 1 FROM media.storage_route) OR EXISTS(SELECT 1 FROM media.storage_activation) OR EXISTS(SELECT 1 FROM media.storage_activation_guard) THEN RAISE EXCEPTION 'storage finalization not ready'; END IF;
  END IF;
 END IF;
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
