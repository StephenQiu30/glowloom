-- Lanverse MVP table: public.render_snapshots
CREATE TABLE public.render_snapshots (
    id uuid NOT NULL,
    episode_id uuid NOT NULL,
    submission_scope text NOT NULL,
    idempotency_key varchar(128) NOT NULL,
    request_hash varchar(64) NOT NULL,
    initial_task_id uuid,
    shot_spec_version_id uuid NOT NULL,
    subtitle_version_id uuid NOT NULL,
    input_refs_json jsonb NOT NULL,
    segments_json jsonb NOT NULL,
    timebase bigint NOT NULL DEFAULT 90000,
    width integer NOT NULL DEFAULT 720,
    height integer NOT NULL DEFAULT 1280,
    fps integer NOT NULL DEFAULT 24,
    audio_rate integer NOT NULL DEFAULT 48000,
    audio_channels integer NOT NULL DEFAULT 2,
    normalization_json jsonb NOT NULL,
    recipe_hash varchar(64) NOT NULL,
    content_hash varchar(64) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_render_snapshots PRIMARY KEY (id),
    CONSTRAINT uq_render_snapshots_submission
        UNIQUE (submission_scope, idempotency_key),
    CONSTRAINT uq_render_snapshots_episode_id UNIQUE (episode_id, id),
    CONSTRAINT ck_render_snapshots_invariants CHECK (
        request_hash ~ '^[0-9a-f]{64}$'
        AND recipe_hash ~ '^[0-9a-f]{64}$'
        AND content_hash ~ '^[0-9a-f]{64}$'
        AND jsonb_typeof(input_refs_json) = 'object'
        AND jsonb_typeof(segments_json) = 'array'
        AND jsonb_typeof(normalization_json) = 'object'
        AND char_length(btrim(submission_scope)) > 0
        AND idempotency_key ~ '^[A-Za-z0-9._:-]{8,128}$'
        AND jsonb_array_length(segments_json) BETWEEN 6 AND 10
        AND timebase = 90000
        AND width = 720
        AND height = 1280
        AND fps = 24
        AND audio_rate = 48000
        AND audio_channels = 2
    )
);
