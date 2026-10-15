-- Caller owns the transaction. Unknown historical ownership remains quarantined.
ALTER TABLE workspace.project_folder ADD COLUMN owner_scope_id uuid;
ALTER TABLE workspace.project_folder_command ADD COLUMN owner_scope_id uuid;
ALTER TABLE workspace.project_folder_placement ADD COLUMN owner_scope_id uuid;
ALTER TABLE workspace.project_copy_job ADD COLUMN owner_scope_id uuid;

-- A personal directory can be mapped only when every retained member and cover
-- proves the same personal scope. Never infer team ownership from an author.
UPDATE workspace.project_folder f SET owner_scope_id=s.id
FROM workspace.creation_scope s WHERE s.kind='personal' AND s.owner_user_id=f.actor_id AND s.physical_scope_id=f.org_id
AND NOT EXISTS(SELECT 1 FROM workspace.project_folder_placement m JOIN workspace.project p ON p.id=m.project_id WHERE m.folder_id=f.id AND p.owner_scope_id IS DISTINCT FROM s.id)
AND (f.cover_project_id IS NULL OR EXISTS(SELECT 1 FROM workspace.project p WHERE p.id=f.cover_project_id AND p.owner_scope_id=s.id));
UPDATE workspace.project_folder_placement m SET owner_scope_id=p.owner_scope_id
FROM workspace.project p WHERE p.id=m.project_id AND p.org_id=m.org_id AND p.owner_scope_id IS NOT NULL
AND (m.folder_id IS NULL OR EXISTS(SELECT 1 FROM workspace.project_folder f WHERE f.id=m.folder_id AND f.owner_scope_id=p.owner_scope_id));
UPDATE workspace.project_copy_job j SET owner_scope_id=p.owner_scope_id
FROM workspace.project p,workspace.project t WHERE p.id=j.source_project_id AND t.id=j.target_project_id
AND p.org_id=j.org_id AND t.org_id=j.org_id AND p.owner_scope_id=t.owner_scope_id;
-- Historic folder receipts lack explicit selected scope; preserve bytes and deny replay.

ALTER TABLE workspace.project_folder ADD CONSTRAINT project_folder_scope_fk FOREIGN KEY(owner_scope_id,org_id) REFERENCES workspace.creation_scope(id,physical_scope_id);
ALTER TABLE workspace.project_folder_command ADD CONSTRAINT project_folder_command_scope_fk FOREIGN KEY(owner_scope_id,org_id) REFERENCES workspace.creation_scope(id,physical_scope_id);
ALTER TABLE workspace.project_folder_placement ADD CONSTRAINT project_folder_placement_scope_fk FOREIGN KEY(owner_scope_id,org_id) REFERENCES workspace.creation_scope(id,physical_scope_id);
ALTER TABLE workspace.project_copy_job ADD CONSTRAINT project_copy_job_scope_fk FOREIGN KEY(owner_scope_id,org_id) REFERENCES workspace.creation_scope(id,physical_scope_id);
CREATE INDEX project_folder_scope_actor_idx ON workspace.project_folder(owner_scope_id,actor_id,update_time DESC,id DESC);
CREATE INDEX project_copy_scope_idx ON workspace.project_copy_job(owner_scope_id,source_project_id,create_time DESC,id DESC);

CREATE FUNCTION workspace.validate_secondary_scope() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='project_copy_job' THEN
  IF TG_OP='UPDATE' AND (NEW.owner_scope_id,NEW.org_id,NEW.actor_id,NEW.source_project_id,NEW.target_project_id) IS DISTINCT FROM (OLD.owner_scope_id,OLD.org_id,OLD.actor_id,OLD.source_project_id,OLD.target_project_id) THEN
   RAISE EXCEPTION 'copy scope binding is immutable';
  END IF;
  IF TG_OP='INSERT' AND (NEW.owner_scope_id IS NULL OR NOT EXISTS(SELECT 1 FROM workspace.project p JOIN workspace.project t ON t.id=NEW.target_project_id WHERE p.id=NEW.source_project_id AND p.org_id=NEW.org_id AND t.org_id=NEW.org_id AND p.owner_scope_id=NEW.owner_scope_id AND t.owner_scope_id=NEW.owner_scope_id)) THEN
   RAISE EXCEPTION 'copy scope binding invalid';
  END IF;
 ELSE
  IF NEW.owner_scope_id IS NULL OR NOT EXISTS(SELECT 1 FROM workspace.project p WHERE p.id=NEW.project_id AND p.org_id=NEW.org_id AND p.owner_scope_id=NEW.owner_scope_id) OR (NEW.folder_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM workspace.project_folder f WHERE f.id=NEW.folder_id AND f.org_id=NEW.org_id AND f.actor_id=NEW.actor_id AND f.owner_scope_id=NEW.owner_scope_id)) THEN
   RAISE EXCEPTION 'folder placement scope binding invalid';
  END IF;
  IF TG_OP='UPDATE' AND (NEW.owner_scope_id,NEW.org_id,NEW.actor_id,NEW.project_id) IS DISTINCT FROM (OLD.owner_scope_id,OLD.org_id,OLD.actor_id,OLD.project_id) THEN
   RAISE EXCEPTION 'folder placement scope binding is immutable';
  END IF;
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER project_copy_scope_binding BEFORE INSERT OR UPDATE ON workspace.project_copy_job FOR EACH ROW EXECUTE FUNCTION workspace.validate_secondary_scope();
CREATE TRIGGER project_folder_placement_scope_binding BEFORE INSERT OR UPDATE ON workspace.project_folder_placement FOR EACH ROW EXECUTE FUNCTION workspace.validate_secondary_scope();
