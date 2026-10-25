/** Controlled browser acceptance: actual components, fixture API, no real account or supplier. */
import React from "react";
import { createRoot } from "react-dom/client";
import axios from "axios";
import { ThemeProvider } from "next-themes";
import { Toaster } from "../../src/components/ui/sonner";
import "../../src/app/globals.css";
const ids = [
  "22222222-2222-4222-8222-222222222222",
  "33333333-3333-4333-8333-333333333333",
  "44444444-4444-4444-8444-444444444444",
];
const users = ["林知夏", "周予安", "陈墨"].map((display_name, i) => ({
  id: ids[i],
  display_name,
  email: ["lin@example.test", "zhou@example.test", "chen@example.test"][i],
  status: i === 2 ? "disabled" : "active",
  platform_role: "creator",
  revision: "1",
  active_team_count: 2,
  create_time: "2026-10-24T00:00:00Z",
}));
const providers = ["OpenRouter", "火山方舟", "备用连接"].map((name, i) => ({
  provider: {
    id: ids[i],
    name,
    key: `provider-${i}`,
    revision: 1,
    status: i === 2 ? "disabled" : "active",
    adapter_key: i === 1 ? "volcengine_ark" : "openrouter",
    region: i === 1 ? "domestic" : "overseas",
  },
  credential: {
    id: ids[(i + 1) % 3],
    status: "active",
    last4: "1234",
    last_test_result: "ok",
  },
  model_count: 2,
}));
const models = ["对话模型", "图像模型", "视频模型"].map((display_name, i) => ({
  id: ids[i],
  display_name,
  provider_id: ids[i],
  revision: 1,
  model_key: `model-${i}`,
  capability: ["text.generate", "image.generate", "video.generate"][i],
  status: i === 2 ? "draft" : "active",
}));
const receipts = new Map<string, unknown>();
let loseResponse = new URLSearchParams(location.search).has("lost");
axios.defaults.adapter = async (config) => {
  const path = config.url ?? "";
  const method = config.method ?? "get";
  const body =
    typeof config.data === "string"
      ? JSON.parse(config.data)
      : (config.data ?? {});
  const key = String(config.headers?.get("Idempotency-Key") ?? "");
  const target = path.split("/")[4];
  let data: unknown = {};
  if (method !== "get" && key && receipts.has(key)) data = receipts.get(key);
  else if (path === "/api/platform/users")
    data = { items: users, next_cursor: "" };
  else if (path.includes("/commands/"))
    data = receipts.get(path.split("/").at(-1)!);
  else if (path.startsWith("/api/platform/users/")) {
    const user = users.find((u) => u.id === target)!;
    if (method === "get")
      data = {
        user,
        allowed_actions:
          user.status === "active"
            ? ["disable", "revoke_sessions"]
            : ["enable"],
        teams: [],
      };
    else {
      const action = path.endsWith("revoke-sessions")
        ? "revoke_sessions"
        : path.split("/").at(-1)!;
      if (action !== "revoke_sessions")
        user.status = action === "disable" ? "disabled" : "active";
      user.revision = String(Number(user.revision) + 1);
      data = { request_id: key, target_id: user.id, action };
    }
  } else if (path === "/api/admin/providers")
    data = { items: providers, next_cursor: "" };
  else if (path === "/api/admin/models")
    data = { items: models, next_cursor: "" };
  else if (path === "/api/admin/provider-presets") data = { items: [] };
  else if (path === "/api/admin/capabilities")
    data = [
      { key: "text.generate" },
      { key: "image.generate" },
      { key: "video.generate" },
    ];
  else if (path.startsWith("/api/admin/config-requests/"))
    data = {
      action: config.params.action,
      target_id: config.params.target_id,
      request_id: path.split("/").at(-1),
      result: receipts.get(path.split("/").at(-1)!),
    };
  else if (path.startsWith("/api/admin/providers/")) {
    const item = providers.find((p) => p.provider.id === target)!;
    if (path.endsWith("/test"))
      data = {
        check_id: crypto.randomUUID(),
        accepted: true,
        provider_id: target,
        credential_id: item.credential.id,
      };
    else if (method === "patch") {
      item.provider.status = body.status;
      item.provider.revision++;
      data = item.provider;
    } else data = item;
  } else if (path.startsWith("/api/admin/models/")) {
    const model = models.find((m) => m.id === target)!;
    if (method !== "get") {
      model.status = body.status;
      model.revision++;
      data = model;
    } else data = { model };
  } else if (path === "/api/platform/settings")
    data = {
      revision: "1",
      registration_enabled: true,
      team_creation_enabled: false,
    };
  else if (!path.includes("reauth"))
    throw new Error(`Unmocked fixture request: ${method} ${path}`);
  if (key && method !== "get") {
    receipts.set(key, data);
    await new Promise((r) => setTimeout(r, 400));
    if (loseResponse) {
      loseResponse = false;
      throw new axios.AxiosError(
        "fixture lost response",
        "ERR_NETWORK",
        config,
      );
    }
  }
  return {
    data: structuredClone(data),
    status: 200,
    statusText: "OK",
    headers: {},
    config,
  };
};
const [
  { UserDirectory },
  { ProvidersPage },
  { ModelsPage },
  { PlatformSettingsPage },
  { setCSRFToken },
] = await Promise.all([
  import("../../src/app/users/_components/users-page"),
  import("../../src/app/providers/_components/providers-page"),
  import("../../src/app/models/_components/models-page"),
  import("../../src/app/platform/settings/_components/platform-settings-page"),
  import("../../src/lib/request"),
]);
setCSRFToken("fixture-only");
const screen = new URLSearchParams(location.search).get("screen") ?? "users";
createRoot(document.getElementById("root")!).render(
  <ThemeProvider
    attribute="class"
    forcedTheme={new URLSearchParams(location.search).get("theme") ?? "dark"}
  >
    <div className="border-b p-3 text-xs text-muted-foreground">
      受控验收 · 模拟 API · 无真实账号或供应商操作
    </div>
    <main className="p-4 md:p-6">
      {screen === "users" ? (
        <UserDirectory />
      ) : screen === "providers" ? (
        <ProvidersPage />
      ) : screen === "models" ? (
        <ModelsPage />
      ) : (
        <PlatformSettingsPage />
      )}
    </main>
    <Toaster />
  </ThemeProvider>,
);
