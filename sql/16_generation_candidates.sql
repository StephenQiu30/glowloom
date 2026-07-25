-- Lanverse MVP table: public.generation_candidates
CREATE TABLE public.generation_candidates (
    id uuid NOT NULL,
    episode_id uuid NOT NULL,
    task_id uuid NOT NULL,
    attempt_id uuid NOT NULL,
    output_slot text NOT NULL,
    usage_type text NOT NULL,
    usage_id uuid NOT NULL,
    input_version_id uuid NOT NULL,
    input_hash varchar(64) NOT NULL,
    media_version_id uuid NOT NULL,
    status text NOT NULL DEFAULT 'pending_media',
    blocked_reason text,
    created_at timestamptz NOT NULL DEFAULT now(),
    finalized_at timestamptz,
    CONSTRAINT pk_generation_candidates PRIMARY KEY (id),
    CONSTRAINT uq_generation_candidates_media_version UNIQUE (media_version_id),
    CONSTRAINT uq_generation_candidates_attempt_slot UNIQUE (attempt_id, output_slot),
    CONSTRAINT uq_generation_candidates_episode_slot_id UNIQUE (
        episode_id,
        usage_type,
        usage_id,
        input_version_id,
        input_hash,
        id
    ),
    CONSTRAINT ck_generation_candidates_invariants CHECK (
        input_hash ~ '^[0-9a-f]{64}$'
        AND usage_type IN (
            'asset_image',
            'shot_image',
            'shot_video',
            'speech_audio'
        )
        AND (output_slot = 'primary' OR output_slot ~ '^extra/[0-9]+$')
        AND status IN ('pending_media', 'ready', 'blocked')
        AND (
            (status = 'blocked' AND blocked_reason IS NOT NULL)
            OR (status <> 'blocked' AND blocked_reason IS NULL)
        )
        AND (blocked_reason IS NULL OR char_length(btrim(blocked_reason)) > 0)
        AND (
            (status IN ('ready', 'blocked') AND finalized_at IS NOT NULL)
            OR (status = 'pending_media' AND finalized_at IS NULL)
        )
        AND (output_slot = 'primary' OR status = 'blocked')
    )
);
