-- Intentionally fails when model assets remain; never discards uploaded assets.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM media.media_asset WHERE kind = 'model') THEN
    RAISE EXCEPTION 'model assets remain; refusing to remove the model asset contract';
  END IF;
END $$;
ALTER TABLE media.media_asset DROP CONSTRAINT media_asset_kind_check;
ALTER TABLE media.media_asset ADD CONSTRAINT media_asset_kind_check
  CHECK (kind IN ('image', 'video', 'audio', 'document'));
ALTER TABLE media.media_asset DROP CONSTRAINT media_asset_model_facts_check;
