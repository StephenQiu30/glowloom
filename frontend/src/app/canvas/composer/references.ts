import {
  emptyPromptDraft,
  validatePromptDraft,
  type PromptDraft,
} from "@/components/generation/prompt-document";
import type { CanvasReferenceSource } from "@/components/generation/reference-source";
import { RequestError } from "@/lib/request";
import { placeNode } from "../node/placement";
import {
  createDraftNode,
  createMediaNode,
  createResourceNode,
  isDraft,
  isAnnotation,
  richCanvasDraft,
  type DraftNode,
  type Node,
} from "../document";

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

export function connectedReferenceSources(
  nodes: Node[],
  edges: API.Edge[],
  ownerID: string,
): (CanvasReferenceSource & { edgeID: string })[] {
  const sources = new Map(
    referenceSources(nodes).map((source) => [source.nodeID, source]),
  );
  const seen = new Set<string>();
  return edges.flatMap((edge) => {
    if (!isAnnotation(edge) || !edge.id) return [];
    const otherID =
      edge.target_node_id === ownerID
        ? edge.source_node_id
        : edge.source_node_id === ownerID
          ? edge.target_node_id
          : undefined;
    const source =
      otherID && otherID !== ownerID ? sources.get(otherID) : undefined;
    if (!source || seen.has(source.nodeID)) return [];
    seen.add(source.nodeID);
    return [{ ...source, edgeID: edge.id }];
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
  const neighbors = new Map<string, Node[]>();
  for (const edge of edges) {
    if (!isAnnotation(edge)) continue;
    const source = nodeByID.get(edge.source_node_id ?? "");
    const target = nodeByID.get(edge.target_node_id ?? "");
    if (!source || !target) continue;
    for (const [owner, other] of [
      [source, target],
      [target, source],
    ]) {
      const list = neighbors.get(owner.id) ?? [];
      list.push(other);
      neighbors.set(owner.id, list);
    }
  }
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
          : bindingEdge?.source_node_id === target.id &&
              isAnnotation(bindingEdge)
            ? nodeByID.get(bindingEdge.target_node_id ?? "")
            : undefined;
      const source =
        connected?.ref_type === "media_asset" &&
        connected.ref_id === assetID &&
        connected.node_type === ref.mediaKind
          ? connected
          : (neighbors
              .get(target.id)
              ?.find(
                (node) =>
                  node.ref_type === "media_asset" &&
                  node.ref_id === assetID &&
                  node.node_type === ref.mediaKind,
              ) ?? mediaNodes.get(`${ref.mediaKind}/${assetID}`));
      if (!source) continue;
      const pair = `${source.id}/${target.id}`;
      const prior = pairs.get(pair) ?? pairs.get(`${target.id}/${source.id}`);
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

/** Reuse media resources when a library/upload binding enters a canvas draft. */
export function materializeReferences(
  nodes: Node[],
  edges: API.Edge[],
  ownerID: string,
  draft: PromptDraft,
  projectID: string,
) {
  const owner = nodes.find((n) => n.id === ownerID);
  if (!owner || !isDraft(owner)) throw new Error("创作节点已不可编辑。");
  const next = [...nodes];
  const lines = [...edges];
  for (const ref of draft.references) {
    if (ref.source.kind !== "media") continue;
    const id = ref.source.sourceId;
    let resource = next.find(
      (n) =>
        n.ref_type === "media_asset" &&
        n.ref_id === id &&
        n.node_type === ref.mediaKind,
    );
    if (!resource) {
      if (next.length >= 2000) throw new Error("画布节点已达到上限。");
      resource = createMediaNode(
        {
          id,
          project_id: projectID,
          kind: ref.mediaKind,
          revision: Number(ref.source.sourceVersion),
        },
        projectID,
        next.length,
      );
      resource.x = owner.x - (resource.width ?? 320) - 96;
      resource.y = owner.y;
      resource = placeNode(resource, next);
      next.push(resource);
    }
    if (
      !lines.some(
        (e) =>
          (e.source_node_id === resource!.id && e.target_node_id === ownerID) ||
          (e.target_node_id === resource!.id && e.source_node_id === ownerID),
      )
    ) {
      if (lines.length >= 4000) throw new Error("画布连线已达到上限。");
      lines.push({
        id: crypto.randomUUID(),
        edge_type: "annotation",
        source_node_id: resource.id,
        target_node_id: ownerID,
      });
    }
  }
  return {
    nodes: next.map((n) =>
      n.id === ownerID
        ? { ...owner, config: { draft: richCanvasDraft(draft) } }
        : n,
    ),
    edges: lines,
  };
}
