-- Never discard frozen history evidence or a pending script checkpoint.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM workspace.project_copy_job WHERE manifest ? 'script' OR script_receipt IS NOT NULL OR stage='script') THEN
  RAISE EXCEPTION 'script copy evidence exists; downgrade refused';
 END IF;
END $$;
REVOKE UPDATE(script_receipt) ON workspace.project_copy_job FROM lanverse_app;
ALTER TABLE workspace.project_copy_job DROP COLUMN script_receipt;
ALTER TABLE workspace.project_copy_job DROP CONSTRAINT project_copy_job_stage_check;
ALTER TABLE workspace.project_copy_job ADD CONSTRAINT project_copy_job_stage_check
 CHECK (stage IN ('media','canvases','finalizing','cleanup','complete'));
