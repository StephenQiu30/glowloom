import { expect, it } from "vitest";
import { createNode } from "../document";
import { CanvasNodeType, type CanvasDocument } from "../model";
import {
  bindCanvasConnectionPreview,
  canvasConnectionPath,
} from "./connections";
import { applyCanvasNodeDragPreview } from "./viewport-dom";
it("DOM rAF拖动事件同步SVG两条路径，取消预览恢复且不修改正式文档", () => {
  const from = createNode(CanvasNodeType.Text, { x: 0, y: 0 }, "from"),
    to = createNode(CanvasNodeType.Text, { x: 500, y: 0 }, "to"),
    edge = { id: "edge", fromNodeId: "from", toNodeId: "to" };
  const doc: CanvasDocument = {
    id: "d",
    projectId: "p",
    name: "n",
    scope: {},
    revision: 1,
    nodes: [from, to],
    connections: [edge],
    viewport: { x: 0, y: 0, k: 1 },
  };
  const container = document.createElement("div");
  container.innerHTML =
    '<svg><path data-connection-path-id="edge"/><path data-connection-path-id="edge"/></svg>';
  document.body.append(container);
  const dispose = bindCanvasConnectionPreview(container, doc);
  applyCanvasNodeDragPreview(container, {
    x: 100,
    y: 30,
    nodeIds: new Set(["from"]),
  });
  const moved = canvasConnectionPath(
    edge,
    { ...from, position: { x: 100, y: 30 } },
    to,
  );
  expect(
    [...container.querySelectorAll("path")].map((path) =>
      path.getAttribute("d"),
    ),
  ).toEqual([moved, moved]);
  expect(doc.nodes[0].position).toEqual({ x: 0, y: 0 });
  applyCanvasNodeDragPreview(container, null);
  expect(container.querySelector("path")?.getAttribute("d")).toBe(
    canvasConnectionPath(edge, from, to),
  );
  dispose();
  container.remove();
});
it("折叠组连接锚点投影到84px视觉高度，保留服务端展开尺寸", () => {
  const from = createNode(CanvasNodeType.Frame, { x: 10, y: 20 }, "group"),
    to = createNode(CanvasNodeType.Text, { x: 800, y: 20 }, "text");
  from.metadata!.frame!.collapsed = true;
  expect(
    canvasConnectionPath(
      { id: "e", fromNodeId: from.id, toNodeId: to.id },
      from,
      to,
    ),
  ).toMatch(/^M 650 62 /);
  expect(from.height).toBe(420);
});
