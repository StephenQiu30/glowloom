ALTER TABLE operation.operation ADD COLUMN prompt_preparation jsonb;

ALTER TABLE operation.operation ADD CONSTRAINT ck_operation_prompt_preparation CHECK (
    prompt_preparation IS NULL OR (
        jsonb_typeof(prompt_preparation) = 'object'
        AND octet_length(prompt_preparation::text) <= 4096
        AND prompt_preparation - ARRAY['version','operation','policy','template_id','template_version','customization_id','customization_revision','request_sha256','user_prompt_sha256','content_sha256'] = '{}'::jsonb
        AND prompt_preparation->'version' = '1'::jsonb
        AND origin = 'canvas' AND target_type = 'free'
        AND prompt_preparation->>'operation' IN (
            'chapter_assets_extract', 'character_extract', 'character_turnaround',
            'storyboard_plan', 'storyboard_repair', 'storyboard_first_frame',
            'storyboard_video', 'short_drama_outline', 'skill_draft'
        )
        AND prompt_preparation->>'request_sha256' ~ '^[0-9a-f]{64}$'
        AND prompt_preparation->>'user_prompt_sha256' ~ '^[0-9a-f]{64}$'
        AND prompt_preparation->>'content_sha256' ~ '^[0-9a-f]{64}$'
        AND ((prompt_preparation->>'operation' IN ('character_turnaround','storyboard_first_frame') AND capability = 'image.generate')
          OR (prompt_preparation->>'operation' = 'storyboard_video' AND capability = 'video.generate')
          OR (prompt_preparation->>'operation' NOT IN ('character_turnaround','storyboard_first_frame','storyboard_video') AND capability = 'text.structured'))
        AND (
            (prompt_preparation->>'policy' = 'bypass_video'
             AND prompt_preparation->>'operation' = 'storyboard_video'
             AND NOT (prompt_preparation ?| ARRAY['template_id','template_version','customization_id','customization_revision'])
             AND prompt_preparation->>'content_sha256' = prompt_preparation->>'user_prompt_sha256')
            OR
            (prompt_preparation->>'policy' = 'compiled'
             AND prompt_preparation->>'operation' <> 'storyboard_video'
             AND prompt_preparation->>'template_id' ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
             AND prompt_preparation->>'template_id' <> '00000000-0000-0000-0000-000000000000'
             AND prompt_preparation->'template_version' = '1'::jsonb
             AND ((NOT (prompt_preparation ? 'customization_id') AND NOT (prompt_preparation ? 'customization_revision'))
               OR (prompt_preparation->>'customization_id' ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
                   AND prompt_preparation->>'customization_id' <> '00000000-0000-0000-0000-000000000000'
                   AND jsonb_typeof(prompt_preparation->'customization_revision') = 'number'
                   AND prompt_preparation->>'customization_revision' ~ '^[1-9][0-9]{0,9}$'
                   AND (prompt_preparation->>'customization_revision')::numeric <= 2147483647)))
        )
    ) IS TRUE
);

COMMENT ON COLUMN operation.operation.prompt_preparation IS 'Immutable template policy evidence; final text lives only in operation_input.prompt. Existing column UPDATE grants deliberately exclude this column.';
