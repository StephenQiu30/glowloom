-- Lanverse MVP table: public.adoptions
CREATE TABLE public.adoptions (
    id uuid NOT NULL,
    episode_id uuid NOT NULL,
    usage_type text NOT NULL,
    usage_id uuid NOT NULL,
    input_version_id uuid NOT NULL,
    input_hash varchar(64) NOT NULL,
    version integer NOT NULL,
    candidate_id uuid NOT NULL,
    supersedes_id uuid,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    superseded_at timestamptz,
    CONSTRAINT pk_adoptions PRIMARY KEY (id),
    CONSTRAINT uq_adoptions_slot_version UNIQUE (
        usage_type,
        usage_id,
        input_version_id,
        input_hash,
        version
    ),
    CONSTRAINT uq_adoptions_episode_slot_id UNIQUE (
        episode_id,
        usage_type,
        usage_id,
        input_version_id,
        input_hash,
        id
    ),
    CONSTRAINT fk_adoptions_episode FOREIGN KEY (episode_id)
        REFERENCES public.episodes (id)
        MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT fk_adoptions_candidate FOREIGN KEY (
        episode_id,
        usage_type,
        usage_id,
        input_version_id,
        input_hash,
        candidate_id
    ) REFERENCES public.generation_candidates (
        episode_id,
        usage_type,
        usage_id,
        input_version_id,
        input_hash,
        id
    ) MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT fk_adoptions_supersedes FOREIGN KEY (
        episode_id,
        usage_type,
        usage_id,
        input_version_id,
        input_hash,
        supersedes_id
    ) REFERENCES public.adoptions (
        episode_id,
        usage_type,
        usage_id,
        input_version_id,
        input_hash,
        id
    ) MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT ck_adoptions_invariants CHECK (
        input_hash ~ '^[0-9a-f]{64}$'
        AND version > 0
        AND usage_type IN (
            'asset_image',
            'shot_image',
            'shot_video',
            'speech_audio'
        )
        AND status IN ('active', 'superseded')
        AND (
            (status = 'superseded' AND superseded_at IS NOT NULL)
            OR (status = 'active' AND superseded_at IS NULL)
        )
        AND (supersedes_id IS NULL OR supersedes_id <> id)
    )
);

CREATE UNIQUE INDEX uq_adoptions_active_slot
    ON public.adoptions (usage_type, usage_id, input_version_id, input_hash)
    WHERE status = 'active';

CREATE INDEX ix_adoptions_episode_slot_created_at
    ON public.adoptions (
        episode_id,
        usage_type,
        usage_id,
        input_version_id,
        input_hash,
        created_at DESC
    );

CREATE INDEX ix_adoptions_candidate_fk
    ON public.adoptions (
        episode_id,
        usage_type,
        usage_id,
        input_version_id,
        input_hash,
        candidate_id
    );

CREATE INDEX ix_adoptions_supersedes_fk
    ON public.adoptions (
        episode_id,
        usage_type,
        usage_id,
        input_version_id,
        input_hash,
        supersedes_id
    );
