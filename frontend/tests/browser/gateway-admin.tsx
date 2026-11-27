/** All management responses are synthetic; this fixture never contacts suppliers. */
import React from "react";
import { createRoot } from "react-dom/client";
import { AiServiceAdmin } from "../../src/components/ai-service/ai-service-admin";
import { SpaceContext } from "../../src/components/spaces/space-context";
import { installDevelopmentTransport } from "../../src/lib/development-transport";
import type {
  GatewayPreset,
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
      "720p": { geometries: { "16:9": { width: 1280, height: 720 } } },
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

let quickCandidate: typeof candidate | null = null;
let quickSends = 0;
let quickVerificationReads = 0;

installDevelopmentTransport({
  matches: (path) =>
    [
      "/tests/browser/ai-presets.html",
      "/tests/browser/onboarding.html",
      "/tests/browser/pool-sale.html",
    ].includes(path),
  request: async (url, options) => {
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
        plan: {
          credits: preset.credits,
          estimated_credits: preset.estimatedCredits,
          purchase_micros: preset.purchaseMicros,
          probes: [
            {
              name: "text2video",
              input: {
                prompt: "固定机位，窗边的陶瓷杯。",
                params: { duration: 5, ratio: "16:9", resolution: "720p" },
              },
            },
          ],
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
      };
      if (
        input.key !== "fixture-ark-key" ||
        input.credits !== quickCandidate.plan.credits ||
        input.purchase_micros !== quickCandidate.plan.purchase_micros ||
        quickSends !== 0
      )
        throw new Error("Invalid budget confirmation");
      quickSends++;
      quickCandidate = { ...quickCandidate, state: "verifying" };
      return quickCandidate;
    }
    if (
      showQuick &&
      quickCandidate &&
      url === `/api/ai/admin/onboarding/${candidate.id}`
    ) {
      if (
        quickCandidate.state === "verifying" &&
        ++quickVerificationReads === 2
      ) {
        quickCandidate = {
          ...quickCandidate,
          state: "active",
          available: true,
        };
      }
      return quickCandidate;
    }
    if (options.method && options.method.toUpperCase() !== "GET")
      throw new Error("Unexpected fixture mutation");
    if (showPrepared && url === `/api/ai/admin/onboarding/${candidate.id}`)
      return candidate;
    if (url !== "/api/ai/admin/configuration")
      throw new Error("Unexpected fixture endpoint");
    return {
      models: [],
      routes: [],
      credentials: [],
      quotas: [],
      unknown: [],
      onboarding: showPrepared
        ? [candidate]
        : quickCandidate
          ? [quickCandidate]
          : [],
      ...(showCreditReview ? { credit_grants: [creditOrder] } : {}),
      ...(showArkEstimate || showQuick
        ? {
            model_presets: showQuick
              ? [...quickPresets, ...textProviderPresets]
              : [arkPreset],
            media_drivers: [
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
  showQuick ? (
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
      <AiServiceAdmin />
    </SpaceContext.Provider>
  ) : (
    <AiServiceAdmin />
  ),
);
