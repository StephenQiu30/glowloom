CREATE SCHEMA operation;

CREATE TABLE operation.batch (
  id                 uuid PRIMARY KEY,
  project_id         uuid NOT NULL REFERENCES workspace.project(id),
  kind               text NOT NULL CHECK (kind IN ('keyframe', 'video', 'tts', 'reference', 'parse', 'storyboard', 'mixed')),
  scope              jsonb NOT NULL CHECK (jsonb_typeof(scope) = 'object'),
  status             text NOT NULL CHECK (status IN ('quoted', 'confirmed', 'running', 'finished', 'cancelled', 'expired')),
  paused_reason      text,
  total_count        integer NOT NULL CHECK (total_count > 0),
  succeeded_count    integer NOT NULL DEFAULT 0 CHECK (succeeded_count >= 0),
  failed_count       integer NOT NULL DEFAULT 0 CHECK (failed_count >= 0),
  unknown_count      integer NOT NULL DEFAULT 0 CHECK (unknown_count >= 0),
  quote_total_micros bigint NOT NULL CHECK (quote_total_micros >= 0),
  workflow_id        text,
  create_time        timestamptz NOT NULL DEFAULT now(),
  update_time        timestamptz NOT NULL DEFAULT now(),
  is_delete          boolean NOT NULL DEFAULT false,
  CONSTRAINT uq_batch_project_id UNIQUE (project_id, id),
  CONSTRAINT ck_batch_counts CHECK (succeeded_count::bigint + failed_count + unknown_count <= total_count)
);

CREATE TABLE operation.operation (
  id                       uuid PRIMARY KEY,
  project_id               uuid NOT NULL REFERENCES workspace.project(id),
  batch_id                 uuid,
  target_type              text CHECK (target_type IN ('shot_frame', 'shot_take', 'reference_slot', 'dialogue_audio', 'voice_preview', 'episode_split', 'episode_parse', 'bible_extract', 'scene_storyboard', 'agent_session', 'free')),
  target_id                uuid,
  target_key               text,
  target_version_no        integer,
  capability               text NOT NULL CHECK (btrim(capability) <> ''),
  mode                     text NOT NULL CHECK (btrim(mode) <> ''),
  model_profile_version_id uuid,
  price_rule_version_id    uuid,
  params                   jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(params) = 'object'),
  output_count             integer NOT NULL DEFAULT 1 CHECK (output_count BETWEEN 1 AND 8),
  input_hash               text NOT NULL CHECK (btrim(input_hash) <> ''),
  origin                   text NOT NULL CHECK (origin IN ('pipeline', 'batch', 'canvas', 'agent', 'upload', 'system')),
  status                   text NOT NULL CHECK (status IN ('draft', 'quoted', 'confirmed', 'submitting', 'submitted', 'succeeded', 'ingesting', 'completed', 'failed', 'unknown', 'reconciling', 'manual', 'cancelling', 'cancelled', 'expired')),
  failure_code             text,
  failure_message          text,
  retryable                boolean,
  quote_micros             bigint CHECK (quote_micros >= 0),
  quote_detail             jsonb,
  quote_expires_at         timestamptz,
  reused_from_id           uuid,
  force_regenerate         boolean NOT NULL DEFAULT false,
  reservation_id           uuid,
  provider_request_key     text UNIQUE,
  workflow_id              text UNIQUE,
  confirmed_at             timestamptz,
  confirmed_by             uuid,
  started_at               timestamptz,
  finished_at              timestamptz,
  settled_micros           bigint CHECK (settled_micros >= 0),
  cost_estimated           boolean NOT NULL DEFAULT false,
  region                   text CHECK (region IN ('domestic', 'overseas')),
  trace_id                 text,
  create_time              timestamptz NOT NULL DEFAULT now(),
  update_time              timestamptz NOT NULL DEFAULT now(),
  is_delete                boolean NOT NULL DEFAULT false,
  CONSTRAINT uq_operation_project_id UNIQUE (project_id, id),
  CONSTRAINT fk_operation_batch_project FOREIGN KEY (project_id, batch_id)
    REFERENCES operation.batch(project_id, id),
  CONSTRAINT fk_operation_reused_project FOREIGN KEY (project_id, reused_from_id)
    REFERENCES operation.operation(project_id, id),
  CONSTRAINT ck_operation_reuse_self CHECK (reused_from_id IS DISTINCT FROM id),
  CONSTRAINT ck_operation_quote_amount CHECK (
    status <> 'quoted' OR (quote_micros IS NOT NULL AND quote_expires_at IS NOT NULL AND quote_expires_at > create_time)
  ),
  CONSTRAINT ck_operation_agent_session CHECK (
    target_type <> 'agent_session' OR (model_profile_version_id IS NULL AND price_rule_version_id IS NULL)
  ),
  CONSTRAINT ck_operation_upload CHECK (
    origin = 'upload' OR
    (region IS NOT NULL AND
      (model_profile_version_id IS NOT NULL OR target_type IS NOT DISTINCT FROM 'agent_session'))
  )
);

CREATE INDEX ix_operation_project_status ON operation.operation (project_id, status, create_time DESC);
CREATE INDEX ix_operation_target ON operation.operation (target_type, target_id, create_time DESC);
CREATE INDEX ix_operation_reuse ON operation.operation (project_id, input_hash) WHERE status = 'completed';
CREATE INDEX ix_operation_inflight ON operation.operation (status, started_at)
  WHERE status IN ('submitting', 'submitted', 'unknown', 'reconciling', 'manual', 'ingesting');

CREATE TABLE operation.operation_input (
  id             uuid PRIMARY KEY,
  operation_id   uuid NOT NULL REFERENCES operation.operation(id),
  seq_no         integer NOT NULL CHECK (seq_no >= 0),
  role           text NOT NULL CHECK (btrim(role) <> ''),
  ref_type       text NOT NULL CHECK (btrim(ref_type) <> ''),
  ref_id         uuid,
  ref_version    text,
  text_value     text,
  media_asset_id uuid,
  mask_asset_id  uuid,
  create_time    timestamptz NOT NULL DEFAULT now(),
  update_time    timestamptz NOT NULL DEFAULT now(),
  is_delete      boolean NOT NULL DEFAULT false,
  CONSTRAINT uq_operation_input_natural UNIQUE (operation_id, seq_no)
);

CREATE TABLE billing.reservation (
  id            uuid PRIMARY KEY,
  project_id    uuid NOT NULL REFERENCES workspace.project(id),
  operation_id  uuid NOT NULL UNIQUE,
  amount_micros bigint NOT NULL CHECK (amount_micros >= 0),
  status        text NOT NULL CHECK (status IN ('held', 'settled', 'released')),
  create_time   timestamptz NOT NULL DEFAULT now(),
  closed_at     timestamptz,
  update_time   timestamptz NOT NULL DEFAULT now(),
  is_delete     boolean NOT NULL DEFAULT false,
  CONSTRAINT uq_reservation_project_operation_id UNIQUE (project_id, operation_id, id),
  CONSTRAINT fk_reservation_operation_project FOREIGN KEY (project_id, operation_id)
    REFERENCES operation.operation(project_id, id)
);

CREATE INDEX ix_reservation_held ON billing.reservation (project_id) WHERE status = 'held';

ALTER TABLE operation.operation
  ADD CONSTRAINT fk_operation_reservation_identity
  FOREIGN KEY (project_id, id, reservation_id)
  REFERENCES billing.reservation(project_id, operation_id, id);
