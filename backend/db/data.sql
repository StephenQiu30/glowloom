-- Built-in public pricing references. Apply after schema.sql as the schema owner.
-- This seed contains no connection, credential, account pricing, or release data.
WITH sources AS (
    SELECT 'https://docs.volcengine.com/docs/ark/model-pricing'::text AS ark,
           'https://api-docs.deepseek.com/zh-cn/quick_start/pricing/'::text AS deepseek,
           'https://developers.openai.com/api/docs/pricing'::text AS openai
), presets(adapter_key, provider_model_id, currency, source_url, reference_lines, unit, rule, executable_until) AS (
    SELECT 'volcengine_ark', 'doubao-seedream-5-0-flash-260915', 'CNY', ark,
           ARRAY['生成图片 ¥0.12 / 张', '参考图片免费'], 'per_image', '{"base_micros":120000}'::jsonb, NULL::date FROM sources
    UNION ALL SELECT 'volcengine_ark', 'doubao-seedream-5-0-pro-260628', 'CNY', ark,
           ARRAY['按输出像素计费：≤261 万像素 ¥0.30 / 张，>261 万 ¥0.60 / 张',
                 '图层拆分：两档分别为 ¥0.15 和 ¥0.30 / 层',
                 '参考图片：首张免费，从第 2 张起 ¥0.02 / 张'], NULL, NULL, NULL FROM sources
    UNION ALL SELECT 'volcengine_ark', 'doubao-seedance-2-0-mini-260615', 'CNY', ark,
           ARRAY['480p / 720p，每百万视频 Token：无视频参考 ¥23，有视频参考 ¥14'], NULL, NULL, NULL FROM sources
    UNION ALL SELECT 'volcengine_ark', 'doubao-seedance-2-0-fast-260128', 'CNY', ark,
           ARRAY['480p / 720p，每百万视频 Token：无视频参考 ¥37，有视频参考 ¥22'], NULL, NULL, NULL FROM sources
    UNION ALL SELECT 'volcengine_ark', 'doubao-seedance-2-0-260128', 'CNY', ark,
           ARRAY['每百万视频 Token（无视频参考 / 有视频参考）',
                 '480p / 720p：¥46 / ¥28；1080p：¥51 / ¥31；4K：¥26 / ¥16'], NULL, NULL, NULL FROM sources
    UNION ALL SELECT 'volcengine_ark', 'doubao-seedance-2-5-260628', 'CNY', ark,
           ARRAY['正式生成，每百万视频 Token（无视频参考 / 有视频参考）',
                 '480p / 720p：¥70 / ¥42；1080p：¥77 / ¥46', 'Draft 样片与正式生成分开计费'], NULL, NULL, NULL FROM sources
    UNION ALL SELECT 'volcengine_ark', 'doubao-seed-2-1-lite-260915', 'CNY', ark,
           ARRAY['每百万 Token：文本输入 ¥0.80、缓存命中 ¥0.16、输出 ¥2.70',
                 '音频输入 ¥12、缓存命中 ¥2.40；缓存存储 ¥0.017 / 百万 Token / 小时'], NULL, NULL, NULL FROM sources
    UNION ALL SELECT 'volcengine_ark', 'doubao-seed-2-1-pro-260915', 'CNY', ark,
           ARRAY['每百万 Token：输入 ¥6、缓存命中 ¥1.20、输出 ¥30',
                 '缓存存储 ¥0.017 / 百万 Token / 小时'], NULL, NULL, NULL FROM sources
    UNION ALL SELECT 'deepseek', 'deepseek-flash', 'CNY', deepseek,
           ARRAY['每百万 Token（未命中输入 / 命中输入 / 输出）',
                 '高峰 ¥2 / ¥0.04 / ¥8；空闲 ¥1 / ¥0.02 / ¥4'], 'per_1k_tokens',
           '{"input_micros_per_1k":2000,"cached_input_micros_per_1k":40,"output_micros_per_1k":8000,"time_pricing":"deepseek_cn_2026"}', DATE '2027-01-01' FROM sources
    UNION ALL SELECT 'deepseek', 'deepseek-v4-pro', 'CNY', deepseek,
           ARRAY['每百万 Token（未命中输入 / 命中输入 / 输出）',
                 '高峰 ¥9 / ¥0.30 / ¥27；空闲 ¥4.50 / ¥0.15 / ¥13.50'], 'per_1k_tokens',
           '{"input_micros_per_1k":9000,"cached_input_micros_per_1k":300,"output_micros_per_1k":27000,"time_pricing":"deepseek_cn_2026"}', DATE '2027-01-01' FROM sources
    UNION ALL SELECT 'openai', id, 'USD', openai,
           ARRAY['标准档，每百万 Token：文本输入 $5、图片输入 $8、图片输出 $30',
                 '缓存输入：文本 $1.25、图片 $2'], NULL, NULL, NULL
           FROM sources CROSS JOIN (VALUES ('gpt-image-2.5-flare'), ('gpt-image-2.5-sunburst')) AS image(id)
    UNION ALL SELECT 'openai', id, 'USD', openai,
           ARRAY['标准短上下文，每百万 Token（输入 / 缓存命中 / 输出）', rates,
                 '长上下文、缓存写入和其他服务档另计'], NULL, NULL, NULL
           FROM sources CROSS JOIN (VALUES
             ('gpt-6.1-sol', '$2 / $0.10 / $10'),
             ('gpt-6.1-terra', '$1 / $0.05 / $5'),
             ('gpt-6-luna', '$0.10 / $0.01 / $0.50'),
             ('gpt-6-astra', '$10 / $1 / $50')) AS chat(id, rates)
)
INSERT INTO catalog.model_price_preset (adapter_key, provider_model_id, currency, source_url, reference_lines, unit, rule, executable_until)
SELECT adapter_key, provider_model_id, currency, source_url, to_jsonb(reference_lines), unit, rule, executable_until FROM presets
ON CONFLICT (adapter_key, provider_model_id) DO UPDATE
SET currency=EXCLUDED.currency, source_url=EXCLUDED.source_url, reference_lines=EXCLUDED.reference_lines,
    unit=EXCLUDED.unit, rule=EXCLUDED.rule, executable_until=EXCLUDED.executable_until;
