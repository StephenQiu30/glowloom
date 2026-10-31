import type { CanvasNodeMetadata } from "../types/canvas";

export const canvasTextBackgrounds = [
  {
    value: "default",
    label: "无颜色",
    color: "var(--canvas-reference-background)",
  },
  { value: "cyan", label: "青绿色", color: "var(--canvas-text-cyan)" },
  { value: "blue", label: "靛蓝色", color: "var(--canvas-text-blue)" },
  { value: "purple", label: "紫色", color: "var(--canvas-text-purple)" },
  { value: "orange", label: "橙色", color: "var(--canvas-text-orange)" },
  { value: "yellow", label: "黄色", color: "var(--canvas-text-yellow)" },
] as const;

export function canvasTextBackground(
  value: CanvasNodeMetadata["textBackground"],
) {
  const choice = canvasTextBackgrounds.find((item) => item.value === value);
  if (!choice || choice.value === "default")
    return "linear-gradient(to bottom right, var(--canvas-reference-text-start), var(--canvas-reference-text-end))";
  const tint = `color-mix(in srgb, ${choice.color} 16%, transparent)`;
  return `linear-gradient(${tint}, ${tint}), var(--canvas-reference-background)`;
}
