-- Existing canvas databases only. Back up and rehearse restoration first.
-- Execute as the schema owner with psql --single-transaction -v ON_ERROR_STOP=1.
-- No documents, nodes, history, receipts, or historical client IDs are rewritten.
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

DO $$
BEGIN
  IF to_regclass('canvas.document') IS NULL
     OR to_regclass('canvas.command_log') IS NULL
     OR to_regclass('canvas.node') IS NULL
     OR to_regclass('canvas.save_receipt') IS NULL
     OR NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='glowloom_app' AND NOT rolsuper) THEN
    RAISE EXCEPTION 'existing canvas schema and glowloom_app role required';
  END IF;
END $$;

ALTER TABLE canvas.document
  ADD COLUMN IF NOT EXISTS schema_version integer NOT NULL DEFAULT 1;
ALTER TABLE canvas.node
  ADD COLUMN IF NOT EXISTS content_version bigint NOT NULL DEFAULT 1;
ALTER TABLE canvas.command_log ADD COLUMN IF NOT EXISTS client_id uuid;
ALTER TABLE canvas.document ALTER COLUMN revision TYPE bigint;
ALTER TABLE canvas.command_log ALTER COLUMN revision TYPE bigint;
ALTER TABLE canvas.command_log ALTER COLUMN base_revision TYPE bigint;
-- Historical version-0 documents and commands remain version 0. New documents
-- use version 1; the existing OT service upgrades a document when it accepts edits.
ALTER TABLE canvas.document ALTER COLUMN operation_version SET DEFAULT 1;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='canvas.document'::regclass AND conname='document_schema_version_check') THEN
    ALTER TABLE canvas.document ADD CONSTRAINT document_schema_version_check CHECK (schema_version > 0);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='canvas.node'::regclass AND conname='node_content_version_check') THEN
    ALTER TABLE canvas.node ADD CONSTRAINT node_content_version_check CHECK (content_version > 0);
  END IF;
END $$;

CREATE OR REPLACE FUNCTION canvas.guard_save_receipt() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF (SELECT rolsuper FROM pg_roles WHERE rolname=current_user) THEN
    IF TG_OP='DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
  END IF;
  IF TG_OP='UPDATE'
     AND (to_jsonb(NEW)-'response_body')=(to_jsonb(OLD)-'response_body')
     AND NEW.response_body='{"retention_expired":true}'::jsonb
     AND EXISTS (
       SELECT 1 FROM canvas.document d JOIN workspace.project p ON p.id=d.project_id
       WHERE d.id=OLD.document_id AND p.is_delete AND p.purge_after<=statement_timestamp()
     ) THEN
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'canvas save receipts are immutable' USING ERRCODE='42501';
END $$;

CREATE OR REPLACE TRIGGER canvas_save_receipt_immutable
BEFORE UPDATE OR DELETE ON canvas.save_receipt
FOR EACH ROW EXECUTE FUNCTION canvas.guard_save_receipt();

REVOKE UPDATE, DELETE ON canvas.save_receipt FROM glowloom_app;
GRANT UPDATE(response_body) ON canvas.save_receipt TO glowloom_app;
