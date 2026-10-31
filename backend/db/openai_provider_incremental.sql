-- SRC-AI-31: bootstrap OpenAI for existing installations without rewriting snapshots.
INSERT INTO catalog.provider_preset(id,slug,origin)
VALUES('6da07a10-0000-4000-8000-000000000009','openai','builtin')
ON CONFLICT(slug) DO NOTHING;

INSERT INTO catalog.provider_preset_version(id,preset_id,version_no,payload,content_hash)
SELECT '6da07a11-0000-4000-8000-000000000009',id,1,
 '{"schema_version":1,"name":"OpenAI","config":{"adapter_key":"openai","base_url":"https://api.openai.com/v1","region":"overseas","capabilities":["text","image"],"timeout_seconds":120,"concurrency_limit":2,"rate_limit_per_min":30}}'::jsonb,
 'adf7b0f4aafb3fcf51928c275d6e501d509f0b0009a576bb56570e8fdae66be5'
FROM catalog.provider_preset WHERE slug='openai' AND origin='builtin'
ON CONFLICT(preset_id,version_no) DO NOTHING;
