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
  nodes: Node[];
  edges: API.Edge[];
};
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
  return c as Canvas;
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
export function isEditableNode(n: Node): boolean {
  return !n.last_operation_id && (isDraft(n) || isMediaNode(n));
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
      : 200 / 288;
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
    width: 288,
    height: Math.max(40, Math.min(2000, Math.round(288 * ratio))),
  };
}
export function duplicateNode(n: Node): Node {
  if (!isEditableNode(n)) throw new RequestError("invalid_request");
  const copy = structuredClone(n);
  copy.id = crypto.randomUUID();
  copy.title = Array.from(n.title).slice(0, 125).join("") + " 副本";
  copy.x = Math.min(1e6, n.x + 32);
  copy.y = Math.min(1e6, n.y + 32);
  delete copy.input_revision;
  return copy;
}
export function sameEditable(a: Node, b: Node): boolean {
  if (isMediaNode(a) && isMediaNode(b))
    return (
      a.id === b.id &&
      a.node_type === b.node_type &&
      a.ref_id === b.ref_id &&
      a.title === b.title &&
      a.x === b.x &&
      a.y === b.y &&
      a.z_index === b.z_index &&
      a.width === b.width &&
      a.height === b.height &&
      a.parent_id === b.parent_id
    );
  if (!isDraft(a) || !isDraft(b))
    return JSON.stringify(a) === JSON.stringify(b);
  return (
    a.id === b.id &&
    a.node_type === b.node_type &&
    a.title === b.title &&
    a.x === b.x &&
    a.y === b.y &&
    a.width === b.width &&
    a.height === b.height &&
    sameDraft(a.config.draft, b.config.draft)
  );
}
export function commandBatch(base: Canvas, nodes: Node[]): API.CommandsInput {
  const byID = new Map(base.nodes.map((n) => [n.id, n]));
  const commands: API.Command[] = [];
  const added: API.Node[] = [],
    moves: API.Move[] = [],
    names: API.Name[] = [],
    sizes: API.Size[] = [];
  const ids = new Set<string>();
  for (const n of nodes) {
    if (ids.has(n.id)) throw new RequestError("invalid_request");
    ids.add(n.id);
    const prior = byID.get(n.id);
    if (
      prior &&
      isMediaNode(n) &&
      isMediaNode(prior) &&
      isEditableNode(prior)
    ) {
      if (
        n.ref_id !== prior.ref_id ||
        n.node_type !== prior.node_type ||
        n.parent_id !== prior.parent_id ||
        n.z_index !== prior.z_index ||
        n.last_operation_id !== prior.last_operation_id ||
        !n.title.trim() ||
        Array.from(n.title).length > 128 ||
        !position(n.x) ||
        !position(n.y) ||
        !dimension(n.width) ||
        !dimension(n.height)
      )
        throw new RequestError("invalid_request");
      if (prior.x !== n.x || prior.y !== n.y)
        moves.push({ id: n.id, x: n.x, y: n.y });
      if (prior.width !== n.width || prior.height !== n.height)
        sizes.push({
          id: n.id,
          width: n.width ?? 288,
          height: n.height ?? 200,
        });
      if (prior.title !== n.title) names.push({ id: n.id, title: n.title });
      continue;
    }
    if (!isDraft(n)) {
      if (
        !prior &&
        isMediaNode(n) &&
        isEditableNode(n) &&
        isProjectID(n.id) &&
        n.title.trim() &&
        Array.from(n.title).length <= 128 &&
        position(n.x) &&
        position(n.y) &&
        dimension(n.width) &&
        dimension(n.height) &&
        Number.isInteger(n.z_index)
      ) {
        const write = { ...n };
        delete write.input_revision;
        added.push({ ...write, config: apiConfig(write.config) });
        continue;
      }
      if (!prior || !sameEditable(prior, n))
        throw new RequestError("invalid_request");
      continue;
    }
    if (
      !validDraft(n.config.draft) ||
      !position(n.x) ||
      !position(n.y) ||
      !dimension(n.width) ||
      !dimension(n.height) ||
      !n.title.trim() ||
      Array.from(n.title).length > 128
    )
      throw new RequestError("invalid_request");
    if (!prior) {
      const write = { ...n };
      delete write.input_revision;
      added.push({ ...write, config: apiConfig(write.config) });
      continue;
    }
    if (!isDraft(prior) || prior.node_type !== n.node_type)
      throw new RequestError("invalid_request");
    if (prior.x !== n.x || prior.y !== n.y)
      moves.push({ id: n.id, x: n.x, y: n.y });
    if (prior.width !== n.width || prior.height !== n.height)
      sizes.push({ id: n.id, width: n.width ?? 288, height: n.height ?? 200 });
    if (prior.title !== n.title) names.push({ id: n.id, title: n.title });
    if (!sameDraft(prior.config.draft, n.config.draft))
      commands.push({
        type: "UpdateNodeConfig",
        id: n.id,
        config: apiConfig(n.config),
      });
  }
  if (base.nodes.some((n) => !ids.has(n.id)))
    throw new RequestError("invalid_request");
  if (sizes.length) commands.unshift({ type: "ResizeNodes", sizes });
  if (names.length) commands.unshift({ type: "RenameNodes", names });
  if (moves.length) commands.unshift({ type: "MoveNodes", moves });
  if (added.length) commands.unshift({ type: "AddNodes", nodes: added });
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
  return c;
}

function sameDraftInput(a: Draft, b: Draft) {
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
