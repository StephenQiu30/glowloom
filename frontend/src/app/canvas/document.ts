import {
  validatePromptDraft,
  type PromptDraft,
} from "@/components/generation/prompt-document";
import { canonical, draftRequest } from "@/components/generation/draft-save";
import { RequestError } from "@/lib/request";
import { isProjectID } from "@/components/projects/project";

export type Draft = {
  composer?: PromptDraft;
  prompt: string;
  negative_prompt: string;
  aspect_ratio: string;
  candidate_count: number;
};
export type Node = Omit<API.Node, "config"> & {
  id: string;
  title: string;
  node_type: string;
  node_action: string;
  config: Omit<API.NodeConfig, "draft"> & { draft?: Draft };
  x: number;
  y: number;
  z_index: number;
};
export type DraftNode = Node & {
  node_type: "image_generation" | "video_generation";
  config: { draft: Draft };
};
export type Canvas = {
  id: string;
  project_id: string;
  revision: number;
  read_only_reason?: string;
  nodes: Node[];
  edges: API.Edge[];
  viewport?: CanvasViewport;
};
export type CanvasViewport = { x: number; y: number; zoom: number };
export const defaultViewport: CanvasViewport = { x: 0, y: 0, zoom: 1 };
export type GraphOptions = { edges?: API.Edge[]; viewport?: CanvasViewport };
export function isDraft(node: Node): node is DraftNode {
  return (
    node.node_type === "image_generation" ||
    node.node_type === "video_generation"
  );
}
function position(value: unknown): value is number {
  return (
    typeof value === "number" &&
    Number.isFinite(value) &&
    Math.abs(value) <= 1e6
  );
}
function dimension(value: unknown, max = 2000): boolean {
  return (
    value === undefined ||
    (typeof value === "number" &&
      Number.isFinite(value) &&
      value >= 40 &&
      value <= max)
  );
}
function validDraft(value: unknown): value is Draft {
  const d = value as Partial<Draft> | null;
  if (d?.composer) {
    try {
      validatePromptDraft(d.composer);
      return (
        d.prompt === "" &&
        d.negative_prompt === "" &&
        d.aspect_ratio === "" &&
        d.candidate_count === 0
      );
    } catch {
      return false;
    }
  }

  return (
    !!d &&
    typeof d.prompt === "string" &&
    Array.from(d.prompt).length <= 10000 &&
    typeof d.negative_prompt === "string" &&
    Array.from(d.negative_prompt).length <= 10000 &&
    ["1:1", "9:16", "16:9"].includes(d.aspect_ratio ?? "") &&
    Number.isInteger(d.candidate_count) &&
    d.candidate_count! >= 1 &&
    d.candidate_count! <= 8
  );
}
export function requireCanvas(
  value: unknown,
  projectID: string,
  canvasID: string,
): Canvas {
  const c = value as Partial<Canvas> | null;
  if (
    !c ||
    c.id !== canvasID ||
    c.project_id !== projectID ||
    !isProjectID(c.id) ||
    !Number.isSafeInteger(c.revision) ||
    c.revision! < 1 ||
    !Array.isArray(c.nodes) ||
    c.nodes.length > 2000 ||
    !Array.isArray(c.edges) ||
    c.edges.length > 4000
  )
    throw new RequestError("invalid_receipt");
  const ids = new Set<string>();
  for (const n of c.nodes) {
    if (
      !n ||
      !isProjectID(n.id) ||
      ids.has(n.id) ||
      typeof n.title !== "string" ||
      !n.title.trim() ||
      Array.from(n.title).length > 128 ||
      typeof n.node_type !== "string" ||
      typeof n.node_action !== "string" ||
      !n.config ||
      !position(n.x) ||
      !position(n.y) ||
      !dimension(n.width, n.node_type === "group" ? 100000 : 2000) ||
      !dimension(n.height, n.node_type === "group" ? 100000 : 2000) ||
      !Number.isInteger(n.z_index)
    )
      throw new RequestError("invalid_receipt");
    if (
      !c.read_only_reason &&
      isDraft(n) &&
      (n.node_action !== "generate" ||
        !!n.ref_id ||
        !!n.ref_type ||
        !!n.last_operation_id ||
        !validDraft(n.config.draft) ||
        !Number.isSafeInteger(n.input_revision) ||
        n.input_revision! < 1)
    )
      throw new RequestError("invalid_receipt");
    ids.add(n.id);
  }
  if (
    c.viewport &&
    (!position(c.viewport.x) ||
      !position(c.viewport.y) ||
      !Number.isFinite(c.viewport.zoom) ||
      c.viewport.zoom < 0.05 ||
      c.viewport.zoom > 8)
  )
    throw new RequestError("invalid_receipt");
  const edgeIDs = new Set<string>();
  for (const edge of c.edges) {
    if (
      !edge ||
      !isProjectID(edge.id) ||
      edgeIDs.has(edge.id) ||
      !edge.source_node_id ||
      !ids.has(edge.source_node_id) ||
      !edge.target_node_id ||
      !ids.has(edge.target_node_id) ||
      typeof edge.edge_type !== "string"
    )
      throw new RequestError("invalid_receipt");
    edgeIDs.add(edge.id);
  }
  // Protected history is an opaque read snapshot; never normalize unknown config.
  return {
    ...c,
    nodes: c.read_only_reason ? c.nodes : c.nodes.map(normalizeNode),
  } as Canvas;
}
export function createDraftNode(
  kind: DraftNode["node_type"],
  ratio: string,
  index: number,
): DraftNode {
  return {
    id: crypto.randomUUID(),
    title: kind === "image_generation" ? "图片草稿" : "视频草稿",
    node_type: kind,
    node_action: "generate",
    config: {
      draft: {
        prompt: "",
        negative_prompt: "",
        aspect_ratio: ratio,
        candidate_count: 1,
      },
    },
    x: 80 + (index % 3) * 340,
    y: 80 + Math.floor(index / 3) * 240,
    z_index: 0,
    width: 320,
    height: 320,
  };
}
function sameDraft(a: Draft, b: Draft) {
  if (a.composer || b.composer) return canonical(a) === canonical(b);

  return (
    a.prompt === b.prompt &&
    a.negative_prompt === b.negative_prompt &&
    a.aspect_ratio === b.aspect_ratio &&
    a.candidate_count === b.candidate_count
  );
}
export function isMediaNode(n: Node): boolean {
  return (
    ["image", "video", "audio", "model"].includes(n.node_type) &&
    n.node_action === "resource" &&
    n.ref_type === "media_asset" &&
    isProjectID(n.ref_id) &&
    Object.keys(n.config).length === 0
  );
}
export function isResourceNode(n: Node): boolean {
  if (isMediaNode(n)) return true;
  if (n.node_action !== "resource" || n.ref_type || n.ref_id) return false;
  const keys = Object.keys(n.config).filter(
    (key) => n.config[key as keyof Node["config"]] !== undefined,
  );
  return n.node_type === "text"
    ? keys.length === 1 &&
        keys[0] === "text" &&
        typeof n.config.text === "string" &&
        Array.from(n.config.text).length <= 10000
    : n.node_type === "group" &&
        keys.length === 1 &&
        keys[0] === "collapsed" &&
        typeof n.config.collapsed === "boolean";
}
export function isToolNode(n: Node): boolean {
  if (n.node_action !== "tool" || n.ref_id || n.ref_type) return false;
  const key =
    n.node_type === "timeline"
      ? "timeline"
      : n.node_type === "director"
        ? "director"
        : n.node_type === "batch_table"
          ? "batch_table"
          : n.node_type === "generation"
            ? "generation"
            : undefined;
  return (
    !!key &&
    Object.keys(n.config).filter(
      (k) => n.config[k as keyof API.NodeConfig] !== undefined,
    ).length === 1 &&
    n.config[key]?.version === 1
  );
}
export function isEditableNode(n: Node): boolean {
  return (
    !n.last_operation_id && (isDraft(n) || isResourceNode(n) || isToolNode(n))
  );
}
export function createResourceNode(
  kind: "text" | "group",
  point: { x: number; y: number },
): Node {
  return {
    id: crypto.randomUUID(),
    title: kind === "text" ? "文本" : "分组",
    node_type: kind,
    node_action: "resource",
    config: kind === "text" ? { text: "" } : { collapsed: false },
    x: point.x,
    y: point.y,
    width: kind === "group" ? 640 : 320,
    height: kind === "group" ? 400 : 320,
    z_index: kind === "group" ? -1 : 0,
  };
}
export function isAnnotation(edge: API.Edge): boolean {
  return (
    edge.edge_type === "annotation" && !edge.role && edge.binding === undefined
  );
}
export function createMediaNode(
  asset: API.AssetSummary,
  projectID: string,
  index: number,
): Node {
  if (
    !isProjectID(asset.id) ||
    asset.project_id !== projectID ||
    !["image", "video", "audio", "model"].includes(asset.kind ?? "")
  )
    throw new RequestError("invalid_receipt");
  const ratio =
    asset.width && asset.height && asset.width > 0 && asset.height > 0
      ? asset.height / asset.width
      : 9 / 16;
  return {
    id: crypto.randomUUID(),
    title: Array.from(asset.file_name?.trim() || "项目素材")
      .slice(0, 128)
      .join(""),
    node_type: asset.kind!,
    node_action: "resource",
    ref_type: "media_asset",
    ref_id: asset.id,
    config: {},
    x: 80 + (index % 3) * 340,
    y: 80 + Math.floor(index / 3) * 260,
    z_index: 0,
    width: 320,
    height: Math.max(40, Math.min(2000, Math.round(320 * ratio))),
  };
}
export function duplicateNode(n: Node): Node {
  if (!isEditableNode(n)) throw new RequestError("invalid_request");
  const copy = structuredClone(n);
  copy.id = crypto.randomUUID();
  copy.title = Array.from(n.title).slice(0, 125).join("") + " 副本";
  copy.x = Math.min(1e6, n.x + 32);
  copy.y = Math.min(1e6, n.y + 32);
  if (isDraft(copy) && copy.config.draft.composer) {
    for (const ref of copy.config.draft.composer.references) {
      if (ref.source.kind === "pending_upload") {
        ref.bindingGeneration++;
        ref.source = {
          kind: "pending_upload",
          clientUploadId: crypto.randomUUID(),
          uploadAttemptId: crypto.randomUUID(),
        };
      }
    }
  }
  delete copy.input_revision;
  return copy;
}
export function sameEditable(a: Node, b: Node): boolean {
  const { input_revision: aRevision, ...left } = normalizeNode(a);
  const { input_revision: bRevision, ...right } = normalizeNode(b);
  void aRevision;
  void bRevision;
  return canonical(left) === canonical(right);
}
export function commandBatch(
  base: Canvas,
  nodes: Node[],
  options: GraphOptions = {},
): API.CommandsInput {
  if (base.read_only_reason)
    throw new RequestError("canvas_graph_read_only", 422);
  if (nodes.length > 2000) throw new RequestError("batch_too_large");
  const byID = new Map(base.nodes.map((n) => [n.id, n]));
  const commands: API.Command[] = [];
  const added: API.Node[] = [],
    moves: API.Move[] = [],
    names: API.Name[] = [],
    sizes: API.Size[] = [],
    parents: API.Parent[] = [],
    indices: API.ZIndex[] = [];
  const ids = new Set<string>();
  const target = new Map(nodes.map((n) => [n.id, n]));
  for (const n of nodes) {
    if (ids.has(n.id)) throw new RequestError("invalid_request");
    ids.add(n.id);
    const prior = byID.get(n.id);
    if (prior && !isEditableNode(prior)) {
      if (!sameEditable(prior, n)) throw new RequestError("invalid_request");
      continue;
    }
    if (
      !isEditableNode(n) ||
      !isProjectID(n.id) ||
      !n.title.trim() ||
      Array.from(n.title).length > 128 ||
      !position(n.x) ||
      !position(n.y) ||
      !dimension(n.width, n.node_type === "group" ? 100000 : 2000) ||
      !dimension(n.height, n.node_type === "group" ? 100000 : 2000) ||
      !Number.isInteger(n.z_index) ||
      (isDraft(n) && !validDraft(n.config.draft))
    )
      throw new RequestError("invalid_request");
    if (n.parent_id) {
      const seen = new Set([n.id]);
      let parent = target.get(n.parent_id);
      while (parent) {
        if (
          seen.has(parent.id) ||
          parent.node_type !== "group" ||
          parent.node_action !== "resource"
        )
          throw new RequestError("invalid_request");
        seen.add(parent.id);
        parent = parent.parent_id ? target.get(parent.parent_id) : undefined;
      }
      if (!target.has(n.parent_id)) throw new RequestError("invalid_request");
    }
    if (!prior) {
      const write = { ...n };
      delete write.input_revision;
      added.push({ ...write, config: apiConfig(write.config) });
      continue;
    }
    if (
      n.node_type !== prior.node_type ||
      n.node_action !== prior.node_action ||
      n.ref_id !== prior.ref_id ||
      n.ref_type !== prior.ref_type ||
      n.last_operation_id !== prior.last_operation_id
    )
      throw new RequestError("invalid_request");
    if (
      isDraft(prior) &&
      prior.config.draft.composer &&
      (!isDraft(n) || !n.config.draft.composer)
    )
      throw new RequestError("invalid_request");
    if (n.x !== prior.x || n.y !== prior.y)
      moves.push({ id: n.id, x: n.x, y: n.y });
    if (n.title !== prior.title) names.push({ id: n.id, title: n.title });
    if (n.width !== prior.width || n.height !== prior.height)
      sizes.push({ id: n.id, width: n.width ?? 288, height: n.height ?? 200 });
    if (n.parent_id !== prior.parent_id)
      parents.push({ id: n.id, parent_id: n.parent_id ?? undefined });
    if (n.z_index !== prior.z_index)
      indices.push({ id: n.id, z_index: n.z_index });
    if (canonical(n.config) !== canonical(prior.config))
      commands.push({
        type: "UpdateNodeConfig",
        id: n.id,
        config: apiConfig(n.config),
      });
  }
  const removed = base.nodes.filter((n) => !ids.has(n.id));
  if (removed.some((n) => !isEditableNode(n) || isDraft(n)))
    throw new RequestError("invalid_request");
  if (sizes.length) commands.unshift({ type: "ResizeNodes", sizes });
  if (names.length) commands.unshift({ type: "RenameNodes", names });
  if (moves.length) commands.unshift({ type: "MoveNodes", moves });
  if (added.length) commands.unshift({ type: "AddNodes", nodes: added });
  if (parents.length) commands.push({ type: "SetNodeParents", parents });
  if (indices.length)
    commands.push({ type: "SetNodeZIndex", z_indices: indices });
  const edges = options.edges ?? base.edges;
  if (edges.length > 4000) throw new RequestError("batch_too_large");
  const oldEdges = new Map(base.edges.map((e) => [e.id, e]));
  const newEdges = new Map(edges.map((e) => [e.id, e]));
  if (newEdges.size !== edges.length) throw new RequestError("invalid_request");
  const disconnected: string[] = [],
    connected: API.Edge[] = [];
  for (const edge of base.edges) {
    if (newEdges.has(edge.id)) {
      if (canonical(newEdges.get(edge.id)) !== canonical(edge))
        throw new RequestError("invalid_request");
    } else {
      if (!isAnnotation(edge)) throw new RequestError("invalid_request");
      disconnected.push(edge.id!);
    }
  }
  for (const edge of edges) {
    if (oldEdges.has(edge.id)) continue;
    const source = target.get(edge.source_node_id ?? ""),
      dest = target.get(edge.target_node_id ?? "");
    if (
      !isProjectID(edge.id) ||
      !isAnnotation(edge) ||
      !source ||
      !dest ||
      source === dest ||
      !isEditableNode(source) ||
      !isEditableNode(dest)
    )
      throw new RequestError("invalid_request");
    connected.push(edge);
  }
  for (let i = 0; i < disconnected.length; i += 2000)
    commands.push({ type: "Disconnect", ids: disconnected.slice(i, i + 2000) });
  for (let i = 0; i < connected.length; i += 2000)
    commands.push({ type: "Connect", edges: connected.slice(i, i + 2000) });
  for (let i = 0; i < removed.length; i += 2000)
    commands.push({
      type: "DeleteNodes",
      ids: removed.slice(i, i + 2000).map((n) => n.id),
    });
  if (
    options.viewport &&
    canonical(options.viewport) !== canonical(base.viewport ?? defaultViewport)
  ) {
    const v = options.viewport;
    if (
      !position(v.x) ||
      !position(v.y) ||
      !Number.isFinite(v.zoom) ||
      v.zoom < 0.05 ||
      v.zoom > 8
    )
      throw new RequestError("invalid_request");
    commands.push({ type: "SetViewport", viewport: v });
  }
  if (commands.length > 100) throw new RequestError("batch_too_large");
  return { expected_revision: base.revision, commands };
}
export function requireSaveResult(
  value: unknown,
  base: Canvas,
  frozen: Node[],
  body: API.CommandsInput,
): Canvas {
  const c = requireCanvas(value, base.project_id, base.id);
  const results = (value as API.Result).results;
  if (
    c.revision !== base.revision + 1 ||
    !Array.isArray(results) ||
    results.length !== body.commands?.length ||
    results.some((r) => r.ok !== true) ||
    c.nodes.length !== frozen.length
  )
    throw new RequestError("invalid_receipt");
  const nodes = new Map(c.nodes.map((n) => [n.id, n]));
  const previous = new Map(base.nodes.map((n) => [n.id, n]));
  for (const n of frozen) {
    const saved = nodes.get(n.id);
    if (!saved || !sameEditable(n, saved))
      throw new RequestError("invalid_receipt");
    if (isDraft(n)) {
      const prior = previous.get(n.id);
      const expectedInput =
        prior && isDraft(prior)
          ? prior.input_revision! +
            (sameDraftInput(prior.config.draft, n.config.draft) ? 0 : 1)
          : 1;
      if (saved.input_revision !== expectedInput)
        throw new RequestError("invalid_receipt");
    }
  }
  let expectedEdges = base.edges;
  let expectedViewport = base.viewport ?? defaultViewport;
  for (const command of body.commands ?? []) {
    if (command.type === "Disconnect")
      expectedEdges = expectedEdges.filter(
        (edge) => !command.ids?.includes(edge.id!),
      );
    if (command.type === "Connect")
      expectedEdges = [...expectedEdges, ...(command.edges ?? [])];
    if (command.type === "SetViewport")
      expectedViewport = command.viewport as CanvasViewport;
  }
  const sorted = (edges: API.Edge[]) =>
    [...edges].sort((a, b) => a.id!.localeCompare(b.id!));
  if (
    canonical(sorted(c.edges)) !== canonical(sorted(expectedEdges)) ||
    canonical(c.viewport ?? defaultViewport) !== canonical(expectedViewport)
  )
    throw new RequestError("invalid_receipt");
  return c;
}

