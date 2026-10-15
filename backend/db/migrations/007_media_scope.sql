-- Explicit transfer ownership. Unmapped historical jobs remain quarantined.
ALTER TABLE media.transfer_job ADD COLUMN IF NOT EXISTS source_scope_id uuid;
ALTER TABLE media.transfer_job ADD CONSTRAINT transfer_source_scope_fkey FOREIGN KEY(source_scope_id) REFERENCES workspace.creation_scope(id);
ALTER TABLE media.transfer_job ADD COLUMN IF NOT EXISTS target_scope_id uuid;
ALTER TABLE media.transfer_job ADD CONSTRAINT transfer_target_scope_fkey FOREIGN KEY(target_scope_id) REFERENCES workspace.creation_scope(id);
ALTER TABLE media.transfer_job ADD COLUMN IF NOT EXISTS source_org_id uuid;
ALTER TABLE media.transfer_job ADD CONSTRAINT transfer_source_org_fkey FOREIGN KEY(source_org_id) REFERENCES workspace.organization(id);
ALTER TABLE media.transfer_job ADD COLUMN IF NOT EXISTS target_org_id uuid;
ALTER TABLE media.transfer_job ADD CONSTRAINT transfer_target_org_fkey FOREIGN KEY(target_org_id) REFERENCES workspace.organization(id);
UPDATE media.transfer_job j SET source_scope_id=s.id, source_org_id=s.physical_scope_id
FROM media.library l JOIN workspace.creation_scope s ON (l.kind='personal' AND s.kind='personal' AND s.owner_user_id=l.personal_actor_id AND s.physical_scope_id=l.org_id)
WHERE j.source_library_id=l.id AND j.source_kind='personal' AND j.actor_id=l.personal_actor_id AND j.source_scope_id IS NULL;
UPDATE media.transfer_job j SET target_scope_id=s.id, target_org_id=s.physical_scope_id
FROM media.library l JOIN workspace.creation_scope s ON (l.kind='personal' AND s.kind='personal' AND s.owner_user_id=l.personal_actor_id AND s.physical_scope_id=l.org_id)
WHERE j.target_library_id=l.id AND j.target_kind='personal' AND j.actor_id=l.personal_actor_id AND j.target_scope_id IS NULL;
UPDATE media.transfer_job j SET source_scope_id=p.owner_scope_id,source_org_id=p.org_id FROM workspace.project p
WHERE j.source_project_id=p.id AND p.owner_scope_id IS NOT NULL AND j.source_scope_id IS NULL;
UPDATE media.transfer_job j SET target_scope_id=p.owner_scope_id,target_org_id=p.org_id FROM workspace.project p
WHERE j.target_project_id=p.id AND p.owner_scope_id IS NOT NULL AND j.target_scope_id IS NULL;
ALTER TABLE mediatool.export_job ALTER COLUMN actor_role DROP NOT NULL;
ALTER TABLE mediatool.transcription_job ALTER COLUMN actor_role DROP NOT NULL;
ALTER TABLE mediatool.depth_job ALTER COLUMN actor_role DROP NOT NULL;

CREATE OR REPLACE FUNCTION media.check_transfer_scope() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE source media.library%ROWTYPE; target media.library%ROWTYPE;
BEGIN
 SELECT * INTO STRICT source FROM media.library WHERE id=NEW.source_library_id;
 SELECT * INTO STRICT target FROM media.library WHERE id=NEW.target_library_id;
 IF NEW.source_scope_id IS NULL OR NEW.target_scope_id IS NULL OR NEW.source_org_id IS NULL OR NEW.target_org_id IS NULL
 OR source.org_id<>NEW.source_org_id OR target.org_id<>NEW.target_org_id OR source.kind<>NEW.source_kind OR target.kind<>NEW.target_kind
 OR source.project_id IS DISTINCT FROM NEW.source_project_id OR target.project_id IS DISTINCT FROM NEW.target_project_id
 OR (source.kind='personal' AND source.personal_actor_id<>NEW.actor_id) OR (target.kind='personal' AND target.personal_actor_id<>NEW.actor_id)
 OR NOT EXISTS(SELECT 1 FROM workspace.creation_scope s WHERE s.id=NEW.source_scope_id AND s.physical_scope_id=source.org_id AND
 ((source.kind='personal' AND s.kind='personal' AND s.owner_user_id=NEW.actor_id) OR (source.kind='project' AND EXISTS(SELECT 1 FROM workspace.project p WHERE p.id=source.project_id AND p.owner_scope_id=s.id))))
 OR NOT EXISTS(SELECT 1 FROM workspace.creation_scope s WHERE s.id=NEW.target_scope_id AND s.physical_scope_id=target.org_id AND
 ((target.kind='personal' AND s.kind='personal' AND s.owner_user_id=NEW.actor_id) OR (target.kind='project' AND EXISTS(SELECT 1 FROM workspace.project p WHERE p.id=target.project_id AND p.owner_scope_id=s.id))))
 OR (source.kind='project' AND source.org_id<>NEW.org_id) OR (target.kind='project' AND target.org_id<>NEW.org_id)
 THEN RAISE EXCEPTION 'media transfer scope mismatch' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;

