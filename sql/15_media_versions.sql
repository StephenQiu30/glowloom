-- Lanverse MVP table: public.media_versions
CREATE TABLE public.media_versions (
    id uuid NOT NULL,
    media_object_id uuid NOT NULL,
    version integer NOT NULL,
    parent_id uuid,
    origin_attempt_id uuid NOT NULL,
    output_slot text NOT NULL,
    bucket text NOT NULL,
    object_key text NOT NULL,
    mime_type text NOT NULL,
    byte_size bigint,
    sha256 varchar(64),
    status text NOT NULL DEFAULT 'pending',
    width integer,
    height integer,
    duration_ticks bigint,
    timebase bigint,
    probe_summary_json jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    finalized_at timestamptz,
    CONSTRAINT pk_media_versions PRIMARY KEY (id),
    CONSTRAINT uq_media_versions_object_version UNIQUE (media_object_id, version),
    CONSTRAINT uq_media_versions_object_id UNIQUE (media_object_id, id),
    CONSTRAINT uq_media_versions_attempt_slot UNIQUE (origin_attempt_id, output_slot),
    CONSTRAINT uq_media_versions_id_attempt_slot
        UNIQUE (id, origin_attempt_id, output_slot),
    CONSTRAINT uq_media_versions_bucket_key UNIQUE (bucket, object_key),
    CONSTRAINT ck_media_versions_invariants CHECK (
        version > 0
        AND (sha256 IS NULL OR sha256 ~ '^[0-9a-f]{64}$')
        AND (
            probe_summary_json IS NULL
            OR jsonb_typeof(probe_summary_json) = 'object'
        )
        AND (output_slot IN ('primary', 'mp4', 'srt', 'manifest')
             OR output_slot ~ '^extra/[0-9]+$')
        AND char_length(btrim(bucket)) > 0
        AND char_length(btrim(object_key)) > 0
        AND char_length(btrim(mime_type)) > 0
        AND (byte_size IS NULL OR byte_size >= 0)
        AND (width IS NULL OR width > 0)
        AND (height IS NULL OR height > 0)
        AND (duration_ticks IS NULL OR duration_ticks > 0)
        AND (timebase IS NULL OR timebase > 0)
        AND status IN ('pending', 'ready', 'invalid')
        AND (
            (status IN ('ready', 'invalid') AND finalized_at IS NOT NULL)
            OR (status = 'pending' AND finalized_at IS NULL)
        )
        AND (
            status <> 'ready'
            OR (byte_size IS NOT NULL AND sha256 IS NOT NULL)
        )
        AND (parent_id IS NULL OR parent_id <> id)
    )
);