export function sameDraftInput(a: Draft, b: Draft) {
  if (!a.composer || !b.composer) return sameDraft(a, b);
  const intent = (d: PromptDraft) => ({
    ...d,
    labelState: {
      allocations: Object.fromEntries(
        d.references.map((r) => [r.id, d.labelState.allocations[r.id]]),
      ),
    },
  });
  return canonical(intent(a.composer)) === canonical(intent(b.composer));
}
export function richCanvasDraft(draft: PromptDraft): Draft {
  return {
    composer: draft,
    prompt: "",
    negative_prompt: "",
    aspect_ratio: "",
    candidate_count: 0,
  };
}

function apiConfig(config: Node["config"]): API.NodeConfig {
  return {
    ...config,
    draft: config.draft
      ? {
          ...config.draft,
          composer: config.draft.composer
            ? draftRequest(config.draft.composer)
            : undefined,
        }
      : undefined,
  };
}

export function removeCanvasNode(nodes: Node[], id: string): Node[] {
  const removed = nodes.find((n) => n.id === id);
  if (!removed || !isEditableNode(removed))
    throw new RequestError("invalid_request");
  return nodes
    .filter((n) => n.id !== id)
    .map((n) => {
      const config = { ...n.config };
      if (config.batch_table)
        config.batch_table = {
          ...config.batch_table,
          rows: config.batch_table.rows?.map((row) => ({
            ...row,
            input_node_ids: row.input_node_ids?.map((source) =>
              source === id ? null : source,
            ) as typeof row.input_node_ids,
          })),
        };
      if (config.timeline)
        config.timeline = {
          ...config.timeline,
          clips: config.timeline.clips?.map((c) =>
            c.node_id === id
              ? { ...c, node_id: undefined, asset_id: removed.ref_id }
              : c,
          ),
        };
      if (config.director)
        config.director = {
          ...config.director,
          objects: config.director.objects?.map((o) =>
            o.source_node_id === id
              ? { ...o, source_node_id: undefined, asset_id: removed.ref_id }
              : o,
          ),
        };
      return {
        ...n,
        config,
        parent_id: n.parent_id === id ? undefined : n.parent_id,
      };
    });
}

// Older stored documents and idempotent receipts encode absent timeline references as null.
function normalizeNode(node: Node): Node {
  // Go omits an empty optional screenshot list in both live and idempotent receipts.
  if (node.config.director) {
    node = {
      ...node,
      config: {
        ...node.config,
        director: {
          ...node.config.director,
          shots: node.config.director.shots?.map((shot) => {
            const clean = { ...shot };
            if (clean.screenshots?.length === 0) delete clean.screenshots;
            return clean;
          }),
        },
      },
    };
  }
  if (!node.config.timeline) return node;
  return {
    ...node,
    config: {
      ...node.config,
      timeline: {
        ...node.config.timeline,
        clips: node.config.timeline.clips?.map((clip) => {
          const clean = { ...clip };
          if (clean.node_id === null) delete clean.node_id;
          if (clean.asset_id === null) delete clean.asset_id;
          if (clean.source_duration_ms === null)
            delete clean.source_duration_ms;
          if (clean.crop === null) delete clean.crop;
          return clean;
        }),
      },
    },
  };
}
