import type { CanvasNodeData, CanvasNodeMetadata } from "../types/canvas";

export type CanvasColorTag = NonNullable<
  CanvasNodeMetadata["colorTags"]
>[number];
export const canvasColorTags = [
  { value: "cyan", label: "青绿色", color: "var(--canvas-text-cyan)" },
  { value: "blue", label: "靛蓝色", color: "var(--canvas-text-blue)" },
  { value: "purple", label: "紫色", color: "var(--canvas-text-purple)" },
  { value: "orange", label: "橙色", color: "var(--canvas-text-orange)" },
  { value: "yellow", label: "黄色", color: "var(--canvas-text-yellow)" },
] as const;

export function changeCanvasColorTags(
  nodes: CanvasNodeData[],
  selected: Set<string>,
  tag: CanvasColorTag | null,
): CanvasNodeData[] {
  const targets = nodes.filter((node) => selected.has(node.id));
  if (!targets.length) return nodes;
  const remove =
    tag !== null &&
    targets.every((node) => node.metadata?.colorTags?.includes(tag));
  return nodes.map((node) => {
    if (!selected.has(node.id)) return node;
    const previous = node.metadata?.colorTags ?? [];
    const next =
      tag === null
        ? []
        : remove
          ? previous.filter((value) => value !== tag)
          : previous.includes(tag)
            ? previous
            : [...previous, tag];
    if (
      previous.length === next.length &&
      previous.every((value, index) => next[index] === value)
    )
      return node;
    return { ...node, metadata: { ...node.metadata, colorTags: next } };
  });
}
