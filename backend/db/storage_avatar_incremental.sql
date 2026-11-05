-- Add routed ownership for avatar objects after the storage ledger expansion.
-- This is additive and must be applied before starting the routed avatar path.
BEGIN;
DO $$
BEGIN
  IF to_regclass('media.storage_work_intent') IS NULL THEN
    RAISE EXCEPTION 'storage ledger expansion must be applied before avatar routing';
  END IF;
END $$;

ALTER TABLE media.storage_work_intent
  DROP CONSTRAINT IF EXISTS storage_work_intent_kind_check;
ALTER TABLE media.storage_work_intent
  ADD CONSTRAINT storage_work_intent_kind_check
  CHECK (kind = ANY (ARRAY['operation','script','export','depth','transcription','package','transfer','project-copy','subject-voice','avatar']::text[]));
COMMIT;
