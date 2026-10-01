import { describe, expect, it } from "vitest";
import { createNode } from "../document";
import {
  CanvasNodeType,
  type CanvasConnection,
  type CanvasNodeData,
} from "../model";
import {
  canvasNodeRenderBudget,
  canvasRenderBounds,
  createConnectionRenderIndex,
  createRenderIndex,
  visibleCanvasConnections,
  visibleCanvasNodes,
} from "./virtualization";

const viewport = { x: 0, y: 0, k: 1 };
const size = { width: 1000, height: 600 };
function node(id: string, x = 0, y = 0) {
  return createNode(CanvasNodeType.Text, { x, y }, id);
}
function edge(id: string, fromNodeId: string, toNodeId: string) {
  return { id, fromNodeId, toNodeId } satisfies CanvasConnection;
}
function ids(nodes: CanvasNodeData[]) {
  return nodes.map((item) => item.id);
}

describe("画布节点渲染预算与迟滞", () => {
  it("缩放预算在14%与28%切换，正式2000节点不因裁剪改变", () => {
    const nodes = Array.from({ length: 2000 }, (_, i) => node(String(i)));
    const before = structuredClone(nodes);
    for (const [k, budget] of [
      [0.05, 280],
      [0.139, 280],
      [0.14, 420],
      [0.279, 420],
      [0.28, 720],
      [4, 720],
    ]) {
      expect(canvasNodeRenderBudget(k)).toBe(budget);
      expect(
        visibleCanvasNodes(
          createRenderIndex(nodes),
          { ...viewport, k },
          size,
          new Set(),
          nodes,
        ),
      ).toHaveLength(budget);
    }
    expect(nodes).toEqual(before);
  });

  it("进入和保留边距按屏幕像素换算，性能模式不扩大首次挂载区", () => {
    expect(canvasRenderBounds({ x: -100, y: 40, k: 2 }, size, true)).toEqual({
      enter: { left: -14, top: -84, right: 614, bottom: 344 },
      retain: { left: -270, top: -340, right: 870, bottom: 600 },
    });
    expect(canvasRenderBounds(viewport, size, false)).toEqual({
      enter: { left: -192, top: -192, right: 1192, bottom: 792 },
      retain: { left: -384, top: -384, right: 1384, bottom: 984 },
    });
  });

  it("边缘首次不挂载，上一帧已挂载节点在保留区内稳定并在离开后释放", () => {
    const nodes = [node("near"), node("edge", 1250)];
    const index = createRenderIndex(nodes);
    expect(
      ids(visibleCanvasNodes(index, viewport, size, new Set(), nodes)),
    ).toEqual(["near"]);
    expect(
      ids(
        visibleCanvasNodes(index, viewport, size, new Set(), nodes, {
          previouslyRenderedIds: new Set(["edge"]),
        }),
      ),
    ).toEqual(["near", "edge"]);
    expect(
      ids(
        visibleCanvasNodes(
          index,
          { ...viewport, x: 500 },
          size,
          new Set(),
          nodes,
          {
            previouslyRenderedIds: new Set(["edge"]),
          },
        ),
      ),
    ).toEqual(["near"]);
  });

  it("拥挤视口优先保留离屏选中和交互节点，且不超预算", () => {
    const nodes = Array.from({ length: 2000 }, (_, i) => node(String(i)));
    nodes[1998].position.x = 90000;
    nodes[1999].position.x = 90000;
    const visible = visibleCanvasNodes(
      createRenderIndex(nodes),
      viewport,
      size,
      new Set(["1998"]),
      nodes,
      { interactingNodeIds: new Set(["1999"]) },
    );
    expect(visible).toHaveLength(720);
    expect(ids(visible)).toContain("1998");
    expect(ids(visible)).toContain("1999");
    expect(new Set(ids(visible)).size).toBe(720);
  });

  it("全选2000节点仍有预算，交互节点优先于其他选中节点", () => {
    const nodes = Array.from({ length: 2000 }, (_, i) => node(String(i)));
    const visible = visibleCanvasNodes(
      createRenderIndex(nodes),
      { ...viewport, k: 0.1 },
      size,
      new Set(ids(nodes)),
      nodes,
      { interactingNodeIds: new Set(["1999"]) },
    );
    expect(visible).toHaveLength(280);
    expect(ids(visible)).toContain("1999");
  });

  it("保留区满额时新进入视口的节点不会被旧卡片挤掉", () => {
    const previous = Array.from({ length: 720 }, (_, i) =>
      node(String(i), 1250),
    );
    const nodes = [...previous, node("new")];
    const visible = visibleCanvasNodes(
      createRenderIndex(nodes),
      viewport,
      size,
      new Set(),
      nodes,
      { previouslyRenderedIds: new Set(ids(previous)) },
    );
    expect(visible).toHaveLength(720);
    expect(ids(visible)).toContain("new");
  });

  it("选中或交互不强行挂载折叠祖先内节点，分组保持在普通节点下层", () => {
    const group = createNode(CanvasNodeType.Frame, { x: 0, y: 0 }, "group");
    group.metadata!.frame!.collapsed = true;
    group.zIndex = 10;
    const hidden = node("hidden");
    hidden.parentId = group.id;
    const nodes = [node("visible"), hidden, group];
    const visible = visibleCanvasNodes(
      createRenderIndex(nodes),
      viewport,
      size,
      new Set([hidden.id]),
      nodes,
      { interactingNodeIds: new Set([hidden.id]) },
    );
    expect(ids(visible)).toEqual(["group", "visible"]);
    expect(group.height).toBe(420);
  });
});

