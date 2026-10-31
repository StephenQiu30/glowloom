import { canvasInputPort } from "../constants/canvas-input-ports";
import { imageReferenceLabel } from "@/components/canvas/runtime/lib/generation/image-reference-prompt";
import { getNodeDefinition } from "@/components/canvas/lib/node-registry";
import {
  getDataUrlByteSize,
  readImageMeta,
} from "@/components/canvas/runtime/lib/media/image-utils";
import { imageToDataUrl } from "@/components/canvas/runtime/lib/media/image-storage";
import {
  CanvasNodeType,
  type CanvasConnection,
  type CanvasNodeData,
} from "@/components/canvas/types/canvas";

export type CanvasResourceKind = "image" | "video" | "audio" | "text";

export type CanvasResourceReference = {
  id: string;
  nodeId: string;
  kind: CanvasResourceKind;
  label: string;
  title: string;
  previewUrl?: string;
  file?: CanvasContract.CanvasFileReferenceDTO;
  text?: string;
  active: boolean;
};

/** UI references identify source nodes, while document edges may bind several files or ports. */
export function buildConnectedNodeMap(
  nodes: CanvasNodeData[],
  connections: CanvasConnection[],
) {
  const byId = new Map(nodes.map((node) => [node.id, node]));
  const sourcesByTarget = new Map<string, Map<string, CanvasNodeData>>();
  const ordinalsByTarget = new Map<string, Map<string, Map<string, number>>>();
  const ordered = connections.toSorted((left, right) => {
    const a = left.documentData;
    const b = right.documentData;
    if (!a?.toPortId || !b?.toPortId) return 0;
    return (
      left.toNodeId.localeCompare(right.toNodeId) ||
      a.toPortId.localeCompare(b.toPortId) ||
      a.order - b.order
    );
  });
  for (const connection of ordered) {
    const source = byId.get(connection.fromNodeId);
    if (!source) continue;
    let sources = sourcesByTarget.get(connection.toNodeId);
    if (!sources) {
      sources = new Map();
      sourcesByTarget.set(connection.toNodeId, sources);
    }
    let projection = source.referenceProjection;
    const edge = connection.documentData;
    const port = edge && canvasInputPort(edge.toPortId);
    if (
      edge &&
      port &&
      (port.source === "text" ||
        port.source === "image" ||
        port.source === "video" ||
        port.source === "audio")
    ) {
      const kind = port.source;
      let byKind = ordinalsByTarget.get(connection.toNodeId);
      if (!byKind) {
        byKind = new Map();
        ordinalsByTarget.set(connection.toNodeId, byKind);
      }
      let ordinals = byKind.get(kind);
      if (!ordinals) {
        ordinals = new Map();
        byKind.set(kind, ordinals);
      }
      const purpose =
        edge.fromPortId === "last_frame" ? "LAST_FRAME" : "CONTENT";
      const refs =
        (edge.binding.kind === "RUN_OUTPUT"
          ? source.metadata?.generationOutput?.files
          : source.documentData?.fileReferences) ?? [];
      const selected = refs.filter(
        (ref) =>
          ref.purpose === purpose &&
          (edge.binding.kind === "FILE_REFERENCE"
            ? ref.referenceKey === edge.binding.referenceKey
            : edge.binding.selection?.mode === "ITEM"
              ? ref.referenceKey === `output-${edge.binding.selection.index}`
              : true),
      );
      // Pending output keeps a provisional slot; resolved files determine actual ordinals.
      const keys = selected.length
        ? selected.map((ref) => `${ref.fileId}:${ref.version}:${edge.toPortId}`)
        : [`node:${source.id}:${edge.toPortId}`];
      for (const key of keys) {
        if (!ordinals.has(key)) ordinals.set(key, ordinals.size);
      }
      projection = {
        kind,
        label: labelForKind(kind, ordinals.get(keys[0])!),
        files: selected,
      };
    }
    const allocation = nodeReferenceAllocation(
      byId.get(connection.toNodeId),
      source.id,
    );
    if (allocation && projection)
      projection = {
        ...projection,
        label: labelForKind(allocation.kind, allocation.ordinal - 1),
      };
    if (!sources.has(source.id))
      sources.set(
        source.id,
        projection ? { ...source, referenceProjection: projection } : source,
      );
  }
  return new Map(
    [...sourcesByTarget].map(([targetId, sources]) => [
      targetId,
      [...sources.values()],
    ]),
  );
}

