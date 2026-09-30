// Adapted from BeefTV use-canvas-node-operations: ID remap, world offset, internal wires.
import { z } from "zod";
import {
  CanvasNodeType,
  type CanvasConnection,
  type CanvasNodeData,
  type Position,
} from "../model";
import { createNode } from "../document";
import { getFrameChildIds } from "./frame";
export type CanvasClipboard = {
  nodes: CanvasNodeData[];
  connections: CanvasConnection[];
};

const FORMAT = "lanverse.canvas";
export const MAX_CANVAS_CLIPBOARD_BYTES = 1 << 20;
const uuid = z
  .string()
  .uuid()
  .refine((value) => value !== "00000000-0000-0000-0000-000000000000");
const coordinate = z.number().finite().min(-1e6).max(1e6);
const position = z.strictObject({ x: coordinate, y: coordinate });
const text = z.string().refine((value) => Array.from(value).length <= 10000);
const title = z
  .string()
  .refine(
    (value) => value.trim().length > 0 && Array.from(value).length <= 128,
  );
const fields = {
  id: uuid,
  title,
  position,
  parentId: uuid.optional(),
  zIndex: z.number().int().min(-1e6).max(1e6),
};
const dimensions = {
  width: z.number().finite().min(40).max(2000),
  height: z.number().finite().min(40).max(2000),
};
const wireNode = z.discriminatedUnion("type", [
  z.strictObject({
    ...fields,
    ...dimensions,
    type: z.literal(CanvasNodeType.Text),
    config: z.strictObject({ text }),
  }),
  z.strictObject({
    ...fields,
    width: z.number().finite().min(40).max(100000),
    height: z.number().finite().min(40).max(100000),
    type: z.literal(CanvasNodeType.Frame),
    config: z.strictObject({ collapsed: z.boolean() }),
  }),
  ...[CanvasNodeType.Image, CanvasNodeType.Video, CanvasNodeType.Audio].map(
    (type) =>
      z.strictObject({
        ...fields,
        ...dimensions,
        type: z.literal(type),
        assetId: uuid,
        config: z.strictObject({}),
      }),
  ),
]);
const wireGraph = z.strictObject({
  format: z.literal(FORMAT),
  version: z.literal(1),
  projectId: uuid,
  nodes: z.array(wireNode).min(1).max(2000),
  connections: z
    .array(z.strictObject({ id: uuid, fromNodeId: uuid, toNodeId: uuid }))
    .max(4000),
});
type WireGraph = z.infer<typeof wireGraph>;

// Clipboard failures expose only stable messages, never native error details or payloads.
export class CanvasClipboardError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "CanvasClipboardError";
  }
}
function invalidClipboard(): never {
  throw new CanvasClipboardError("剪贴板画布数据无效或超出容量限制。");
}
function project(projectId: string) {
  if (!uuid.safeParse(projectId).success) invalidClipboard();
}
function bounded(value: string) {
  // Check UTF-16 length first to avoid encoding an arbitrarily large input.
  if (
    value.length > MAX_CANVAS_CLIPBOARD_BYTES ||
    new TextEncoder().encode(value).byteLength > MAX_CANVAS_CLIPBOARD_BYTES
  )
    invalidClipboard();
}
function validateGraph(value: unknown): WireGraph {
  const parsed = wireGraph.safeParse(value);
  if (!parsed.success) invalidClipboard();
  const graph = parsed.data;
  const nodes = new Map(graph.nodes.map((node) => [node.id, node]));
  if (nodes.size !== graph.nodes.length) invalidClipboard();
  const edges = new Set<string>();
  for (const edge of graph.connections) {
    if (
      edges.has(edge.id) ||
      edge.fromNodeId === edge.toNodeId ||
      !nodes.has(edge.fromNodeId) ||
      !nodes.has(edge.toNodeId)
    )
      invalidClipboard();
    edges.add(edge.id);
  }
  const resolved = new Set<string>();
  for (const node of graph.nodes) {
    const visited = new Set<string>();
    let current: (typeof graph.nodes)[number] | undefined = node;
    while (current && !resolved.has(current.id)) {
      if (visited.has(current.id)) invalidClipboard();
      visited.add(current.id);
      if (!current.parentId) break;
      const parent = nodes.get(current.parentId);
      if (!parent || parent.type !== CanvasNodeType.Frame) invalidClipboard();
      current = parent;
    }
    visited.forEach((id) => resolved.add(id));
  }
  return graph;
}
function safeNode(
  node: CanvasNodeData,
  ids: ReadonlySet<string>,
): CanvasNodeData {
  return {
    id: node.id,
    type: node.type,
    title: node.title,
    position: { ...node.position },
    width: node.width,
    height: node.height,
    zIndex: node.zIndex,
    ...(node.parentId && ids.has(node.parentId)
      ? { parentId: node.parentId }
      : {}),
    ...(node.type === CanvasNodeType.Text
      ? { metadata: { content: node.metadata?.content ?? "" } }
      : {}),
    ...(node.type === CanvasNodeType.Frame
      ? {
          metadata: {
            frame: {
              collapsed: node.metadata?.frame?.collapsed ?? false,
              expandedWidth: node.width,
              expandedHeight: node.height,
            },
          },
        }
      : {}),
    ...([
      CanvasNodeType.Image,
      CanvasNodeType.Video,
      CanvasNodeType.Audio,
    ].includes(node.type) && node.assetId
      ? { assetId: node.assetId }
      : {}),
  };
}

