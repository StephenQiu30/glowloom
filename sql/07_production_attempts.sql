-- Lanverse MVP table: public.production_attempts
CREATE TABLE public.production_attempts (
    id uuid NOT NULL,
    task_id uuid NOT NULL,
    snapshot_id uuid NOT NULL,
    attempt_no integer NOT NULL,
    parent_attempt_id uuid,
    status text NOT NULL DEFAULT 'created',
    provider_id text,
    provider_request_key text,
    provider_request_id text,
    usage_json jsonb NOT NULL,
    safety_json jsonb NOT NULL,
    execution_metadata_json jsonb NOT NULL,
    error_code text,
    error_summary text,
    created_at timestamptz NOT NULL DEFAULT now(),
    submitted_at timestamptz,
    started_at timestamptz,
    finished_at timestamptz,
    CONSTRAINT pk_production_attempts PRIMARY KEY (id),
    CONSTRAINT uq_production_attempts_task_number UNIQUE (task_id, attempt_no),
    CONSTRAINT uq_production_attempts_task_id UNIQUE (task_id, id),
    CONSTRAINT fk_production_attempts_task_snapshot
        FOREIGN KEY (task_id, snapshot_id)
        REFERENCES public.production_tasks (id, snapshot_id)
        MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT fk_production_attempts_parent
        FOREIGN KEY (task_id, parent_attempt_id)
        REFERENCES public.production_attempts (task_id, id)
        MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT ck_production_attempts_invariants CHECK (
        jsonb_typeof(usage_json) = 'object'
        AND jsonb_typeof(safety_json) = 'object'
        AND jsonb_typeof(execution_metadata_json) = 'object'
        AND attempt_no BETWEEN 1 AND 3
        AND status IN (
            'created',
            'submitted',
            'provider_running',
            'downloading',
            'postprocessing',
            'succeeded',
            'failed',
            'cancelled',
            'unknown'
        )
        AND (
            (provider_request_key IS NULL AND provider_request_id IS NULL)
            OR (provider_id IS NOT NULL AND char_length(btrim(provider_id)) > 0)
        )
        AND (
            provider_request_key IS NULL
            OR char_length(btrim(provider_request_key)) > 0
        )
        AND (
            provider_request_id IS NULL
            OR char_length(btrim(provider_request_id)) > 0
        )
        AND (parent_attempt_id IS NULL OR parent_attempt_id <> id)
        AND (
            (status IN ('succeeded', 'failed', 'cancelled') AND finished_at IS NOT NULL)
            OR (status NOT IN ('succeeded', 'failed', 'cancelled') AND finished_at IS NULL)
        )
        AND (
            (
                status IN ('failed', 'unknown')
                AND error_code IS NOT NULL
                AND error_summary IS NOT NULL
            )
            OR (
                status NOT IN ('failed', 'unknown')
                AND error_code IS NULL
                AND error_summary IS NULL
            )
        )
        AND (
            error_code IS NULL
            OR (char_length(btrim(error_code)) > 0 AND char_length(error_code) <= 100)
        )
        AND (
            error_summary IS NULL
            OR (
                char_length(btrim(error_summary)) > 0
                AND char_length(error_summary) <= 500
            )
        )
        AND (submitted_at IS NULL OR submitted_at >= created_at)
        AND (
            started_at IS NULL
            OR (submitted_at IS NOT NULL AND started_at >= submitted_at)
        )
        AND (
            finished_at IS NULL
            OR finished_at >= COALESCE(started_at, submitted_at, created_at)
        )
    )
);

CREATE UNIQUE INDEX uq_production_attempts_provider_key
    ON public.production_attempts (provider_id, provider_request_key)
    WHERE provider_request_key IS NOT NULL;

CREATE UNIQUE INDEX uq_production_attempts_provider_request_id
    ON public.production_attempts (provider_id, provider_request_id)
    WHERE provider_request_id IS NOT NULL;

CREATE INDEX ix_production_attempts_snapshot_fk
    ON public.production_attempts (task_id, snapshot_id);

CREATE INDEX ix_production_attempts_parent_fk
    ON public.production_attempts (task_id, parent_attempt_id);
