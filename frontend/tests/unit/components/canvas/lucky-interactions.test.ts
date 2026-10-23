import { afterEach, expect, it, vi } from "vitest";
import {
  CanvasNodeDragSession,
  applyCanvasDragPositions,
} from "@/components/canvas/node-drag";
import { canvasConnectionGeometry } from "@/components/canvas/connection-geometry";
import { canvasWheelViewport } from "@/components/canvas/wheel";
import { canvasPoint } from "@/components/canvas/viewport";
import type { CanvasNodeData } from "@/components/canvas/types";

afterEach(() => {
  vi.useRealTimers();
  document.body.replaceChildren();
});
const node = (id: string, x: number, y: number): CanvasNodeData => ({
  id,
  type: "text",
  title: id,
  position: { x, y },
  width: 120,
  height: 100,
});
function mount(nodes: CanvasNodeData[]) {
  const container = document.createElement("div");
  for (const n of nodes) {
    const el = document.createElement("div");
    el.dataset.nodeId = n.id;
    el.style.transform = `translate(${n.position.x}px, ${n.position.y}px)`;
    container.append(el);
  }
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.dataset.canvasDragGuides = "";
  container.append(svg);
  document.body.append(container);
  return container;
}
it.each([0.5, 1, 2])(
  "%s倍下12单位吸附与组成员保持同一位移，取消清除迟到帧",
  (scale) => {
    vi.useFakeTimers();
    const nodes = [
      node("a", 0, 0),
      { ...node("b", 24, 52), metadata: { groupId: "a" } },
    ];
    const container = mount(nodes);
    const session = new CanvasNodeDragSession({
      container,
      nodes,
      connections: [],
      movedIds: new Set(["a", "b"]),
      start: { x: 0, y: 0 },
      scale,
      snap: true,
    });
    session.move({ x: 19 * scale, y: 19 * scale });
    vi.advanceTimersByTime(20);
    expect(
      container.querySelector<HTMLElement>('[data-node-id="b"]')?.style
        .transform,
    ).toBe("translate(48px, 76px)");
    expect(nodes[0].position).toEqual({ x: 0, y: 0 });
    session.cancel();
    vi.advanceTimersByTime(20);
    expect(
      container.querySelector<HTMLElement>('[data-node-id="b"]')?.style
        .transform,
    ).toBe("translate(24px, 52px)");
    expect(session.finish()).toBeNull();
  },
);
it("松手消费最后坐标，近距离对齐，位置合并保留最新正文且不复活删除节点", () => {
  vi.useFakeTimers();
  const nodes = [node("a", 0, 0), node("b", 145, 300)];
  const container = mount(nodes);
  const session = new CanvasNodeDragSession({
    container,
    nodes,
    connections: [],
    movedIds: new Set(["a"]),
    start: { x: 0, y: 0 },
    scale: 1,
    snap: true,
  });
  const positions = session.finish({ x: 19, y: 0 })!;
  expect(positions.get("a")).toEqual({ x: 25, y: 0 });
  const latest = [{ ...nodes[0], metadata: { content: "远端最新正文" } }];
  const merged = applyCanvasDragPositions(latest, positions);
  expect(merged[0].metadata?.content).toBe("远端最新正文");
  expect(applyCanvasDragPositions([], positions)).toEqual([]);
  session.settle(merged, []);
  expect(container.firstElementChild?.getAttribute("style")).toContain("25px");
});
it("反向连线与拖动预览使用同一曲线，取消恢复命中线及可见线", () => {
  vi.useFakeTimers();
  const nodes = [node("a", 300, 0), node("b", 0, 0)];
  const container = mount(nodes);
  const svg = container.querySelector("svg")!;
  const edge = document.createElementNS(svg.namespaceURI, "g");
  edge.setAttribute("data-canvas-edge-id", "edge");
  for (let i = 0; i < 2; i++)
    edge.append(document.createElementNS(svg.namespaceURI, "path"));
  svg.append(edge);
  const initial = canvasConnectionGeometry({ x: 420, y: 50 }, { x: 0, y: 50 });
  const session = new CanvasNodeDragSession({
    container,
    nodes,
    connections: [{ id: "edge", fromNodeId: "a", toNodeId: "b" }],
    movedIds: new Set(["a"]),
    start: { x: 0, y: 0 },
    scale: 1,
    snap: false,
  });
  session.move({ x: 40, y: 20 });
  vi.advanceTimersByTime(20);
  const next = canvasConnectionGeometry({ x: 460, y: 70 }, { x: 0, y: 50 });
  expect(
    [...edge.querySelectorAll("path")].map((p) => p.getAttribute("d")),
  ).toEqual([next.path, next.path]);
  session.cancel();
  expect(edge.querySelector("path")?.getAttribute("d")).toBe(initial.path);
});
it("滚轮单位归一化、捏合边界与光标锚点保持", () => {
  const view = { x: 140, y: -50, k: 1 },
    anchor = { x: 500, y: 300 };
  const base = {
    deltaX: 0,
    deltaY: 0,
    deltaMode: 0,
    ctrlKey: false,
    metaKey: false,
    shiftKey: false,
  };
  expect(
    canvasWheelViewport(
      view,
      { ...base, deltaY: 1, deltaMode: 2 },
      anchor,
      800,
    ),
  ).toEqual({ x: 140, y: -850, k: 1 });
  for (const deltaY of [-10000, -12, 100, 10000]) {
    const next = canvasWheelViewport(
      view,
      { ...base, ctrlKey: true, deltaY },
      anchor,
      800,
    );
    expect(next.k).toBeGreaterThanOrEqual(0.05);
    expect(next.k).toBeLessThanOrEqual(5);
    expect(canvasPoint(anchor, next).x).toBeCloseTo(
      canvasPoint(anchor, view).x,
    );
    expect(canvasPoint(anchor, next).y).toBeCloseTo(
      canvasPoint(anchor, view).y,
    );
  }
});
