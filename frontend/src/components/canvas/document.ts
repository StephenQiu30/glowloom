import type {
  CanvasDocument,
  CanvasGraph,
  CanvasNodeData,
  NodeType,
  Position,
} from "./types";

const specs = {
  text: [340, 240, "文本"],
  image: [340, 240, "图片"],
  video: [420, 236, "视频"],
  audio: [340, 120, "音频"],
  group: [760, 480, "分组"],
} as const;
// Upstream node factory, limited to editing types; IDs use the browser UUID API.
export function createNode(type: NodeType, center: Position): CanvasNodeData {
  const [width, height, title] = specs[type];
  return {
    id: crypto.randomUUID(),
    type,
    title,
    position: { x: center.x - width / 2, y: center.y - height / 2 },
    width,
    height,
    metadata: { content: "" },
  };
}
export function selectionMembers(graph: CanvasGraph, selected: Set<string>) {
  return new Set(
    graph.nodes
      .filter(
        (n) =>
          selected.has(n.id) ||
          (n.metadata?.groupId && selected.has(n.metadata.groupId)),
      )
      .map((n) => n.id),
  );
}
export function removeSelection(
  graph: CanvasGraph,
  selected: Set<string>,
): CanvasGraph {
  const ids = selectionMembers(graph, selected);
  return {
    nodes: graph.nodes.filter((n) => !ids.has(n.id)),
    connections: graph.connections.filter(
      (e) =>
        !selected.has(e.id) && !ids.has(e.fromNodeId) && !ids.has(e.toNodeId),
    ),
  };
}
export function duplicateSelection(
  graph: CanvasGraph,
  selected: Set<string>,
): CanvasGraph {
  const ids = selectionMembers(graph, selected);
  const mapping = new Map([...ids].map((id) => [id, crypto.randomUUID()]));
  return {
    nodes: graph.nodes
      .filter((n) => ids.has(n.id))
      .map((n) => ({
        ...n,
        id: mapping.get(n.id)!,
        position: { x: n.position.x + 40, y: n.position.y + 40 },
        metadata: {
          ...n.metadata,
          groupId: n.metadata?.groupId
            ? mapping.get(n.metadata.groupId)
            : undefined,
        },
      })),
    connections: graph.connections
      .filter((e) => ids.has(e.fromNodeId) && ids.has(e.toNodeId))
      .map((e) => ({
        id: crypto.randomUUID(),
        fromNodeId: mapping.get(e.fromNodeId)!,
        toNodeId: mapping.get(e.toNodeId)!,
      })),
  };
}
function record(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === "object" && !Array.isArray(value);
}
function finite(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}
function position(value: unknown) {
  return record(value) && finite(value.x) && finite(value.y);
}
export function parseDocument(text: string): CanvasDocument {
  const d: unknown = JSON.parse(text);
  if (
    !record(d) ||
    d.format !== "glowloom-canvas" ||
    d.version !== 1 ||
    !Array.isArray(d.nodes) ||
    !Array.isArray(d.connections) ||
    !record(d.viewport) ||
    !position(d.viewport) ||
    !finite(d.viewport.k) ||
    d.viewport.k < 0.05 ||
    d.viewport.k > 5
  )
    throw new Error("不支持的画布文件格式");
  const ids = new Set<string>();
  for (const n of d.nodes) {
    if (
      !record(n) ||
      typeof n.id !== "string" ||
      !n.id ||
      ids.has(n.id) ||
      typeof n.type !== "string" ||
      !Object.hasOwn(specs, n.type) ||
      typeof n.title !== "string" ||
      !position(n.position) ||
      !finite(n.width) ||
      n.width <= 0 ||
      !finite(n.height) ||
      n.height <= 0
    )
      throw new Error("画布包含无效或不支持的节点");
    ids.add(n.id);
    if (n.metadata !== undefined) {
      if (!record(n.metadata)) throw new Error("素材信息无效");
      const m = n.metadata;
      if (m.content !== undefined && typeof m.content !== "string")
        throw new Error("节点内容无效");
      if (m.groupId !== undefined && typeof m.groupId !== "string")
        throw new Error("分组信息无效");
      if (
        n.type !== "text" &&
        m.content &&
        !new RegExp(`^data:${n.type}/[a-zA-Z0-9.+-]+;base64,`).test(
          String(m.content),
        )
      )
        throw new Error("画布素材必须内嵌在文件中");
      for (const key of ["naturalWidth", "naturalHeight"])
        if (m[key] !== undefined && (!finite(m[key]) || m[key] <= 0))
          throw new Error("素材尺寸无效");
    }
  }
  const groups = new Set(
    d.nodes.filter((n) => n.type === "group").map((n) => n.id),
  );
  for (const n of d.nodes)
    if (
      n.metadata?.groupId &&
      (n.type === "group" || !groups.has(n.metadata.groupId))
    )
      throw new Error("分组引用无效");
  const connectionIDs = new Set<string>();
  for (const e of d.connections) {
    if (
      !record(e) ||
      typeof e.id !== "string" ||
      !e.id ||
      ids.has(e.id) ||
      connectionIDs.has(e.id) ||
      typeof e.fromNodeId !== "string" ||
      typeof e.toNodeId !== "string" ||
      !ids.has(e.fromNodeId) ||
      !ids.has(e.toNodeId) ||
      e.fromNodeId === e.toNodeId ||
      groups.has(e.fromNodeId) ||
      groups.has(e.toNodeId)
    )
      throw new Error("画布包含无效的连线");
    connectionIDs.add(e.id);
  }
  return d as CanvasDocument;
}
export const emptyDocument: CanvasDocument = {
  format: "glowloom-canvas",
  version: 1,
  nodes: [],
  connections: [],
  viewport: { x: 0, y: 0, k: 1 },
};
export function downloadFile(content: Blob, name: string) {
  const url = URL.createObjectURL(content);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
export async function readMedia(file: File, center: Position) {
  const kind = file.type.split("/")[0];
  if (kind !== "image" && kind !== "video" && kind !== "audio")
    throw new Error(`不支持的素材：${file.name}`);
  if (file.size > 50 * 1024 * 1024) throw new Error("单个素材需小于 50 MB");
  const content = await new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = () => reject(new Error("素材读取失败"));
    reader.readAsDataURL(file);
  });
  const node = createNode(kind, center);
  node.title = file.name;
  node.metadata = { content, mimeType: file.type };
  if (kind === "image") {
    const image = await new Promise<HTMLImageElement>((resolve, reject) => {
      const image = new Image();
      image.onload = () => resolve(image);
      image.onerror = () => reject(new Error("图片无法解码"));
      image.src = content;
    });
    node.metadata.naturalWidth = image.naturalWidth;
    node.metadata.naturalHeight = image.naturalHeight;
    const scale = Math.min(
      1,
      640 / image.naturalWidth,
      640 / image.naturalHeight,
    );
    node.width = image.naturalWidth * scale;
    node.height = image.naturalHeight * scale;
    node.position = {
      x: center.x - node.width / 2,
      y: center.y - node.height / 2,
    };
  }
  return node;
}
