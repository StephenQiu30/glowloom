import { containerKind } from "../lib/canvas-containers";
import type { CanvasNodeData } from "../types/canvas";
export const canvasSearchTypes = [
  { type: "all", label: "全部" },
  { type: "image", label: "图片" },
  { type: "video", label: "视频" },
  { type: "audio", label: "音频" },
  { type: "text", label: "文本" },
  { type: "subject", label: "主体" },
  { type: "timeline", label: "时间线" },
  { type: "cinema_studio", label: "导演工具箱" },
  { type: "group", label: "组" },
  { type: "stack", label: "堆叠" },
  { type: "other", label: "其他" },
] as const;
export type CanvasSearchType = (typeof canvasSearchTypes)[number]["type"];
export function canvasSearchType(
  type: string,
  node?: CanvasNodeData,
): CanvasSearchType {
  if (node && containerKind(node) === "stack") return "stack";
  if (type === "image_generation") return "image";
  if (type === "video_generation") return "video";
  return canvasSearchTypes.some(
    (item) => item.type === type && item.type !== "all",
  )
    ? (type as CanvasSearchType)
    : "other";
}
export function canvasNodeTypeLabel(type: string, node?: CanvasNodeData) {
  if (type === "director") return "导演台";
  return canvasSearchTypes.find(
    (item) => item.type === canvasSearchType(type, node),
  )!.label;
}
export function canvasNodeName(node: Pick<CanvasNodeData, "title">) {
  return node.title.trim() || "未命名节点";
}
export function searchCanvasNodes(
  nodes: readonly CanvasNodeData[],
  query: string,
  type: CanvasSearchType,
) {
  const term = query.trim().toLocaleLowerCase();
  const matched = nodes.filter(
    (node) => !term || canvasNodeName(node).toLocaleLowerCase().includes(term),
  );
  const counts = Object.fromEntries(
    canvasSearchTypes.map((item) => [item.type, 0]),
  ) as Record<CanvasSearchType, number>;
  counts.all = matched.length;
  for (const node of matched) counts[canvasSearchType(node.type, node)]++;
  return {
    counts,
    results:
      type === "all"
        ? matched
        : matched.filter((node) => canvasSearchType(node.type, node) === type),
  };
}
