-- Lanverse MVP table: public.creative_asset_versions
CREATE TABLE public.creative_asset_versions (
    id uuid NOT NULL,
    asset_id uuid NOT NULL,
    episode_id uuid NOT NULL,
    version integer NOT NULL,
    parent_id uuid,
    source_script_version_id uuid NOT NULL,
    origin_task_id uuid,
    asset_type text NOT NULL,
    name text NOT NULL,
    description text NOT NULL,
    content_hash varchar(64) NOT NULL,
    status text NOT NULL DEFAULT 'draft',
    resource_version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    confirmed_at timestamptz,
    CONSTRAINT pk_creative_asset_versions PRIMARY KEY (id),
    CONSTRAINT uq_creative_asset_versions_asset_version UNIQUE (asset_id, version),
    CONSTRAINT uq_creative_asset_versions_asset_episode_id
        UNIQUE (asset_id, episode_id, id),
    CONSTRAINT ck_creative_asset_versions_invariants CHECK (
        version > 0
        AND resource_version > 0
        AND content_hash ~ '^[0-9a-f]{64}$'
        AND status IN ('draft', 'confirmed', 'superseded')
        AND ((status <> 'draft' AND confirmed_at IS NOT NULL)
             OR (status = 'draft' AND confirmed_at IS NULL))
        AND asset_type IN ('character', 'scene', 'visual_style')
        AND char_length(btrim(name)) > 0
        AND char_length(btrim(description)) > 0
        AND (parent_id IS NULL OR parent_id <> id)
    )
);
