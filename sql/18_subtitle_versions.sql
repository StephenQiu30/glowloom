-- Lanverse MVP table: public.subtitle_versions
CREATE TABLE public.subtitle_versions (
    id uuid NOT NULL,
    episode_id uuid NOT NULL,
    version integer NOT NULL,
    parent_id uuid,
    script_version_id uuid NOT NULL,
    shot_spec_version_id uuid NOT NULL,
    input_refs_json jsonb NOT NULL,
    language text NOT NULL,
    cues_json jsonb NOT NULL,
    cue_count integer NOT NULL,
    content_hash varchar(64) NOT NULL,
    status text NOT NULL DEFAULT 'draft',
    resource_version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    confirmed_at timestamptz,
    CONSTRAINT pk_subtitle_versions PRIMARY KEY (id),
    CONSTRAINT uq_subtitle_versions_episode_version UNIQUE (episode_id, version),
    CONSTRAINT uq_subtitle_versions_episode_id UNIQUE (episode_id, id),
    CONSTRAINT ck_subtitle_versions_invariants CHECK (
        version > 0
        AND resource_version > 0
        AND jsonb_typeof(input_refs_json) = 'object'
        AND jsonb_typeof(cues_json) = 'array'
        AND content_hash ~ '^[0-9a-f]{64}$'
        AND status IN ('draft', 'confirmed', 'superseded')
        AND ((status <> 'draft' AND confirmed_at IS NOT NULL)
             OR (status = 'draft' AND confirmed_at IS NULL))
        AND cue_count = jsonb_array_length(cues_json)
        AND cue_count >= 0
        AND char_length(btrim(language)) > 0
        AND (parent_id IS NULL OR parent_id <> id)
    )
);
