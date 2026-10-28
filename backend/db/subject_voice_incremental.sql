-- Requires subject_library_incremental.sql. Additive voice-job storage only.
-- Existing subjects, Bible, models and encrypted credentials are not rewritten.
BEGIN;
SET LOCAL lock_timeout = '5s';
SELECT pg_advisory_xact_lock(63498120261028);
-- A legacy installation may not yet have the profile/location/work subsystem.
-- Do not create or activate storage here. UUID receipts stay independent; API
-- readiness and worker owner proofs still reject execution until real storage
-- dependencies and an explicitly configured route are available.
DO $$ BEGIN
 IF to_regclass('media.storage_work_intent') IS NOT NULL THEN
  ALTER TABLE media.storage_work_intent DROP CONSTRAINT IF EXISTS storage_work_intent_kind_check;
  ALTER TABLE media.storage_work_intent ADD CONSTRAINT storage_work_intent_kind_check CHECK(kind IN ('operation','script','export','depth','transcription','package','transfer','project-copy','subject-voice'));
 END IF;
END $$;
CREATE TABLE IF NOT EXISTS subject.voice_job (
 id uuid PRIMARY KEY,
 actor_id uuid NOT NULL REFERENCES identity."user"(id),
 owner_scope_id uuid NOT NULL,
 physical_scope_id uuid NOT NULL,
 personal_scope_id uuid NOT NULL,
 personal_org_id uuid NOT NULL,
 draft_intent_id uuid NOT NULL,
 idem_key uuid NOT NULL,
 request_sha256 text NOT NULL CHECK(request_sha256 ~ '^[a-f0-9]{64}$'),
 name text NOT NULL CHECK(char_length(name) BETWEEN 1 AND 20 AND name=btrim(name)),
 text text NOT NULL CHECK(char_length(text) BETWEEN 1 AND 500),
 release_id uuid NOT NULL REFERENCES catalog.model_release(id),
 provider_id uuid NOT NULL REFERENCES catalog.provider(id),
 config_version_id uuid NOT NULL REFERENCES catalog.provider_config_version(id),
 credential_id uuid NOT NULL REFERENCES catalog.provider_credential(id),
 model_version_id uuid NOT NULL REFERENCES catalog.model_profile_version(id),
 price_version_id uuid NOT NULL REFERENCES catalog.price_rule_version(id),
 resource_id text NOT NULL CHECK(resource_id='seed-tts-2.0'),
 voice_key text NOT NULL CHECK(char_length(voice_key) BETWEEN 1 AND 512),
 speech_rate integer NOT NULL DEFAULT 0 CHECK(speech_rate BETWEEN -50 AND 100),
 upstream_cost jsonb NOT NULL CHECK(jsonb_typeof(upstream_cost)='object'),
 moderation text NOT NULL CHECK(moderation='provider'),
 event_id uuid NOT NULL UNIQUE,
 asset_id uuid NOT NULL UNIQUE,
 output_asset_id uuid REFERENCES media.media_asset(id),
 storage_intent_id uuid,
 status text NOT NULL DEFAULT 'queued' CHECK(status IN ('queued','dispatching','ingesting','succeeded','failed','unknown')),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 dispatch_started_at timestamptz,
 execution_id uuid,
 provider_log_id text NOT NULL DEFAULT '' CHECK(char_length(provider_log_id)<=256),
 error_code text NOT NULL DEFAULT '' CHECK(char_length(error_code)<=100),
 audio_location_id uuid,
 manifest_location_id uuid,
 audio_sha256 text CHECK(audio_sha256 IS NULL OR audio_sha256 ~ '^[a-f0-9]{64}$'),
 audio_byte_size bigint CHECK(audio_byte_size IS NULL OR audio_byte_size BETWEEN 1 AND 104857600),
 usage jsonb CHECK(usage IS NULL OR jsonb_typeof(usage)='object'),
 result jsonb CHECK(result IS NULL OR jsonb_typeof(result)='object'),
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 completed_at timestamptz,
 UNIQUE(actor_id,idem_key),
 FOREIGN KEY(owner_scope_id,physical_scope_id) REFERENCES workspace.creation_scope(id,physical_scope_id),
 FOREIGN KEY(personal_scope_id,personal_org_id) REFERENCES workspace.creation_scope(id,physical_scope_id),
 CHECK((dispatch_started_at IS NULL)=(execution_id IS NULL)),
 CHECK(status NOT IN ('dispatching','ingesting','succeeded','unknown') OR dispatch_started_at IS NOT NULL),
 CHECK((status='succeeded')=(result IS NOT NULL)),
 CHECK((output_asset_id IS NULL) OR output_asset_id=asset_id),
 CHECK(status<>'succeeded' OR (output_asset_id IS NOT NULL AND audio_sha256 IS NOT NULL AND audio_byte_size IS NOT NULL AND completed_at IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS voice_job_owner_idx ON subject.voice_job(owner_scope_id,actor_id,created_at,id);
CREATE INDEX IF NOT EXISTS voice_job_provider_dispatch_idx ON subject.voice_job(provider_id,dispatch_started_at) WHERE dispatch_started_at IS NOT NULL;
REVOKE UPDATE,DELETE ON subject.voice_job FROM glowloom_app;
GRANT SELECT,INSERT ON subject.voice_job TO glowloom_app;
GRANT UPDATE(status,revision,dispatch_started_at,execution_id,provider_log_id,error_code,audio_location_id,manifest_location_id,audio_sha256,audio_byte_size,usage,result,output_asset_id,updated_at,completed_at) ON subject.voice_job TO glowloom_app;
-- No DELETE or mutation of original actor/scope/intent/key/catalog/voice/cost.
COMMIT;