/** Only typed presentation config and media identity travel between tabs. */
export function encodeCanvasClipboard(
  clipboard: CanvasClipboard,
  projectId: string,
): string {
  project(projectId);
  const ids = new Set(clipboard.nodes.map((node) => node.id));
  const graph = validateGraph({
    format: FORMAT,
    version: 1,
    projectId,
    nodes: clipboard.nodes.map((source) => {
      const node = safeNode(source, ids);
      const content = node.metadata?.content ?? "";
      const collapsed = node.metadata?.frame?.collapsed ?? false;
      delete node.metadata;
      return {
        ...node,
        config:
          node.type === CanvasNodeType.Text
            ? { text: content }
            : node.type === CanvasNodeType.Frame
              ? { collapsed }
              : {},
      };
    }),
    connections: clipboard.connections.map(({ id, fromNodeId, toNodeId }) => ({
      id,
      fromNodeId,
      toNodeId,
    })),
  });
  const encoded = JSON.stringify(graph);
  bounded(encoded);
  return encoded;
}

/** Foreign or malformed graph envelopes fail; ordinary text becomes a text resource. */
export function decodeCanvasClipboard(
  value: string,
  projectId: string,
  textPosition: Position = { x: 0, y: 0 },
): CanvasClipboard {
  project(projectId);
  bounded(value);
  let parsed: unknown;
  try {
    parsed = JSON.parse(value);
  } catch {
    /* Native plain text is a valid input. */
  }
  if (parsed && typeof parsed === "object" && "format" in parsed) {
    const graph = validateGraph(parsed);
    if (graph.projectId !== projectId)
      throw new CanvasClipboardError(
        "剪贴板画布属于其他项目，不能粘贴到当前项目。",
      );
    return {
      nodes: graph.nodes.map((node) => {
        const { config, ...data } = node;
        return {
          ...data,
          ...(node.type === CanvasNodeType.Text && "text" in config
            ? { metadata: { content: config.text } }
            : {}),
          ...(node.type === CanvasNodeType.Frame && "collapsed" in config
            ? {
                metadata: {
                  frame: {
                    collapsed: config.collapsed,
                    expandedWidth: node.width,
                    expandedHeight: node.height,
                  },
                },
              }
            : {}),
        };
      }),
      connections: graph.connections,
    };
  }
  // A malformed claimed graph must not silently become a text node.
  if (value.includes(`"format"`) && value.includes(FORMAT)) invalidClipboard();
  if (
    !value.trim() ||
    !text.safeParse(value).success ||
    !position.safeParse(textPosition).success
  )
    invalidClipboard();
  const node = createNode(CanvasNodeType.Text, textPosition);
  node.metadata = { content: value };
  return { nodes: [node], connections: [] };
}

/** No memory fallback: success means the browser actually wrote the system clipboard. */
export async function writeCanvasClipboard(
  clipboard: CanvasClipboard,
  projectId: string,
): Promise<void> {
  const value = encodeCanvasClipboard(clipboard, projectId);
  const native = globalThis.navigator?.clipboard;
  if (!native?.writeText)
    throw new CanvasClipboardError(
      "系统剪贴板不可用，请使用支持剪贴板的安全浏览器环境。",
    );
  try {
    await native.writeText(value);
  } catch {
    throw new CanvasClipboardError(
      "写入系统剪贴板失败，请允许剪贴板权限后重试。",
    );
  }
}

/** Every paste reads the system clipboard, including after refresh or in another tab. */
export async function readCanvasClipboard(
  projectId: string,
  textPosition?: Position,
): Promise<CanvasClipboard> {
  const native = globalThis.navigator?.clipboard;
  if (!native?.readText)
    throw new CanvasClipboardError(
      "系统剪贴板不可用，请使用支持剪贴板的安全浏览器环境。",
    );
  let value: string;
  try {
    value = await native.readText();
  } catch {
    throw new CanvasClipboardError(
      "读取系统剪贴板失败，请允许剪贴板权限后重试。",
    );
  }
  return decodeCanvasClipboard(value, projectId, textPosition);
}
export function copySelection(
  nodes: CanvasNodeData[],
  connections: CanvasConnection[],
  selected: Set<string>,
): CanvasClipboard {
  const ids = new Set(selected);
  selected.forEach((id) =>
    getFrameChildIds(id, nodes).forEach((child) => ids.add(child)),
  );
  return {
    nodes: nodes
      .filter((node) => ids.has(node.id))
      .map((node) => safeNode(node, ids)),
    connections: structuredClone(
      connections.filter(
        (edge) => ids.has(edge.fromNodeId) && ids.has(edge.toNodeId),
      ),
    ),
  };
}
export function pasteSelection(
  clipboard: CanvasClipboard,
  offset = 40,
  newId: () => string = () => crypto.randomUUID(),
): CanvasClipboard {
  const ids = new Map(clipboard.nodes.map((node) => [node.id, newId()]));
  const sourceIds = new Set(ids.keys());
  return {
    nodes: clipboard.nodes.map((node) => ({
      ...safeNode(node, sourceIds),
      id: ids.get(node.id)!,
      title: `${Array.from(node.title).slice(0, 120).join("")} 副本`,
      position: { x: node.position.x + offset, y: node.position.y + offset },
      parentId: node.parentId ? ids.get(node.parentId) : undefined,
    })),
    connections: clipboard.connections.map((edge) => ({
      id: newId(),
      fromNodeId: ids.get(edge.fromNodeId)!,
      toNodeId: ids.get(edge.toNodeId)!,
    })),
  };
}
