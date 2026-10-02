-- Existing immutable admission responses and manifests retain their nil script bytes.
ALTER TABLE workspace.project_copy_job ADD COLUMN script_receipt jsonb
 CHECK (script_receipt IS NULL OR jsonb_typeof(script_receipt)='object');
ALTER TABLE workspace.project_copy_job DROP CONSTRAINT project_copy_job_stage_check;
ALTER TABLE workspace.project_copy_job ADD CONSTRAINT project_copy_job_stage_check
 CHECK (stage IN ('media','script','canvases','finalizing','cleanup','complete'));
GRANT UPDATE(script_receipt) ON workspace.project_copy_job TO lanverse_app;
