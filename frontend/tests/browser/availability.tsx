/** Isolated browser fixture. Every API response is synthetic; supplier calls are forbidden. */
import React, { useState } from "react";
import { createRoot } from "react-dom/client";
import axios from "axios";
import { ThemeProvider } from "next-themes";
import { Toaster } from "../../src/components/ui/sonner";
import { ModelPicker } from "../../src/components/generation/model-picker";
import { Button } from "../../src/components/ui/button";
import "../../src/app/globals.css";

const provider = {
  id: "provider",
  name: "测试服务连接",
  adapter_key: "openrouter",
  region: "overseas",
  status: "active",
  revision: 1,
};
const availability = {
  available: true,
  mode_availability: [
    { mode: "text2image", available: true, unavailable_reasons: [] },
    {
      mode: "image2image",
      available: false,
      unavailable_reasons: ["mode_unverified"],
    },
  ],
  unavailable_reasons: [],
};
const version = {
  id: "version",
  modes: ["text2image", "image2image"],
  limits: { max_outputs: 1 },
  param_schema: [],
};
const models: API.CreatedModel[] = [
  {
    ...availability,
    id: "model",
    provider_id: provider.id,
    display_name: "Seedream 4.5 可辨识长模型名称",
    model_key: "fixture-image",
    capability: "image.generate",
    status: "active",
    current_version_id: version.id,
  },
  {
    id: "pending",
    provider_id: provider.id,
    display_name: "缺少价格与验证的模型",
    capability: "audio.generate",
    status: "active",
    available: false,
    unavailable_reasons: ["price_unverified", "mode_unverified"],
    mode_availability: [
      {
        mode: "omni_audio",
        available: false,
        unavailable_reasons: ["price_unverified", "mode_unverified"],
      },
    ],
  },
];
const publicModels: API.ModelResponse[] = [
  {
    ...availability,
    id: "model",
    display_name: models[0].display_name,
    current_version: version,
  },
];
axios.defaults.adapter = async (config) => {
  const path = config.url ?? "";
  if (config.method !== "get")
    throw new Error("Browser acceptance permits read-only mock API calls only");
  let data: unknown = { items: [] };
  if (path === "/api/admin/models")
    data = {
      items: models.filter(
        (m) =>
          !config.params?.status ||
          m.available === (config.params.status === "available"),
      ),
    };
  else if (path === "/api/admin/models/model")
    data = {
      model: models[0],
      versions: [version],
      prices: [],
      price_candidates: [],
    };
  else if (path === "/api/admin/models/pending")
    data = { model: models[1], versions: [], prices: [], price_candidates: [] };
  else if (path === "/api/admin/providers")
    data = {
      items: [
        { provider, available: true, available_model_count: 1, model_count: 2 },
      ],
    };
  else if (path === "/api/admin/capabilities")
    data = [
      { key: "image.generate", modes: version.modes },
      { key: "audio.generate", modes: ["omni_audio"] },
    ];
  else if (path === "/api/admin/provider-adapters")
    data = [{ adapter_key: "openrouter", display_name: "OpenRouter" }];
  return { data, status: 200, statusText: "OK", headers: {}, config };
};
const [{ ModelsPage }, { ProvidersPage }] = await Promise.all([
  import("../../src/app/models/_components/models-page"),
  import("../../src/app/providers/_components/providers-page"),
]);
function Acceptance() {
  const [mode, setMode] = useState("text2image");
  const [selected, setSelected] = useState<string>();
  return (
    <ThemeProvider attribute="class" forcedTheme="dark">
      <p className="p-3">模拟 API 验收 · 供应商生成请求数 0</p>
      <main>
        {new URLSearchParams(location.search).has("providers") ? (
          <ProvidersPage />
        ) : (
          <ModelsPage />
        )}
        <section className="space-y-3 p-3" aria-label="按模式选择模型">
          <h2>创作模式</h2>
          <Button
            onClick={() =>
              setMode(mode === "text2image" ? "image2image" : "text2image")
            }
          >
            {mode === "text2image" ? "切换图生图" : "切换文生图"}
          </Button>
          <ModelPicker
            models={publicModels}
            mode={mode}
            value={selected}
            disabled={false}
            onSelect={(m) => setSelected(m.current_version?.id)}
          />
          <p role="status">
            {mode} · {selected ?? "未选择"}
          </p>
        </section>
      </main>
      <Toaster />
    </ThemeProvider>
  );
}
createRoot(document.getElementById("root")!).render(<Acceptance />);
