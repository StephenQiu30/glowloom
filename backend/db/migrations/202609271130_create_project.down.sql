DROP TRIGGER trg_project_protect_update ON workspace.project;
DROP FUNCTION workspace.protect_project_update();
ALTER TABLE workspace.style_preset DROP CONSTRAINT fk_style_preset_project;
DROP TABLE workspace.project;