describe("画布连线可视区裁剪", () => {
  it("裁掉离屏边，保留两端离屏但横穿视口的连接", () => {
    const nodes = [
      node("left", -2000),
      node("right", 2000),
      node("far", 20000),
      node("farther", 22000),
    ];
    const connections = [
      edge("crossing", "left", "right"),
      edge("far", "far", "farther"),
    ];
    expect(
      visibleCanvasConnections(
        createConnectionRenderIndex(nodes, connections),
        viewport,
        size,
      ).map((item) => item.edge.id),
    ).toEqual(["crossing"]);
  });

  it("折叠多层分组投影到可见祖先，内部边与缺失端点不渲染", () => {
    const group = createNode(CanvasNodeType.Frame, { x: 0, y: 0 }, "group");
    group.metadata!.frame!.collapsed = true;
    const inner = createNode(
      CanvasNodeType.Frame,
      { x: 5000, y: 5000 },
      "inner",
    );
    inner.parentId = group.id;
    const child = node("child", 5500, 5500);
    child.parentId = inner.id;
    const nodes = [group, inner, child, node("target", 800)];
    const visible = visibleCanvasConnections(
      createConnectionRenderIndex(nodes, [
        edge("outside", "child", "target"),
        edge("inside", "child", "inner"),
        edge("missing", "missing", "target"),
      ]),
      viewport,
      size,
    );
    expect(visible).toHaveLength(1);
    expect(visible[0].edge.id).toBe("outside");
    expect(visible[0].from).toBe(group);
    expect(visible[0].to.id).toBe("target");
  });

  it("折叠锚点裁剪使用84px可见高度，不使用420px展开高度", () => {
    const group = createNode(CanvasNodeType.Frame, { x: 0, y: -500 }, "group");
    group.metadata!.frame!.collapsed = true;
    const child = node("child", 0, 0);
    child.parentId = group.id;
    const target = node("target", 800, -700);
    expect(
      visibleCanvasConnections(
        createConnectionRenderIndex(
          [group, child, target],
          [edge("hidden", "child", "target")],
        ),
        viewport,
        size,
      ),
    ).toEqual([]);
  });

  it("选中边和交互节点相关边优先保留，其他离屏边不挂载", () => {
    const nodes = [node("a", 10000), node("b", 12000), node("c", 14000)];
    const connections = [edge("selected", "a", "b"), edge("drag", "b", "c")];
    const index = createConnectionRenderIndex(nodes, connections);
    expect(visibleCanvasConnections(index, viewport, size)).toEqual([]);
    expect(
      visibleCanvasConnections(index, viewport, size, {
        selectedEdgeId: "selected",
        interactingNodeIds: new Set(["c"]),
      }).map((item) => item.edge.id),
    ).toEqual(["selected", "drag"]);
  });

  it("拖动可把原离屏边带入视口，临时投影不修改正式节点或边", () => {
    const nodes = [node("a", 10000), node("b", 12000)];
    const connections = [edge("drag", "a", "b")];
    const before = structuredClone({ nodes, connections });
    const visible = visibleCanvasConnections(
      createConnectionRenderIndex(nodes, connections),
      viewport,
      size,
      {
        dragPreview: { nodeIds: new Set(["a", "b"]), x: -10000, y: 0 },
      },
    );
    expect(visible).toHaveLength(1);
    expect(visible[0].from.position).toEqual({ x: 0, y: 0 });
    expect(visible[0].to.position).toEqual({ x: 2000, y: 0 });
    expect({ nodes, connections }).toEqual(before);
  });

  it("拖动折叠分组时使用组的临时锚点，保留后代对应的外连边", () => {
    const group = createNode(CanvasNodeType.Frame, { x: 10000, y: 0 }, "group");
    group.metadata!.frame!.collapsed = true;
    const child = node("child", 10500);
    child.parentId = group.id;
    const nodes = [group, child, node("target", 800)];
    const visible = visibleCanvasConnections(
      createConnectionRenderIndex(nodes, [edge("drag", "child", "target")]),
      viewport,
      size,
      {
        dragPreview: { nodeIds: new Set([group.id]), x: -10000, y: 0 },
      },
    );
    expect(visible[0].from.id).toBe(group.id);
    expect(visible[0].from.position.x).toBe(0);
    expect(group.position.x).toBe(10000);
    expect(child.position.x).toBe(10500);
  });

  it("连接数量受5000上限约束，最后一条选中边不被预算淘汰", () => {
    const connections = Array.from({ length: 5001 }, (_, i) =>
      edge(String(i), "a", "b"),
    );
    const visible = visibleCanvasConnections(
      createConnectionRenderIndex([node("a"), node("b", 500)], connections),
      viewport,
      size,
      { selectedEdgeId: "5000" },
    );
    expect(visible).toHaveLength(5000);
    expect(visible.map((item) => item.edge.id)).toContain("5000");
    expect(connections).toHaveLength(5001);
  });
});
