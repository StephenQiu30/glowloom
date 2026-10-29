-- Add portable timeline artifacts without changing existing playable media outputs.
-- Apply with the schema owner. No rows or original assets are deleted.
BEGIN;
ALTER TABLE mediatool.export_job ADD COLUMN IF NOT EXISTS artifact_id uuid;
CREATE TABLE IF NOT EXISTS mediatool.export_artifact (
 id uuid PRIMARY KEY,
 job_id uuid NOT NULL REFERENCES mediatool.export_job(id),
 project_id uuid NOT NULL,
 attempt integer NOT NULL CHECK(attempt BETWEEN 1 AND 100),
 format text NOT NULL CHECK(format IN ('media_bundle','fcpxml','xmeml')),
 format_version text NOT NULL,
 file_name text NOT NULL CHECK(file_name ~ '^timeline-[a-f0-9-]{36}\.zip$'),
 mime_type text NOT NULL CHECK(mime_type='application/zip'),
 byte_size bigint NOT NULL CHECK(byte_size BETWEEN 1 AND 524288000),
 sha256 text NOT NULL CHECK(sha256 ~ '^[a-f0-9]{64}$'),
 object_key text NOT NULL,
 object_location_id uuid REFERENCES media.object_location(id),
 created_at timestamptz NOT NULL,
 UNIQUE(id,job_id,project_id,attempt),
 UNIQUE(job_id,attempt),
 CHECK((format='media_bundle' AND format_version='1') OR (format='fcpxml' AND format_version='1.10') OR (format='xmeml' AND format_version='5')),
 CHECK(object_key ~ '^projects/[a-f0-9-]{36}/exports/[0-9]{4}/[0-9]{2}/[a-f0-9-]{36}\.zip$')
);
ALTER TABLE mediatool.export_job DROP CONSTRAINT IF EXISTS export_job_output_kind_check;
ALTER TABLE mediatool.export_job ADD CONSTRAINT export_job_output_kind_check CHECK(output_kind IN ('video','audio','media_bundle','fcpxml','xmeml'));
ALTER TABLE mediatool.export_job DROP CONSTRAINT IF EXISTS export_job_check;
ALTER TABLE mediatool.export_job ADD CONSTRAINT export_job_check CHECK((asset_id IS NULL AND artifact_id IS NULL AND sha256 IS NULL) OR (num_nonnulls(asset_id,artifact_id)=1 AND sha256 IS NOT NULL));
ALTER TABLE mediatool.export_job DROP CONSTRAINT IF EXISTS export_job_check1;
ALTER TABLE mediatool.export_job ADD CONSTRAINT export_job_check1 CHECK(status NOT IN ('review_required','succeeded') OR num_nonnulls(asset_id,artifact_id)=1);
ALTER TABLE mediatool.export_job DROP CONSTRAINT IF EXISTS export_job_output_identity_check;
ALTER TABLE mediatool.export_job ADD CONSTRAINT export_job_output_identity_check CHECK((output_kind IN ('video','audio') AND artifact_id IS NULL) OR (output_kind IN ('media_bundle','fcpxml','xmeml') AND asset_id IS NULL));
ALTER TABLE mediatool.export_job DROP CONSTRAINT IF EXISTS export_job_artifact_identity_fkey;
ALTER TABLE mediatool.export_job ADD CONSTRAINT export_job_artifact_identity_fkey FOREIGN KEY(artifact_id,id,project_id,attempt) REFERENCES mediatool.export_artifact(id,job_id,project_id,attempt);
GRANT SELECT,INSERT ON mediatool.export_artifact TO glowloom_app;
GRANT UPDATE(artifact_id) ON mediatool.export_job TO glowloom_app;
COMMIT;
