import { expect, it } from "vitest";
import { templateFromRelease } from "@/app/providers/_components/preset-template";
const detail: API.AdminModelDetail = {
  model: { id: "model", display_name: "Text", capability: "text.generate" },
  versions: [
    { id: "new", provider_model_id: "new" },
    {
      id: "old",
      provider_model_id: "reviewed",
      modes: ["text_to_text"],
      limits: { max_outputs: 1 },
      param_schema: [],
      expected_max_ms: 60000,
      queue: "provider",
      moderation: "provider",
      supports_query: false,
    },
  ],
  prices: [
    {
      id: "price",
      unit: "per_request",
      rule: { base_micros: 10 },
      currency: "CNY",
    },
  ],
};
const release: API.ModelRelease = {
  id: "abcdef12-1111-4111-8111-111111111111",
  model_id: "model",
  model_version_id: "old",
  price_version_id: "price",
  status: "active",
  validated_modes: ["text_to_text"],
  upstream_cost: {
    currency: "CNY",
    max_micros: 20,
    source: "reviewed fixture",
    reviewed_at: "2026-10-18T00:00:00Z",
  },
};
it("复制明确发布的模型和价格，排除身份、验证和上线状态", () => {
  const t = templateFromRelease(detail, release);
  expect(t.provider_model_id).toBe("reviewed");
  expect(t.price?.rule).toEqual({ base_micros: 10 });
  for (const k of [
    "model_id",
    "id",
    "status",
    "validated_modes",
    "credential_id",
  ])
    expect(t).not.toHaveProperty(k);
});
it("拒绝跨模型、丢失版本或不支持的执行配置", () => {
  expect(() =>
    templateFromRelease(detail, { ...release, model_id: "other" }),
  ).toThrow();
  expect(() =>
    templateFromRelease(detail, { ...release, model_version_id: "missing" }),
  ).toThrow();
  expect(() =>
    templateFromRelease(
      {
        ...detail,
        versions: [{ ...detail.versions![1], supports_cancel: true }],
      },
      release,
    ),
  ).toThrow();
});
