REVOKE UPDATE (status, moderation_status, moderation_detail, sha256,
  byte_size, mime_type, width, height, duration_ms, fps,
  audio_channels, codec, failure_reason, revision)
  ON TABLE media.media_asset FROM lanverse_app;
REVOKE UPDATE (status, closed_at, update_time)
  ON TABLE billing.reservation FROM lanverse_app;
REVOKE UPDATE (status, failure_code, failure_message, retryable,
  provider_request_key, workflow_id, started_at, finished_at,
  settled_micros, cost_estimated)
  ON TABLE operation.operation FROM lanverse_app;
DROP TABLE operation.provider_call;
DROP TABLE operation.operation_output;
DROP TABLE operation.operation_event;
ALTER TABLE media.media_asset DROP CONSTRAINT uq_media_asset_project_id;
