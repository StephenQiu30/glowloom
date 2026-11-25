package postgres

import (
	"encoding/json"

	"gorm.io/gorm"
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
 'rates',COALESCE(price.reference_lines,'[]'::jsonb),
 'currency',price.currency,'request_micros',CASE WHEN price.unit IN ('per_image','per_request')
 AND (price.executable_until IS NULL OR price.executable_until>=CURRENT_DATE)
 THEN price.rule->'base_micros' END) AS document
 FROM catalog.provider_preset p
 JOIN LATERAL (SELECT id,payload FROM catalog.provider_preset_version WHERE preset_id=p.id
 ORDER BY version_no DESC LIMIT 1) v ON true
 JOIN catalog.model_preset m ON m.preset_version_id=v.id
 LEFT JOIN catalog.model_price_preset price ON price.adapter_key=m.payload->>'adapter_key'
 AND price.provider_model_id=m.payload->>'provider_model_id'
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
