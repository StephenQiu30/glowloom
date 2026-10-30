/** Source-component browser acceptance; deterministic local transport, no real data or suppliers. */
import React from "react";
import { createRoot } from "react-dom/client";
import { ThemeProvider } from "next-themes";
import { SessionProvider } from "../../src/components/auth/session";
import { SpaceProvider } from "../../src/components/spaces/space-context";
import { BasicLayout } from "../../src/components/layout/basic-layout";
import { AssetWorkspace } from "../../src/app/assets/_components/asset-workspace";
import { readAssetLocation } from "../../src/app/assets/_components/asset-location";
import { TooltipProvider } from "../../src/components/ui/tooltip";
import { Toaster } from "../../src/components/ui/sonner";
import { installDevelopmentTransport } from "../../src/lib/development-transport";
import { RequestError } from "../../src/lib/request";
import "../../src/app/globals.css";

const uuid = (value: number) =>
  `00000000-0000-4000-8000-${String(value).padStart(12, "0")}`;
const actor = {
  id: uuid(1),
  email: "assets@example.test",
  display_name: "资产验收（合成）",
  platform_role: "creator",
  status: "active",
  revision: "1",
  capabilities: [],
};
const session = {
  actor,
  session_id: uuid(2),
  csrf_token: "fixture-only",
  credential_epoch: "1",
  persistent: false,
  last_active_at: new Date().toISOString(),
  absolute_expires_at: new Date(Date.now() + 3600000).toISOString(),
};
const project = {
  id: uuid(3),
  canvas_id: uuid(4),
  name: "合成验收项目",
  revision: 2,
  aspect_ratio: "16:9",
  style_key: null,
  update_time: "2026-10-28T01:00:00Z",
  status: "active",
  is_delete: false,
};
const scenario = new URLSearchParams(location.search).get("case");
const svg =
  '<svg xmlns="http://www.w3.org/2000/svg" width="960" height="640" viewBox="0 0 960 640"><defs><linearGradient id="a"><stop stop-color="#23404b"/><stop offset="1" stop-color="#a4bbc1"/></linearGradient></defs><rect width="960" height="640" fill="url(#a)"/><circle cx="720" cy="145" r="70" fill="#f4e6bf"/><path d="M0 540L260 170 520 540 730 300 960 640H0" fill="#344c52"/><text x="35" y="600" fill="white" font-family="sans-serif" font-size="30">SYNTHETIC ACCEPTANCE FIXTURE</text></svg>';
