import type { RequestOptions } from "@/lib/request";
import { RequestError } from "@/lib/request";
import type { Project } from "@/components/projects/project";
import { canonical } from "@/components/generation/draft-save";
import {
  createDraftNode,
  createMediaNode,
  createResourceNode,
  isDraft,
  requireCanvas,
  sameDraftInput,
  type Canvas,
  type Node,
} from "../document";

export const previewActor = "f0000000-0000-4000-8000-000000000001";
export const previewScope = "f0000000-0000-4000-8000-000000000002";
const projectID = "f0000000-0000-4000-8000-000000000003";
type Asset = { asset: API.AssetSummary; item: string; url?: string };
type PreviewResult = Canvas & { results: API.CommandResult[] };
type Receipt = { key: string; body: string; result: PreviewResult };
type State = {
  canvas: Canvas;
  assets: Asset[];
  receipts: Receipt[];
  uploads?: { key: string; fingerprint: string; asset: API.AssetSummary }[];
  preferences?: API.PreferencesView;
};

export async function previewUploadFingerprint(file: File) {
  const digest = await crypto.subtle.digest(
    "SHA-256",
    await file.arrayBuffer(),
  );
  return canonical({
    name: file.name,
    type: file.type,
    size: file.size,
    sha256: Array.from(new Uint8Array(digest), (x) =>
      x.toString(16).padStart(2, "0"),
    ).join(""),
  });
}

