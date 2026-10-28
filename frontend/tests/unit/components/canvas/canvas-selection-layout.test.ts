import { expect, it } from "vitest";
import { layoutCanvasSelection } from "@/components/canvas/lib/canvas-selection-layout";
import type { CanvasNodeData } from "@/components/canvas/types/canvas";
const node = (id: string, width: number, height: number): CanvasNodeData => ({
  id,
  type: "image",
  title: id,
  position: { x: 100, y: 100 },
  width,
  height,
});
it("宫格使用选中节点的稳定次序和最大格尺寸，不改未选中节点", () => {
  const nodes = [
    node("a", 320, 200),
    node("b", 500, 300),
    node("c", 240, 180),
    node("d", 500, 500),
  ];
  const next = layoutCanvasSelection(nodes, new Set(["c", "a", "b"]), "grid");
  expect(next.map((n) => n.position)).toEqual([
    { x: 100, y: 100 },
    { x: 648, y: 100 },
    { x: 100, y: 448 },
    { x: 100, y: 100 },
  ]);
  expect(next[3]).toBe(nodes[3]);
  expect(nodes[1].position).toEqual({ x: 100, y: 100 });
});
it("智能布局不重叠，选择组时同步移动其成员而不改成员相对坐标", () => {
  const group = { ...node("g", 600, 400), type: "group" };
  const child = {
    ...node("child", 200, 160),
    position: { x: 124, y: 152 },
    metadata: { groupId: "g" },
  };
  const nodes = [node("a", 320, 200), group, child, node("b", 240, 180)];
  const next = layoutCanvasSelection(
    nodes,
    new Set(["a", "g", "b", "child"]),
    "smart",
  );
  expect(next[2].position.x - next[1].position.x).toBe(24);
  expect(next[2].position.y - next[1].position.y).toBe(52);
  const roots = next.filter((n) => n.id !== "child");
  for (let i = 0; i < roots.length; i++)
    for (let j = i + 1; j < roots.length; j++) {
      const a = roots[i],
        b = roots[j];
      expect(
        a.position.x + a.width <= b.position.x ||
          b.position.x + b.width <= a.position.x ||
          a.position.y + a.height <= b.position.y ||
          b.position.y + b.height <= a.position.y,
      ).toBe(true);
    }
});
