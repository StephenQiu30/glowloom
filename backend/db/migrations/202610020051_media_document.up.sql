-- Existing legacy rows are retained. New writes and all owning reads are closed.
ALTER TABLE media.media_asset ADD CONSTRAINT media_asset_document_facts_check
  CHECK (kind <> 'document' OR (
    origin IN ('upload', 'system')
    AND byte_size BETWEEN 1 AND 20971520
    AND sha256 IS NOT NULL AND sha256 ~ '^[0-9a-f]{64}$'
    AND codec IS NOT NULL
    AND width IS NULL AND height IS NULL AND duration_ms IS NULL
    AND fps IS NULL AND audio_channels IS NULL
    AND file_name IS NOT NULL AND octet_length(file_name) BETWEEN 1 AND 255
    AND file_name = btrim(file_name) AND file_name !~ '[[:cntrl:]/\\]'
    AND ((mime_type = 'text/plain' AND codec = 'txt'
      AND lower(file_name) LIKE '%.txt' AND object_key LIKE '%.txt')
      OR (mime_type = 'application/vnd.openxmlformats-officedocument.wordprocessingml.document'
        AND codec = 'docx' AND lower(file_name) LIKE '%.docx' AND object_key LIKE '%.docx'))
  )) NOT VALID;
