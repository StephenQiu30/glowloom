-- Lanverse MVP table: public.media_objects
CREATE TABLE public.media_objects (
    id uuid NOT NULL,
    episode_id uuid NOT NULL,
    media_kind text NOT NULL,
    source_kind text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_media_objects PRIMARY KEY (id),
    CONSTRAINT fk_media_objects_episode FOREIGN KEY (episode_id)
        REFERENCES public.episodes (id)
        MATCH SIMPLE ON UPDATE RESTRICT ON DELETE RESTRICT
        NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT ck_media_objects_invariants CHECK (
        (source_kind = 'provider' AND media_kind IN ('image', 'video', 'audio'))
        OR (source_kind = 'ffmpeg' AND media_kind = 'video')
        OR (
            source_kind = 'application'
            AND media_kind IN ('subtitle', 'manifest')
        )
    )
);

CREATE INDEX ix_media_objects_episode_created_at
    ON public.media_objects (episode_id, created_at);
