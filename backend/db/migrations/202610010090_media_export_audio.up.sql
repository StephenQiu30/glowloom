-- Output selection is a frozen creation fact. Existing timeline exports are
-- video, and the application role receives no permission to mutate this column.
ALTER TABLE mediatool.export_job ADD COLUMN output_kind text NOT NULL DEFAULT 'video'
 CHECK(output_kind IN ('video','audio'));
