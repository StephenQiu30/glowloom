-- SRC-AI-34: bootstrap Vercel AI Gateway for existing installations without rewriting snapshots.
INSERT INTO catalog.provider_preset(id,slug,origin)
VALUES('6da07a10-0000-4000-8000-00000000000a','vercel','builtin')
ON CONFLICT(slug) DO NOTHING;

INSERT INTO catalog.provider_preset_version(id,preset_id,version_no,payload,content_hash)
SELECT '6da07a11-0000-4000-8000-00000000000a',id,1,
 '{"schema_version":1,"name":"Vercel","config":{"adapter_key":"vercel","base_url":"https://ai-gateway.vercel.sh/v1","region":"overseas","capabilities":["text","image"],"timeout_seconds":120,"concurrency_limit":2,"rate_limit_per_min":60}}'::jsonb,
 '7d956200f9fdcf5c429faeb3abf459a45b09cfcb280366e98f478ef41842e2d2'
FROM catalog.provider_preset WHERE slug='vercel' AND origin='builtin'
ON CONFLICT(preset_id,version_no) DO NOTHING;
