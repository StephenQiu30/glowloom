-- Lanverse MVP table: public.delivery_versions
CREATE TABLE public.delivery_versions (
    id uuid NOT NULL,
    episode_id uuid NOT NULL,
    version integer NOT NULL,
    render_task_id uuid NOT NULL,
    final_attempt_id uuid,
    retry_of_delivery_id uuid,
    render_snapshot_id uuid NOT NULL,
    mp4_media_version_id uuid,
    srt_media_version_id uuid,
    manifest_media_version_id uuid,
    artifact_summary_json jsonb,
    ffmpeg_version text,
    ffprobe_summary_json jsonb,
    status text NOT NULL DEFAULT 'rendering',
    error_code text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    CONSTRAINT pk_delivery_versions PRIMARY KEY (id),
    CONSTRAINT uq_delivery_versions_episode_version UNIQUE (episode_id, version),
    CONSTRAINT uq_delivery_versions_episode_id UNIQUE (episode_id, id),
    CONSTRAINT uq_delivery_versions_render_task UNIQUE (render_task_id),
    CONSTRAINT ck_delivery_versions_invariants CHECK (
        version > 0
        AND (
            artifact_summary_json IS NULL
            OR jsonb_typeof(artifact_summary_json) = 'object'
        )
        AND (
            ffprobe_summary_json IS NULL
            OR jsonb_typeof(ffprobe_summary_json) = 'object'
        )
        AND status IN ('rendering', 'ready', 'failed', 'cancelled')
        AND (
            (status IN ('ready', 'failed', 'cancelled') AND finished_at IS NOT NULL)
            OR (status = 'rendering' AND finished_at IS NULL)
        )
        AND (retry_of_delivery_id IS NULL OR retry_of_delivery_id <> id)
        AND (
            status <> 'ready'
            OR (
                final_attempt_id IS NOT NULL
                AND mp4_media_version_id IS NOT NULL
                AND srt_media_version_id IS NOT NULL
                AND manifest_media_version_id IS NOT NULL
                AND mp4_media_version_id <> srt_media_version_id
                AND mp4_media_version_id <> manifest_media_version_id
                AND srt_media_version_id <> manifest_media_version_id
                AND artifact_summary_json IS NOT NULL
                AND ffprobe_summary_json IS NOT NULL
                AND ffmpeg_version IS NOT NULL
                AND char_length(btrim(ffmpeg_version)) > 0
                AND error_code IS NULL
            )
        )
        AND (
            status <> 'failed'
            OR (error_code IS NOT NULL AND char_length(btrim(error_code)) > 0)
        )
    )
);
