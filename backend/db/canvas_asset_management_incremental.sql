-- Existing database only; additive metadata, no graph or execution rewrites.
BEGIN;
ALTER TABLE canvas.document
 ADD COLUMN IF NOT EXISTS asset_collection text NOT NULL DEFAULT 'current';
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='canvas.document'::regclass AND conname='document_asset_collection_check') THEN
  ALTER TABLE canvas.document ADD CONSTRAINT document_asset_collection_check CHECK(asset_collection IN ('current','legacy'));
 END IF;
END $$;
CREATE TABLE IF NOT EXISTS canvas.asset_preference (
 actor_id uuid NOT NULL REFERENCES identity."user"(id),
 scope_id uuid NOT NULL REFERENCES workspace.creation_scope(id),
 document_id uuid NOT NULL REFERENCES canvas.document(id),
 favorite boolean NOT NULL DEFAULT false,
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(actor_id,scope_id,document_id)
);
GRANT SELECT,INSERT,UPDATE ON canvas.asset_preference TO glowloom_app;
CREATE INDEX IF NOT EXISTS ix_canvas_asset_favorites ON canvas.asset_preference(actor_id,scope_id,document_id) WHERE favorite;
COMMIT;
