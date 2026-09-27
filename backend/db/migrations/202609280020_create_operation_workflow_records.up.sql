-- Durable operation history, outputs and provider evidence for DES-02 / DES-04.
CREATE TABLE operation.operation_event (
  id            uuid PRIMARY KEY,
  operation_id  uuid NOT NULL REFERENCES operation.operation(id),
  from_status   text,
  to_status     text NOT NULL,
  reason        text,
  detail        jsonb CHECK (detail IS NULL OR jsonb_typeof(detail) = 'object'),
  create_time   timestamptz NOT NULL DEFAULT now(),
  update_time   timestamptz NOT NULL DEFAULT now(),
  is_delete     boolean NOT NULL DEFAULT false
);

CREATE INDEX ix_operation_event_op
  ON operation.operation_event (operation_id, create_time);

-- The composite key prevents an output from pointing at another project's
-- media asset even when both referenced rows exist.
ALTER TABLE media.media_asset
  ADD CONSTRAINT uq_media_asset_project_id UNIQUE (project_id, id);

CREATE TABLE operation.operation_output (
  id                 uuid PRIMARY KEY,
  project_id         uuid NOT NULL,
  operation_id       uuid NOT NULL,
  seq_no             integer NOT NULL CHECK (seq_no >= 0),
  kind               text NOT NULL CHECK (kind IN ('media', 'json')),
  media_asset_id     uuid,
  json_payload       jsonb,
  moderation_status  text NOT NULL DEFAULT 'pending'
    CHECK (moderation_status IN ('pending', 'passed', 'rejected', 'skipped')),
  moderation_reason  text,
  create_time        timestamptz NOT NULL DEFAULT now(),
  update_time        timestamptz NOT NULL DEFAULT now(),
  is_delete          boolean NOT NULL DEFAULT false,
  CONSTRAINT uq_operation_output UNIQUE (operation_id, seq_no),
  CONSTRAINT fk_operation_output_operation_project
    FOREIGN KEY (project_id, operation_id)
    REFERENCES operation.operation(project_id, id),
  CONSTRAINT fk_operation_output_media_project
    FOREIGN KEY (project_id, media_asset_id)
    REFERENCES media.media_asset(project_id, id),
  CONSTRAINT ck_operation_output_content CHECK (
    (kind = 'media' AND media_asset_id IS NOT NULL AND json_payload IS NULL) OR
    (kind = 'json' AND media_asset_id IS NULL AND
      json_payload IS NOT NULL AND jsonb_typeof(json_payload) = 'object')
  )
);

CREATE INDEX ix_operation_output_project_media
  ON operation.operation_output (project_id, media_asset_id)
  WHERE media_asset_id IS NOT NULL AND NOT is_delete;

CREATE TABLE operation.provider_call (
  id                       uuid NOT NULL,
  project_id               uuid NOT NULL,
  operation_id             uuid NOT NULL,
  attempt                  integer NOT NULL CHECK (attempt > 0),
  action                   text NOT NULL
    CHECK (action IN ('submit', 'query', 'cancel', 'callback', 'moderate', 'llm')),
  provider_key             text NOT NULL CHECK (btrim(provider_key) <> ''),
  provider_task_id         text,
  request_summary          jsonb NOT NULL
    CHECK (jsonb_typeof(request_summary) = 'object'),
  response_summary         jsonb
    CHECK (response_summary IS NULL OR jsonb_typeof(response_summary) = 'object'),
  http_status              integer,
  outcome                  text NOT NULL
    CHECK (outcome IN ('ok', 'error', 'timeout', 'unknown')),
  usage                    jsonb CHECK (usage IS NULL OR jsonb_typeof(usage) = 'object'),
  cost_micros              bigint CHECK (cost_micros IS NULL OR cost_micros >= 0),
  latency_ms               integer CHECK (latency_ms IS NULL OR latency_ms >= 0),
  region                   text NOT NULL CHECK (region IN ('domestic', 'overseas')),
  run_id                   uuid,
  call_seq                 integer CHECK (call_seq IS NULL OR call_seq > 0),
  request_key              text,
  model_profile_version_id uuid,
  price_rule_version_id    uuid,
  create_time              timestamptz NOT NULL DEFAULT now(),
  update_time              timestamptz NOT NULL DEFAULT now(),
  is_delete                boolean NOT NULL DEFAULT false,
  PRIMARY KEY (id, create_time),
  CONSTRAINT fk_provider_call_operation_project
    FOREIGN KEY (project_id, operation_id)
    REFERENCES operation.operation(project_id, id)
) PARTITION BY RANGE (create_time);

CREATE INDEX ix_provider_call_operation
  ON operation.provider_call (operation_id, create_time);

-- Keep the current UTC month and three future months ready for writes. A
-- default partition preserves writes until partition maintenance is installed.
DO $$
DECLARE
  first_month timestamp := date_trunc('month', now() AT TIME ZONE 'UTC');
  month_index integer;
  month_start timestamptz;
  month_end timestamptz;
BEGIN
  FOR month_index IN 0..3 LOOP
    month_start := (first_month + make_interval(months => month_index)) AT TIME ZONE 'UTC';
    month_end := (first_month + make_interval(months => month_index + 1)) AT TIME ZONE 'UTC';
    EXECUTE format(
      'CREATE TABLE operation.%I PARTITION OF operation.provider_call FOR VALUES FROM (%L) TO (%L)',
      'provider_call_' || to_char(month_start AT TIME ZONE 'UTC', 'YYYYMM'),
      month_start,
      month_end
    );
  END LOOP;
END $$;

CREATE TABLE operation.provider_call_default
  PARTITION OF operation.provider_call DEFAULT;

GRANT SELECT, INSERT ON TABLE operation.operation_event TO lanverse_app;
GRANT SELECT, INSERT, UPDATE (moderation_status, moderation_reason, update_time)
  ON TABLE operation.operation_output TO lanverse_app;
GRANT SELECT, INSERT, UPDATE ON TABLE operation.provider_call TO lanverse_app;
GRANT UPDATE (status, failure_code, failure_message, retryable,
  provider_request_key, workflow_id, started_at, finished_at,
  settled_micros, cost_estimated)
  ON TABLE operation.operation TO lanverse_app;
GRANT UPDATE (status, closed_at, update_time)
  ON TABLE billing.reservation TO lanverse_app;
GRANT UPDATE (status, moderation_status, moderation_detail, sha256,
  byte_size, mime_type, width, height, duration_ms, fps,
  audio_channels, codec, failure_reason, revision)
  ON TABLE media.media_asset TO lanverse_app;
