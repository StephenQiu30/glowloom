-- The immutable creator remains provenance; authorized commands select the next actor.
ALTER TABLE workspace.project_copy_job
  ADD COLUMN execution_actor_id uuid REFERENCES identity."user"(id);
GRANT UPDATE(execution_actor_id) ON workspace.project_copy_job TO lanverse_app;
