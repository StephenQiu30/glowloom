// Copy only portable reviewed values from the explicitly selected binding.
export function templateFromRelease(
  detail: API.AdminModelDetail,
  release: API.ModelRelease,
): API.PresetModelTemplate {
  const model = detail.model;
  const version = detail.versions?.find(
    (v) => v.id === release.model_version_id,
  );
  const price = detail.prices?.find((p) => p.id === release.price_version_id);
  const cost = release.upstream_cost;
  if (
    !model?.id ||
    release.model_id !== model.id ||
    !release.id ||
    !version?.provider_model_id ||
    !version.modes?.length ||
    !price?.unit ||
    !price.rule ||
    !cost?.source ||
    !cost.reviewed_at ||
    cost.max_micros == null
  )
    throw new Error("来源发布的模型、价格或成本记录不完整。");
  if (
    version.queue !== "provider" ||
    version.moderation !== "provider" ||
    version.supports_cancel ||
    version.supports_callback ||
    version.supports_query !== (model.capability === "video.generate")
  )
    throw new Error("来源发布使用了当前预设不支持的执行配置。");
  if (
    ![
      "per_request",
      "per_image",
      "per_second",
      "per_1k_tokens",
      "per_1k_chars",
    ].includes(price.unit)
  )
    throw new Error("来源发布的价格单位不受支持。");
  return {
    key: `model-${release.id.slice(0, 8)}`,
    name: model.display_name,
    capability: model.capability,
    provider_model_id: version.provider_model_id,
    modes: version.modes,
    limits: version.limits,
    param_schema: version.param_schema,
    expected_max_ms: version.expected_max_ms,
    price: {
      unit: price.unit as API.PriceUnit,
      rule: price.rule,
      currency: price.currency,
      fx_rate_to_cny: price.fx_rate_to_cny ?? "",
    },
    upstream_cost: {
      currency: cost.currency,
      max_micros: cost.max_micros,
      source: cost.source,
      reviewed_at: cost.reviewed_at,
    },
  };
}
