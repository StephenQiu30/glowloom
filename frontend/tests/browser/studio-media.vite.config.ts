import path from "node:path";
import type { IncomingMessage, ServerResponse } from "node:http";
import react from "@vitejs/plugin-react";

/** Test-only HTTP backend. No upstream proxy or production service is configured. */
const uuid = (n: number) =>
  `22222222-2222-4222-8222-${String(n).padStart(12, "0")}`;
const actorID = "11111111-1111-4111-8111-111111111111";
const orgID = uuid(1),
  scopeID = uuid(2),
  projectID = uuid(3);
const stamp = "2026-11-07T12:00:00Z";
type Media = {
  id: string;
  revision: number;
  kind: string;
  file_name: string;
  status: string;
  width?: number;
  height?: number;
  duration_ms?: number;
};
type Item = {
  id: string;
  asset_id: string;
  revision: number;
  kind: string;
  title: string;
  favorite: boolean;
  catalog_state: string;
  updated_at: string;
  media: Media;
};
type Style = {
  id: string;
  name: string;
  category: string;
  revision: number;
  owned: boolean;
  can_edit: boolean;
  prompt_fragment: string;
  negative_prompt: string;
  reference_asset_ids: string[];
  cover_asset_id: string | null;
  cover_item_id?: string;
  references_available: boolean;
  reference_items: {
    id: string;
    revision: number;
    asset_id: string;
    asset_revision: number;
    title: string;
  }[];
  deleted?: boolean;
};
type Event = {
  sequence: number;
  method: string;
  path: string;
  key?: string;
  status: number;
  note?: string;
  body?: unknown;
};
let events: Event[] = [],
  serial = 1000,
  items: Item[] = [],
  styles: Style[] = [];
