BEGIN;
LOCK TABLE media.upload_request IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
  IF EXISTS(SELECT 1 FROM media.upload_request) THEN
    RAISE EXCEPTION 'cannot remove durable upload idempotency receipts';
  END IF;
END $$;
DROP TABLE media.upload_request;
COMMIT;
