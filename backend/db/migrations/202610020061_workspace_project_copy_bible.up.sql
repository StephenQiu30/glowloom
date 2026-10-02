-- Existing nil-Bible admission responses and immutable manifests retain their bytes.
ALTER TABLE workspace.project_copy_job ADD COLUMN bible_receipt jsonb
 CHECK (bible_receipt IS NULL OR jsonb_typeof(bible_receipt)='object');
ALTER TABLE workspace.project_copy_job DROP CONSTRAINT project_copy_job_stage_check;
ALTER TABLE workspace.project_copy_job ADD CONSTRAINT project_copy_job_stage_check
 CHECK (stage IN ('media','bible','script','canvases','finalizing','cleanup','complete'));
GRANT UPDATE(bible_receipt) ON workspace.project_copy_job TO lanverse_app;
