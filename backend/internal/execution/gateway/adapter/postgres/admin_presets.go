package postgres

import (
	"encoding/json"
	"reflect"
	"strings"

	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
)

// modelPresets reads only published catalogue defaults for the onboarding form.
// The projection deliberately excludes credentials and account-specific pricing.
func modelPresets(tx *gorm.DB) ([]json.RawMessage, error) {
	var rows []struct{ Document []byte }
	err := tx.Raw(`SELECT jsonb_build_object(
 'key',v.id::text || '/' || m.model_key, 'provider',v.payload->>'name',
 'adapter',m.payload->>'adapter_key','base_url',v.payload->'config'->>'base_url',
 'name',m.payload->>'display_name','upstream',m.payload->>'provider_model_id',
 'capability',m.payload->>'capability','modes',m.payload->'modes',
 'fields',COALESCE(m.payload->'execution'->'param_schema','[]'::jsonb),
 'concurrency',v.payload->'config'->'concurrency_limit','rpm',v.payload->'config'->'rate_limit_per_min',
 'rates',COALESCE(price.reference_lines,'[]'::jsonb),'official_price_observed_on',price.official_price_observed_on,
 'currency',tariff.currency,'price_unit',tariff.unit,'price_fx',reviewed.fx,'video_estimate',price.video_estimate,
 'price_source_url',tariff.source_url,'quick_onboarding', CASE WHEN p.origin='builtin' AND price.quick_onboarding IS NOT NULL AND price.video_estimate IS NOT NULL
 AND m.payload->'modes' @> '["text2video"]'::jsonb THEN
 jsonb_strip_nulls(jsonb_build_object('resolution',price.quick_onboarding->>'resolution')) END,'request_micros',CASE WHEN tariff.unit IN ('per_image','per_request') AND tariff.current
 AND (tariff.rule - ARRAY['base_micros','pricing_strategy'])='{}'::jsonb
 AND COALESCE(tariff.rule->>'pricing_strategy','') IN ('','image_flat_v1')
 THEN tariff.rule->'base_micros' END,
 'media_tariff',CASE WHEN m.payload->>'capability' IN ('audio.generate','image.generate','video.generate') AND tariff.current
 AND (reviewed.unit IS NULL OR reviewed.rule=price.rule AND reviewed.currency::text=price.currency)
 AND price.gateway_media_tariff IS NOT NULL THEN price.gateway_media_tariff || jsonb_build_object(
   'source_url',price.source_url,'observed_on',price.official_price_observed_on
 ) END,
 'text_tariff',CASE WHEN m.payload->>'capability'='text.generate' AND tariff.current
 AND (reviewed.unit IS NULL OR reviewed.rule=price.rule AND reviewed.currency::text=price.currency)
 AND price.gateway_text_tariff IS NOT NULL THEN price.gateway_text_tariff || jsonb_build_object(
   'source_url',price.source_url,'observed_on',price.official_price_observed_on
 ) END,
 'token_price',CASE WHEN m.payload->>'capability'='text.generate' AND tariff.unit='per_1k_tokens' AND tariff.current
 AND COALESCE(tariff.rule->>'pricing_strategy','') IN ('','token_io_v1')
 AND (tariff.rule - ARRAY['input_micros_per_1k','cached_input_micros_per_1k','output_micros_per_1k','pricing_strategy'])='{}'::jsonb
 AND jsonb_typeof(tariff.rule->'input_micros_per_1k')='number'
 AND jsonb_typeof(tariff.rule->'output_micros_per_1k')='number'
 AND COALESCE(jsonb_typeof(tariff.rule->'cached_input_micros_per_1k'),'number')='number'
 THEN jsonb_strip_nulls(jsonb_build_object(
   'input',(tariff.rule->>'input_micros_per_1k')::numeric * 1000,
   'cache_read',COALESCE(tariff.rule->>'cached_input_micros_per_1k',tariff.rule->>'input_micros_per_1k')::numeric * 1000,
   'cache_write',CASE WHEN reviewed.unit IS NULL AND price.adapter_key IN ('openrouter','vercel')
     AND (price.official_price->>'input_cache_write') ~ '^[0-9]+([.][0-9]+)?$'
     THEN trim_scale((price.official_price->>'input_cache_write')::numeric * 1000000000000) END,
   'output',(tariff.rule->>'output_micros_per_1k')::numeric * 1000
 )) END) AS document
 FROM catalog.provider_preset p
 JOIN LATERAL (SELECT id,payload FROM catalog.provider_preset_version WHERE preset_id=p.id
 ORDER BY version_no DESC LIMIT 1) v ON true
 JOIN catalog.model_preset m ON m.preset_version_id=v.id
 LEFT JOIN catalog.model_price_preset price ON price.adapter_key=m.payload->>'adapter_key'
 AND price.provider_model_id=m.payload->>'provider_model_id'
 LEFT JOIN LATERAL (
  SELECT proc.unit,proc.rule,proc.currency,proc.source_url,proc.fx_rate_to_cny::text AS fx
  FROM catalog.model_price_candidate snapshot
  JOIN catalog.price_rule_version proc ON proc.id=snapshot.published_price_version_id
  WHERE snapshot.adapter_key=m.payload->>'adapter_key'
  AND snapshot.provider_model_id=m.payload->>'provider_model_id' AND snapshot.status='published'
  AND NOT proc.is_delete AND proc.effective_from<=now() AND proc.reviewed_at<=now()
  AND proc.currency IN ('CNY','USD') AND proc.source_url IS NOT NULL
  ORDER BY proc.effective_from DESC,proc.reviewed_at DESC,proc.id LIMIT 1
 ) reviewed ON true
 CROSS JOIN LATERAL (SELECT
  COALESCE(reviewed.unit,price.unit) AS unit,COALESCE(reviewed.rule,price.rule) AS rule,
  COALESCE(reviewed.currency::text,price.currency) AS currency,
  COALESCE(reviewed.source_url,price.source_url) AS source_url,
  (reviewed.unit IS NOT NULL OR price.executable_until IS NULL OR price.executable_until>=CURRENT_DATE) AS current
 ) tariff
 WHERE NOT p.archived AND NOT p.emergency_disabled
 AND m.payload->>'adapter_key'=v.payload->'config'->>'adapter_key'
 AND m.payload->>'lifecycle' IN ('active','alias')
 AND m.payload->>'provider_model_id'<>''
 AND m.payload->>'requires_model_selection'='false'
 AND (m.payload->>'capability'<>'audio.generate' OR m.payload->>'adapter_key'='volcengine_speech')
 ORDER BY p.slug,m.ordinal,m.model_key`).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]json.RawMessage, 0, len(rows))
	for _, row := range rows {
		out = append(out, json.RawMessage(row.Document))
	}
	return out, nil
}

