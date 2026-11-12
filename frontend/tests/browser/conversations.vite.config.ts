import path from "node:path";
import { createReadStream, readFileSync } from "node:fs";
import type { IncomingMessage, ServerResponse } from "node:http";
import { randomUUID, createHash } from "node:crypto";
import react from "@vitejs/plugin-react";
const pid = "11111111-1111-4111-8111-111111111114",
  sid = "11111111-1111-4111-8111-111111111115";
type Run = {
  root_run_id: string;
  session_id: string;
  submission_id: string;
  goal: string;
  input: Record<string, unknown>;
  manifest: Record<string, unknown>;
  execution_available: boolean;
  snapshot_cursor: string;
  events: unknown[];
  messages_page: { items: unknown[]; through: string };
  state: {
    status: string;
    execution_revision: number;
    authorization_revision: number;
    production?: Record<string, unknown>;
  };
};
let revision = 0,
  stops = 0,
  control: Record<string, unknown> = {};
const runs: Run[] = [];
const exportJobs = new Map<
  string,
  { id: string; status: string; revision: number; output_kind: string }
>();
const reviewKeys = new Map<string, unknown>();
const productionLedger = {
  accepted_goals: [] as unknown[],
  reviews: [] as string[],
  downloads: [] as string[],
};
const videoExport = "11111111-1111-4111-8111-111111111151",
  projectExport = "11111111-1111-4111-8111-111111111152",
  timeline = "11111111-1111-4111-8111-111111111153",
  canvasID = "11111111-1111-4111-8111-111111111154";
