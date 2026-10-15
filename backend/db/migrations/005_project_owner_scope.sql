-- Expansion only. The caller owns the transaction and bounded lock timeout.
-- Preserve physical storage and author IDs; infer personal ownership only from
-- the existing explicit owner, never from an organization name or shared key.
ALTER TABLE workspace.creation_scope
    ADD CONSTRAINT creation_scope_physical_unique UNIQUE (id,physical_scope_id);
ALTER TABLE workspace.project
    ADD COLUMN owner_scope_id uuid,
    ADD CONSTRAINT project_owner_scope_fk FOREIGN KEY (owner_scope_id,org_id)
        REFERENCES workspace.creation_scope(id,physical_scope_id) ON DELETE RESTRICT;
UPDATE workspace.project p SET owner_scope_id=s.id
FROM workspace.creation_scope s
WHERE p.owner_scope_id IS NULL AND s.kind='personal'
    AND p.owner_id=s.owner_user_id AND p.org_id=s.physical_scope_id;
CREATE INDEX project_owner_scope_idx ON workspace.project(owner_scope_id,is_delete,update_time DESC,id DESC);
-- Unmapped legacy projects remain null and must be denied by the scoped reader.

-- Project presets inherit their explicit project scope. Legacy organization-wide
-- presets have no verified owner; keep them inaccessible until separately mapped.
ALTER TABLE workspace.style_preset
 ADD COLUMN owner_scope_id uuid,
 ADD CONSTRAINT style_preset_owner_scope_fk FOREIGN KEY(owner_scope_id,org_id)
 REFERENCES workspace.creation_scope(id,physical_scope_id) ON DELETE RESTRICT;
UPDATE workspace.style_preset s SET owner_scope_id=p.owner_scope_id
FROM workspace.project p WHERE s.project_id=p.id AND s.org_id=p.org_id;
CREATE INDEX style_preset_owner_scope_idx ON workspace.style_preset(owner_scope_id,is_delete);

-- New imports no longer snapshot a synthetic global content role. Keep historic
-- values for audit compatibility; authorization uses current project scope.
ALTER TABLE script.import_job ALTER COLUMN actor_role DROP NOT NULL;