ALTER TABLE media.library_command DROP CONSTRAINT IF EXISTS library_command_org_id_actor_id_fkey;
ALTER TABLE media.library_command ADD CONSTRAINT library_command_actor_scope_user_fk FOREIGN KEY(actor_id) REFERENCES identity."user"(id);
ALTER TABLE media.library_command ADD CONSTRAINT library_command_physical_org_fk FOREIGN KEY(org_id) REFERENCES workspace.organization(id);

DO $$ BEGIN IF to_regclass('media.package_command') IS NOT NULL THEN
ALTER TABLE media.package_command DROP CONSTRAINT IF EXISTS package_command_org_id_actor_id_fkey;
ALTER TABLE media.package_command ADD CONSTRAINT package_command_actor_scope_user_fk FOREIGN KEY(actor_id) REFERENCES identity."user"(id);
ALTER TABLE media.package_command ADD CONSTRAINT package_command_physical_org_fk FOREIGN KEY(org_id) REFERENCES workspace.organization(id);
END IF; END $$;

DO $$ BEGIN IF to_regclass('media.package_job') IS NOT NULL THEN
ALTER TABLE media.package_job DROP CONSTRAINT IF EXISTS package_job_org_id_actor_id_fkey;
ALTER TABLE media.package_job ADD CONSTRAINT package_job_actor_scope_user_fk FOREIGN KEY(actor_id) REFERENCES identity."user"(id);
ALTER TABLE media.package_job ADD CONSTRAINT package_job_physical_org_fk FOREIGN KEY(org_id) REFERENCES workspace.organization(id);
END IF; END $$;

ALTER TABLE media.purge_command DROP CONSTRAINT IF EXISTS purge_command_org_id_actor_id_fkey;
ALTER TABLE media.purge_command ADD CONSTRAINT purge_command_actor_scope_user_fk FOREIGN KEY(actor_id) REFERENCES identity."user"(id);
ALTER TABLE media.purge_command ADD CONSTRAINT purge_command_physical_org_fk FOREIGN KEY(org_id) REFERENCES workspace.organization(id);

ALTER TABLE media.purge_job DROP CONSTRAINT IF EXISTS purge_job_org_id_actor_id_fkey;
ALTER TABLE media.purge_job ADD CONSTRAINT purge_job_actor_scope_user_fk FOREIGN KEY(actor_id) REFERENCES identity."user"(id);
ALTER TABLE media.purge_job ADD CONSTRAINT purge_job_physical_org_fk FOREIGN KEY(org_id) REFERENCES workspace.organization(id);

ALTER TABLE media.transfer_command DROP CONSTRAINT IF EXISTS transfer_command_org_id_actor_id_fkey;
ALTER TABLE media.transfer_command ADD CONSTRAINT transfer_command_actor_scope_user_fk FOREIGN KEY(actor_id) REFERENCES identity."user"(id);
ALTER TABLE media.transfer_command ADD CONSTRAINT transfer_command_physical_org_fk FOREIGN KEY(org_id) REFERENCES workspace.organization(id);

ALTER TABLE media.transfer_job DROP CONSTRAINT IF EXISTS transfer_job_org_id_actor_id_fkey;
ALTER TABLE media.transfer_job ADD CONSTRAINT transfer_job_actor_scope_user_fk FOREIGN KEY(actor_id) REFERENCES identity."user"(id);
ALTER TABLE media.transfer_job ADD CONSTRAINT transfer_job_physical_org_fk FOREIGN KEY(org_id) REFERENCES workspace.organization(id);

CREATE OR REPLACE FUNCTION media.protect_transfer_ownership() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF ROW(NEW.actor_id,NEW.org_id,NEW.source_library_id,NEW.target_library_id,NEW.source_kind,NEW.target_kind,NEW.source_project_id,NEW.target_project_id,NEW.source_scope_id,NEW.target_scope_id,NEW.source_org_id,NEW.target_org_id)
 IS DISTINCT FROM ROW(OLD.actor_id,OLD.org_id,OLD.source_library_id,OLD.target_library_id,OLD.source_kind,OLD.target_kind,OLD.source_project_id,OLD.target_project_id,OLD.source_scope_id,OLD.target_scope_id,OLD.source_org_id,OLD.target_org_id)
 THEN RAISE EXCEPTION 'media transfer ownership is immutable' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER transfer_ownership_immutable BEFORE UPDATE ON media.transfer_job FOR EACH ROW EXECUTE FUNCTION media.protect_transfer_ownership();