export function buildNodeMentionReferences(
  node: CanvasNodeData,
  nodes: CanvasNodeData[],
  connections: CanvasConnection[],
) {
  const config = connections.find(
    (edge) =>
      edge.fromNodeId === node.id &&
      nodes.find((n) => n.id === edge.toNodeId)?.type === CanvasNodeType.Config,
  );
  const owner = config
    ? (nodes.find((n) => n.id === config.toNodeId) ?? node)
    : node;
  return labelResourceNodes(
    getMentionResourceNodes(node.id, nodes, connections),
    true,
  ).map((reference) => {
    const allocation = nodeReferenceAllocation(owner, reference.nodeId);
    return allocation
      ? {
          ...reference,
          label: labelForKind(allocation.kind, allocation.ordinal - 1),
        }
      : reference;
  });
}

export function buildCanvasResourceReferences(nodes: CanvasNodeData[]) {
  return labelResourceNodes(nodes, true);
}

export async function resolveCanvasReferenceImages(
  references: CanvasResourceReference[],
  nodes: CanvasNodeData[],
) {
  const nodesById = new Map(nodes.map((node) => [node.id, node]));
  return Promise.all(
    references
      .filter((reference) => reference.kind === "image")
      .map(async (reference) => {
        const node = nodesById.get(reference.nodeId);
        if (!node) throw new Error(`画布素材“${reference.title}”已不存在`);
        const metadata = node.metadata;
        const dataUrl = await imageToDataUrl({
          storageKey: metadata?.storageKey,
          url: reference.previewUrl,
        });
        if (!dataUrl.startsWith("data:image/"))
          throw new Error(`无法读取画布图片“${reference.title}”`);
        const meta =
          metadata?.naturalWidth && metadata.naturalHeight
            ? {
                width: metadata.naturalWidth,
                height: metadata.naturalHeight,
                mimeType:
                  metadata.mimeType ||
                  dataUrl.match(/^data:([^;]+)/)?.[1] ||
                  "image/png",
              }
            : await readImageMeta(dataUrl);
        return {
          id: `canvas:${node.id}`,
          name: reference.title,
          type: metadata?.mimeType || meta.mimeType,
          size: metadata?.bytes || getDataUrlByteSize(dataUrl),
          width: meta.width,
          height: meta.height,
          url: reference.previewUrl || dataUrl,
          dataUrl,
        };
      }),
  );
}

export function getMentionResourceNodes(
  nodeId: string,
  nodes: CanvasNodeData[],
  connections: CanvasConnection[],
) {
  const configInputs = expandGroupResourceNodes(
    getConnectedConfigInputNodes(nodeId, nodes, connections),
    nodes,
  );
  if (configInputs.length) return configInputs;
  const ownInputs = expandGroupResourceNodes(
    getContextInputNodes(nodeId, nodes, connections),
    nodes,
  );
  if (ownInputs.length) return ownInputs;
  return [];
}

export function getGenerationResourceNodes(
  nodeId: string,
  nodes: CanvasNodeData[],
  connections: CanvasConnection[],
) {
  const configInputs = getConnectedConfigInputNodes(nodeId, nodes, connections);
  if (configInputs.length) return configInputs;
  const ownInputs = getContextInputNodes(nodeId, nodes, connections);
  if (ownInputs.length) return ownInputs;
  return [];
}

function getContextInputNodes(
  nodeId: string,
  nodes: CanvasNodeData[],
  connections: CanvasConnection[],
) {
  return (buildConnectedNodeMap(nodes, connections).get(nodeId) ?? []).filter(
    (node) => isCanvasReferenceNode(node, nodes),
  );
}

function getConnectedConfigInputNodes(
  nodeId: string,
  nodes: CanvasNodeData[],
  connections: CanvasConnection[],
) {
  const configConnection = connections.find(
    (connection) =>
      connection.fromNodeId === nodeId &&
      nodes.find((node) => node.id === connection.toNodeId)?.type ===
        CanvasNodeType.Config,
  );
  if (!configConnection) return [];
  return getContextInputNodes(
    configConnection.toNodeId,
    nodes,
    connections,
  ).filter((node) => node.id !== nodeId);
}

