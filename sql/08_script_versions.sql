-- Lanverse MVP table: public.script_versions
CREATE TABLE public.script_versions (
    id uuid NOT NULL,
    episode_id uuid NOT NULL,
    version integer NOT NULL,
    parent_id uuid,
    source_revision_id uuid NOT NULL,
    schema_version text NOT NULL,
    content_json jsonb NOT NULL,
    content_hash varchar(64) NOT NULL,
    origin_task_id uuid,
    model_profile_id text,
    provider_id text,
    model_id text,
    prompt_version text,
    status text NOT NULL DEFAULT 'draft',
    resource_version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    confirmed_at timestamptz,
    CONSTRAINT pk_script_versions PRIMARY KEY (id),
    CONSTRAINT uq_script_versions_episode_version UNIQUE (episode_id, version),
    CONSTRAINT uq_script_versions_episode_id UNIQUE (episode_id, id),
    CONSTRAINT fk_script_versions_episode FOREIGN KEY (episode_id)
        REFERENCES public.episodes (id)
        MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT fk_script_versions_parent FOREIGN KEY (episode_id, parent_id)
        REFERENCES public.script_versions (episode_id, id)
        MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT fk_script_versions_source
        FOREIGN KEY (episode_id, source_revision_id)
        REFERENCES public.source_revisions (episode_id, id)
        MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT fk_script_versions_origin_task
        FOREIGN KEY (episode_id, origin_task_id)
        REFERENCES public.production_tasks (episode_id, id)
        MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT ck_script_versions_invariants CHECK (
        version > 0
        AND resource_version > 0
        AND jsonb_typeof(content_json) = 'object'
        AND content_hash ~ '^[0-9a-f]{64}$'
        AND status IN ('draft', 'confirmed', 'superseded')
        AND ((status <> 'draft' AND confirmed_at IS NOT NULL)
             OR (status = 'draft' AND confirmed_at IS NULL))
        AND char_length(btrim(schema_version)) > 0
        AND (parent_id IS NULL OR parent_id <> id)
        AND num_nonnulls(
            origin_task_id,
            model_profile_id,
            provider_id,
            model_id,
            prompt_version
        ) IN (0, 5)
        AND (model_profile_id IS NULL OR char_length(btrim(model_profile_id)) > 0)
        AND (provider_id IS NULL OR char_length(btrim(provider_id)) > 0)
        AND (model_id IS NULL OR char_length(btrim(model_id)) > 0)
        AND (prompt_version IS NULL OR char_length(btrim(prompt_version)) > 0)
    )
);

CREATE UNIQUE INDEX uq_script_versions_current
    ON public.script_versions (episode_id)
    WHERE status = 'confirmed';

CREATE UNIQUE INDEX uq_script_versions_origin_task
    ON public.script_versions (origin_task_id)
    WHERE origin_task_id IS NOT NULL;

CREATE INDEX ix_script_versions_parent_fk
    ON public.script_versions (episode_id, parent_id);

CREATE INDEX ix_script_versions_source_fk
    ON public.script_versions (episode_id, source_revision_id);

CREATE INDEX ix_script_versions_origin_task_fk
    ON public.script_versions (episode_id, origin_task_id);
