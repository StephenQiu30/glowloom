-- A downgrade must not discard admitted complete-history evidence or its checkpoint.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM workspace.project_copy_job WHERE manifest ? 'bible' OR bible_receipt IS NOT NULL OR stage='bible') THEN
  RAISE EXCEPTION 'Bible copy evidence exists; downgrade refused';
 END IF;
END $$;
REVOKE UPDATE(bible_receipt) ON workspace.project_copy_job FROM lanverse_app;
ALTER TABLE workspace.project_copy_job DROP COLUMN bible_receipt;
ALTER TABLE workspace.project_copy_job DROP CONSTRAINT project_copy_job_stage_check;
ALTER TABLE workspace.project_copy_job ADD CONSTRAINT project_copy_job_stage_check
 CHECK (stage IN ('media','script','canvases','finalizing','cleanup','complete'));
