-- Lanverse MVP table: public.task_jobs
CREATE TABLE public.task_jobs (
    id uuid NOT NULL,
    task_id uuid NOT NULL,
    payload_json jsonb NOT NULL,
    state text NOT NULL DEFAULT 'pending',
    lease_owner text,
    lease_until timestamptz,
    attempts integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_error_code text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    CONSTRAINT pk_task_jobs PRIMARY KEY (id),
    CONSTRAINT uq_task_jobs_task_id UNIQUE (task_id),
    CONSTRAINT fk_task_jobs_task FOREIGN KEY (task_id)
        REFERENCES public.production_tasks (id)
        MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT ck_task_jobs_invariants CHECK (
        jsonb_typeof(payload_json) = 'object'
        AND state IN ('pending', 'leased', 'completed', 'failed')
        AND attempts >= 0
        AND (
            (
                state = 'leased'
                AND lease_owner IS NOT NULL
                AND char_length(btrim(lease_owner)) > 0
                AND lease_until IS NOT NULL
            )
            OR (
                state <> 'leased'
                AND lease_owner IS NULL
                AND lease_until IS NULL
            )
        )
        AND (
            (state = 'completed' AND completed_at IS NOT NULL)
            OR (state <> 'completed' AND completed_at IS NULL)
        )
        AND (completed_at IS NULL OR completed_at >= created_at)
        AND (
            last_error_code IS NULL
            OR (
                char_length(btrim(last_error_code)) > 0
                AND char_length(last_error_code) <= 100
            )
        )
    )
);

CREATE INDEX ix_task_jobs_pending
    ON public.task_jobs (next_attempt_at, created_at)
    WHERE state = 'pending';

CREATE INDEX ix_task_jobs_leased
    ON public.task_jobs (lease_until)
    WHERE state = 'leased';