const previewURL = URL.createObjectURL(
  new Blob([svg], { type: "image/svg+xml" }),
);
const imageBytes = Uint8Array.from(
  atob(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a7S8AAAAASUVORK5CYII=",
  ),
  (char) => char.charCodeAt(0),
);
const tasks = (["image", "video", "audio", "document"] as const).map(
  (kind, index) => ({
    id: uuid(10 + index),
    project_id: project.id,
    capability: kind === "document" ? "text.structured" : `${kind}.generate`,
    mode:
      kind === "video"
        ? "lip_sync"
        : kind === "audio"
          ? "song"
          : "text_to_image",
    model_name: "合成模型",
    status: "completed",
    create_time: "2026-10-28T02:30:00Z",
    prompt_preview: `${{ image: "山谷晨光", video: "沿山谷缓缓推进", audio: "宁静的山间旋律", document: "山谷场景的分镜说明" }[kind]}（合成验收）`,
    history_revision: 1,
    params: { aspect_ratio: "16:9", seed: 42 },
    inputs: [
      {
        role: "prompt",
        text: "山谷、晨光、层叠山峦；此内容仅用于源码组件验收。",
      },
    ],
    outputs: [0, 1].map((outputIndex) => ({
      id: uuid(100 + index * 10 + outputIndex),
      kind: kind === "document" ? "json" : kind,
      media_asset_id:
        kind === "document" ? null : uuid(200 + index * 10 + outputIndex),
      moderation_status: "passed",
      favorite: outputIndex === 0,
      metadata_revision: 1,
      resolution: kind === "image" ? "2K" : kind === "video" ? "1080P" : "",
      ratio: kind === "image" || kind === "video" ? "16:9" : "",
      generation_mode:
        kind === "video" ? "lip_sync" : kind === "audio" ? "song" : "",
      process_types: [],
      can_download: true,
      can_publish: kind === "image" || kind === "video",
      text:
        kind === "document"
          ? JSON.stringify(
              { scene: outputIndex + 1, description: "合成分镜正文" },
              null,
              2,
            )
          : undefined,
    })),
  }),
);
if (scenario === "original" || scenario === "mixed") {
  const document = tasks.find((task) => task.capability === "text.structured")!;
  document.outputs.forEach((output, index) => {
    if (scenario === "original" || index === 1) {
      output.kind = "document";
      output.media_asset_id = uuid(900 + index);
      output.text = undefined;
    }
  });
}
const containers = (["current", "legacy"] as const).flatMap(
  (collection, index) =>
    ["canvas", "short_drama"].map((kind, offset) => ({
      id: uuid(300 + index * 10 + offset),
      project_id: project.id,
      name:
        scenario === "text"
          ? "长名称画布 · 山谷晨光与海边香氛的品牌视觉参考（合成验收）"
          : `${collection === "legacy" ? "旧草稿" : kind === "short_drama" ? "山谷短剧" : "山谷画布"}（合成验收）`,
      kind,
      collection,
      revision: "5",
      project_revision: 2,
      is_main: false,
      favorite: false,
      favorite_revision: "0",
      cover_asset_id: uuid(200),
      cover_unavailable: false,
      can_copy_project: collection === "current",
      created_at: "2026-10-27T01:00:00Z",
      updated_at:
        scenario === "text" ? "2024-10-28T01:00:00Z" : "2026-10-28T01:00:00Z",
    })),
);
const receipts = new Map<string, unknown>();
let lost = scenario === "lost";
installDevelopmentTransport({
  matches: (path) => path === "/tests/browser/assets.html",
  request: async (url, options) => {
    const params = options.params ?? {},
      body = options.data ?? {},
      method = options.method ?? "GET";
    const key = String(
      (options.headers as Record<string, string> | undefined)?.[
        "Idempotency-Key"
      ] ?? "",
    );
    if (key && receipts.has(key)) return receipts.get(key);
    if (url === "/api/session") return session;
    if (url === "/api/me/preferences")
      return {
        revision: "1",
        theme: "dark",
        canvas_background: "dots",
        prompt_suffix: "",
      };
    if (url === "/api/me/spaces")
      return {
        items: [
          {
            id: uuid(5),
            name: "合成个人空间",
            kind: "personal",
            role: "owner",
            writable: true,
          },
        ],
      };
    if (url === "/api/projects") return { items: [], next_cursor: null };
    if (url === `/api/projects/${project.id}`) return project;
    if (url === "/api/operations/history") {
      if (scenario === "error")
        throw new RequestError("dependency_unavailable", 503);
      if (scenario === "forbidden") throw new RequestError("forbidden", 403);
      if (scenario === "loading")
        await new Promise((resolve) => setTimeout(resolve, 1500));
      let items =
        scenario === "empty"
          ? []
          : tasks.filter((task) =>
              task.capability.startsWith(
                params.kind === "document" ? "text." : `${params.kind}.`,
              ),
            );
      if (params.search)
        items = items.filter((task) =>
          task.prompt_preview.includes(params.search),
        );
      return {
        items: items.map((task) => ({
          ...task,
          outputs: task.outputs.filter(
            (output) => !params.favorite_only || output.favorite,
          ),
        })),
        next_cursor: null,
      };
    }
    if (url === "/api/operations/history/commands") {
      const result = {
        request_id: key,
        action: body.action,
        results: body.targets.map(
          (target: API.PersonalHistoryTarget, index: number) => {
            const task = tasks.find((item) => item.id === target.operation_id)!,
              output = task.outputs.find(
                (item) => item.id === target.output_id,
              );
            const ok = scenario !== "partial" || index === 0;
            if (ok) {
              if (body.action === "favorite") {
                for (const candidate of output ? [output] : task.outputs) {
                  candidate.favorite = body.favorited;
                  candidate.metadata_revision++;
                }
              } else
                task.outputs = task.outputs.filter(
                  (item) => target.output_id && item.id !== target.output_id,
                );
              task.history_revision++;
            }
            return {
              operation_id: target.operation_id,
              output_id: target.output_id ?? null,
              ok,
              code: ok ? "applied" : "revision_conflict",
              revision: output?.metadata_revision ?? task.history_revision,
              affected_output_ids: output
                ? [output.id]
                : task.outputs.map((item) => item.id),
            };
          },
        ),
      };
      receipts.set(key, result);
      if (lost) {
        lost = false;
        throw new RequestError("dependency_unavailable", 503);
      }
      return result;
    }
    if (url === "/api/operations/history/publications") {
      const result = {
        items: body.targets.map(
          (target: API.HistoryPublicationTarget, index: number) => ({
            ...target,
            status:
              scenario === "partial" && index > 0 ? "failed" : "published",
            work_id: uuid(500 + index),
            url: `/works/${uuid(500 + index)}`,
            error_code:
              scenario === "partial" && index > 0
                ? "fixture_rejected"
                : undefined,
          }),
        ),
      };
      receipts.set(key, result);
      return result;
    }
    if (/\/api\/operations\/[^/]+$/.test(url))
      return tasks.find((task) => url.endsWith(task.id));
    if (url.endsWith("/preview")) {
      const id = url.split("/").at(-2)!;
      return {
        asset: {
          id,
          project_id: project.id,
          kind:
            tasks
              .flatMap((task) => task.outputs)
              .find((output) => output.media_asset_id === id)?.kind ?? "image",
        },
        url: previewURL,
        expires_at: new Date(Date.now() + 3600000).toISOString(),
      };
    }
    if (url.endsWith("/download"))
      return new Blob(["正式文档合成原件，仅用于源码组件验收。"], {
        type: "text/plain",
      });
    if (url.endsWith("/original"))
      return new Blob([imageBytes], { type: "image/png" });
    if (url === "/api/canvas-assets/templates")
      return {
        items: [
          {
            key:
              params.kind === "short_drama"
                ? "storyboard-three-shots-v1"
                : "text-to-image-v1",
            kind: params.kind,
            name: params.kind === "short_drama" ? "三镜分镜" : "文字生图",
            description: "创建可编辑的预填工作流，不自动运行生成。",
            is_template: true,
            node_count: params.kind === "short_drama" ? 7 : 3,
            requires_model: true,
          },
        ],
      };
    if (url === "/api/canvas-assets")
      return {
        id: uuid(800),
        project_id: uuid(801),
        name: "未命名画布",
        kind: body.kind,
        collection: "current",
        revision: "1",
        project_revision: 1,
        template_key: body.template_key ?? "",
        is_template: false,
      };
    if (url === "/api/canvases")
      return {
        items: containers.filter(
          (item) =>
            item.kind === params.kind &&
            item.collection === params.collection &&
            (!params.favorite_only || item.favorite),
        ),
        next_cursor: null,
      };
    if (url === "/api/canvas-assets/commands") {
      const result = {
        results: body.items.map((target: API.AssetCommandItem) => {
          const item = containers.find((item) => item.id === target.id)!;
          if (body.action === "rename") item.name = body.name;
          if (body.action === "favorite") {
            item.favorite = body.favorite;
            item.favorite_revision = String(Number(item.favorite_revision) + 1);
          }
          if (body.action === "delete")
            containers.splice(containers.indexOf(item), 1);
          return {
            id: item.id,
            status: "succeeded",
            code: "applied",
            revision: item.revision,
            favorite: item.favorite,
            favorite_revision: item.favorite_revision,
            project_revision: item.project_revision,
          };
        }),
        succeeded: body.items.length,
        failed: 0,
      };
      receipts.set(key, result);
      return result;
    }
    if (url.startsWith("/api/projects/") && url.endsWith("/models"))
      return { items: [] };
    if (method === "GET") return { items: [], next_cursor: null };
    throw new Error(`Unmocked fixture command: ${method} ${url}`);
  },
});
const initialLocation = readAssetLocation(new URLSearchParams(location.search));
createRoot(document.getElementById("root")!).render(
  <ThemeProvider attribute="class" defaultTheme="dark">
    <TooltipProvider>
      <SessionProvider>
        <SpaceProvider>
          <BasicLayout screen="assets">
            <AssetWorkspace initialLocation={initialLocation} />
          </BasicLayout>
        </SpaceProvider>
      </SessionProvider>
    </TooltipProvider>
    <Toaster />
  </ThemeProvider>,
);
