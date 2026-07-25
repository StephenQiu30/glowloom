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
    CONSTRAINT fk_delivery_versions_episode FOREIGN KEY (episode_id)
        REFERENCES public.episodes (id)
        MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT fk_delivery_versions_render_task
        FOREIGN KEY (episode_id, render_task_id)
        REFERENCES public.production_tasks (episode_id, id)
        MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT fk_delivery_versions_retry
        FOREIGN KEY (episode_id, retry_of_delivery_id)
        REFERENCES public.delivery_versions (episode_id, id)
        MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT fk_delivery_versions_snapshot
        FOREIGN KEY (episode_id, render_snapshot_id)
        REFERENCES public.render_snapshots (episode_id, id)
        MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT fk_delivery_versions_final_attempt
        FOREIGN KEY (render_task_id, final_attempt_id)
        REFERENCES public.production_attempts (task_id, id)
        MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT fk_delivery_versions_mp4 FOREIGN KEY (mp4_media_version_id)
        REFERENCES public.media_versions (id)
        MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT fk_delivery_versions_srt FOREIGN KEY (srt_media_version_id)
        REFERENCES public.media_versions (id)
        MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT fk_delivery_versions_manifest FOREIGN KEY (manifest_media_version_id)
        REFERENCES public.media_versions (id)
        MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
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

CREATE INDEX ix_delivery_versions_render_task_fk
    ON public.delivery_versions (episode_id, render_task_id);

CREATE INDEX ix_delivery_versions_retry_fk
    ON public.delivery_versions (episode_id, retry_of_delivery_id);

CREATE INDEX ix_delivery_versions_snapshot_fk
    ON public.delivery_versions (episode_id, render_snapshot_id);

CREATE INDEX ix_delivery_versions_final_attempt_fk
    ON public.delivery_versions (render_task_id, final_attempt_id);

CREATE INDEX ix_delivery_versions_mp4_fk
    ON public.delivery_versions (mp4_media_version_id);

CREATE INDEX ix_delivery_versions_srt_fk
    ON public.delivery_versions (srt_media_version_id);

CREATE INDEX ix_delivery_versions_manifest_fk
    ON public.delivery_versions (manifest_media_version_id);
