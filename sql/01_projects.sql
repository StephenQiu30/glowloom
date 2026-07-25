-- Lanverse MVP table: public.projects
CREATE TABLE public.projects (
    id uuid NOT NULL,
    title varchar(120) NOT NULL,
    status text NOT NULL DEFAULT 'active',
    aspect_ratio text NOT NULL DEFAULT '9:16',
    width integer NOT NULL DEFAULT 720,
    height integer NOT NULL DEFAULT 1280,
    fps integer NOT NULL DEFAULT 24,
    timebase bigint NOT NULL DEFAULT 90000,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_projects PRIMARY KEY (id),
    CONSTRAINT ck_projects_invariants CHECK (
        char_length(btrim(title)) > 0
        AND char_length(title) <= 120
        AND status = 'active'
        AND aspect_ratio = '9:16'
        AND width = 720
        AND height = 1280
        AND fps = 24
        AND timebase = 90000
    )
);
