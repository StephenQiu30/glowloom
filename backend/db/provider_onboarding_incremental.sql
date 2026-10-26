-- SRC-AI-26. Additive and repeatable; no user credentials or historical bindings are changed.
BEGIN;
ALTER TABLE catalog.model_profile ADD COLUMN IF NOT EXISTS candidate_snapshot jsonb;
ALTER TABLE catalog.provider_check ADD COLUMN IF NOT EXISTS model_id uuid REFERENCES catalog.model_profile(id);
ALTER TABLE catalog.provider_check ADD COLUMN IF NOT EXISTS model_version_id uuid REFERENCES catalog.model_profile_version(id);
ALTER TABLE catalog.provider_check ADD COLUMN IF NOT EXISTS model_result text NOT NULL DEFAULT '';
ALTER TABLE catalog.provider_check ADD COLUMN IF NOT EXISTS task_id text NOT NULL DEFAULT '';
ALTER TABLE catalog.provider_check ADD COLUMN IF NOT EXISTS dispatch_started_at timestamptz;
ALTER TABLE catalog.provider_check DROP CONSTRAINT IF EXISTS provider_check_kind_check;
ALTER TABLE catalog.provider_check ADD CONSTRAINT provider_check_kind_check CHECK(kind IN ('connectivity','discovery','model_access'));
ALTER TABLE catalog.model_release ADD COLUMN IF NOT EXISTS onboarding_check_id uuid REFERENCES catalog.provider_check(id);
INSERT INTO catalog.provider_preset_version(id,preset_id,version_no,payload,content_hash) VALUES('6da07a12-0000-4000-8000-000000000001','6da07a10-0000-4000-8000-000000000001',2,'{"schema_version":1,"name":"火山方舟","config":{"adapter_key":"volcengine_ark","base_url":"https://ark.cn-beijing.volces.com/api/v3","region":"domestic","capabilities":["video","text","image"],"timeout_seconds":120,"concurrency_limit":2,"rate_limit_per_min":30}}'::jsonb,'77174ea84bdea57c6c3eef8fc352d2eb02a5c1db13645a3c801a05de33b7c94c') ON CONFLICT(id) DO NOTHING;
COMMIT;
