-- Apply after media_project_export_incremental.sql, with the schema owner.
BEGIN;
-- Native timeline draft ZIPs use a distinct serializer, never a renamed XML archive.
ALTER TABLE mediatool.export_artifact DROP CONSTRAINT IF EXISTS export_artifact_format_check;
ALTER TABLE mediatool.export_artifact ADD CONSTRAINT export_artifact_format_check CHECK(format IN ('media_bundle','fcpxml','xmeml','jianying'));
ALTER TABLE mediatool.export_artifact DROP CONSTRAINT IF EXISTS export_artifact_check;
ALTER TABLE mediatool.export_artifact ADD CONSTRAINT export_artifact_check CHECK((format='media_bundle' AND format_version='1') OR (format='fcpxml' AND format_version='1.10') OR (format='xmeml' AND format_version='5') OR (format='jianying' AND format_version='134.0.0'));
ALTER TABLE mediatool.export_job DROP CONSTRAINT IF EXISTS export_job_output_kind_check;
ALTER TABLE mediatool.export_job ADD CONSTRAINT export_job_output_kind_check CHECK(output_kind IN ('video','audio','media_bundle','fcpxml','xmeml','jianying'));
ALTER TABLE mediatool.export_job DROP CONSTRAINT IF EXISTS export_job_output_identity_check;
ALTER TABLE mediatool.export_job ADD CONSTRAINT export_job_output_identity_check CHECK((output_kind IN ('video','audio') AND artifact_id IS NULL) OR (output_kind IN ('media_bundle','fcpxml','xmeml','jianying') AND asset_id IS NULL));

-- Native-app capability: private signed envelope, fixed expiry and no re-signing.
CREATE TABLE IF NOT EXISTS mediatool.export_launch (
 token_hash text PRIMARY KEY CHECK(token_hash ~ '^[a-f0-9]{64}$'),
 job_id uuid NOT NULL REFERENCES mediatool.export_job(id) ON DELETE CASCADE,
 artifact_id uuid NOT NULL REFERENCES mediatool.export_artifact(id) ON DELETE CASCADE,
 project_id uuid NOT NULL, revision bigint NOT NULL CHECK(revision>0),
 actor jsonb NOT NULL, envelope jsonb NOT NULL CHECK(octet_length(envelope::text)<=16384),
 expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(expires_at>created_at AND expires_at<=created_at+interval '10 minutes')
);
CREATE INDEX IF NOT EXISTS export_launch_expiry_idx ON mediatool.export_launch(expires_at);
GRANT SELECT,INSERT,DELETE ON mediatool.export_launch TO glowloom_app;
COMMIT;
