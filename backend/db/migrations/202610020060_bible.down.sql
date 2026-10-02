-- All Bible content is task-owned; rollback removes its schema only.
ALTER TABLE script.dialogue_line DROP COLUMN character_version_id;
DROP SCHEMA IF EXISTS bible CASCADE;
