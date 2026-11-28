/** All management responses are synthetic; this fixture never contacts suppliers. */
import React from "react";
import { createRoot } from "react-dom/client";
import { AiServiceAdmin } from "../../src/components/ai-service/ai-service-admin";
import { Toaster } from "../../src/components/ui/sonner";
import { RequestError } from "../../src/lib/request";
import { SpaceContext } from "../../src/components/spaces/space-context";
import { installDevelopmentTransport } from "../../src/lib/development-transport";
import type {
  GatewayModel,
  GatewayPreset,
  GatewayRoute,
  OnboardingReview,
} from "../../src/lib/ai-service-admin";
import "../../src/app/globals.css";
const showPrepared = new URLSearchParams(location.search).has("prepared");
const candidate: OnboardingReview = {
  id: "44444444-4444-4444-8444-444444444444",
  model_id: "33333333-3333-4333-8333-333333333333",
  state: "prepared",
  upstream_model: "fixture-video",
  scope_id: "11111111-1111-4111-8111-111111111111",
  project_id: "22222222-2222-4222-8222-222222222222",
  plan: {
    credits: 75,
    purchase_micros: 7452000,
    probes: [{ name: "文生视频", input: {} }],
  },
  progress: [],
};
const showArkEstimate = new URLSearchParams(location.search).has(
  "ark-estimate",
);
const arkPreset = {
  key: "volcengine-ark/seedance-25",
  provider: "火山方舟",
  name: "Seedance 2.5",
  upstream: "doubao-seedance-2-5-260628",
  adapter: "volcengine_ark",
  base_url: "https://ark.cn-beijing.volces.com/api/v3",
  capability: "video.generate",
  modes: ["text2video"],
  fields: [],
  rates: [],
  currency: "CNY",
  request_micros: null,
  concurrency: 1,
  rpm: 60,
  video_estimate: {
    min_reference_seconds: 2,
    max_reference_seconds: 30,
    min_output_seconds: 4,
    max_output_seconds: 30,
    resolutions: {
      "720p": {
        cny_per_million_without_video: 70,
        geometries: { "16:9": { width: 1280, height: 720 } },
      },
      "1080p": {
        cny_per_million_without_video: 77,
        geometries: { "16:9": { width: 1920, height: 1080 } },
      },
    },
  },
  price_source_url: "https://docs.volcengine.com/docs/ark/model-pricing",
} satisfies GatewayPreset;
const params = new URLSearchParams(location.search);
const showCreditReview = params.has("credit-review");
let creditOrder: API.CreditGrantOrder = {
  id: "55555555-5555-4555-8555-555555555555",
  scope_id: "22222222-2222-4222-8222-222222222222",
  proposer_id: params.has("own-order")
    ? "11111111-1111-4111-8111-111111111111"
    : "33333333-3333-4333-8333-333333333333",
  recipient_kind: "personal",
  recipient_label: "测试创作空间",
  key: "sample-grant-1",
  credits: 25,
  reason: "验收赠送",
  state: "pending",
  created_at: "2026-11-27T00:00:00Z",
};
const showQuick = params.has("quick-preset");
const insufficientCredits = params.has("insufficient-credits");
const quickPresets = [
  [
    "ark-video-mini",
    "Seedance 2.0 Mini",
    "doubao-seedance-2-0-mini-260615",
    25,
    75,
    7452000,
  ],
  [
    "ark-video-fast",
    "Seedance 2.0 Fast",
    "doubao-seedance-2-0-fast-260128",
    40,
    120,
    11988000,
  ],
  [
    "ark-video",
    "Seedance 2.0",
    "doubao-seedance-2-0-260128",
    50,
    150,
    14904000,
  ],
  [
    "ark-video-25-flash",
    "Seedance 2.5 Flash",
    "doubao-seedance-2-5-flash-261115",
    50,
    299,
    29808000,
  ],
  [
    "ark-video-25",
    "Seedance 2.5",
    "doubao-seedance-2-5-260628",
    76,
    454,
    45360000,
  ],
].map(([key, name, upstream, estimatedCredits, credits, purchaseMicros]) => ({
  ...arkPreset,
  key: `6da07a12-0000-4000-8000-000000000001/${key}`,
  name: String(name),
  upstream: String(upstream),
  quick_onboarding: { resolution: "720p" },
  estimatedCredits: Number(estimatedCredits),
  credits: Number(credits),
  purchaseMicros: Number(purchaseMicros),
}));
const textProviderPresets = [
  [
    "deepseek",
    "DeepSeek",
    "DeepSeek V4 Flash",
    "deepseek-v4-flash",
    "https://api.deepseek.com",
  ],
  [
    "deepseek",
    "DeepSeek",
    "DeepSeek V4 Pro",
    "deepseek-v4-pro",
    "https://api.deepseek.com",
  ],
  ["openai", "OpenAI", "GPT-5.4", "gpt-5.4", "https://api.openai.com/v1"],
  [
    "openrouter",
    "OpenRouter",
    "DeepSeek V4 Flash",
    "deepseek/deepseek-v4-flash",
    "https://openrouter.ai/api/v1",
  ],
].map(
  ([adapter, provider, name, upstream, base_url]) =>
    ({
      key: `fixture-text/${upstream}`,
      adapter,
      provider,
      name,
      upstream,
      base_url,
      capability: "text.generate",
      modes: [],
      fields: [],
      rates: [],
      currency: "USD",
      request_micros: null,
      concurrency: 1,
      rpm: 60,
    }) satisfies GatewayPreset,
);

