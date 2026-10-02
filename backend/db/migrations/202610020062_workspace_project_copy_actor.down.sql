DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM workspace.project_copy_job
             WHERE execution_actor_id IS NOT NULL AND execution_actor_id <> actor_id) THEN
    RAISE EXCEPTION 'project copy control actor history requires preservation';
  END IF;
END $$;
REVOKE UPDATE(execution_actor_id) ON workspace.project_copy_job FROM lanverse_app;
ALTER TABLE workspace.project_copy_job DROP COLUMN execution_actor_id;
