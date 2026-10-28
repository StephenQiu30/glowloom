-- Apply once to an existing database before starting the asset-inventory build.
-- The constant default preserves every existing canvas and its content unchanged.
BEGIN;
ALTER TABLE canvas.document
  ADD COLUMN IF NOT EXISTS kind text NOT NULL DEFAULT 'canvas';
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='canvas.document'::regclass AND conname='document_kind_check'
  ) THEN
    ALTER TABLE canvas.document ADD CONSTRAINT document_kind_check
      CHECK (kind IN ('canvas','short_drama'));
  END IF;
END $$;
CREATE INDEX IF NOT EXISTS ix_canvas_document_asset_kind
  ON canvas.document(kind,update_time DESC,id DESC) WHERE NOT is_delete;
COMMIT;
