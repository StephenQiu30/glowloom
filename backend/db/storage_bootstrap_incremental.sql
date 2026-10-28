-- Empty deployment proof contains a measured closed inventory, not a provider claim.
-- Execute explicitly in one transaction. It does not prove a live cloud bucket.
CREATE TABLE IF NOT EXISTS media.storage_empty_deployment_evidence (
 config_sha256 text PRIMARY KEY CHECK(config_sha256 ~ '^[a-f0-9]{64}$'),
 evidence_sha256 text NOT NULL CHECK(evidence_sha256 ~ '^[a-f0-9]{64}$'),
 inventory jsonb NOT NULL CHECK(jsonb_typeof(inventory)='object'),
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.storage_empty_deployment_evidence'::regclass AND tgname='keep_storage_evidence' AND NOT tgisinternal) THEN
 CREATE TRIGGER keep_storage_evidence BEFORE UPDATE OR DELETE ON media.storage_empty_deployment_evidence FOR EACH ROW EXECUTE FUNCTION media.keep_storage_evidence();
 END IF;
END $$;
GRANT SELECT ON media.storage_empty_deployment_evidence TO glowloom_app;
