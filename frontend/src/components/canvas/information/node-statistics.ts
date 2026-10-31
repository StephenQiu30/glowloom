import type { CanvasNodeData } from "../types/canvas";
export const nodeCategories = [
  { type: "image", label: "图片" },
  { type: "video", label: "视频" },
  { type: "audio", label: "音频" },
  { type: "text", label: "文本" },
  { type: "timeline", label: "时间线" },
  { type: "subject", label: "主体" },
  { type: "external", label: "外部" },
  { type: "other", label: "其他" },
] as const;
export function canvasNodeStatistics(nodes: Pick<CanvasNodeData, "type">[]) {
  const counts = new Map<string, number>();
  for (const node of nodes) {
    const type =
      node.type === "director" || node.type === "cinema_studio"
        ? "external"
        : nodeCategories.some((item) => item.type === node.type)
          ? node.type
          : "other";
    counts.set(type, (counts.get(type) ?? 0) + 1);
  }
  return nodeCategories.map((item) => ({
    ...item,
    count: counts.get(item.type) ?? 0,
  }));
}
/** Preserve all six minor digits without losing precision through Number conversion. */
export function formatCanvasCharge(micros: string) {
  if (!/^\d+$/.test(micros)) throw new Error("费用数据无效");
  const value = BigInt(micros),
    whole = value / BigInt(1_000_000);
  const fraction = (value % BigInt(1_000_000))
    .toString()
    .padStart(6, "0")
    .replace(/0+$/, "");
  return `¥${whole.toLocaleString("zh-CN")}${fraction ? `.${fraction}` : ""}`;
}
