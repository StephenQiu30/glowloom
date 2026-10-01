import { expect, it } from "vitest";
import { CanvasNodeType, type CanvasDocument } from "../model";
import { applyCommands, createNode, inverseCommands } from "../document";
import { copySelection, pasteSelection } from "./clipboard";
import {
  getFrameChildIds,
  isNodeHiddenByCollapsedFrame,
  selectionRootNodes,
  moveNodesWithChildren,
  applyFrameDrop,
} from "./frame";
import { viewportAtScale } from "./viewport";
import { buildCanvasSpatialIndex, canvasNodeBounds } from "./spatial-index";
import {
  createCanvasSelectionBounds,
  canvasSelectionHitsBounds,
} from "./selection";
import { createRenderIndex, visibleCanvasNodes } from "./virtualization";
const nested = () => {
  const outer = createNode(CanvasNodeType.Frame, { x: 0, y: 0 }, "outer"),
    inner = createNode(CanvasNodeType.Frame, { x: 50, y: 60 }, "inner"),
    text = createNode(CanvasNodeType.Text, { x: 80, y: 90 }, "text");
  inner.parentId = outer.id;
  text.parentId = inner.id;
  return [outer, inner, text];
};
it("嵌套分组移动/复制展开整个子树，粘贴重映射所有父关系", () => {
  const nodes = nested();
  expect([...getFrameChildIds("outer", nodes)]).toEqual(["inner", "text"]);
  const copy = copySelection(nodes, [], new Set(["outer"]));
  expect(copy.nodes).toHaveLength(3);
  let id = 0;
  const pasted = pasteSelection(copy, 40, () => String(++id));
  expect(pasted.nodes[2].parentId).toBe(pasted.nodes[1].id);
  expect(pasted.nodes[2].position).toEqual({ x: 120, y: 130 });
});
it("同时选祖先和后代只拖一次完整子树，对齐/键盘移动保持world delta", () => {
  const nodes = nested();
  expect(
    selectionRootNodes(nodes, new Set(["outer", "text"])).map(
      (node) => node.id,
    ),
  ).toEqual(["outer"]);
  const moved = moveNodesWithChildren(
    nodes,
    new Map([
      ["outer", { x: 100, y: 200 }],
      ["text", { x: 999, y: 999 }],
    ]),
  );
  expect(moved.map((node) => node.position)).toEqual([
    { x: 100, y: 200 },
    { x: 150, y: 260 },
    { x: 180, y: 290 },
  ]);
  expect(() => applyFrameDrop(nodes, new Set(["outer"]), "inner")).toThrow(
    /循环/,
  );
});
it("本地服务命令允许无环嵌套组，拒绝跨层祖先循环", () => {
  const doc: CanvasDocument = {
    id: "doc",
    projectId: "p",
    name: "name",
    scope: {},
    revision: 1,
    viewport: { x: 0, y: 0, k: 1 },
    nodes: nested(),
    connections: [],
  };
  expect(() =>
    applyCommands(doc, [
      {
        type: "SetNodeParents",
        parents: [{ id: "outer", parent_id: "inner" }],
      },
    ]),
  ).toThrow(/循环/);
});
it("折叠祖先隐藏所有后代；删组与撤销恢复嵌套父关系", () => {
  const nodes = nested();
  nodes[0].metadata!.frame!.collapsed = true;
  expect(isNodeHiddenByCollapsedFrame(nodes[2], nodes)).toBe(true);
  const doc: CanvasDocument = {
    id: "doc",
    projectId: "project",
    name: "画布",
    scope: {},
    revision: 1,
    viewport: { x: 0, y: 0, k: 1 },
    nodes,
    connections: [],
  };
  const commands = [{ type: "DeleteNodes" as const, ids: ["outer"] }];
  expect(
    applyCommands(
      applyCommands(doc, commands),
      inverseCommands(doc, commands),
    ).nodes.find((node) => node.id === "inner")?.parentId,
  ).toBe("outer");
});
it("锚点缩放保留世界中心并限制5%～400%", () => {
  const before = { x: -200, y: -100, k: 0.5 },
    size = { width: 1000, height: 600 };
  for (const scale of [0.001, 100]) {
    const after = viewportAtScale(before, size, scale);
    expect((size.width / 2 - after.x) / after.k).toBe(
      (size.width / 2 - before.x) / before.k,
    );
    expect(after.k).toBe(scale < 1 ? 0.05 : 4);
  }
});
it("空间索引处理负坐标与边界，不误命中远处对象", () => {
  const nodes = [
    createNode(CanvasNodeType.Text, { x: -320, y: -220 }, "negative"),
    createNode(CanvasNodeType.Text, { x: 20000, y: 20000 }, "far"),
  ];
  const index = buildCanvasSpatialIndex(
    nodes.map((node) => ({
      id: node.id,
      bounds: canvasNodeBounds(node),
      value: node,
    })),
  );
  expect(
    index
      .query({ left: -1, top: -1, right: 1, bottom: 1 })
      .map((node) => node.id),
  ).toEqual(["negative"]);
});
it("框选区分包含与相交，裁剪保留离屏选中节点", () => {
  const selection = createCanvasSelectionBounds(0, 0, 100, 100),
    bounds = { left: 80, top: 80, right: 180, bottom: 180 };
  expect(canvasSelectionHitsBounds(selection, bounds, "contain")).toBe(false);
  expect(canvasSelectionHitsBounds(selection, bounds, "intersect")).toBe(true);
  const nodes = [
    createNode(CanvasNodeType.Text, { x: 0, y: 0 }, "near"),
    createNode(CanvasNodeType.Text, { x: 20000, y: 0 }, "far"),
  ];
  expect(
    visibleCanvasNodes(
      createRenderIndex(nodes),
      { x: 0, y: 0, k: 1 },
      { width: 1000, height: 600 },
      new Set(["far"]),
      nodes,
    ).map((node) => node.id),
  ).toEqual(["near", "far"]);
});
it("折叠组可见边界不包含展开时的空白高度", () => {
  const group = nested()[0];
  group.metadata!.frame!.collapsed = true;
  expect(canvasNodeBounds(group).bottom).toBe(84);
});
