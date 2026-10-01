DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM mediatool.depth_job WHERE active_worker IS NOT NULL OR needs_reconciliation OR status IN ('queued','running','cancel_requested','review_required')) THEN
  RAISE EXCEPTION 'depth jobs are active or retain unresolved private results';
 END IF;
END $$;
DROP TABLE mediatool.depth_command,mediatool.depth_object,mediatool.depth_result_intent,mediatool.depth_job;