// validatePresetTariff prevents a public tariff from being submitted for a
// different upstream ID, URL or currency. Explicit custom contracts use no preset.
func validatePresetTariff(tx *gorm.DB, key string, price domain.Price, route *domain.Route) error {
	if key == "" || price.TextTariff == nil && price.MediaTariff == nil {
		return nil
	}
	rows, err := modelPresets(tx)
	if err != nil {
		return err
	}
	for _, raw := range rows {
		var preset struct {
			Key         string              `json:"key"`
			Currency    string              `json:"currency"`
			BaseURL     string              `json:"base_url"`
			Upstream    string              `json:"upstream"`
			TextTariff  *domain.TextTariff  `json:"text_tariff"`
			MediaTariff *domain.MediaTariff `json:"media_tariff"`
		}
		if err := json.Unmarshal(raw, &preset); err != nil {
			return err
		}
		if preset.Key != key {
			continue
		}
		if preset.Currency != price.Currency || !reflect.DeepEqual(preset.TextTariff, price.TextTariff) ||
			!reflect.DeepEqual(preset.MediaTariff, price.MediaTariff) ||
			route != nil && (route.UpstreamModel != preset.Upstream || strings.TrimRight(route.BaseURL, "/") != strings.TrimRight(preset.BaseURL, "/")) {
			return domain.ErrInvalid
		}
		return nil
	}
	return domain.ErrInvalid
}