let controls = {
  listError: false,
  mediaError: false,
  detailError: false,
  unavailable: false,
  saveLost: false,
  conflict: false,
  deleteBound: false,
  linkMode: "reject",
  latency: 200,
};
const receipts = new Map<string, { fingerprint: string; value: unknown }>();
const jobs = new Map<string, Record<string, unknown>>();
const project = {
  id: projectID,
  canvas_id: uuid(4),
  name: "分析风格 · 合成独立上下文",
  aspect_ratio: "16:9",
  style_key: null,
  revision: 1,
  update_time: stamp,
  status: "active",
  is_delete: false,
};
function mediaItem(n: number, kind: string, title: string): Item {
  return {
    id: uuid(n),
    asset_id: uuid(n),
    revision: 1,
    title,
    kind,
    favorite: n % 2 === 0,
    catalog_state: "active",
    updated_at: stamp,
    media: {
      id: uuid(n),
      kind,
      revision: 1,
      file_name: `${title}.${kind === "image" ? "png" : kind === "video" ? "webm" : "txt"}`,
      status: "ready",
      ...(kind === "document" ? {} : { width: 2048, height: 1536 }),
      ...(kind === "video" ? { duration_ms: 2000 } : {}),
    },
  };
}
function decorate(value: Style): Style {
  const available =
    !controls.unavailable || value.reference_asset_ids.length === 0;
  const refs = available
    ? value.reference_asset_ids
        .map((id) => items.find((i) => i.asset_id === id))
        .filter((i): i is Item => Boolean(i))
    : [];
  const { cover_item_id: _cover, ...base } = value;
  void _cover;
  return {
    ...base,
    references_available:
      available && refs.length === value.reference_asset_ids.length,
    reference_items: refs.map((i) => ({
      id: i.id,
      revision: i.revision,
      asset_id: i.asset_id,
      asset_revision: i.media.revision,
      title: i.title,
    })),
    ...(refs[0] ? { cover_item_id: refs[0].id } : {}),
  };
}
function reset() {
  items = [
    ...Array.from({ length: 8 }, (_, i) =>
      mediaItem(
        100 + i,
        "image",
        [
          "山谷晨光",
          "海边夕照",
          "水墨远山",
          "城市霓虹",
          "雾林侧光",
          "黏土角色",
          "漫画色块",
          "柔光人像",
        ][i]!,
      ),
    ),
    ...Array.from({ length: 3 }, (_, i) =>
      mediaItem(120 + i, "video", `合成视频 ${i + 1}`),
    ),
    ...Array.from({ length: 2 }, (_, i) =>
      mediaItem(130 + i, "document", `小说片段 ${i + 1}`),
    ),
  ];
  styles = Array.from({ length: 33 }, (_, i) => ({
    id: uuid(200 + i),
    name:
      i < 30
        ? ["电影柔光", "水墨留白", "三维黏土"][i % 3]! +
          (i > 2 ? ` ${i + 1}` : "")
        : ["我的海边风格", "我的无参考风格", "我的可修复风格"][i - 30]!,
    category: ["realistic", "2d", "3d"][i % 3]!,
    revision: 1,
    owned: i >= 30,
    can_edit: i >= 30,
    prompt_fragment:
      "低饱和画面、柔和侧光、层次分明的构图与细腻材质。此为合成风格描述，不代表真实账号。",
    negative_prompt: i < 30 ? "避免杂乱背景与过度锐化" : "",
    reference_asset_ids: i === 31 ? [] : [items[i % 8]!.id],
    cover_asset_id: i === 31 ? null : items[i % 8]!.id,
    references_available: true,
    reference_items: [],
  }));
  events = [];
  receipts.clear();
  jobs.clear();
  serial = 1000;
  controls = {
    listError: false,
    mediaError: false,
    detailError: false,
    unavailable: false,
    saveLost: false,
    conflict: false,
    deleteBound: false,
    linkMode: "reject",
    latency: 200,
  };
}
reset();
async function handle(req: IncomingMessage, res: ServerResponse) {
  const url = new URL(req.url!, `http://${req.headers.host}`),
    route = url.pathname,
    method = req.method ?? "GET";
  const key = String(req.headers["idempotency-key"] ?? "");
  let raw = Buffer.alloc(0);
  if (!["GET", "HEAD"].includes(method)) {
    for await (const chunk of req) {
      raw = Buffer.concat([raw, Buffer.from(chunk)]);
      if (raw.length > 22 * 1024 * 1024) {
        res.writeHead(413);
        res.end();
        return;
      }
    }
  }
  let body: Record<string, unknown> = {};
  if (req.headers["content-type"]?.includes("application/json")) {
    try {
      body = JSON.parse(raw.toString() || "{}");
    } catch {
      res.writeHead(422);
      res.end('{"code":"invalid_request"}');
      return;
    }
  }
  const reply = (value: unknown, status = 200, note?: string) => {
    if (route !== "/api/fixture/studio-media")
      events.push({
        sequence: events.length + 1,
        method,
        path: route + url.search,
        ...(key ? { key } : {}),
        status,
        ...(note ? { note } : {}),
        ...(!["GET", "HEAD"].includes(method)
          ? {
              body: req.headers["content-type"]?.includes("multipart")
                ? { multipart_bytes: raw.length }
                : body,
            }
          : {}),
      });
    res.writeHead(status, {
      "Content-Type": "application/json",
      "Cache-Control": "no-store",
      "X-Fixture": "studio-media",
    });
    res.end(JSON.stringify(value));
  };
  const fail = (code: string, status: number) => reply({ code }, status);
  if (route === "/api/fixture/studio-media") {
    if (method === "POST") {
      if (body.reset) reset();
      else
        for (const field of Object.keys(controls))
          if (field in body) Object.assign(controls, { [field]: body[field] });
    }
    return reply({
      fixture: true,
      controls,
      actorID,
      scopeID,
      projectID,
      receipts: receipts.size,
      styles: styles.filter((s) => !s.deleted).length,
      events: events.slice(-100),
    });
  }
  await new Promise((resolve) => setTimeout(resolve, controls.latency));
  if (
    method !== "GET" &&
    (req.headers["x-csrf-token"] !== "studio-fixture-csrf" ||
      req.headers["x-creation-scope"] !== scopeID)
  )
    return fail("fixture_identity_proof_missing", 403);
  const fingerprint = `${method} ${route} ${raw.toString()}`;
  if (key && receipts.has(key)) {
    const saved = receipts.get(key)!;
    return saved.fingerprint === fingerprint
      ? reply(saved.value, 200, "REPLAY: same key + payload; no new mutation")
      : fail("idempotency_key_reused", 409);
  }
  const remember = (value: unknown, lose = false) => {
    receipts.set(key, { fingerprint, value: structuredClone(value) });
    if (lose) {
      controls.saveLost = false;
      return reply(
        { code: "fixture_receipt_lost" },
        503,
        "Committed; receipt intentionally lost. Retry SAME key.",
      );
    }
    return reply(value);
  };
  if (route === "/api/style-library" && method === "GET") {
    if (controls.listError) return fail("dependency_unavailable", 503);
    const query = url.searchParams,
      q = (query.get("q") ?? "").toLowerCase(),
      mine = query.get("mine"),
      category = query.get("category"),
      offset = Number((query.get("cursor") ?? "page:0").split(":")[1]),
      limit = Number(query.get("limit") ?? 24);
    if (!Number.isSafeInteger(offset) || offset < 0 || limit !== 24)
      return fail("invalid_request", 422);
    const results = styles.filter(
      (s) =>
        !s.deleted &&
        (mine === "true" ? s.owned : mine === "false" ? !s.owned : true) &&
        (!category || s.category === category) &&
        (!q || s.name.toLowerCase().includes(q)),
    );
    return reply({
      items: results.slice(offset, offset + limit).map((s) => {
        const {
          prompt_fragment: _p,
          negative_prompt: _n,
          ...summary
        } = decorate(s);
        void _p;
        void _n;
        return summary;
      }),
      next_cursor:
        offset + limit < results.length ? `page:${offset + limit}` : null,
    });
  }
  const match = route.match(/^\/api\/style-library\/([^/]+)$/),
    existing = match
      ? styles.find((s) => s.id === match[1] && !s.deleted)
      : undefined;
  if (match && !existing) return fail("not_found", 404);
  if (existing && method === "GET")
    return controls.detailError
      ? fail("dependency_unavailable", 503)
      : reply(decorate(existing));
  if (
    (route === "/api/style-library" && method === "POST") ||
    (existing && ["PATCH", "DELETE"].includes(method))
  ) {
    if (!/^[a-f0-9-]{36}$/i.test(key)) return fail("invalid_request", 422);
    if (existing && !existing.owned) return fail("not_found", 404);
    if (existing && controls.conflict) {
      controls.conflict = false;
      existing.revision++;
      return fail("revision_conflict", 409);
    }
    if (existing && body.expected_revision !== existing.revision)
      return fail("revision_conflict", 409);
    if (method === "DELETE") {
      if (controls.deleteBound) return fail("style_preset_in_use", 409);
      existing!.deleted = true;
      existing!.revision++;
      return remember(
        {
          request_id: key,
          action: "delete",
          preset: decorate(existing!),
          deleted: true,
        },
        controls.saveLost,
      );
    }
    const refs = body.reference_asset_ids;
    if (
      typeof body.name !== "string" ||
      !body.name.trim() ||
      Array.from(body.name).length > 20 ||
      typeof body.prompt_fragment !== "string" ||
      !body.prompt_fragment.trim() ||
      Array.from(body.prompt_fragment).length > 2000 ||
      !["realistic", "2d", "3d"].includes(String(body.category)) ||
      !Array.isArray(refs) ||
      refs.length > 4 ||
      new Set(refs).size !== refs.length
    )
      return fail("invalid_request", 422);
    if (
      (controls.unavailable && refs.length) ||
      refs.some((id) => !items.some((i) => i.id === id && i.kind === "image"))
    )
      return fail("style_reference_unavailable", 409);
    const next: Style = {
      ...(existing ?? {
        id: uuid(serial++),
        revision: 0,
        owned: true,
        can_edit: true,
        negative_prompt: "",
        references_available: true,
        reference_items: [],
      }),
      name: body.name.trim(),
      prompt_fragment: body.prompt_fragment.trim(),
      category: String(body.category),
      reference_asset_ids: refs as string[],
      cover_asset_id: (refs[0] as string) ?? null,
    };
    next.revision++;
    if (existing) styles.splice(styles.indexOf(existing), 1, next);
    else styles.push(next);
    return remember(
      {
        request_id: key,
        action: existing ? "patch" : "create",
        preset: decorate(next),
        deleted: false,
      },
      controls.saveLost,
    );
  }
  if (route === "/api/media/library" && method === "GET") {
    if (controls.mediaError) return fail("dependency_unavailable", 503);
    const query = url.searchParams,
      scope = query.get("scope") ?? "personal";
    const all = items.filter(
      (i) =>
        (!query.get("kind") || i.kind === query.get("kind")) &&
        (!query.get("search") || i.title.includes(query.get("search")!)) &&
        (query.get("favorite_only") !== "true" || i.favorite) &&
        (!query.get("resolutions") || i.kind === "image"),
    );
    const page = Number(query.get("page") ?? 1),
      size = Number(query.get("page_size") ?? 24);
    return reply({
      current_actor_id: actorID,
      current_org_id: orgID,
      library_id: uuid(scope === "personal" ? 10 : 11),
      scope: {
        kind: scope,
        ...(scope === "project" ? { project_id: projectID } : {}),
      },
      revision: 1,
      page,
      page_size: size,
      total: all.length,
      folders: [],
      items: all.slice((page - 1) * size, page * size),
    });
  }
  const itemMatch = route.match(
      /^\/api\/media\/library\/items\/([^/]+)(?:\/(preview|download))?$/,
    ),
    item = itemMatch ? items.find((i) => i.id === itemMatch[1]) : undefined;
  if (itemMatch && !item) return fail("not_found", 404);
  if (item) {
    if (itemMatch![2] === "preview")
      return reply({
        asset: item.media,
        url: `${url.origin}/fixture-studio/${item.id}.${item.kind === "video" ? "webm" : "svg"}`,
        expires_at: new Date(Date.now() + 3600000).toISOString(),
      });
    if (itemMatch![2] === "download") {
      res.writeHead(200, {
        "Content-Type": "text/plain",
        "Content-Disposition": 'attachment; filename="fixture-reference.txt"',
      });
      res.end(
        "第1集\n这是本地合成小说文档原件。没有任何真实账号或供应商内容。",
      );
      return;
    }
    return reply(item);
  }
  if (route === "/api/media/library/uploads" && method === "POST") {
    if (!key || !req.headers["content-type"]?.startsWith("multipart/form-data"))
      return fail("invalid_request", 422);
    const fileName =
        raw.toString().match(/filename="([^"]+)"/)?.[1] ?? "fixture.png",
      ext = fileName.split(".").at(-1)?.toLowerCase(),
      kind = ["mp4", "mov", "webm"].includes(ext ?? "")
        ? "video"
        : ["txt", "docx"].includes(ext ?? "")
          ? "document"
          : "image";
    const added = mediaItem(serial++, kind, fileName);
    added.media.file_name = fileName;
    items.push(added);
    return remember({ asset: added.media });
  }
  if (route === "/api/media/library/video-links" && method === "POST") {
    if (controls.linkMode === "reject")
      return fail("video_link_unresolvable", 422);
    if (
      !key ||
      typeof body.url !== "string" ||
      !/^https:\/\/(?:[^/]+\.)?(douyin|iesdouyin|toutiao|ixigua)\.com\//.test(
        body.url,
      ) ||
      body.local_review_confirmed !== true
    )
      return fail("invalid_request", 422);
    const added = mediaItem(serial++, "video", "链接导入的合成视频");
    items.push(added);
    return remember({ asset: added.media }, controls.linkMode === "lost");
  }
  if (route === "/api/creation-context" && method === "POST")
    return reply(project);
  if (route === `/api/projects/${projectID}` && method === "GET")
    return reply(project);
  if (route === "/api/media/library/transfers" && method === "POST") {
    if (!key || !Array.isArray(body.items)) return fail("invalid_request", 422);
    const source = body.items as { id: string; revision: number }[];
    if (
      source.some(
        (s) => !items.some((i) => i.id === s.id && i.revision === s.revision),
      )
    )
      return fail("revision_conflict", 409);
    const job = {
      id: uuid(serial++),
      current_actor_id: actorID,
      current_org_id: orgID,
      source: body.source,
      target: body.target,
      revision: 1,
      status: "succeeded",
      items: source.map((s, index) => {
        const old = items.find((i) => i.id === s.id)!,
          next = {
            ...structuredClone(old),
            id: uuid(serial++),
            asset_id: uuid(serial++),
          };
        next.media.id = next.asset_id;
        items.push(next);
        return {
          index,
          source_item_id: old.id,
          target_item_id: next.id,
          target_asset_id: next.asset_id,
          status: "succeeded",
        };
      }),
    };
    jobs.set(job.id, job);
    return remember(job);
  }
  if (route.startsWith("/api/media/library/transfers/") && method === "GET") {
    const job = jobs.get(route.split("/").at(-1)!);
    return job ? reply(job) : fail("not_found", 404);
  }
  if (route === `/api/projects/${projectID}/models` && method === "GET")
    return reply({ items: [] });
  if (route === "/api/skills" && method === "GET") return reply({ items: [] });
  // No conversation execution is provided. Analysis may prepare, but can never call a supplier.
  return fail("fixture_route_not_allowed", 404);
}
const clip =
  "GkXfo59ChoEBQveBAULygQRC84EIQoKEd2VibUKHgQJChYECGFOAZwEAAAAAAAQDEU2bdLpNu4tTq4QVSalmU6yBoU27i1OrhBZUrmtTrIHYTbuMU6uEElTDZ1OsggElTbuMU6uEHFO7a1OsggPt7AEAAAAAAABZAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAVSalmsirXsYMPQkBNgI1MYXZmNjIuMTIuMTAyV0GNTGF2ZjYyLjEyLjEwMkSJiECfQAAAAAAAFlSua8iuAQAAAAAAAD/XgQFzxYjViWcxkOIhrJyBACK1nIN1bmSIgQCGhVZfVlA4g4EBI+ODhAX14QDgkLCBoLqBWpqBAlWwhFW5gQESVMNn/HNzoGPAgGfImkWjh0VOQ09ERVJEh41MYXZmNjIuMTIuMTAyc3PWY8CLY8WI1YlnMZDiIaxnyKFFo4dFTkNPREVSRIeUTGF2YzYyLjI4LjEwMiBsaWJ2cHhnyKFFo4hEVVJBVElPTkSHkzAwOjAwOjAyLjAwMDAwMDAwMAAfQ7Z1QkHngQCj0IEAAIDwBQCdASqgAFoAAEcIhYWIhYSIAgICdaoD+AIGmhak569GXiOevRl4jnr0ZeI569GXiOevRl4jlAD+/g0P/89M15g/yc/+Uq8Fjc3wo5iBAGQAEQIAARAQABgAGFgv9AAIgIEAAACjmIEAyAARAgABEBAAGAAYWC/0AAiAgQAAAKOYgQEsABECAAEQEAAYABhYL/QACICBAAAAo5iBAZAAEQIAARAQABgAGFgv9AAIgIEAAACjmIEB9AARAgABEBAAGAAYWC/0AAiAgQAAAKOYgQJYABECAAEQEAAYABhYL/QACICBAAAAo5eBArwA8QEAARAQFGAAYWC/0AAiAgQAAKOYgQMgABECAAEQEAAYABhYL/QACICBAAAAo5iBA4QAEQIAARAQABgAGFgv9AAIgIEAAACjmIED6AARAgABEBAAGAAYWC/0AAiAgQAAAKOYgQRMABECAAEQEAAYABhYL/QACICBAAAAo5iBBLAAEQIAARAQABgAGFgv9AAIgIEAAACjmIEFFAARAgABEBAAGAAYWC/0AAiAgQAAAKOYgQV4ABECAAEQEAAYABhYL/QACICBAAAAo5iBBdwAEQIAARAQABgAGFgv9AAIgIEAAACjmIEGQAARAgABEBAAGAAYWC/0AAiAgQAAAKOYgQakABECAAEQEAAYABhYL/QACICBAAAAo5eBBwgA8QEAARAQFGAAYWC/0AAiAgQAAKOYgQdsABECAAEQEAAYABhYL/QACICBAAAAHFO7a5G7j7OBALeK94EB8YIBpvCBAw==";
