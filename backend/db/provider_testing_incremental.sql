-- SRC-AI-33: additive service-test evidence; safe for an existing catalog.
ALTER TABLE catalog.provider_check ALTER COLUMN credential_id DROP NOT NULL;
ALTER TABLE catalog.provider_check ADD COLUMN IF NOT EXISTS request_id uuid;
ALTER TABLE catalog.provider_check ADD COLUMN IF NOT EXISTS network_result text NOT NULL DEFAULT '';
ALTER TABLE catalog.provider_check ADD COLUMN IF NOT EXISTS http_status integer NOT NULL DEFAULT 0;
ALTER TABLE catalog.provider_check ADD COLUMN IF NOT EXISTS elapsed_ms bigint NOT NULL DEFAULT 0;
ALTER TABLE catalog.provider_check ADD COLUMN IF NOT EXISTS attempts integer NOT NULL DEFAULT 0;
ALTER TABLE catalog.provider_check ADD COLUMN IF NOT EXISTS parent_check_id uuid;
ALTER TABLE catalog.provider_check ADD COLUMN IF NOT EXISTS tracking_ended_at timestamptz;
ALTER TABLE catalog.provider_check ADD COLUMN IF NOT EXISTS tracking_note text NOT NULL DEFAULT '';
ALTER TABLE catalog.provider_check DROP CONSTRAINT IF EXISTS provider_check_kind_check;
ALTER TABLE catalog.provider_check ADD CONSTRAINT provider_check_kind_check CHECK (kind IN ('connectivity','credential_auth','reachability','model_query','discovery','model_access'));
