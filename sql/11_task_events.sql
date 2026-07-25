-- Lanverse MVP table: public.task_events
CREATE TABLE public.task_events (
    event_id uuid NOT NULL,
    task_id uuid NOT NULL,
    task_resource_version bigint NOT NULL,
    event_type text NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    correlation_id uuid NOT NULL,
    data_json jsonb NOT NULL,
    CONSTRAINT pk_task_events PRIMARY KEY (event_id),
    CONSTRAINT uq_task_events_task_resource_version
        UNIQUE (task_id, task_resource_version),
    CONSTRAINT fk_task_events_task FOREIGN KEY (task_id)
        REFERENCES public.production_tasks (id)
        MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT ck_task_events_invariants CHECK (
        task_resource_version > 0
        AND jsonb_typeof(data_json) = 'object'
        AND event_type IN (
            'task.accepted',
            'task.started',
            'task.progressed',
            'task.cancel_requested',
            'task.cancelled',
            'task.succeeded',
            'task.failed',
            'task.unknown',
            'task.reconciled',
            'attempt.created',
            'attempt.updated',
            'output.recorded'
        )
    )
);

CREATE INDEX ix_task_events_task_occurred_at
    ON public.task_events (task_id, occurred_at);