function hasGroupResources(node: CanvasNodeData, nodes: CanvasNodeData[]) {
  return (
    node.type === CanvasNodeType.Group &&
    getGroupResourceNodes(node.id, nodes).length > 0
  );
}

export function isCanvasReferenceNode(
  node: CanvasNodeData,
  nodes: CanvasNodeData[],
) {
  return isResourceNode(node) || hasGroupResources(node, nodes);
}

function expandGroupResourceNodes(
  inputNodes: CanvasNodeData[],
  nodes: CanvasNodeData[],
) {
  const resources = inputNodes.flatMap((node) =>
    node.type === CanvasNodeType.Group
      ? getGroupResourceNodes(node.id, nodes)
      : [node],
  );
  return [...new Map(resources.map((node) => [node.id, node])).values()];
}

export function getGroupResourceNodes(
  groupId: string,
  nodes: CanvasNodeData[],
) {
  return nodes.filter(
    (node) => node.metadata?.groupId === groupId && isResourceNode(node),
  );
}

function labelResourceNodes(nodes: CanvasNodeData[], active: boolean) {
  const counts: Record<CanvasResourceKind, number> = {
    image: 0,
    video: 0,
    audio: 0,
    text: 0,
  };
  return nodes.flatMap((node): CanvasResourceReference[] => {
    const kind = node.referenceProjection?.kind ?? resourceKind(node);
    if (!kind) return [];
    const resource = getNodeDefinition(node.type)?.resource?.(node);
    const index = counts[kind]++;
    const label = node.referenceProjection?.label ?? labelForKind(kind, index);
    const file = node.referenceProjection
      ? node.referenceProjection.files[0]
      : (
          node.metadata?.generationOutput?.files ??
          node.documentData?.fileReferences
        )?.find((item) => item.purpose === "CONTENT");
    return [
      {
        id: node.id,
        nodeId: node.id,
        kind,
        label,
        title: node.title || label,
        file,
        previewUrl:
          node.referenceProjection || file
            ? undefined
            : node.metadata?.content || resource?.url,
        text: resourceText(node),
        active,
      },
    ];
  });
}

function labelForKind(kind: CanvasResourceKind, index: number) {
  if (kind === "image") return imageReferenceLabel(index);
  if (kind === "video") return `参考视频${index + 1}`;
  if (kind === "audio") return `参考音频${index + 1}`;
  return `文本${index + 1}`;
}

function isResourceNode(node: CanvasNodeData) {
  return Boolean(resourceKind(node));
}

function resourceText(node: CanvasNodeData): string | undefined {
  if (node.type === CanvasNodeType.Text)
    return node.metadata?.content || node.metadata?.prompt;
  const resource = getNodeDefinition(node.type)?.resource?.(node);
  return resource?.kind === "text" ? resource.text : undefined;
}

function resourceKind(node: CanvasNodeData): CanvasResourceKind | null {
  // Document nodes keep their identity while media previews load or outputs are pending.
  if (
    node.documentData &&
    ["image", "video", "audio", "text"].includes(node.type)
  )
    return node.type as CanvasResourceKind;
  if (node.type === CanvasNodeType.Image && node.metadata?.content)
    return "image";
  if (node.type === CanvasNodeType.Video && node.metadata?.content)
    return "video";
  if (node.type === CanvasNodeType.Audio && node.metadata?.content)
    return "audio";
  if (
    node.type === CanvasNodeType.Text &&
    (node.metadata?.content || node.metadata?.prompt)
  )
    return "text";
  // Plugin nodes declare their input eligibility through definition.resource.
  return getNodeDefinition(node.type)?.resource?.(node)?.kind || null;
}

function nodeReferenceAllocation(
  node: CanvasNodeData | undefined,
  id: string,
): { kind: CanvasResourceKind; ordinal: number } | undefined {
  const labels = node?.serviceData?.config?.editor?.metadata
    ?.referenceLabels as
    Record<string, { kind: CanvasResourceKind; ordinal: number }> | undefined;
  const value = labels?.[id];
  return value &&
    ["image", "video", "audio", "text"].includes(value.kind) &&
    Number.isSafeInteger(value.ordinal) &&
    value.ordinal > 0
    ? value
    : undefined;
}
