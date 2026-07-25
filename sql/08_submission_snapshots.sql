-- Lanverse MVP table: public.submission_snapshots
CREATE TABLE public.submission_snapshots (
    id uuid NOT NULL,
    episode_id uuid NOT NULL,
    type text NOT NULL,
    capability text,
    input_refs_json jsonb NOT NULL,
    prompt text,
    parameters_json jsonb NOT NULL,
    parameters_hash varchar(64) NOT NULL,
    model_profile_id text,
    provider_id text,
    model_id text,
    route_version text,
    schema_version text NOT NULL,
    content_hash varchar(64) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_submission_snapshots PRIMARY KEY (id),
    CONSTRAINT uq_submission_snapshots_episode_id UNIQUE (episode_id, id),
    CONSTRAINT ck_submission_snapshots_invariants CHECK (
        jsonb_typeof(input_refs_json) = 'object'
        AND jsonb_typeof(parameters_json) = 'object'
        AND parameters_hash ~ '^[0-9a-f]{64}$'
        AND content_hash ~ '^[0-9a-f]{64}$'
        AND char_length(btrim(schema_version)) > 0
        AND (
            (
                type IN ('generate_script', 'generate_storyboard')
                AND capability = 'text'
                AND num_nonnulls(
                    model_profile_id,
                    provider_id,
                    model_id,
                    prompt,
                    route_version
                ) = 5
                AND char_length(btrim(model_profile_id)) > 0
                AND char_length(btrim(provider_id)) > 0
                AND char_length(btrim(model_id)) > 0
                AND char_length(btrim(prompt)) > 0
                AND char_length(prompt) <= 4000
                AND char_length(btrim(route_version)) > 0
                AND parameters_json <> '{}'::jsonb
            )
            OR (
                type = 'generate_media'
                AND capability IN ('image', 'video', 'tts')
                AND num_nonnulls(
                    model_profile_id,
                    provider_id,
                    model_id,
                    prompt,
                    route_version
                ) = 5
                AND char_length(btrim(model_profile_id)) > 0
                AND char_length(btrim(provider_id)) > 0
                AND char_length(btrim(model_id)) > 0
                AND char_length(btrim(prompt)) > 0
                AND char_length(prompt) <= 4000
                AND char_length(btrim(route_version)) > 0
                AND parameters_json <> '{}'::jsonb
            )
            OR (
                type = 'render_episode'
                AND num_nonnulls(
                    capability,
                    model_profile_id,
                    provider_id,
                    model_id,
                    prompt,
                    route_version
                ) = 0
                AND parameters_json = '{}'::jsonb
            )
        )
    )
);
