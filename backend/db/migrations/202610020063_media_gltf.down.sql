DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM media.media_asset WHERE kind='model' AND mime_type='model/gltf+json') THEN
    RAISE EXCEPTION 'cannot remove JSON glTF facts while retained model originals exist';
  END IF;
END $$;
ALTER TABLE media.media_asset DROP CONSTRAINT media_asset_model_facts_check;
ALTER TABLE media.media_asset ADD CONSTRAINT media_asset_model_facts_check
  CHECK (kind <> 'model' OR (origin = 'upload' AND mime_type = 'model/gltf-binary'
    AND byte_size BETWEEN 1 AND 67108864 AND codec IS NOT NULL AND codec = 'glb2'
    AND width IS NULL AND height IS NULL AND duration_ms IS NULL AND fps IS NULL AND audio_channels IS NULL));
