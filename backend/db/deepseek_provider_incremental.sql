-- SRC-AI-30: bootstrap DeepSeek for existing installations without rewriting snapshots.
INSERT INTO catalog.provider_preset(id,slug,origin)
VALUES('6da07a10-0000-4000-8000-000000000004','deepseek','builtin')
ON CONFLICT(slug) DO NOTHING;

INSERT INTO catalog.provider_preset_version(id,preset_id,version_no,payload,content_hash)
SELECT '6da07a11-0000-4000-8000-000000000004',id,1,
 '{"schema_version":1,"name":"DeepSeek","config":{"adapter_key":"deepseek","base_url":"https://api.deepseek.com","region":"domestic","capabilities":["text"],"timeout_seconds":120,"concurrency_limit":2,"rate_limit_per_min":30}}'::jsonb,
 'da870135adccd6c8d3f5ab3073ed31876fcef9c6b654c90286f5cdc2290c8f79'
FROM catalog.provider_preset WHERE slug='deepseek' AND origin='builtin'
ON CONFLICT(preset_id,version_no) DO NOTHING;
