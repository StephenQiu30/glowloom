-- Lanverse MVP table: public.episodes
CREATE TABLE public.episodes (
    id uuid NOT NULL,
    project_id uuid NOT NULL,
    target_min_ticks bigint NOT NULL DEFAULT 2700000,
    target_max_ticks bigint NOT NULL DEFAULT 5400000,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_episodes PRIMARY KEY (id),
    CONSTRAINT uq_episodes_project_id UNIQUE (project_id),
    CONSTRAINT fk_episodes_project FOREIGN KEY (project_id)
        REFERENCES public.projects (id)
        MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT ck_episodes_invariants CHECK (
        target_min_ticks = 2700000
        AND target_max_ticks = 5400000
    )
);
