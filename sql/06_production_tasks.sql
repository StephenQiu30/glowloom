-- Lanverse MVP table: public.production_tasks
CREATE TABLE public.production_tasks (
    id uuid NOT NULL,
    episode_id uuid NOT NULL,
    snapshot_id uuid NOT NULL,
    type text NOT NULL,
    scope_json jsonb NOT NULL,
    idempotency_scope text NOT NULL,
    idempotency_key varchar(128) NOT NULL,
    status text NOT NULL DEFAULT 'queued',
    progress_json jsonb NOT NULL,
    retry_of_task_id uuid,
    error_code text,
    error_json jsonb,
    next_action text,
    resource_version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    CONSTRAINT pk_production_tasks PRIMARY KEY (id),
    CONSTRAINT uq_production_tasks_idempotency
        UNIQUE (idempotency_scope, idempotency_key),
    CONSTRAINT uq_production_tasks_id_snapshot UNIQUE (id, snapshot_id),
    CONSTRAINT uq_production_tasks_episode_id UNIQUE (episode_id, id),
    CONSTRAINT fk_production_tasks_episode FOREIGN KEY (episode_id)
        REFERENCES public.episodes (id)
        MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT fk_production_tasks_snapshot FOREIGN KEY (episode_id, snapshot_id)
        REFERENCES public.submission_snapshots (episode_id, id)
        MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT fk_production_tasks_retry
        FOREIGN KEY (episode_id, retry_of_task_id)
        REFERENCES public.production_tasks (episode_id, id)
        MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT ck_production_tasks_invariants CHECK (
        jsonb_typeof(scope_json) = 'object'
        AND jsonb_typeof(progress_json) = 'object'
        AND (error_json IS NULL OR jsonb_typeof(error_json) = 'object')
        AND resource_version > 0
        AND type IN (
            'generate_script',
            'generate_storyboard',
            'generate_media',
            'render_episode'
        )
        AND status IN (
            'queued',
            'running',
            'cancelling',
            'cancelled',
            'succeeded',
            'failed',
            'unknown'
        )
        AND char_length(btrim(idempotency_scope)) > 0
        AND idempotency_key ~ '^[A-Za-z0-9._:-]{8,128}$'
        AND (
            (status IN ('cancelled', 'succeeded', 'failed') AND finished_at IS NOT NULL)
            OR (status NOT IN ('cancelled', 'succeeded', 'failed') AND finished_at IS NULL)
        )
        AND (
            (
                status IN ('failed', 'unknown')
                AND error_code IS NOT NULL
                AND error_json IS NOT NULL
                AND next_action IS NOT NULL
            )
            OR (
                status NOT IN ('failed', 'unknown')
                AND error_code IS NULL
                AND error_json IS NULL
                AND next_action IS NULL
            )
        )
        AND (
            error_code IS NULL
            OR (char_length(btrim(error_code)) > 0 AND char_length(error_code) <= 100)
        )
        AND (
            next_action IS NULL
            OR (char_length(btrim(next_action)) > 0 AND char_length(next_action) <= 200)
        )
        AND (retry_of_task_id IS NULL OR retry_of_task_id <> id)
    )
);

CREATE INDEX ix_production_tasks_nonterminal
    ON public.production_tasks (status, updated_at)
    WHERE status IN ('queued', 'running', 'cancelling', 'unknown');

CREATE INDEX ix_production_tasks_episode_created_at
    ON public.production_tasks (episode_id, created_at DESC);

CREATE INDEX ix_production_tasks_snapshot_fk
    ON public.production_tasks (episode_id, snapshot_id);

CREATE INDEX ix_production_tasks_retry_fk
    ON public.production_tasks (episode_id, retry_of_task_id);
