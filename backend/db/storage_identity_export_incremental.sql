-- Route private identity exports through the shared physical object ledger.
-- Apply after storage_ledger_expand_incremental.sql and before deploying the
-- routed identity export implementation.
BEGIN;
DO $$
BEGIN
  IF to_regclass('identity.directory_export') IS NULL
     OR to_regclass('media.object_location') IS NULL
     OR to_regclass('media.storage_work_intent') IS NULL THEN
    RAISE EXCEPTION 'identity exports and storage ledger must exist before routed export columns';
  END IF;
END $$;

ALTER TABLE identity.directory_export ADD COLUMN IF NOT EXISTS storage_location_id uuid;
ALTER TABLE identity.directory_export ADD COLUMN IF NOT EXISTS storage_intent_id uuid;
ALTER TABLE identity.directory_export ADD COLUMN IF NOT EXISTS storage_profile_id text;
ALTER TABLE identity.directory_export ADD COLUMN IF NOT EXISTS storage_bucket text;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='identity.directory_export'::regclass AND conname='directory_export_storage_identity_check') THEN
    ALTER TABLE identity.directory_export ADD CONSTRAINT directory_export_storage_identity_check CHECK (
      (private_object_id IS NULL AND storage_location_id IS NULL AND storage_intent_id IS NULL AND storage_profile_id IS NULL AND storage_bucket IS NULL)
      OR (private_object_id IS NOT NULL AND storage_location_id IS NOT NULL AND storage_intent_id IS NOT NULL AND storage_profile_id IS NOT NULL AND storage_bucket IS NOT NULL)
    );
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='identity.directory_export'::regclass AND conname='directory_export_storage_location_id_fkey') THEN
    ALTER TABLE ONLY identity.directory_export
      ADD CONSTRAINT directory_export_storage_location_id_fkey FOREIGN KEY (storage_location_id) REFERENCES media.object_location(id) ON DELETE RESTRICT;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='identity.directory_export'::regclass AND conname='directory_export_storage_intent_id_fkey') THEN
    ALTER TABLE ONLY identity.directory_export
      ADD CONSTRAINT directory_export_storage_intent_id_fkey FOREIGN KEY (storage_intent_id) REFERENCES media.storage_work_intent(id) ON DELETE RESTRICT;
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS directory_export_storage_idx
  ON identity.directory_export (storage_profile_id,status,expires_at)
  WHERE private_object_id IS NOT NULL;
COMMIT;