const arkTextPreset = {
  key: "fixture-text/doubao-seed-2-1-pro-260915",
  adapter: arkPreset.adapter,
  provider: arkPreset.provider,
  name: "Doubao Seed 2.1 Pro",
  upstream: "doubao-seed-2-1-pro-260915",
  base_url: arkPreset.base_url,
  capability: "text.generate",
  modes: ["text_to_text"],
  fields: [],
  rates: ["每百万 Token：输入 ¥6、缓存命中 ¥1.20、输出 ¥30"],
  currency: "CNY",
  request_micros: null,
  token_price: { input: 6_000_000, cache_read: 1_200_000, output: 30_000_000 },
  concurrency: 2,
  rpm: 30,
} satisfies GatewayPreset;

const tariffFixture = {
  ...arkTextPreset,
  key: "fixture-text/character",
  name: "Doubao Seed Character · 251128",
  upstream: "doubao-seed-character-251128",
  token_price: undefined,
  text_tariff: {
    max_input: 96000,
    tiers: [
      {
        from_input: 0,
        input: "0.8",
        cache_read: "0.16",
        cache_write: "0.8",
        output: "2",
      },
      {
        from_input: 32001,
        input: "1.2",
        cache_read: "0.16",
        cache_write: "1.2",
        output: "6",
      },
    ],
  },
} satisfies GatewayPreset;

const imageTariffFixture = {
  key: "fixture-image/sunburst",
  provider: "OpenAI",
  name: "GPT Image 2.5 Sunburst",
  upstream: "gpt-image-2.5-sunburst",
  adapter: "openai",
  base_url: "https://api.openai.com/v1",
  capability: "image.generate",
  modes: ["text2image"],
  fields: [],
  rates: ["文本输入 $5、图像输出 $30 / 百万 Token"],
  currency: "USD",
  price_fx: "7",
  request_micros: null,
  media_tariff: {
    meter: "image_tokens_v1",
    unit_price: "5",
    unit_size: 1000000,
    output_token_price: "30",
  },
  concurrency: 1,
  rpm: 60,
} satisfies GatewayPreset;

const videoTariffFixture = {
  ...imageTariffFixture,
  key: "fixture-video/motion",
  provider: "即梦动作模仿",
  name: "动作模仿 2.0",
  upstream: "jimeng_dreamactor_m20_gen_video",
  adapter: "volcengine_motion",
  base_url: "https://openapi.cv.volces.com",
  capability: "video.generate",
  modes: ["motion"],
  rates: ["¥0.40 / 秒"],
  currency: "CNY",
  price_fx: "1",
  media_tariff: {
    meter: "output_video_seconds_v1",
    adapter: "volcengine_motion",
    unit_price: "0.4",
    unit_size: 1,
    max_quantity: 31,
  },
} satisfies GatewayPreset;

const showManagement = params.has("management");
const section = params.get("section");
const managementPrice = {
  input: 6000000,
  output: 30000000,
  cache_read: 1200000,
  cache_write: 0,
  request: 0,
  currency: "CNY",
  fx: "1",
  multiplier: "1",
  discount: "1",
};
let managementModel: GatewayModel = {
  id: candidate.model_id,
  name: "Doubao Seed 2.1 Pro",
  protocol: "chat",
  price: managementPrice,
  max_input: 4096,
  max_output: 1024,
  purchase_cap: 1000000,
  tools: false,
  stream: false,
};
const managementRoute = {
  id: candidate.project_id,
  model_id: managementModel.id,
  supplier: candidate.scope_id,
  credential: candidate.project_id,
  group: "方舟创作账号",
  base_url: arkPreset.base_url,
  upstream_model: arkTextPreset.upstream,
  protocol: "chat",
  purchase: managementPrice,
  priority: 0,
} satisfies GatewayRoute;
const failedCandidate = {
  ...candidate,
  state: "failed" as const,
  supplier_id: managementRoute.supplier,
  group: managementRoute.group,
  provider_key: "volcengine_ark",
};
let stagedTariffModel: GatewayModel | null = null;
let quickCandidate: OnboardingReview | null = null;
let quickSends = 0;