function exportBytes(id: string) {
  const directory = process.env.GLOWLOOM_AGENT_SHOT_ARTIFACT_DIR;
  if (!directory) throw new Error("Actual D1 test artifacts must be provided");
  return readFileSync(
    path.join(
      directory,
      `glowloom-agent-contract-browser.${id === videoExport ? "mp4" : "zip"}`,
    ),
  );
}
function exportJob(id: string) {
  const current = exportJobs.get(id);
  if (!current) return;
  return {
    ...current,
    attempt: 1,
    project_id: pid,
    source: { canvas_id: canvasID, node_id: timeline, revision: 9 },
    sha256: createHash("sha256").update(exportBytes(id)).digest("hex"),
    ...(id === videoExport
      ? { asset_id: "11111111-1111-4111-8111-111111111155" }
      : { artifact_id: "11111111-1111-4111-8111-111111111156" }),
  };
}
function productionFacts() {
  return {
    cost: {
      authorized_limit: 1000000,
      pending_commitment: 0,
      settled_charge: 300000,
      remaining: 700000,
      currency: "CNY",
      unit: "micros",
    },
    deliveries: [
      {
        delivery_id: timeline,
        canvas_id: canvasID,
        timeline_node_id: timeline,
        video_export_id: videoExport,
        project_export_id: projectExport,
      },
    ],
  };
}
const operationID = "11111111-1111-4111-8111-111111111130";
let operationStatus = "completed";
async function handle(req: IncomingMessage, res: ServerResponse) {
  const url = new URL(req.url!, "http://fixture"),
    route = url.pathname;
  let body: Record<string, unknown> = {};
  if (req.method === "POST") {
    let raw = "";
    for await (const chunk of req) raw += chunk;
    body = JSON.parse(raw || "{}");
  }
  const reply = (data: unknown, status = 200) => {
    res.statusCode = status;
    res.setHeader("Content-Type", "application/json");
    res.end(JSON.stringify(data));
  };
  const latest = runs.at(-1),
    id = route.match(/\/runs\/([^/]+)/)?.[1],
    run = runs.find((r) => r.root_run_id === id),
    session = {
      id: sid,
      actor_id: "11111111-1111-4111-8111-111111111111",
      org_id: sid,
      scope_id: sid,
      project_id: pid,
      title: "隔离测试对话",
      execution_revision: revision,
      ...(latest ? { run: latest } : {}),
    };
  if (route === `/api/projects/${pid}/media`) return reply({ items: [] });
  if (route === "/api/fixture/production-ledger")
    return reply({
      ...productionLedger,
      roots: runs.map((item) => item.root_run_id),
    });
  if (route === "/api/fixture/production-ready" && req.method === "POST") {
    if (
      !latest ||
      (latest.manifest.goal_contract as { kind?: string } | undefined)?.kind !==
        "production"
    )
      return reply({ code: "production_goal_required" }, 409);
    exportJobs.set(videoExport, {
      id: videoExport,
      status: "review_required",
      revision: 3,
      output_kind: "video",
    });
    exportJobs.set(projectExport, {
      id: projectExport,
      status: "review_required",
      revision: 3,
      output_kind: "fcpxml",
    });
    latest.state.production = productionFacts();
    latest.state.status = "waiting_delivery";
    latest.state.execution_revision = ++revision;
    latest.snapshot_cursor = `${revision}:1`;
    latest.messages_page.through = latest.snapshot_cursor;
    return reply({});
  }
  const exportMatch = route.match(
    /^\/api\/media-exports\/([^/]+)(?:\/(preview|review|download))?$/,
  );
  if (exportMatch) {
    const id = exportMatch[1]!,
      action = exportMatch[2],
      job = exportJob(id);
    if (!job) return reply({}, 404);
    if (!action) return reply(job);
    if (action === "preview")
      return reply({
        job_id: id,
        revision: job.revision,
        sha256: job.sha256,
        url: `http://127.0.0.1:5198/api/fixture/export-bytes/${id}`,
        expires_at: new Date(Date.now() + 60000).toISOString(),
        ...(id === videoExport
          ? {
              asset: {
                id: "asset_id" in job ? job.asset_id : undefined,
                project_id: pid,
                kind: "video",
              },
            }
          : {
              artifact: {
                id: "artifact_id" in job ? job.artifact_id : undefined,
                job_id: id,
                project_id: pid,
                attempt: 1,
                format: "fcpxml",
                format_version: "1.10",
                sha256: job.sha256,
                mime_type: "application/zip",
                file_name: `timeline-${id}.zip`,
                byte_size: exportBytes(id).length,
                created_at: new Date().toISOString(),
              },
            }),
      });
    if (action === "review" && req.method === "POST") {
      const key = String(req.headers["idempotency-key"] ?? "");
      if (reviewKeys.has(key)) return reply(reviewKeys.get(key));
      if (
        !key ||
        body.project_id !== pid ||
        body.revision !== job.revision ||
        body.sha256 !== job.sha256 ||
        body.local_review_confirmed !== true ||
        job.status !== "review_required"
      )
        return reply({ code: "review_conflict" }, 409);
      exportJobs.set(id, {
        id,
        status: "succeeded",
        revision: job.revision + 1,
        output_kind: job.output_kind,
      });
      const result = exportJob(id);
      reviewKeys.set(key, result);
      productionLedger.reviews.push(id);
      if (latest) {
        latest.state.status = [...exportJobs.values()].every(
          (item) => item.status === "succeeded",
        )
          ? "completed"
          : "waiting_delivery";
        latest.state.execution_revision = ++revision;
        latest.snapshot_cursor = `${revision}:1`;
        latest.messages_page.through = latest.snapshot_cursor;
      }
      return reply(result);
    }
    if (action === "download") {
      if (job.status !== "succeeded") return reply({}, 409);
      productionLedger.downloads.push(id);
      res.setHeader(
        "Content-Type",
        id === videoExport ? "video/mp4" : "application/zip",
      );
      return res.end(exportBytes(id));
    }
  }
  if (route.startsWith("/api/fixture/export-bytes/")) {
    const id = route.split("/").at(-1)!;
    if (!exportJobs.has(id)) return reply({}, 404);
    res.setHeader(
      "Content-Type",
      id === videoExport ? "video/mp4" : "application/zip",
    );
    return res.end(exportBytes(id));
  }
  // Controlled outcomes are fixture-only: every /api request stays in this middleware.
  if (route === "/api/fixture/conversation-outcome" && req.method === "POST") {
    if (!latest) return reply({ code: "send_first" }, 409);
    const outcome = String(body.outcome);
    if (!["completed", "failed", "submitted"].includes(outcome))
      return reply({}, 400);
    operationStatus = outcome;
    latest.state.status = outcome === "failed" ? "failed" : "completed";
    latest.state.execution_revision = ++revision;
    latest.snapshot_cursor = `${revision}:1`;
    latest.messages_page = {
      through: latest.snapshot_cursor,
      items: [
        {
          message_id: randomUUID(),
          run_id: latest.root_run_id,
          conversation_id: 1,
          sequence: `${revision}:1`,
          part_index: 0,
          kind: "assistant",
          content:
            "## 分镜草稿\n\n日落海边，香水瓶映出海浪的碎金。\n\n这是隔离结果示例，不调用真实生成。",
        },
        {
          message_id: randomUUID(),
          run_id: latest.root_run_id,
          conversation_id: 1,
          sequence: `${revision}:2`,
          part_index: 0,
          kind: "operation",
          content: operationID,
        },
      ],
    };
    control = { authorization_revision: 1 };
    return reply({});
  }
  if (route === `/api/operations/${operationID}`)
    return reply({
      id: operationID,
      project_id: pid,
      status: operationStatus,
      capability: "image.generate",
      mode: "text2image",
      model_name: "隔离图片模型",
      editor_draft: latest?.input.draft,
      inputs: [{ role: "prompt", text: "海边日落香水广告图 · 隔离示例" }],
      outputs:
        operationStatus === "completed"
          ? [
              {
                id: "11111111-1111-4111-8111-111111111131",
                kind: "image",
                moderation_status: "passed",
                media_asset_id: "11111111-1111-4111-8111-111111111121",
              },
            ]
          : [],
      ...(operationStatus === "failed"
        ? { failure_code: "provider_error" }
        : {}),
      billing_outcome: {
        delivery_state: operationStatus === "completed" ? "ready" : "pending",
        regenerate: operationStatus === "submitted" ? "blocked" : "allowed",
      },
    });
  if (route === `/api/operations/${operationID}/cancel`) {
    operationStatus = "cancelling";
    return reply({
      target_id: operationID,
      target_type: "operation",
      action: "cancel",
      accepted: true,
    });
  }
  if (route === "/api/models")
    return reply({
      items: [
        {
          id: "planner",
          key: "fixture",
          display_name: "隔离规划模型",
          capability: "text.generate",
          available: true,
          mode_availability: [{ mode: "text_to_text", available: true }],
          status: "active",
          provider_status: "active",
          credential_present: true,
          input_roles: ["prompt", "reference_image"],
          current_version: {
            id: "11111111-1111-4111-8111-111111111119",
            limits: {
              roles: { reference_image: { types: ["image"], max_count: 4 } },
            },
          },
          current_price: { id: "price" },
        },
      ],
    });
  if (route.endsWith("/agent/skills"))
    return reply([
      {
        skill_id: "11111111-1111-4111-8111-111111111141",
        version: "2",
        resource_hash: "a".repeat(64),
        name: "叙事短片导演分镜",
        description: "从故事构思到镜头设计，逐步完成分镜与创作提示词。",
        enabled: true,
      },
      {
        skill_id: "11111111-1111-4111-8111-111111111142",
        version: "1",
        resource_hash: "b".repeat(64),
        name: "系列套图生成",
        description: "围绕同一角色或产品，制作风格一致的系列图片。",
        enabled: true,
      },
      {
        skill_id: "11111111-1111-4111-8111-111111111143",
        version: "1",
        resource_hash: "c".repeat(64),
        name: "爆款电商短视频题材创意",
        description: "整理商品卖点，生成短视频创意与分镜方案。",
        enabled: true,
      },
    ]);
  if (route.endsWith("/agent/sessions"))
    return reply(
      req.method === "POST" ? session : { items: runs.length ? [session] : [] },
    );
  if (route.endsWith(`/sessions/${sid}`)) return reply(session);
  if (route.endsWith("/submissions/current"))
    return reply(
      latest
        ? {
            latest_accepted: {
              root_run_id: latest.root_run_id,
              session_id: sid,
              submission_id: latest.submission_id,
              execution_revision: revision,
              status: latest.state.status,
            },
            unresolved_run_ids: [],
          }
        : {},
    );
  if (route.endsWith(`/sessions/${sid}/runs`)) {
    if (req.method !== "POST")
      return reply({
        items: runs.map((r, i) => ({
          root_run_id: r.root_run_id,
          session_id: sid,
          accepted_revision: i + 1,
          created_at: new Date().toISOString(),
          status: r.state.status,
          input_excerpt: "隔离测试输入",
        })),
      });
    const draft = body.input as Record<string, unknown>,
      doc = draft.document as {
        content: { content?: { type: string; text?: string }[] }[];
      };
    const text = doc.content
      .map(
        (p) =>
          p.content
            ?.filter((n) => n.type === "text")
            .map((n) => n.text)
            .join("") ?? "",
      )
      .join("\n");
    const goal = body.goal_contract as
      { kind?: string; required_deliveries?: number } | undefined;
    if (goal) {
      if (goal.kind !== "production" || goal.required_deliveries !== 1)
        return reply({ code: "invalid_goal" }, 422);
      productionLedger.accepted_goals.push(goal);
    }
    const rid = randomUUID();
    revision++;
    const next: Run = {
      root_run_id: rid,
      session_id: sid,
      submission_id: randomUUID(),
      goal: text,
      input: {
        draft,
        schema_version: 1,
        input_hash: "a".repeat(64),
        plain_text: text,
        prompt: text,
        media_pins: [],
      },
      manifest: {
        ...(body.manifest as Record<string, unknown>),
        ...(goal ? { goal_contract: goal } : {}),
      },
      execution_available: true,
      snapshot_cursor: `${revision}:1`,
      events: [],
      messages_page: { items: [], through: `${revision}:1` },
      state: {
        status: "running",
        execution_revision: revision,
        authorization_revision: 1,
      },
    };
    runs.push(next);
    control = { authorization_revision: 1 };
    return reply({
      root_run_id: rid,
      session_id: sid,
      submission_id: next.submission_id,
      execution_revision: revision,
      status: "accepted",
    });
  }
  if (route.endsWith("/commands/current")) {
    if (control.delivery_status === "pending" && ++stops > 2 && run) {
      control = {
        ...control,
        delivery_status: "applied",
        application_receipt: {},
      };
      run.state.status = "stopped";
      run.state.execution_revision = ++revision;
      run.messages_page.items = [
        {
          message_id: randomUUID(),
          run_id: run.root_run_id,
          conversation_id: 1,
          sequence: `${revision}:1`,
          part_index: 0,
          kind: "assistant",
          content: "已经保留这次的分镜草稿。",
        },
      ];
    }
    return reply(control);
  }
  if (route.endsWith("/commands") && run) {
    stops = 0;
    control = {
      authorization_revision: 2,
      current_decision: {
        root_run_id: run.root_run_id,
        decision_id: randomUUID(),
        action: body.action,
        authorization_revision: 2,
      },
      delivery_status: "pending",
    };
    return reply({});
  }
  if (route.endsWith("/events")) {
    res.writeHead(200, { "Content-Type": "text/event-stream" });
    res.write(": fixture\n\n");
    return;
  }
  if (run && route.includes("/messages/")) {
    const messageID = route.split("/").at(-1);
    const part = (
      run.messages_page.items as { message_id: string; content: string }[]
    ).find((p) => p.message_id === messageID);
    return part ? reply({ content: part.content }) : reply({}, 404);
  }
  if (run) return reply(run);
  if (route.endsWith("/original")) {
    res.setHeader("Content-Type", "image/png");
    res.end(
      Buffer.from(
        "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
        "base64",
      ),
    );
    return;
  }
  if (route.endsWith("/preview")) {
    const asset = route.split("/").at(-2)!;
    return reply({
      asset: { id: asset, project_id: pid, media_kind: "image" },
      url: "http://127.0.0.1:5198/fixture-image.svg",
      expires_at: new Date(Date.now() + 60000).toISOString(),
    });
  }
  if (route.includes("/media/"))
    return reply({
      id: route.split("/").at(-1),
      project_id: pid,
      kind: "image",
      status: "ready",
      revision: 1,
    });
  return reply({ code: "fixture_route_not_allowed" }, 404);
}
const config = {
  root: path.resolve(import.meta.dirname, "../.."),
  plugins: [
    react(),
    {
      name: "conversation-fixture-only-api",
      configureServer(server: {
        middlewares: {
          use: (
            fn: (
              req: IncomingMessage,
              res: ServerResponse,
              next: () => void,
            ) => void,
          ) => void;
        };
      }) {
        server.middlewares.use((req, res, next) => {
          if (req.url === "/fixture-image.svg") {
            res.setHeader("Content-Type", "image/svg+xml");
            res.end(
              '<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100"><rect width="100" height="100" fill="#7a6fce"/><circle cx="50" cy="50" r="28" fill="#d6d0f8"/></svg>',
            );
            return;
          }
          if (req.url?.startsWith("/_next/image?")) {
            const src = new URL(req.url, "http://fixture").searchParams.get(
              "url",
            );
            if (
              !src ||
              !/^\/design\/reference\/(moonlight-whale|amber-perfume|cobalt-headphones)\.png$/.test(
                src,
              )
            ) {
              res.statusCode = 404;
              res.end();
              return;
            }
            res.setHeader("Content-Type", "image/png");
            createReadStream(
              path.resolve(import.meta.dirname, "../../public", `.${src}`),
            ).pipe(res);
            return;
          }
          if (!req.url?.startsWith("/api/")) return next();
          void handle(req, res).catch(() => {
            res.statusCode = 500;
            res.end('{"code":"fixture_error"}');
          });
        });
      },
    },
  ],
  define: { "process.env": "{}" },
  resolve: {
    alias: [
      {
        find: "@/components/auth/session",
        replacement: path.resolve(import.meta.dirname, "ai-presets-session.ts"),
      },
      {
        find: "next/navigation",
        replacement: path.resolve(import.meta.dirname, "assets-navigation.ts"),
      },
      {
        find: "@",
        replacement: path.resolve(import.meta.dirname, "../../src"),
      },
    ],
  },
  server: { host: "127.0.0.1", port: 5198, strictPort: true },
};

export default config;
