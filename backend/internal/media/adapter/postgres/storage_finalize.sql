-- Verify the strict physical object schema. Existing nullable or key-only data fails closed.
SET LOCAL lock_timeout='5s';
LOCK TABLE media.media_asset,media.rendition,media.package_object,media.project_copy_object,media.transfer_object,media.purge_object,mediatool.depth_object,script.object_intent,script.object_state,script.script_source,script.script_version,script.copy_object_intent,script.copy_object_state IN SHARE MODE;
DO $physical_refs$ DECLARE r record; missing boolean; BEGIN
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
 ) refs(table_name,key_column,location_column) LOOP
  EXECUTE format('SELECT EXISTS(SELECT 1 FROM %s WHERE %I IS NULL)',r.table_name,r.location_column) INTO missing;
  IF missing THEN RAISE EXCEPTION 'object location reference is required'; END IF;
  IF NOT EXISTS(
   SELECT 1 FROM pg_constraint c
   JOIN pg_attribute location_attr ON location_attr.attrelid=c.conrelid AND location_attr.attname=r.location_column
   JOIN pg_attribute key_attr ON key_attr.attrelid=c.conrelid AND key_attr.attname=r.key_column
   WHERE c.conrelid=to_regclass(r.table_name) AND c.contype='f' AND c.convalidated
    AND c.confrelid='media.object_location'::regclass
    AND c.conkey=ARRAY[location_attr.attnum,key_attr.attnum]::smallint[]
    AND c.confkey=ARRAY[
     (SELECT attnum FROM pg_attribute WHERE attrelid=c.confrelid AND attname='id'),
     (SELECT attnum FROM pg_attribute WHERE attrelid=c.confrelid AND attname='object_key')
    ]::smallint[]
  ) THEN RAISE EXCEPTION 'object location foreign key is required'; END IF;
 END LOOP;
END $physical_refs$;