const config = {
  root: path.resolve(import.meta.dirname, "../.."),
  plugins: [
    react(),
    {
      name: "studio-media-fixture-only",
      configureServer(server: {
        middlewares: {
          use: (
            handler: (
              req: IncomingMessage,
              res: ServerResponse,
              next: () => void,
            ) => void,
          ) => void;
        };
      }) {
        server.middlewares.use((req, res, next) => {
          if (req.url?.startsWith("/fixture-studio/")) {
            if (req.url.endsWith(".webm")) {
              res.writeHead(200, { "Content-Type": "video/webm" });
              res.end(Buffer.from(clip, "base64"));
              return;
            }
            const n = Number(req.url.match(/(\d+)\.svg$/)?.[1] ?? 1),
              hue = n % 360;
            res.writeHead(200, { "Content-Type": "image/svg+xml" });
            res.end(
              `<svg xmlns="http://www.w3.org/2000/svg" width="640" height="480"><rect width="640" height="480" fill="hsl(${hue} 30% 22%)"/><circle cx="450" cy="120" r="66" fill="hsl(${hue} 50% 75%)"/><path d="M0 480L180 140 380 480 540 270 640 480" fill="hsl(${hue} 35% 45%)"/><text x="24" y="450" fill="white" font-size="22">SYNTHETIC FIXTURE ${n}</text></svg>`,
            );
            return;
          }
          if (!req.url?.startsWith("/api/")) return next();
          void handle(req, res).catch((error) => {
            console.error("Fixture error", error);
            if (!res.headersSent)
              res.writeHead(500, { "Content-Type": "application/json" });
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
  server: { host: "127.0.0.1", port: 5204, strictPort: true },
};
export default config;