export function previewSession(): API.EmailSessionView {
  const now = new Date().toISOString(),
    expires = new Date(Date.now() + 7 * 86400000).toISOString();
  return {
    actor: {
      id: previewActor,
      email: "canvas-preview@example.test",
      display_name: "画布演示",
      platform_role: "creator",
      status: "active",
      revision: "1",
      capabilities: [],
      avatar_asset_id: "",
      email_verified_at: now,
    },
    session_id: "f0000000-0000-4000-8000-000000000004",
    csrf_token: "development-preview-only",
    credential_epoch: "1",
    persistent: false,
    last_active_at: now,
    server_time: now,
    idle_expires_at: expires,
    absolute_expires_at: expires,
    session_protocol: "v1",
    upgrade_available: false,
  };
}
export const previewSpace: API.SpaceView = {
  id: previewScope,
  kind: "personal",
  name: "画布演示",
  role: "owner",
  writable: true,
  team_id: "",
};
export function previewProject(): Project {
  return {
    id: projectID,
    canvas_id: null,
    name: "画布交互演示",
    status: "active",
    is_delete: false,
    revision: 1,
    aspect_ratio: "1:1",
    style_key: null,
    update_time: new Date().toISOString(),
  };
}
export function initialPreviewState(count = 3): State {
  const assets: Asset[] = [
    "amber-perfume",
    "cobalt-headphones",
    "moonlight-whale",
  ].map((name, i) => ({
    item: `f0000000-0000-4000-8000-00000000001${i}`,
    asset: {
      id: `f0000000-0000-4000-8000-00000000002${i}`,
      project_id: projectID,
      kind: "image",
      file_name: `${name}.png`,
      width: 1672,
      height: 941,
      revision: 1,
      mime_type: "image/png",
    },
    url: `/design/reference/${name}.png`,
  }));
  const image = {
    ...createMediaNode(assets[0].asset, projectID, 0),
    title: "香氛参考图",
    x: 160,
    y: 200,
    width: 320,
    height: 180,
  };
  const text = {
    ...createResourceNode("text", { x: 570, y: 200 }),
    title: "创作说明",
    config: {
      text: "围绕香氛产品探索光影与材质。\n可拖动、分组、撤销、裁剪和拆图。\n当前全部为 Mock 数据。",
    },
  };
  const draft = {
    ...createDraftNode("image_generation", "1:1", 0),
    title: "图片创作",
    x: 980,
    y: 200,
    input_revision: 1,
  };
  const nodes: Node[] =
    count === 0
      ? []
      : count === 3
        ? [image, text, draft]
        : Array.from({ length: count }, (_, i) => ({
            ...createResourceNode("text", {
              x: (i % 20) * 360,
              y: Math.floor(i / 20) * 260,
            }),
            title: `演示节点 ${i + 1}`,
            width: 280,
            height: 180,
            config: { text: `Mock ${i + 1}` },
          }));
  return {
    canvas: {
      id: crypto.randomUUID(),
      project_id: projectID,
      revision: 1,
      nodes,
      edges: [],
      viewport: { x: 0, y: 0, zoom: count > 3 ? 0.2 : 1 },
    },
    assets,
    receipts: [],
  };
}
function normalizeConfig(config: API.NodeConfig): Node["config"] {
  const value = JSON.parse(JSON.stringify(config)) as Node["config"];
  if (value.draft?.composer)
    value.draft.composer = {
      ...value.draft.composer,
      mode: value.draft.composer.mode ?? null,
      modelVersion: value.draft.composer.modelVersion ?? null,
    };
  return value;
}
export function applyPreviewCommands(
  base: Canvas,
  body: API.CommandsInput,
): PreviewResult {
  if (body.expected_revision !== base.revision)
    throw new RequestError("revision_conflict", 409);
  let next = structuredClone(base);
  for (const command of body.commands ?? []) {
    const patch = (id: string | undefined, change: Partial<Node>) => {
      next.nodes = next.nodes.map((node) =>
        node.id === id ? { ...node, ...change } : node,
      );
    };
    switch (command.type) {
      case "AddNodes":
        next.nodes.push(
          ...(command.nodes ?? []).map(
            (node) =>
              ({
                ...node,
                config: normalizeConfig(node.config ?? {}),
                ...(node.node_action === "generate"
                  ? { input_revision: 1 }
                  : {}),
              }) as Node,
          ),
        );
        break;
      case "MoveNodes":
        for (const move of command.moves ?? [])
          patch(move.id, { x: move.x!, y: move.y! });
        break;
      case "ResizeNodes":
        for (const size of command.sizes ?? [])
          patch(size.id, { width: size.width, height: size.height });
        break;
      case "RenameNodes":
        for (const name of command.names ?? [])
          patch(name.id, { title: name.title! });
        break;
      case "SetNodeParents":
        for (const parent of command.parents ?? [])
          patch(parent.id, { parent_id: parent.parent_id });
        break;
      case "SetNodeZIndex":
        for (const index of command.z_indices ?? [])
          patch(index.id, { z_index: index.z_index! });
        break;
      case "UpdateNodeConfig":
        patch(command.id, { config: normalizeConfig(command.config ?? {}) });
        break;
      case "DeleteNodes":
        next.nodes = next.nodes.filter(
          (node) => !command.ids?.includes(node.id),
        );
        break;
      case "Connect":
        next.edges.push(...(command.edges ?? []));
        break;
      case "Disconnect":
        next.edges = next.edges.filter(
          (edge) => !command.ids?.includes(edge.id!),
        );
        break;
      case "SetViewport":
        next.viewport = command.viewport as Canvas["viewport"];
        break;
      default:
        throw new RequestError("preview_command_unsupported", 422);
    }
  }
  next.nodes = next.nodes.map((node) => {
    const prior = base.nodes.find((old) => old.id === node.id);
    return isDraft(node)
      ? {
          ...node,
          input_revision:
            prior && isDraft(prior)
              ? prior.input_revision! +
                (sameDraftInput(prior.config.draft, node.config.draft) ? 0 : 1)
              : 1,
        }
      : node;
  });
  next = {
    ...requireCanvas(next, base.project_id, base.id),
    revision: base.revision + 1,
  };
  return { ...next, results: (body.commands ?? []).map(() => ({ ok: true })) };
}
export class CanvasPreviewData {
  state: State;
  project: Project;
  private urls = new Map<string, string>();
  private files = new Map<string, File>();
  private disposed = false;
  constructor(state: State) {
    this.state = state;
    this.project = { ...previewProject(), canvas_id: state.canvas.id };
  }
  static load() {
    return new CanvasPreviewData(initialPreviewState());
  }
  private commit(next: State) {
    this.state = next;
  }
  activate() {
    this.disposed = false;
  }
  dispose() {
    this.disposed = true;
    for (const url of this.urls.values()) URL.revokeObjectURL(url);
    this.urls.clear();
  }
  async reset(count: number) {
    this.dispose();
    this.files.clear();
    this.activate();
    this.commit(initialPreviewState(count));
  }
  async media(asset: Asset) {
    if (asset.url) return new URL(asset.url, location.origin).href;
    if (!this.urls.has(asset.asset.id!)) {
      const file = this.files.get(asset.asset.id!);
      if (!file) throw new RequestError("preview_asset_missing", 404);
      if (this.disposed)
        throw new DOMException("Preview disposed", "AbortError");
      this.urls.set(asset.asset.id!, URL.createObjectURL(file));
    }
    return this.urls.get(asset.asset.id!)!;
  }
  async request(url: string, options: RequestOptions): Promise<unknown> {
    const method = options.method?.toUpperCase() ?? "GET",
      params = options.params ?? {},
      data = options.data;
    if (url === "/api/session") return previewSession();
    if (url === "/api/me/spaces") return { items: [previewSpace] };
    if (url === "/api/me/preferences") {
      const current = this.state.preferences ?? {
        revision: "1",
        theme: "dark",
        prompt_suffix: "",
      };
      if (method === "PATCH") {
        const input = data as API.PreferencesUpdateRequest;
        if (input.expected_revision !== current.revision)
          throw new RequestError("revision_conflict", 409);
        if (!["light", "dark", "system"].includes(input.theme ?? ""))
          throw new RequestError("invalid_request", 422);
        const preferences = {
          ...current,
          theme: input.theme,
          revision: String(Number(current.revision) + 1),
        };
        this.commit({ ...this.state, preferences });
        return preferences;
      }
      return current;
    }
    if (url === "/api/me/avatar") throw new RequestError("not_found", 404);
    if (url === `/api/projects/${projectID}` && method === "GET")
      return this.project;
    if (url === `/api/canvases/${this.state.canvas.id}`)
      return structuredClone(this.state.canvas);
    if (url === `/api/canvases/${this.state.canvas.id}/commands`) {
      const proof = String(
        (options.headers as Record<string, string>)?.["Idempotency-Key"] ?? "",
      );
      if (!proof) throw new RequestError("idempotency_required", 422);
      const body = data as API.CommandsInput,
        encoded = canonical(body),
        prior = this.state.receipts.find((receipt) => receipt.key === proof);
      if (prior) {
        if (prior.body !== encoded)
          throw new RequestError("idempotency_conflict", 409);
        return structuredClone(prior.result);
      }
      const result = applyPreviewCommands(this.state.canvas, body);
      const canvas = requireCanvas(result, projectID, this.state.canvas.id);
      const receipts = [
        ...this.state.receipts.slice(-2),
        { key: proof, body: encoded, result },
      ];
      this.commit({ ...this.state, canvas, receipts });
      return structuredClone(result);
    }
    if (url === "/api/models")
      return {
        items: [
          {
            id: "f0000000-0000-4000-8000-000000000030",
            key: "preview-image",
            display_name: "图片模型 · Mock",
            capability: "image.generate",
            status: "active",
            provider_status: "active",
            credential_present: true,
            input_roles: ["reference_image"],
            current_version: {
              id: "f0000000-0000-4000-8000-000000000031",
              modes: ["text_to_image", "image_to_image"],
              limits: {
                max_outputs: 4,
                roles: {
                  reference_image: {
                    types: ["image"],
                    min_count: 0,
                    max_count: 8,
                  },
                },
              },
              param_schema: [
                {
                  field: "aspect_ratio",
                  label: "画面比例",
                  type: "string",
                  component: "select",
                  enum: ["1:1", "16:9", "9:16", "4:3", "3:4"],
                  default: "1:1",
                },
              ],
            },
          },
        ],
      };
    if (
      /\/operations$/.test(url) ||
      /\/prompt-drafts$/.test(url) ||
      /\/bible\//.test(url)
    )
      return { items: [] };
    if (/\/prompt-submissions\/current$/.test(url)) return { epoch: 0 };
    if (url === `/api/projects/${projectID}/media` && method === "GET")
      return {
        items: this.state.assets
          .filter((item) => !params.kind || item.asset.kind === params.kind)
          .map((item) => item.asset),
        next_cursor: null,
      };
    if (url.endsWith("/media/uploads") && data instanceof FormData) {
      const file = data.get("file");
      if (!(file instanceof File))
        throw new RequestError("invalid_request", 422);
      const proof = String(
        (options.headers as Record<string, string>)?.["Idempotency-Key"] ?? "",
      );
      if (!proof) throw new RequestError("idempotency_required", 422);
      const fingerprint = await previewUploadFingerprint(file);
      const prior = this.state.uploads?.find(
        (receipt) => receipt.key === proof,
      );
      if (prior) {
        if (prior.fingerprint !== fingerprint)
          throw new RequestError("idempotency_conflict", 409);
        return { asset: structuredClone(prior.asset) };
      }
      const id = crypto.randomUUID();
      let width = 320,
        height = 320;
      if (file.type.startsWith("image/")) {
        const bitmap = await createImageBitmap(file);
        width = bitmap.width;
        height = bitmap.height;
        bitmap.close();
      }
      const item: Asset = {
        item: crypto.randomUUID(),
        asset: {
          id,
          project_id: projectID,
          kind: file.type.startsWith("video/")
            ? "video"
            : file.type.startsWith("audio/")
              ? "audio"
              : "image",
          width,
          height,
          file_name: file.name,
          mime_type: file.type,
          byte_size: file.size,
          revision: 1,
        },
      };
      this.files.set(id, file);
      try {
        this.commit({
          ...this.state,
          assets: [...this.state.assets, item],
          uploads: [
            ...(this.state.uploads ?? []),
            { key: proof, fingerprint, asset: item.asset },
          ],
        });
      } catch (error) {
        this.files.delete(id);
        throw error;
      }
      return { asset: item.asset };
    }
    const mediaID = url.match(/\/media\/([^/]+)\/preview$/)?.[1];
    if (mediaID) {
      const item = this.state.assets.find((item) => item.asset.id === mediaID);
      if (!item) throw new RequestError("not_found", 404);
      return {
        asset: item.asset,
        url: await this.media(item),
        expires_at: new Date(Date.now() + 3600000).toISOString(),
      };
    }
    if (url === "/api/media/library") {
      const items = this.state.assets.filter(
        (item) => !params.kind || item.asset.kind === params.kind,
      );
      return {
        scope: { kind: "project", project_id: projectID },
        current_actor_id: previewActor,
        current_org_id: previewScope,
        library_id: previewScope,
        revision: 1,
        page: Number(params.page ?? 1),
        page_size: 120,
        total: items.length,
        folders: [],
        items: items
          .slice(
            (Number(params.page ?? 1) - 1) * 120,
            Number(params.page ?? 1) * 120,
          )
          .map((item) => ({
            id: item.item,
            asset_id: item.asset.id,
            kind: item.asset.kind,
            catalog_state: "active",
            title: item.asset.file_name,
            media: item.asset,
          })),
      };
    }
    const itemID = url.match(
      /\/media\/library\/items\/([^/]+)\/download$/,
    )?.[1];
    if (itemID) {
      const item = this.state.assets.find((item) => item.item === itemID);
      if (!item) throw new RequestError("not_found", 404);
      if (!item.url) return this.files.get(item.asset.id!);
      // eslint-disable-next-line no-restricted-syntax -- Reads a checked-in /design/ fixture, never a business API.
      const response = await fetch(item.url, {
        signal: options.signal as AbortSignal,
      });
      if (!response.ok) throw new Error("示例图片不可用");
      return response.blob();
    }
    throw new RequestError("Mock预览尚未接入此操作", 501);
  }
}
