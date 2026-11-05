BEGIN;

ALTER TABLE catalog.provider DROP CONSTRAINT IF EXISTS provider_status_check;
ALTER TABLE catalog.provider ADD CONSTRAINT provider_status_check
  CHECK (status IN ('active', 'disabled', 'archived'));

ALTER TABLE catalog.model_profile DROP CONSTRAINT IF EXISTS model_profile_status_check;
ALTER TABLE catalog.model_profile ADD CONSTRAINT model_profile_status_check
  CHECK (status IN ('active', 'disabled', 'archived'));

COMMIT;
