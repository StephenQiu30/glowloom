-- Current transfer output facts. Apply only after pre-cutover active transfers
-- and their object debt have been settled; there is deliberately no backfill.
BEGIN;

LOCK TABLE media.transfer_job, media.transfer_item, media.transfer_object IN SHARE ROW EXCLUSIVE MODE;
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM media.transfer_object) THEN
    RAISE EXCEPTION 'transfer output attempt cutover requires empty pre-cutover object facts';
  END IF;
  IF EXISTS (SELECT 1 FROM media.transfer_job WHERE status IN ('queued','running','needs_reconciliation','cancel_requested') OR NOT process_ended OR execution_unconfirmed) THEN
    RAISE EXCEPTION 'settle pre-cutover active transfer execution and object debt before cutover';
  END IF;
END $$;

CREATE TABLE media.transfer_output_attempt (
  job_id uuid NOT NULL,
  item_index integer NOT NULL,
  output_attempt integer NOT NULL CHECK (output_attempt > 0),
  work_intent_id uuid UNIQUE REFERENCES media.storage_work_intent(id),
  frozen bytea NOT NULL CHECK (octet_length(frozen) BETWEEN 2 AND 1048576),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (job_id, item_index, output_attempt),
  FOREIGN KEY (job_id, item_index) REFERENCES media.transfer_item(job_id, item_index)
);

ALTER TABLE media.transfer_object ADD COLUMN output_attempt integer NOT NULL CHECK (output_attempt > 0);
ALTER TABLE media.transfer_object DROP CONSTRAINT transfer_object_pkey;
ALTER TABLE media.transfer_object ADD CONSTRAINT transfer_object_pkey PRIMARY KEY (job_id, item_index, output_attempt, rendition_kind);
ALTER TABLE media.transfer_object ADD CONSTRAINT transfer_object_output_attempt_fkey
  FOREIGN KEY (job_id, item_index, output_attempt) REFERENCES media.transfer_output_attempt(job_id, item_index, output_attempt);

CREATE FUNCTION media.reject_transfer_output_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'transfer physical output identity is append-only' USING ERRCODE = '42501';
END $$;

CREATE TRIGGER transfer_output_attempt_immutable BEFORE UPDATE OR DELETE ON media.transfer_output_attempt
FOR EACH ROW EXECUTE FUNCTION media.reject_transfer_output_mutation();

CREATE FUNCTION media.protect_transfer_object_attempt() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF ROW(NEW.job_id, NEW.item_index, NEW.output_attempt, NEW.rendition_kind, NEW.source_location_id,
         NEW.source_object_key, NEW.target_location_id, NEW.target_object_key, NEW.content_type)
     IS DISTINCT FROM
     ROW(OLD.job_id, OLD.item_index, OLD.output_attempt, OLD.rendition_kind, OLD.source_location_id,
         OLD.source_object_key, OLD.target_location_id, OLD.target_object_key, OLD.content_type)
     OR (OLD.byte_size IS NOT NULL AND NEW.byte_size IS DISTINCT FROM OLD.byte_size)
     OR (OLD.sha256 IS NOT NULL AND NEW.sha256 IS DISTINCT FROM OLD.sha256)
     OR (OLD.source_verified AND NOT NEW.source_verified)
     OR (OLD.write_started AND NOT NEW.write_started)
     OR (OLD.status = 'removed' AND NEW.status <> 'removed')
     OR (OLD.status = 'verified' AND NEW.status NOT IN ('verified','removed')) THEN
    RAISE EXCEPTION 'transfer object identity and receipts cannot regress' USING ERRCODE = '23514';
  END IF;
  RETURN NEW;
END $$;

CREATE TRIGGER transfer_object_attempt_immutable BEFORE UPDATE ON media.transfer_object
FOR EACH ROW EXECUTE FUNCTION media.protect_transfer_object_attempt();

GRANT SELECT, INSERT ON media.transfer_output_attempt TO glowloom_app;
-- Existing transfer_object SELECT/INSERT and column grants remain sufficient.
-- No UPDATE is granted on output_attempt, frozen physical identity or work IDs.

COMMIT;
