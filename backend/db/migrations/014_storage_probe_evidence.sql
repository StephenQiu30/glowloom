-- Probe ownership is separate from reviewed bucket admission evidence.
CREATE TABLE IF NOT EXISTS media.storage_probe_intent (
 id uuid PRIMARY KEY,
 profile_id text NOT NULL,
 bucket text NOT NULL,
 config_sha256 text NOT NULL CHECK(config_sha256 ~ '^[a-f0-9]{64}$'),
 sentinel_key text NOT NULL,
 removal_key text,
 mode text NOT NULL CHECK(mode IN ('read_only','read_write')),
 state text NOT NULL DEFAULT 'running' CHECK(state IN ('running','complete','cleaning','cleaned')),
 evidence_sha256 text CHECK(evidence_sha256 ~ '^[a-f0-9]{64}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(profile_id,bucket) REFERENCES media.storage_profile(id,bucket),
 CHECK((mode='read_write' AND removal_key=sentinel_key||'.remove') OR (mode='read_only' AND removal_key IS NULL))
);
CREATE OR REPLACE FUNCTION media.keep_storage_probe_intent() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (to_jsonb(OLD)-'state'-'evidence_sha256'-'updated_at') IS DISTINCT FROM (to_jsonb(NEW)-'state'-'evidence_sha256'-'updated_at')
  OR (OLD.state='running' AND NEW.state NOT IN ('running','complete','cleaning'))
  OR (OLD.state='complete' AND NEW.state<>'complete')
  OR (OLD.state='cleaning' AND NEW.state NOT IN ('cleaning','cleaned'))
  OR (OLD.state='cleaned' AND NEW.state<>'cleaned')
  OR (OLD.evidence_sha256 IS NOT NULL AND OLD.evidence_sha256 IS DISTINCT FROM NEW.evidence_sha256) THEN
  RAISE EXCEPTION 'probe identity/outcome cannot be rebound' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.storage_probe_intent'::regclass AND tgname='keep_storage_probe_intent') THEN
  CREATE TRIGGER keep_storage_probe_intent BEFORE UPDATE ON media.storage_probe_intent FOR EACH ROW EXECUTE FUNCTION media.keep_storage_probe_intent();
 END IF;
END $$;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='media.storage_profile'::regclass AND conname='storage_profile_resource_pair_key') THEN
  ALTER TABLE media.storage_profile ADD CONSTRAINT storage_profile_resource_pair_key UNIQUE(id,resource_identity);
 END IF;
END $$;
CREATE TABLE IF NOT EXISTS media.storage_bucket_evidence (
 id uuid PRIMARY KEY,
 profile_id text NOT NULL,
 resource_identity text NOT NULL,
 config_sha256 text NOT NULL CHECK(config_sha256 ~ '^[a-f0-9]{64}$'),
 evidence_sha256 text NOT NULL CHECK(evidence_sha256 ~ '^[a-f0-9]{64}$'),
 contract text NOT NULL CHECK(contract='private-flat-never-versioned-standard-v1'),
 expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(profile_id,resource_identity) REFERENCES media.storage_profile(id,resource_identity)
);
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.storage_bucket_evidence'::regclass AND tgname='keep_storage_evidence') THEN
  CREATE TRIGGER keep_storage_evidence BEFORE UPDATE OR DELETE ON media.storage_bucket_evidence FOR EACH ROW EXECUTE FUNCTION media.keep_storage_evidence();
 END IF;
END $$;
GRANT SELECT,INSERT,UPDATE ON media.storage_probe_intent TO lanverse_app;
GRANT SELECT,INSERT ON media.storage_bucket_evidence TO lanverse_app;
