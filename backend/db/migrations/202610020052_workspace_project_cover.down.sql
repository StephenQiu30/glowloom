DO $$
BEGIN
 IF EXISTS(SELECT 1 FROM workspace.project_change_command) OR EXISTS(SELECT 1 FROM workspace.project WHERE cover_asset_id IS NOT NULL) THEN
  RAISE EXCEPTION 'project change receipts or cover bindings require retention; rollback refused' USING ERRCODE='55000';
 END IF;
END;
$$;
DROP TABLE workspace.project_change_command;
DROP FUNCTION workspace.reject_project_change_command_mutation();
ALTER TABLE workspace.project DROP COLUMN cover_asset_id;
