-- SRC-AI-28: additive bootstrap for the separately authenticated Doubao speech service.
INSERT INTO catalog.capability(id,key,output_type,modes,input_roles)
VALUES('6da07a12-0000-4000-8000-000000000004','audio.generate','audio',ARRAY['text2audio'],ARRAY['prompt'])
ON CONFLICT(key) DO NOTHING;
INSERT INTO catalog.provider_preset(id,slug,origin)
VALUES('6da07a10-0000-4000-8000-000000000005','volcengine-speech','builtin')
ON CONFLICT(slug) DO NOTHING;
INSERT INTO catalog.provider_preset_version(id,preset_id,version_no,payload,content_hash)
SELECT '6da07a11-0000-4000-8000-000000000005',id,1,'{"schema_version":1,"name":"豆包语音","config":{"adapter_key":"volcengine_speech","base_url":"https://openspeech.bytedance.com/api/v3","region":"domestic","capabilities":["audio"],"timeout_seconds":120,"concurrency_limit":2,"rate_limit_per_min":30}}'::jsonb,'eab8b04b9e1de10b21827a6c2603ffed795f326f9b617d801bc99f275f196b0c'
FROM catalog.provider_preset WHERE slug='volcengine-speech' AND origin='builtin'
ON CONFLICT(preset_id,version_no) DO NOTHING;

-- Seedance 2.5 can consume authorized video/audio references in omni mode.
UPDATE catalog.capability SET input_roles=ARRAY(
 SELECT DISTINCT role FROM unnest(input_roles || ARRAY['reference_video','reference_audio']) AS role ORDER BY role
) WHERE key='video.generate';
