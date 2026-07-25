-- Lanverse MVP table: public.task_outputs
CREATE TABLE public.task_outputs (
    id uuid NOT NULL,
    task_id uuid NOT NULL,
    output_type text NOT NULL,
    output_id uuid NOT NULL,
    ordinal integer NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_task_outputs PRIMARY KEY (id),
    CONSTRAINT uq_task_outputs_output UNIQUE (output_type, output_id),
    CONSTRAINT uq_task_outputs_task_slot UNIQUE (task_id, output_type, ordinal),
    CONSTRAINT fk_task_outputs_task FOREIGN KEY (task_id)
        REFERENCES public.production_tasks (id)
        MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT ck_task_outputs_invariants CHECK (
        output_type IN (
            'script_version',
            'creative_asset_version',
            'shot_spec_version',
            'generation_candidate',
            'delivery_version'
        )
        AND ordinal >= 0
    )
);
