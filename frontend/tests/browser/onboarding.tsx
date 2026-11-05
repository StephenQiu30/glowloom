/** Controlled browser fixture: synthetic API responses; no real supplier calls. */
import React, { useState } from "react";
import { createRoot } from "react-dom/client";
import axios from "axios";
import { ThemeProvider } from "next-themes";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "../../src/components/ui/dialog";

import "../../src/app/globals.css";

const options = new URLSearchParams(location.search);
const id = "22222222-2222-4222-8222-222222222222";
const state: API.ProviderOnboardingState = {
  provider_id: id,
  credential_id: "credential-fixture",
  revision: 1,
  status: "disabled",
  models: [
    {
      id: "text",
      version_id: "text-v1",
      capability: "text.generate",
      display_name: "Doubao Seed 2.1 Pro",
      provider_model_id: "doubao-seed-2-1-pro-260915",
      status: "disabled",
    },
    {
      id: "image",
      version_id: "image-v1",
      capability: "image.generate",
      display_name: "Seedream 4.5",
      provider_model_id: "doubao-seedream-4-5-251128",
      status: "disabled",
    },
    {
      id: "video",
      version_id: "video-v1",
      capability: "video.generate",
      display_name: "Seedance 2.0",
      provider_model_id: "doubao-seedance-2-0-260128",
      status: "disabled",
    },
  ],
};
const formal = options.has("formal");
const projectID = "33333333-3333-4333-8333-333333333333";
const operationID = "44444444-4444-4444-8444-444444444444";
let trialStatus = "quoted";
let calls = 0;
let savedModels: string[] = [];
const presets: API.Preset[] = [
  {
    id: "openrouter",
    version_id: "openrouter-v1",
    document: { name: "OpenRouter", config: { region: "overseas" } },
  },
  {
    id: "ark",
    version_id: "ark-v1",
    document: { name: "火山方舟", config: { region: "domestic" } },
    model_catalog: {
      version: "v1",
      models: state.models!.map((m) => ({
        key: m.id,
        display_name: m.display_name,
        capability: m.capability,
        provider_model_id: m.provider_model_id,
      })),
    },
  },
];
if (options.has("long-label")) {
  state.models![0].display_name =
    "Doubao-Seed-Enterprise-".repeat(8) + "多语言长模型名称";
}
axios.defaults.adapter = async (config) => {
  const path = config.url ?? "";
  let data: unknown = {
    ...structuredClone(state),
    models: state.models!.filter((m) => savedModels.includes(m.id!)),
  };
  if (path === "/api/projects") {
    data = {
      items: [
        {
          id: projectID,
          name: "隔离验证项目",
          canvas_id: null,
          aspect_ratio: "16:9",
          style_key: null,
          revision: 1,
          update_time: "2026-11-05T00:00:00Z",
        },
      ],
      next_cursor: null,
    };
  } else if (path.endsWith("/budget")) {
    data = { available_micros: 1000000 };
  } else if (path.endsWith("/trial-recovery")) {
    data = { found: false };
  } else if (path.endsWith("/trial-quotes")) {
    data = {
      operation_id: operationID,
      confirm_key: "stable-fixture-confirm",
      available_micros: 1000000,
      quote_micros: 50000,
      confirmable: !options.has("insufficient"),
      expires_at: "2099-01-01T00:00:00Z",
    };
  } else if (path === `/api/operations/${operationID}/confirm`) {
    if (options.has("insufficient"))
      throw new Error("insufficient quote was confirmed");
    trialStatus = "submitted";
    setTimeout(() => {
      trialStatus = "completed";
    }, 300);
    data = { operation_id: operationID };
  } else if (path === `/api/operations/${operationID}`) {
    data = {
      id: operationID,
      project_id: projectID,
      status: trialStatus,
      quote_micros: 50000,
    };
  } else if (path.endsWith("/connections")) {
    savedModels = JSON.parse(config.data).candidate_keys;
    data = { provider_id: id };
  } else if (path.endsWith("/credentials")) {
    data = {
      id: "credential-fixture",
      provider_id: id,
      status: "active",
      last4: "test",
    };
  } else if (path.endsWith("/test")) {
    calls++;
    const modelID = path.split("/models/")[1]?.split("/")[0] ?? "text";
    state.check = {
      id: "check",
      model_id: modelID,
      model_version_id: state.models?.find((m) => m.id === modelID)?.version_id,
      status: "running",
    };
    data = {
      accepted: true,
      provider_id: id,
      check_id: "check",
      model_id: modelID,
    };
    setTimeout(() => {
      state.check = {
        ...state.check,
        status: options.has("failure") ? "failed" : "completed",
        auth_result: options.has("failure") ? "auth_failed" : "ok",
        model_result: options.has("video") ? "accepted" : "completed",
      };
    }, 500);
  } else if (path.endsWith("/activate")) {
    state.status = "active";
    state.models = state.models!.map((m) => ({ ...m, status: "active" }));
    data = structuredClone(state);
  }
  return { data, status: 200, statusText: "OK", headers: {}, config };
};
const [
  { ConnectionOnboarding },
  { PresetConnect },
  { setCSRFToken },
  { ProviderTrial },
] = await Promise.all([
  import("../../src/app/providers/_components/connection-onboarding"),
  import("../../src/app/providers/_components/preset-connect"),
  import("../../src/lib/request"),
  import("../../src/components/catalog/provider-trial"),
]);
setCSRFToken("fixture-only");
if (options.has("managed"))
  savedModels = state.models!.map((model) => model.id!);
function Fixture() {
  const [done, setDone] = useState(false);
  const [management, setManagement] = useState(options.has("managed"));
  return (
    <ThemeProvider
      attribute="class"
      forcedTheme={options.get("theme") ?? "dark"}
    >
      <p className="p-4 text-sm">
        受控浏览器验收 · 模拟响应 · 无真实供应商调用
      </p>
      {done ? (
        <p role="status" className="p-6">
          已启用 {savedModels.length} 个模型 · 验证调用 {calls} 次
        </p>
      ) : (
        <Dialog open>
          <DialogContent className="flex max-h-[calc(100dvh-2rem)] flex-col overflow-hidden p-6 sm:max-w-[520px]">
            <DialogHeader>
              <DialogTitle size="lg">
                {formal
                  ? "正式发布验证"
                  : management
                    ? "管理服务"
                    : "接入 AI 服务"}
              </DialogTitle>
              <DialogDescription className="sr-only">
                接入与模型验证分离的受控验收。
              </DialogDescription>
            </DialogHeader>
            {formal ? (
              <ProviderTrial
                embedded
                release={{
                  id: "formal-release",
                  required_modes: ["text_to_text"],
                  upstream_cost: { currency: "USD", max_micros: 100 },
                }}
                version={{ param_schema: [] }}
                onVerified={() => undefined}
              />
            ) : management ? (
              <ConnectionOnboarding id={id} onSaved={() => setDone(true)} />
            ) : (
              <PresetConnect
                presets={presets}
                initialPreset="ark"
                onCreated={() => undefined}
                onSaved={() => setManagement(true)}
                onCancel={() => setDone(true)}
              />
            )}
          </DialogContent>
        </Dialog>
      )}
    </ThemeProvider>
  );
}
createRoot(document.getElementById("root")!).render(<Fixture />);
