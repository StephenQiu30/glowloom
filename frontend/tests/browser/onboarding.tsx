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
      capability: "text.generate",
      display_name: "Doubao Seed 2.1 Pro",
      provider_model_id: "doubao-seed-2-1-pro-260915",
      status: "disabled",
    },
    {
      id: "image",
      capability: "image.generate",
      display_name: "Seedream 4.5",
      provider_model_id: "doubao-seedream-4-5-251128",
      status: "disabled",
    },
    {
      id: "video",
      capability: "video.generate",
      display_name: "Seedance 2.0",
      provider_model_id: "doubao-seedance-2-0-260128",
      status: "disabled",
    },
  ],
};
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
  if (path.endsWith("/connections")) {
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
    state.check = { id: "check", model_id: modelID, status: "running" };
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
const [{ ConnectionOnboarding }, { PresetConnect }, { setCSRFToken }] =
  await Promise.all([
    import("../../src/app/providers/_components/connection-onboarding"),
    import("../../src/app/providers/_components/preset-connect"),
    import("../../src/lib/request"),
  ]);
setCSRFToken("fixture-only");
function Fixture() {
  const [done, setDone] = useState(false);
  const [management, setManagement] = useState(false);
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
                {management ? "管理服务" : "接入 AI 服务"}
              </DialogTitle>
              <DialogDescription className="sr-only">
                接入与模型验证分离的受控验收。
              </DialogDescription>
            </DialogHeader>
            {management ? (
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
