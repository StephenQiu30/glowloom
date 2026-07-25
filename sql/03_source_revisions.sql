-- Lanverse MVP table: public.source_revisions
CREATE TABLE public.source_revisions (
    id uuid NOT NULL,
    episode_id uuid NOT NULL,
    version integer NOT NULL,
    parent_id uuid,
    content text NOT NULL,
    normalization_version text NOT NULL,
    codepoint_count integer NOT NULL,
    sha256 varchar(64) NOT NULL,
    rights_basis text NOT NULL,
    rights_declared_at timestamptz NOT NULL,
    status text NOT NULL DEFAULT 'draft',
    resource_version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    confirmed_at timestamptz,
    CONSTRAINT pk_source_revisions PRIMARY KEY (id),
    CONSTRAINT uq_source_revisions_episode_version UNIQUE (episode_id, version),
    CONSTRAINT uq_source_revisions_episode_id UNIQUE (episode_id, id),
    CONSTRAINT fk_source_revisions_episode FOREIGN KEY (episode_id)
        REFERENCES public.episodes (id)
        MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT fk_source_revisions_parent FOREIGN KEY (episode_id, parent_id)
        REFERENCES public.source_revisions (episode_id, id)
        MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT ck_source_revisions_invariants CHECK (
        version > 0
        AND resource_version > 0
        AND sha256 ~ '^[0-9a-f]{64}$'
        AND status IN ('draft', 'confirmed', 'superseded')
        AND ((status <> 'draft' AND confirmed_at IS NOT NULL)
             OR (status = 'draft' AND confirmed_at IS NULL))
        AND codepoint_count = char_length(content)
        AND codepoint_count BETWEEN 300 AND 3000
        AND normalization_version = 'text-normalization-v1'
        AND rights_basis IN ('original', 'licensed')
        AND (parent_id IS NULL OR parent_id <> id)
    )
);

CREATE UNIQUE INDEX uq_source_revisions_current
    ON public.source_revisions (episode_id)
    WHERE status = 'confirmed';

CREATE INDEX ix_source_revisions_parent_fk
    ON public.source_revisions (episode_id, parent_id);
