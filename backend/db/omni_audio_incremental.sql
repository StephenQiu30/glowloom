-- Seed-Audio adds a distinct mode; keep all existing capability contracts.
UPDATE catalog.capability SET modes=ARRAY(SELECT DISTINCT x FROM unnest(modes || ARRAY['omni_audio']) x), input_roles=ARRAY(SELECT DISTINCT x FROM unnest(input_roles || ARRAY['reference_audio','reference_image']) x) WHERE key='audio.generate';

-- A new immutable preset version; existing connection snapshots stay unchanged.
INSERT INTO catalog.provider_preset_version(id,preset_id,version_no,payload,content_hash)
SELECT '6da07a12-0000-4000-8000-000000000005',id,2,'{"schema_version":1,"name":"豆包语音","config":{"adapter_key":"volcengine_speech","base_url":"https://openspeech.bytedance.com/api/v3","region":"domestic","capabilities":["audio"],"timeout_seconds":300,"concurrency_limit":2,"rate_limit_per_min":30}}'::jsonb,'76c0029f543321dec270d711133d523c3ad91eb8819eab84eca7459780986682' FROM catalog.provider_preset WHERE slug='volcengine-speech' AND origin='builtin' ON CONFLICT(preset_id,version_no) DO NOTHING;
