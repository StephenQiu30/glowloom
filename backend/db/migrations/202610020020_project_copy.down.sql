-- Do not downgrade while unpublished copy targets exist.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM workspace.project WHERE status='copying') THEN
  RAISE EXCEPTION 'unfinished project copies prevent downgrade';
 END IF;
END $$;
DROP TABLE media.project_copy_receipt,media.project_copy_object,media.project_copy_snapshot;
DROP TABLE canvas.project_copy_receipt,canvas.project_copy_snapshot;
DROP TABLE workspace.project_copy_command,workspace.project_copy_job;
ALTER TABLE workspace.project DROP CONSTRAINT project_status_check;
ALTER TABLE workspace.project ADD CONSTRAINT project_status_check CHECK(status IN ('active','archived'));