installDevelopmentTransport({
  matches: (path) =>
    [
      "/tests/browser/ai-presets.html",
      "/tests/browser/onboarding.html",
      "/tests/browser/pool-sale.html",
    ].includes(path),
  request: async (url, options) => {
    if (showManagement) {
      const sale = `/api/ai/admin/models/${managementModel.id}/price`;
      if (url === `${sale}/preview`) return { credits: 1 };
      if (url === sale && options.method === "PUT") {
        const { price } = options.data as { price: GatewayModel["price"] };
        managementModel = { ...managementModel, price, sale_version: 1 };
        return managementModel;
      }
      if (url === `/api/ai/admin/onboarding/${candidate.id}`)
        return failedCandidate;
      if (url === "/api/ai/admin/configuration")
        return {
          models: [{ model: managementModel, enabled: true }],
          routes: [
            {
              route: managementRoute,
              provider_key: "volcengine_ark",
              enabled: true,
            },
          ],
          credentials: [
            {
              id: managementRoute.credential,
              supplier_id: managementRoute.supplier,
              state: "active",
            },
          ],
          quotas: [{ name: managementRoute.group, concurrency: 2, rpm: 60 }],
          onboarding: [failedCandidate],
          unknown: [],
        };
    }
    if (
      showQuick &&
      url === "/api/ai/admin/models" &&
      options.method === "POST"
    ) {
      const model = options.data as GatewayModel;
      const validImage =
        model.preset_key === imageTariffFixture.key &&
        model.price.media_tariff?.output_token_price === "30";
      const validText =
        model.preset_key === tariffFixture.key &&
        model.price.currency === "CNY" &&
        model.price.input === 0 &&
        model.price.text_tariff?.tiers[1]?.from_input === 32001;
      if (!validImage && !validText) {
        throw new Error("Expected a structured sale tariff");
      }
      stagedTariffModel = model;
      return undefined;
    }
    if (
      showQuick &&
      url === "/api/ai/admin/routes" &&
      options.method === "POST"
    ) {
      const { route, verification, media_tests } = options.data as {
        route: GatewayRoute;
        verification?: string;
        media_tests?: unknown[];
      };
      const validTariff =
        route.upstream_model === imageTariffFixture.upstream
          ? route.purchase.media_tariff?.output_token_price === "30"
          : route.upstream_model === tariffFixture.upstream &&
            route.purchase.text_tariff?.tiers[1]?.output === "6";
      if (
        route.model_id !== stagedTariffModel?.id ||
        !validTariff ||
        verification ||
        media_tests
      ) {
        throw new Error(
          "Expected a structured purchase tariff for the same model",
        );
      }
      quickCandidate = {
        ...candidate,
        model_id: route.model_id,
        plan: {
          method: "read_only_key",
          credits: 0,
          purchase_micros: 0,
          probes: [],
        },
      };
      return quickCandidate;
    }
    if (
      showArkEstimate &&
      url === "/api/ai/admin/ark-video-estimate" &&
      options.method === "POST"
    ) {
      const fields = options.data as API.ArkVideoEstimateInput;
      if (
        fields.model !== arkPreset.upstream ||
        fields.output_seconds !== 15 ||
        fields.reference_seconds !== 5 ||
        fields.resolution !== "720p" ||
        fields.ratio !== "16:9" ||
        fields.count !== 1
      ) {
        throw new Error("Unexpected Ark estimate fixture inputs");
      }
      return { amount_cny: "22.68" };
    }
    if (
      showCreditReview &&
      url === `/api/ai/admin/credits/${creditOrder.id}/review` &&
      options.method === "POST"
    ) {
      if ((options.data as API.CreditGrantReview).approve !== true) {
        throw new Error("Expected explicit grant approval");
      }
      creditOrder = {
        ...creditOrder,
        state: "issued",
        event_id: "66666666-6666-4666-8666-666666666666",
      };
      return creditOrder;
    }
    if (
      showQuick &&
      url === "/api/creation-context" &&
      options.method === "POST"
    )
      return { id: candidate.project_id };
    if (
      showQuick &&
      url === "/api/ai/admin/onboarding/preset" &&
      options.method === "POST"
    ) {
      const input = options.data as API.QuickOnboardingInput;
      const preset = quickPresets.find(({ key }) => key === input.preset_key);
      if (
        !preset ||
        input.scope_id !== candidate.scope_id ||
        input.project_id !== candidate.project_id
      )
        throw new Error("Invalid preset preparation");
      quickCandidate = {
        ...candidate,
        upstream_model: preset.upstream,
        provider_key: preset.adapter,
        plan: insufficientCredits
          ? {
              credits: preset.credits,
              estimated_credits: preset.estimatedCredits,
              purchase_micros: preset.purchaseMicros,
              probes: [{ name: "text2video", input: {} }],
            }
          : {
              method: "read_only_key",
              credits: 0,
              purchase_micros: 0,
              probes: [],
            },
      };
      return quickCandidate;
    }
    if (
      showQuick &&
      quickCandidate &&
      url === `/api/ai/admin/onboarding/${candidate.id}/verify` &&
      options.method === "POST"
    ) {
      const input = options.data as {
        key: string;
        credits: number;
        purchase_micros: number;
        confirm_trial?: boolean;
      };
      const expectedKey =
        stagedTariffModel?.preset_key === imageTariffFixture.key
          ? "fixture-openai-key"
          : "fixture-ark-key";
      if (
        input.key !== expectedKey ||
        Boolean(input.confirm_trial) !== insufficientCredits ||
        input.credits !== quickCandidate.plan.credits ||
        input.purchase_micros !== quickCandidate.plan.purchase_micros ||
        quickSends !== 0
      )
        throw new Error("Invalid budget confirmation");
      if (insufficientCredits) {
        throw new RequestError("insufficient_credits", 402);
      }
      quickSends++;
      quickCandidate = { ...quickCandidate, state: "active", available: true };
      return quickCandidate;
    }
    if (
      showQuick &&
      quickCandidate &&
      url === `/api/ai/admin/onboarding/${candidate.id}`
    ) {
      return quickCandidate;
    }
    if (options.method && options.method.toUpperCase() !== "GET")
      throw new Error("Unexpected fixture mutation");
    if (showPrepared && url === `/api/ai/admin/onboarding/${candidate.id}`)
      return candidate;
    if (url !== "/api/ai/admin/configuration")
      throw new Error("Unexpected fixture endpoint");
    return {
      models: stagedTariffModel
        ? [{ model: stagedTariffModel, enabled: false, draft: true }]
        : [],
      routes: [],
      credentials: [],
      quotas: [],
      unknown: [],
      key_endpoints: [arkTextPreset, imageTariffFixture].map(
        ({ adapter, base_url }) => ({
          provider: adapter,
          base_url,
        }),
      ),
      onboarding: showPrepared
        ? [candidate]
        : quickCandidate
          ? [quickCandidate]
          : [],
      ...(showCreditReview ? { credit_grants: [creditOrder] } : {}),
      ...(showArkEstimate || showQuick
        ? {
            model_presets: showQuick
              ? [
                  ...quickPresets,
                  arkTextPreset,
                  tariffFixture,
                  ...textProviderPresets,
                  imageTariffFixture,
                  videoTariffFixture,
                ]
              : [arkPreset],
            media_drivers: [
              ...[imageTariffFixture, videoTariffFixture].map((preset) => ({
                key: preset.adapter,
                name: preset.provider,
                base_url: preset.base_url,
                region: "domestic",
                parameters: { [preset.capability]: [] },
                custom_url: false,
              })),
              {
                key: arkPreset.adapter,
                name: arkPreset.provider,
                base_url: "https://ark.cn-beijing.volces.com/api/v3",
                region: "domestic",
                parameters: { "video.generate": [] },
                custom_url: false,
              },
            ],
          }
        : {}),
    };
  },
});
const testSpace = {
  id: candidate.scope_id,
  kind: "personal",
  name: "测试空间",
  role: "owner",
  team_id: "",
  writable: true,
};
createRoot(document.getElementById("root")!).render(
  <>
    <Toaster />
    {showManagement ? (
      <AiServiceAdmin
        entry={
          section === "channels" || section === "supplier"
            ? "providers"
            : "models"
        }
        initialProvider={section === "supplier" ? "volcengine_ark" : undefined}
        entityID={section === "sale" ? managementModel.id : undefined}
      />
    ) : showQuick ? (
      <SpaceContext.Provider
        value={{
          spaces: [testSpace],
          selected: testSpace,
          pending: false,
          error: "",
          refresh: async () => {},
          select: async () => {},
        }}
      >
        <AiServiceAdmin entry="providers" />
      </SpaceContext.Provider>
    ) : (
      <AiServiceAdmin
        entry={
          showCreditReview ? "billing" : showPrepared ? "providers" : "models"
        }
      />
    )}
  </>,
);
