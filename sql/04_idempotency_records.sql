-- Lanverse MVP table: public.idempotency_records
CREATE TABLE public.idempotency_records (
    id uuid NOT NULL,
    owner_module text NOT NULL,
    operation_scope text NOT NULL,
    idempotency_key varchar(128) NOT NULL,
    request_hash varchar(64) NOT NULL,
    state text NOT NULL DEFAULT 'pending',
    response_status integer,
    response_ref_json jsonb,
    request_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    CONSTRAINT pk_idempotency_records PRIMARY KEY (id),
    CONSTRAINT uq_idempotency_records_scope_key
        UNIQUE (operation_scope, idempotency_key),
    CONSTRAINT ck_idempotency_records_invariants CHECK (
        request_hash ~ '^[0-9a-f]{64}$'
        AND (response_ref_json IS NULL OR jsonb_typeof(response_ref_json) = 'object')
        AND owner_module IN (
            'project_catalog',
            'story_development',
            'generation',
            'production_jobs',
            'media_library',
            'delivery'
        )
        AND char_length(btrim(operation_scope)) > 0
        AND idempotency_key ~ '^[A-Za-z0-9._:-]{8,128}$'
        AND state IN ('pending', 'completed')
        AND (response_status IS NULL OR response_status BETWEEN 200 AND 599)
        AND (
            (state = 'pending' AND response_status IS NULL AND completed_at IS NULL)
            OR (
                state = 'completed'
                AND response_status IS NOT NULL
                AND response_ref_json IS NOT NULL
                AND completed_at IS NOT NULL
            )
        )
    )
);
