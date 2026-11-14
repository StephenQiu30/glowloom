/** Controlled browser fixture. No supplier calls, production session or real Key. */
import React from "react";
import { createRoot } from "react-dom/client";
import axios from "axios";
import { ThemeProvider } from "next-themes";
import { Toaster } from "../../src/components/ui/sonner";
import "../../src/app/globals.css";

const providerID = "22222222-2222-4222-8222-222222222222";
const modelID = "33333333-3333-4333-8333-333333333333";
const versionID = "44444444-4444-4444-8444-444444444444";
const priceID = "55555555-5555-4555-8555-555555555555";
const releaseID = "66666666-6666-4666-8666-666666666666";
const presets: API.Preset[] = [
  {
    id: "77777777-7777-4777-8777-777777777777",
    slug: "openrouter",
    origin: "builtin",
    version: 1,
    version_id: "88888888-8888-4888-8888-888888888888",
    revision: 1,
    document: {
      schema_version: 1,
      name: "OpenRouter",
      config: {
        adapter_key: "openrouter",
        base_url: "https://openrouter.ai/api/v1",
        region: "overseas",
        capabilities: ["text", "image", "video"],
        timeout_seconds: 120,
        concurrency_limit: 2,
        rate_limit_per_min: 30,
      },
    },
  },
];
presets[0].model_catalog = {
  version: "fixture-catalog-v1",
  models: [
    {
      key: "or-chat",
      display_name: "GPT-5.4",
      provider_model_id: "openai/gpt-5.4",
      capability: "text.generate",
    },
    {
      key: "or-image",
      display_name: "GPT Image 2",
      provider_model_id: "openai/gpt-image-2",
      capability: "image.generate",
    },
  ],
};
for (const [name, adapter, region] of [
  ["火山方舟", "volcengine_ark", "domestic"],
  ["BytePlus", "byteplus", "overseas"],
  ["DeepSeek", "deepseek", "domestic"],
  ["豆包语音", "volcengine_speech", "domestic"],
] as const) {
  presets.push({
    id: crypto.randomUUID(),
    version_id: crypto.randomUUID(),
    version: 1,
    origin: "builtin",
    document: { name, config: { adapter_key: adapter, region } },
    model_catalog: {
      version: "fixture-catalog-v1",
      models: [
        {
          key: adapter + "-chat",
          display_name:
            adapter === "volcengine_speech"
              ? "豆包语音合成 2.0"
              : name + " 聊天",
          capability:
            adapter === "volcengine_speech"
              ? "audio.generate"
              : "text.generate",
          requires_model_selection:
            adapter !== "deepseek" && adapter !== "volcengine_speech",
        },
      ],
    },
  });
}
const deepseekPreset = presets.find(
  (p) => p.document?.config?.adapter_key === "deepseek",
)!;
deepseekPreset.model_catalog = {
  version: "fixture-catalog-v1",
  models: [
    {
      key: "ds-flash",
      display_name: "DeepSeek Flash",
      provider_model_id: "deepseek-flash",
      capability: "text.generate",
    },
    {
      key: "ds-pro",
      display_name: "DeepSeek V4 Pro",
      provider_model_id: "deepseek-v4-pro",
      capability: "text.generate",
    },
  ],
};
const price = {
  id: priceID,
  unit: "per_request",
  currency: "CNY",
  rule: { base_micros: 100 },
};
const version = {
  id: versionID,
  provider_model_id: "fixture/reviewed-text",
  modes: ["text_to_text"],
  limits: { max_outputs: 1 },
  param_schema: [],
  expected_max_ms: 60000,
  queue: "provider",
  moderation: "provider",
  supports_query: false,
  supports_cancel: false,
  supports_callback: false,
};
const model = {
  id: modelID,
  provider_id: providerID,
  capability: "text.generate",
  display_name: "已审核文本模型",
  revision: 3,
};
const release = {
  id: releaseID,
  model_id: modelID,
  model_version_id: versionID,
  price_version_id: priceID,
  status: "draft",
  upstream_cost: {
    currency: "CNY",
    max_micros: 100,
    source: "synthetic fixture only",
    reviewed_at: "2026-10-18T00:00:00Z",
  },
};
const providers: API.ProviderListItem[] = [
  {
    provider: {
      id: providerID,
      name: "模板来源账号",
      adapter_key: "openrouter",
      region: "overseas",
      status: "disabled",
      revision: 1,
    },
    model_count: 1,
  },
];
const providerConfigs = new Map<string, API.ConnectionConfig>([
  [
    providerID,
    {
      id: "fixture-config",
      version: 1,
      provider_id: providerID,
      config: presets[0].document?.config,
      model_catalog: presets[0].model_catalog,
      configured_models: [],
    },
  ],
]);
// Test-only mock server state survives a page reload. Production stores only the ID.
const trials = new Map<string, API.TrialState>(
  JSON.parse(sessionStorage.getItem("fixture.ai-trials") ?? "[]"),
);
const receipts = new Map<string, unknown>();
const fixtureRequests: { path: string; method?: string }[] = [];
Object.assign(window, { providerFixtureRequests: fixtureRequests });
let failLogoUpload = new URLSearchParams(location.search).has(
  "logo-upload-fail",
);
let loseCreationReceipt = new URLSearchParams(location.search).has(
  "lost-create",
);
axios.defaults.adapter = async (config) => {
  const path = config.url ?? "";
  fixtureRequests.push({ path, method: config.method });
  const body =
    typeof config.data === "string"
      ? JSON.parse(config.data)
      : (config.data ?? {});
  let data: unknown = {};
  if (path === "/api/admin/ai-trials" && config.method === "post") {
    const state: API.TrialState = {
      kind: body.kind,
      model: body.model,
      state: "succeeded",
      turns: [
        {
          request_id: body.request_id,
          prompt: body.prompt,
          state: "succeeded",
          ...(body.kind === "probe"
            ? {
                models: ["fixture-codex"],
                authenticated: true,
                image_generation: true,
              }
            : body.kind === "chat"
              ? { text: "Mock：第一轮聊天完成。" }
              : {
                  artifacts: [
                    {
                      sequence: 1,
                      mime: "image/png",
                      size: 68,
                      sha256: "fixture",
                    },
                  ],
                }),
        },
      ],
    };
    if (body.kind === "image") {
      state.state = "unknown";
      state.turns![0].state = "unknown";
      state.turns![0].artifacts = [];
    }
    trials.set(body.request_id, state);
    data = state;
  } else if (path.startsWith("/api/admin/ai-trials/")) {
    const id = path.split("/")[4];
    const state = trials.get(id);
    if (!state)
      throw new axios.AxiosError(
        "fixture trial missing",
        "ERR_BAD_RESPONSE",
        config,
        undefined,
        {
          data: { code: "trial_not_found" },
          status: 404,
          statusText: "Not Found",
          headers: {},
          config,
        },
      );
    if (path.endsWith("/turns")) {
      state.turns!.push({
        request_id: body.request_id,
        prompt: body.prompt,
        state: "succeeded",
        text: "Mock：同会话续聊完成。",
      });
    }
    if (path.endsWith("/recover")) {
      state.state = "succeeded";
      state.turns![0].state = "succeeded";
      state.turns![0].artifacts = [
        { sequence: 1, mime: "image/png", size: 68, sha256: "fixture" },
      ];
    }
    if (path.endsWith("/cancel")) state.state = "closed";
    data = state;
  } else if (path === "/api/admin/provider-presets" && config.method === "get")
    data = { items: presets };
  else if (path === "/api/admin/provider-adapters")
    data = [
      { adapter_key: "deepseek", check_capabilities: ["model_discovery"] },
      { adapter_key: "openrouter", check_capabilities: ["model_discovery"] },
    ];
  else if (path.endsWith("/model-directory"))
    data = { model_ids: ["fixture-online-model"] };
  else if (path.endsWith("/checks"))
    data = { items: [], current_checks: [], has_more: false };
  else if (path.endsWith("/test-models")) data = [];
  else if (path === "/api/admin/providers") data = { items: providers };
  else if (path === "/api/admin/models") data = { items: [model] };
  else if (path === `/api/admin/models/${modelID}`)
    data = { model, versions: [version], prices: [price] };
  else if (path === `/api/admin/models/${modelID}/releases`)
    data = { items: [release] };
  else if (path.endsWith("/import-preview"))
    data = {
      document: JSON.parse(body.document_json),
      hash: "fixture-preview",
    };
  else if (
    path === "/api/admin/provider-presets" ||
    path.endsWith("/versions")
  ) {
    const preset = {
      id: crypto.randomUUID(),
      slug: body.slug,
      origin: "custom",
      version: 1,
      version_id: crypto.randomUUID(),
      revision: 1,
      document: body.document,
    };
    presets.push(preset);
    data = preset;
  } else if (path.endsWith("/connections")) {
    const id = crypto.randomUUID();
    const p = presets.find((p) => path.includes(p.id!));
    providers.push({
      provider: {
        id,
        name: body.name ?? `${p?.document?.name} 测试连接`,
        logo_asset_id: body.logo_asset_id,
        adapter_key: p?.document?.config?.adapter_key,
        region: p?.document?.config?.region,
        status: "disabled",
        revision: 1,
      },
      model_count:
        (p?.document?.model_templates?.length ?? 0) +
        (Array.isArray(body.candidate_keys) ? body.candidate_keys.length : 0),
    });
    const config: API.ConnectionConfig = {
      id: crypto.randomUUID(),
      provider_id: id,
      version: 1,
      config: p?.document?.config,
      model_catalog: p?.model_catalog,
      configured_models: [],
    };
    providerConfigs.set(id, config);
    data = { provider_id: id, config };
  } else if (path === "/api/admin/providers/custom") {
    const id = crypto.randomUUID();
    providers.push({
      provider: {
        id,
        name: body.name,
        adapter_key: body.config.adapter_key,
        region: body.config.region,
        status: "disabled",
        revision: 1,
        logo_asset_id: body.logo_asset_id,
      },
      model_count: 0,
    });
    const config: API.ConnectionConfig = {
      id: crypto.randomUUID(),
      provider_id: id,
      version: 1,
      config: body.config,
      model_catalog: { version: "fixture-custom-catalog", models: [] },
      configured_models: [],
    };
    providerConfigs.set(id, config);
    data = { provider_id: id, config };
  } else if (path === "/api/admin/provider-logos") {
    if (failLogoUpload) {
      failLogoUpload = false;
      throw new axios.AxiosError(
        "fixture upload failure",
        "ERR_NETWORK",
        config,
      );
    }
    data = { id: crypto.randomUUID(), media_type: "image", mime: "image/png" };
  } else if (path.startsWith("/api/provider-logos/")) {
    data = {
      asset: { id: path.split("/").at(-1) },
      url: "/brand/providers/volcengine.svg",
      expires_at: new Date(Date.now() + 60000).toISOString(),
    };
  } else if (path.endsWith("/logo") && config.method === "patch") {
    const p = providers.find(
      (item) => item.provider?.id === path.split("/").at(-2),
    );
    if (p?.provider) {
      p.provider = {
        ...p.provider,
        logo_asset_id: body.logo_asset_id ?? undefined,
        revision: (p.provider.revision ?? 0) + 1,
      };
      data = p.provider;
    }
  } else if (
    /^\/api\/admin\/providers\/[^/]+\/models$/.test(path) &&
    config.method === "post"
  ) {
    const id = path.split("/").at(-2)!;
    const config = providerConfigs.get(id);
    const candidate = config?.model_catalog?.models?.find(
      (item) => item.key === body.candidate_key,
    );
    const modelID = crypto.randomUUID();
    const provider = providers.find((item) => item.provider?.id === id);
    if (provider) provider.model_count = (provider.model_count ?? 0) + 1;
    config?.configured_models?.push({
      id: modelID,
      provider_model_id: body.custom_model_id ?? candidate?.provider_model_id,
      capability: body.capability ?? candidate?.capability,
    });
    data = {
      id: modelID,
      provider_id: id,
      name: body.custom_model_name ?? candidate?.display_name,
      provider_model_id: body.custom_model_id ?? candidate?.provider_model_id,
      capability: body.capability ?? candidate?.capability,
    };
  } else if (path.endsWith("/credentials") && config.method === "put") {
    const id = path.split("/").at(-2)!;
    data = {
      id: crypto.randomUUID(),
      provider_id: id,
      last4: "test",
      status: "active",
    };
    const p = providers.find((p) => p.provider?.id === id);
    if (p) p.credential = data as API.ProviderCredentialSummary;
  } else if (path.endsWith("/credentials")) data = [];
  else if (path.endsWith("/config"))
    data = providerConfigs.get(path.split("/").at(-2)!);
  else if (path.startsWith("/api/admin/config-requests/")) {
    const requestID = path.split("/").at(-1)!;
    const result = receipts.get(requestID);
    if (!result)
      throw new axios.AxiosError(
        "fixture receipt absent",
        "ERR_BAD_RESPONSE",
        config,
        undefined,
        {
          data: { code: "not_found" },
          status: 404,
          statusText: "Not Found",
          headers: {},
          config,
        },
      );
    data = {
      request_id: requestID,
      action: config.params?.action,
      target_id: config.params?.target_id,
      result,
    };
  } else if (path.startsWith("/api/admin/providers/")) {
    const p = providers.find((p) => p.provider?.id === path.split("/").at(-1));
    if (p?.provider && config.method === "patch") {
      p.provider = {
        ...p.provider,
        name: body.name ?? p.provider.name,
        status: body.status ?? p.provider.status,
        revision: (p.provider.revision ?? 0) + 1,
      };
      data = p.provider;
    } else {
      data = p ? { provider: p.provider, credential: p.credential } : {};
    }
  }
  if (body.request_id) receipts.set(body.request_id, data);
  const key = config.headers?.get("Idempotency-Key");
  if (typeof key === "string") receipts.set(key, data);
  if (
    loseCreationReceipt &&
    (path.endsWith("/connections") || path === "/api/admin/providers/custom")
  ) {
    loseCreationReceipt = false;
    throw new axios.AxiosError(
      "fixture response intentionally lost",
      "ERR_NETWORK",
      config,
    );
  }
  sessionStorage.setItem(
    "fixture.ai-trials",
    JSON.stringify([...trials.entries()]),
  );
  return { data, status: 200, statusText: "OK", headers: {}, config };
};
const [{ ProvidersPage }, { AccountMenu }, { setCSRFToken }] =
  await Promise.all([
    import("../../src/app/providers/_components/providers-page"),
    import("../../src/components/account/account-menu"),
    import("../../src/lib/request"),
  ]);
setCSRFToken("fixture-only");
const actor: API.EmailSessionView["actor"] = {
  bio: "",
  id: "11111111-1111-4111-8111-111111111111",
  email: "fixture@example.test",
  display_name: "浏览器测试管理员",
  email_verified_at: "2026-10-18T00:00:00Z",
  avatar_asset_id: "",
  platform_role: new URLSearchParams(location.search).has("creator")
    ? "creator"
    : "platform_admin",
  status: "active",
  revision: "1",
  capabilities: [],
};
createRoot(document.getElementById("root")!).render(
  <ThemeProvider
    attribute="class"
    forcedTheme={new URLSearchParams(location.search).get("theme") ?? "dark"}
  >
    <div className="flex items-center justify-between gap-3 border-b p-4">
      <p>受控浏览器验证 · 模拟 API · 无真实供应商调用</p>
      <AccountMenu actor={actor} onLogout={() => {}} />
    </div>
    <main>
      <ProvidersPage />
    </main>
    <Toaster />
  </ThemeProvider>,
);
