package postgres

// Only installed models with catalog provenance enter preset pricing review.
// Effective sales are resolved from the active release, not the latest edit.
const adminPresetPricingQuery = `
WITH priced AS (
  SELECT provider.adapter_key AS provider_adapter_key,provider.logo_asset_id AS provider_logo_asset_id,m.id,m.model_key,m.provider_id,m.capability,m.display_name,m.status,m.current_version_id,m.revision,m.create_time,m.update_time,
    EXISTS (SELECT 1 FROM catalog.model_release old WHERE old.model_id=m.id AND old.status IN ('active','retired')) AS has_publication,
    latest.id IS NOT NULL AS has_price,
    COALESCE(active_price.unit, sale.unit, latest.unit, '') AS billing_unit,
    purchase.id IS NOT NULL AS procurement_ready,
    sale.id IS NOT NULL AS sale_ready,
    COALESCE(sale.id=active_price.id,false) AS sale_published,
    provider.status='active' AND EXISTS (
      SELECT 1 FROM catalog.provider_credential c WHERE c.provider_id=provider.id AND c.status='active' AND NOT c.is_delete
    ) AS provider_ready
  FROM catalog.model_profile m
  JOIN catalog.provider provider ON provider.id=m.provider_id AND NOT provider.is_delete
  LEFT JOIN catalog.model_release r ON r.id=m.active_release_id AND r.model_id=m.id AND r.status='active'
  LEFT JOIN catalog.provider_config_version cfg ON cfg.id=r.config_version_id AND cfg.provider_id=provider.id
  LEFT JOIN catalog.provider_preset_version pv ON pv.id=cfg.preset_version_id
  LEFT JOIN catalog.provider_preset preset ON preset.id=pv.preset_id AND NOT preset.emergency_disabled
  LEFT JOIN LATERAL (
    SELECT p.id,p.unit FROM catalog.price_rule_version p WHERE p.model_profile_id=m.id AND NOT p.is_delete
    ORDER BY p.version_no DESC LIMIT 1
  ) latest ON true
  LEFT JOIN LATERAL (
    SELECT p.id FROM catalog.price_rule_version p WHERE p.model_profile_id=m.id AND NOT p.is_delete
      AND p.source_url IS NOT NULL AND p.reviewed_at<=now()
      AND NOT jsonb_exists(p.rule,'procurement_price_version_id') AND NOT jsonb_exists(p.rule,'markup_multiplier') AND NOT jsonb_exists(p.rule,'discount_factor')
    ORDER BY p.version_no DESC LIMIT 1
  ) purchase ON true
  LEFT JOIN LATERAL (
    SELECT p.id,p.unit FROM catalog.price_rule_version p
    JOIN catalog.price_rule_version tariff ON tariff.id::text=p.rule->>'procurement_price_version_id'
      AND tariff.model_profile_id=m.id AND tariff.unit=p.unit AND NOT tariff.is_delete
    WHERE p.model_profile_id=m.id AND NOT p.is_delete
      AND p.source_url IS NOT NULL AND p.reviewed_at<=now()
      AND tariff.source_url IS NOT NULL AND tariff.reviewed_at<=now() AND tariff.effective_from<=p.effective_from
      AND jsonb_exists(p.rule,'cached_input_micros_per_1k')=jsonb_exists(tariff.rule,'cached_input_micros_per_1k')
      AND jsonb_exists(p.rule,'reference_scope')=jsonb_exists(tariff.rule,'reference_scope')
      AND NOT jsonb_exists(tariff.rule,'markup_multiplier') AND NOT jsonb_exists(tariff.rule,'discount_factor') AND NOT jsonb_exists(tariff.rule,'procurement_price_version_id')
    ORDER BY (p.id=r.price_version_id AND p.effective_from<=now()) DESC NULLS LAST, p.version_no DESC LIMIT 1
  ) sale ON true
  LEFT JOIN catalog.price_rule_version active_price ON active_price.id=r.price_version_id
    AND active_price.model_profile_id=m.id AND NOT active_price.is_delete AND active_price.effective_from<=now()
    AND jsonb_exists(active_price.rule,'procurement_price_version_id') AND preset.id IS NOT NULL
  WHERE NOT m.is_delete AND provider.adapter_key IN ? AND (
    m.model_key='preset-' || m.id::text OR
    (m.candidate_snapshot->>'catalog_version' IS NOT NULL
      AND COALESCE(m.candidate_snapshot->'model'->>'source_url','') NOT LIKE 'custom://%')
  )
), models AS (
  SELECT priced.*, CASE
    WHEN status='archived' THEN 'inactive'
    WHEN sale_published AND status='active' AND provider_ready THEN 'billable'
    WHEN sale_published OR (has_publication AND status='disabled') THEN 'inactive'
    WHEN NOT sale_ready THEN 'price_pending'
    ELSE 'usage_pending'
  END AS billing_status FROM priced
)
SELECT * FROM models m WHERE TRUE`
