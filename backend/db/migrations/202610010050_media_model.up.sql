ALTER TABLE media.media_asset DROP CONSTRAINT media_asset_kind_check;
ALTER TABLE media.media_asset ADD CONSTRAINT media_asset_kind_check
  CHECK (kind IN ('image', 'video', 'audio', 'document', 'model'));
ALTER TABLE media.media_asset ADD CONSTRAINT media_asset_model_facts_check
  CHECK (kind <> 'model' OR (origin = 'upload' AND mime_type = 'model/gltf-binary'
    AND byte_size BETWEEN 1 AND 67108864 AND codec IS NOT NULL AND codec = 'glb2'
    AND width IS NULL AND height IS NULL AND duration_ms IS NULL AND fps IS NULL AND audio_channels IS NULL));
