-- Lanverse MVP table: public.shot_spec_versions
CREATE TABLE public.shot_spec_versions (
    id uuid NOT NULL,
    episode_id uuid NOT NULL,
    version integer NOT NULL,
    parent_id uuid,
    script_version_id uuid NOT NULL,
    asset_version_refs_json jsonb NOT NULL,
    shots_json jsonb NOT NULL,
    shot_count integer NOT NULL,
    total_duration_ticks bigint NOT NULL,
    content_hash varchar(64) NOT NULL,
    origin_task_id uuid,
    status text NOT NULL DEFAULT 'draft',
    resource_version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    confirmed_at timestamptz,
    CONSTRAINT pk_shot_spec_versions PRIMARY KEY (id),
    CONSTRAINT uq_shot_spec_versions_episode_version UNIQUE (episode_id, version),
    CONSTRAINT uq_shot_spec_versions_episode_id UNIQUE (episode_id, id),
    CONSTRAINT ck_shot_spec_versions_invariants CHECK (
        version > 0
        AND resource_version > 0
        AND jsonb_typeof(asset_version_refs_json) = 'array'
        AND jsonb_typeof(shots_json) = 'array'
        AND content_hash ~ '^[0-9a-f]{64}$'
        AND status IN ('draft', 'confirmed', 'superseded')
        AND ((status <> 'draft' AND confirmed_at IS NOT NULL)
             OR (status = 'draft' AND confirmed_at IS NULL))
        AND shot_count = jsonb_array_length(shots_json)
        AND shot_count >= 0
        AND total_duration_ticks >= 0
        AND (parent_id IS NULL OR parent_id <> id)
        AND (
            status <> 'confirmed'
            OR (
                shot_count BETWEEN 6 AND 10
                AND total_duration_ticks BETWEEN 2700000 AND 5400000
            )
        )
    )
);
