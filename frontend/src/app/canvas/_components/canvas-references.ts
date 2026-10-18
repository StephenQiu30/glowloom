import {
  emptyPromptDraft,
  validatePromptDraft,
  type PromptDraft,
} from "@/components/generation/prompt-document";
import type { CanvasReferenceSource } from "@/components/generation/reference-source";
import { RequestError } from "@/lib/request";
import {
  createDraftNode,
  createResourceNode,
  isDraft,
  richCanvasDraft,
  type DraftNode,
  type Node,
} from "./editor-state";

export function nodePromptDraft(node: DraftNode): PromptDraft {
  if (node.config.draft.composer)
    return structuredClone(node.config.draft.composer);
  const draft = emptyPromptDraft();
  draft.document.content = node.config.draft.prompt.split("\n").map((text) => ({
    type: "paragraph",
    ...(text ? { content: [{ type: "text" as const, text }] } : {}),
  }));
  draft.params = {
    aspect_ratio: node.config.draft.aspect_ratio,
    output_count: node.config.draft.candidate_count,
    ...(node.config.draft.negative_prompt
      ? { negative_prompt: node.config.draft.negative_prompt }
      : {}),
  };
  return draft;
}

export function referenceSources(nodes: Node[]): CanvasReferenceSource[] {
  return nodes.flatMap((n) => {
    const kind =
      n.node_type === "image_generation"
        ? "image"
        : n.node_type === "video_generation"
          ? "video"
          : n.node_type;
    if (kind !== "image" && kind !== "video" && kind !== "audio") return [];
    return [
      {
        nodeID: n.id,
        title: n.title,
        kind,
        assetID: n.ref_type === "media_asset" ? n.ref_id : undefined,
      },
    ];
  });
}

/** One history transaction contains the new node, its layout edge and any verified input. */
export function adjacentGraph(
  source: Node,
  kind: "text" | "image_generation" | "video_generation",
  side: "left" | "right",
  aspect: string,
  asset?: API.AssetSummary,
  occupied: Node[] = [],
) {
  const node =
    kind === "text"
      ? createResourceNode("text", { x: 0, y: 0 })
      : createDraftNode(kind, aspect, 0);
  node.x = Math.max(
    -1e6,
    Math.min(
      1e6,
      side === "left"
        ? source.x - (node.width ?? 320) - 80
        : source.x + (source.width ?? 320) + 80,
    ),
  );
  node.y = source.y;
  // Use the next free row in the requested column instead of covering an existing source.
  const width = node.width ?? 320,
    height = node.height ?? 320;
  const obstacles = occupied.filter(
    (other) =>
      other.node_type !== "group" &&
      node.x < other.x + (other.width ?? 320) + 24 &&
      node.x + width + 24 > other.x,
  );
  const step = height + 80;
  const candidates = [source.y];
  for (let index = 1; index <= occupied.length + 1; index++)
    candidates.push(source.y + step * index, source.y - step * index);
  const freeY = candidates.find(
    (y) =>
      y >= -1e6 &&
      y <= 1e6 &&
      !obstacles.some(
        (other) =>
          y < other.y + (other.height ?? 320) + 24 && y + height + 24 > other.y,
      ),
  );
  if (freeY === undefined)
    throw new Error("附近没有放置空间，请先调整节点布局。");
  node.y = freeY;
  const edge: API.Edge = {
    id: crypto.randomUUID(),
    edge_type: "annotation",
    source_node_id: side === "left" ? node.id : source.id,
    target_node_id: side === "left" ? source.id : node.id,
  };
  if (asset) {
    if (
      asset.id !== source.ref_id ||
      source.ref_type !== "media_asset" ||
      asset.kind !== source.node_type ||
      !Number.isSafeInteger(asset.revision) ||
      asset.revision! < 1
    )
      throw new RequestError("invalid_receipt");
    if (
      side === "right" &&
      isDraft(node) &&
      (asset.kind === "image" ||
        asset.kind === "video" ||
        asset.kind === "audio")
    ) {
      const draft = nodePromptDraft(node);
      draft.references.push({
        id: edge.id!,
        order: 0,
        bindingGeneration: 1,
        mediaKind: asset.kind,
        role: `reference_${asset.kind}`,
        source: {
          kind: "media",
          sourceId: asset.id!,
          sourceVersion: String(asset.revision),
        },
      });
      draft.labelState.highWater[asset.kind] = 1;
      draft.labelState.allocations[edge.id!] = {
        labelKind: asset.kind,
        ordinal: 1,
      };
      validatePromptDraft(draft);
      node.config = { draft: richCanvasDraft(draft) };
    }
  }
  return { node, edge };
}

/** A projection of actual prompt bindings. Never send these synthetic edges to Connect. */
export function referenceEdges(nodes: Node[], edges: API.Edge[]): API.Edge[] {
  const lines = [...edges];
  const nodeByID = new Map(nodes.map((node) => [node.id, node]));
  const edgeByID = new Map(edges.map((edge) => [edge.id, edge]));
  const mediaNodes = new Map<string, Node>();
  for (const node of nodes) {
    if (
      node.ref_type === "media_asset" &&
      node.ref_id &&
      !mediaNodes.has(`${node.node_type}/${node.ref_id}`)
    )
      mediaNodes.set(`${node.node_type}/${node.ref_id}`, node);
  }
  const pairs = new Map(
    edges.map((edge, index) => [
      `${edge.source_node_id}/${edge.target_node_id}`,
      index,
    ]),
  );
  for (const target of nodes) {
    if (!isDraft(target)) continue;
    for (const ref of target.config.draft.composer?.references ?? []) {
      if (ref.source.kind !== "media") continue;
      const assetID = ref.source.sourceId;
      const bindingEdge = edgeByID.get(ref.id);
      const connected =
        bindingEdge?.target_node_id === target.id
          ? nodeByID.get(bindingEdge.source_node_id ?? "")
          : undefined;
      const source =
        connected?.ref_type === "media_asset" &&
        connected.ref_id === assetID &&
        connected.node_type === ref.mediaKind
          ? connected
          : mediaNodes.get(`${ref.mediaKind}/${assetID}`);
      if (!source) continue;
      const pair = `${source.id}/${target.id}`;
      const prior = pairs.get(pair);
      if (prior !== undefined) {
        if (lines[prior].edge_type === "annotation")
          lines[prior] = { ...lines[prior], edge_type: "prompt_reference" };
      } else {
        pairs.set(pair, lines.length);
        lines.push({
          id: `reference/${target.id}/${ref.id}`,
          edge_type: "prompt_reference",
          source_node_id: source.id,
          target_node_id: target.id,
        });
      }
    }
  }
  return lines;
}
