CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE SCHEMA media;

CREATE TABLE media.media_asset (
  id                   uuid PRIMARY KEY,
  project_id           uuid NOT NULL REFERENCES workspace.project(id),
  kind                 text NOT NULL CHECK (kind IN ('image', 'video', 'audio', 'document')),
  origin               text NOT NULL CHECK (origin IN ('upload', 'generated', 'system')),
  status               text NOT NULL CHECK (status IN ('uploading', 'processing', 'ready', 'rejected', 'failed')),
  object_key           text NOT NULL UNIQUE,
  file_name            text NOT NULL DEFAULT '',
  mime_type            text NOT NULL,
  byte_size            bigint NOT NULL CHECK (byte_size >= 0),
  sha256               text,
  width                integer,
  height               integer,
  duration_ms          integer,
  fps                  numeric(6,3),
  audio_channels       integer,
  codec                text,
  source_operation_id  uuid,
  provider_key         text,
  model_key            text,
  region               text CHECK (region IN ('domestic', 'overseas')),
  moderation_status    text NOT NULL DEFAULT 'pending'
    CHECK (moderation_status IN ('pending', 'passed', 'rejected', 'skipped')),
  moderation_detail    jsonb,
  aigc_marked          boolean NOT NULL DEFAULT false,
  contains_real_person boolean NOT NULL DEFAULT false,
  consent_record_id    uuid,
  upload_id            text,
  failure_reason       text,
  delete_time          timestamptz,
  purge_after          timestamptz,
  revision             integer NOT NULL DEFAULT 1 CHECK (revision > 0),
  create_time          timestamptz NOT NULL DEFAULT now(),
  update_time          timestamptz NOT NULL DEFAULT now(),
  is_delete            boolean NOT NULL DEFAULT false,
  CONSTRAINT fk_media_source_operation_project
    FOREIGN KEY (project_id, source_operation_id)
    REFERENCES operation.operation(project_id, id)
);

CREATE INDEX ix_media_project_kind
  ON media.media_asset (project_id, kind, create_time DESC)
  WHERE NOT is_delete;
CREATE INDEX ix_media_name_trgm
  ON media.media_asset USING gin (file_name gin_trgm_ops);
CREATE INDEX ix_media_sha256
  ON media.media_asset (project_id, sha256)
  WHERE NOT is_delete;

CREATE TABLE media.rendition (
  id             uuid PRIMARY KEY,
  media_asset_id uuid NOT NULL REFERENCES media.media_asset(id),
  kind           text NOT NULL CHECK (kind IN ('thumb_256', 'thumb_640', 'poster', 'proxy_720p', 'waveform')),
  object_key     text NOT NULL,
  width          integer,
  height         integer,
  byte_size      bigint,
  create_time    timestamptz NOT NULL DEFAULT now(),
  update_time    timestamptz NOT NULL DEFAULT now(),
  is_delete      boolean NOT NULL DEFAULT false,
  CONSTRAINT uq_rendition_natural UNIQUE (media_asset_id, kind)
);
