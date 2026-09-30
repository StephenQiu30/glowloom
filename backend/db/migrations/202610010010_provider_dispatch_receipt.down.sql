-- Removing dispatch evidence would turn a sent v1 call into an apparently
-- unclaimed call after reapplying up. Never authorize that destructive rollback.
DO $$
BEGIN
  LOCK TABLE operation.provider_call IN ACCESS EXCLUSIVE MODE;
  IF EXISTS (
    SELECT 1 FROM operation.provider_call
    WHERE dispatch_started_at IS NOT NULL OR receipt IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'provider dispatch evidence prevents rollback';
  END IF;
  ALTER TABLE operation.provider_call
    DROP CONSTRAINT ck_provider_call_receipt,
    DROP COLUMN receipt,
    DROP COLUMN dispatch_started_at;
END $$;
