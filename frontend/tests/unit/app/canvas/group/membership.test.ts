import { expect, it } from "vitest";
import { createResourceNode } from "@/app/canvas/document";
import { reconcileGroupDrop } from "@/app/canvas/group/membership";

const group = () => ({
  ...createResourceNode("group", { x: 0, y: 0 }),
  width: 600,
  height: 600,
});
const text = (x = 100, y = 100) => ({
  ...createResourceNode("text", { x, y }),
  width: 80,
  height: 80,
});

it("拖入分组后保存父关系，拖出解除关系但保留内容与身份", () => {
  const frame = group(),
    node = text();
  const joined = reconcileGroupDrop([frame, node], new Set([node.id]));
  expect(joined[1]).toEqual({ ...node, parent_id: frame.id });
  const outside = { ...joined[1], x: 900 };
  expect(reconcileGroupDrop([frame, outside], new Set([node.id]))[1]).toEqual({
    ...outside,
    parent_id: undefined,
  });
});
it("多选整体落入同一组；跨组时不拆散批量选择", () => {
  const frame = group(),
    a = text(),
    b = text(250);
  expect(
    reconcileGroupDrop([frame, a, b], new Set([a.id, b.id]))
      .slice(1)
      .map((n) => n.parent_id),
  ).toEqual([frame.id, frame.id]);
  expect(
    reconcileGroupDrop([frame, a, { ...b, x: 900 }], new Set([a.id, b.id]))[1]
      .parent_id,
  ).toBeUndefined();
});
it("移动分组保留后代关系；不把祖先装入自己的后代", () => {
  const outer = group(),
    inner = { ...group(), parent_id: outer.id },
    node = { ...text(), parent_id: inner.id };
  const nodes = [outer, inner, node];
  expect(reconcileGroupDrop(nodes, new Set(nodes.map((n) => n.id)))).toEqual(
    nodes,
  );
});
it("重叠时优先最小可编辑展开组，忽略只读和折叠目标", () => {
  const outer = group(),
    inner = { ...group(), width: 300, height: 300 },
    node = text();
  expect(
    reconcileGroupDrop([outer, inner, node], new Set([node.id]))[2].parent_id,
  ).toBe(inner.id);
  expect(
    reconcileGroupDrop(
      [outer, { ...inner, config: { collapsed: true } }, node],
      new Set([node.id]),
    )[2].parent_id,
  ).toBe(outer.id);
});
it("只读节点不改变父关系，缺失的拖动节点不影响图", () => {
  const frame = group(),
    node = { ...text(), last_operation_id: crypto.randomUUID() };
  expect(reconcileGroupDrop([frame, node], new Set([node.id]))).toEqual([
    frame,
    node,
  ]);
  expect(reconcileGroupDrop([frame], new Set(["missing"]))[0]).toBe(frame);
});
