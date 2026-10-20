import { expect, it } from "vitest";
import { createResourceNode } from "../document";
import {
  buildRenderIndex,
  visibleNodes,
  visibleEdges,
  descendants,
  moveSelection,
  connectionPath,
} from "./render-model";

it("2000个密集节点没有渲染截断，离屏查询仍返回全图成员", () => {
  const nodes = Array.from({ length: 2000 }, (_, i) => ({
    ...createResourceNode("text", { x: 0, y: 0 }),
    id: `node-${i}`,
  }));
  const index = buildRenderIndex(nodes);
  expect(
    visibleNodes(index, { x: 0, y: 0, k: 1 }, { width: 1024, height: 768 })
      .length,
  ).toBe(2000);
  const far = { ...nodes[0], id: "far", x: 10000 };
  const sparse = buildRenderIndex([nodes[0], far]);
  expect(
    visibleNodes(sparse, { x: 0, y: 0, k: 1 }, { width: 500, height: 500 }).map(
      (n) => n.id,
    ),
  ).toEqual(["node-0"]);
  expect(
    visibleNodes(
      sparse,
      { x: -10000, y: 0, k: 1 },
      { width: 500, height: 500 },
    ).map((n) => n.id),
  ).toEqual(["far"]);
  expect(
    visibleNodes(
      sparse,
      { x: 0, y: 0, k: 1 },
      { width: 500, height: 500 },
      new Set(["far"]),
    ).map((n) => n.id),
  ).toEqual(["node-0", "far"]);
});
it("负坐标、超大组、边界标题和折叠后代正确查询", () => {
  const group = {
    ...createResourceNode("group", { x: -100000, y: -100000 }),
    width: 100000,
    height: 100000,
  };
  const child = {
    ...createResourceNode("text", { x: 20, y: 20 }),
    parent_id: group.id,
  };
  expect(
    visibleNodes(
      buildRenderIndex([group, child]),
      { x: 0, y: 0, k: 1 },
      { width: 400, height: 400 },
    ).map((n) => n.id),
  ).toContain(group.id);
  const hidden = buildRenderIndex([
    { ...group, config: { collapsed: true } },
    child,
  ]);
  expect(
    visibleNodes(hidden, { x: 0, y: 0, k: 1 }, { width: 400, height: 400 }).map(
      (n) => n.id,
    ),
  ).not.toContain(child.id);
  expect(
    visibleNodes(
      hidden,
      { x: 0, y: 0, k: 1 },
      { width: 400, height: 400 },
      new Set([child.id]),
    ).map((n) => n.id),
  ).toContain(child.id);
});
it("双端离屏但跨越视口的标注线仍渲染", () => {
  const a = createResourceNode("text", { x: -1000, y: 100 });
  const b = createResourceNode("text", { x: 2000, y: 100 });
  const edge = {
    id: "edge",
    edge_type: "annotation",
    source_node_id: a.id,
    target_node_id: b.id,
  };
  const result = visibleEdges(
    [edge],
    new Map([
      [a.id, a],
      [b.id, b],
    ]),
    { x: 0, y: 0, k: 1 },
    { width: 500, height: 500 },
  );
  expect(result).toHaveLength(1);
  expect(result[0].path).toMatch(/^M/);
});

it("反向引用从相邻侧连接，曲线裁剪与实际锚点一致", () => {
  const source = createResourceNode("text", { x: 2000, y: 100 });
  const target = createResourceNode("text", { x: -1000, y: 100 });
  expect(connectionPath(source, target)).toBe(
    "M 2000 260 C 660 260, 660 260, -680 260",
  );
  const result = visibleEdges(
    [{ id: "backward", source_node_id: source.id, target_node_id: target.id }],
    new Map([
      [source.id, source],
      [target.id, target],
    ]),
    { x: 0, y: 0, k: 1 },
    { width: 500, height: 500 },
  );
  expect(result).toHaveLength(1);
  expect(result[0].path).toBe(connectionPath(source, target));
});
it("组及嵌套后代按世界坐标移动一次，锁定成员阻止整组移动", () => {
  const group = createResourceNode("group", { x: 0, y: 0 });
  const nested = {
    ...createResourceNode("group", { x: 10, y: 20 }),
    parent_id: group.id,
  };
  const child = {
    ...createResourceNode("text", { x: 30, y: 40 }),
    parent_id: nested.id,
  };
  const nodes = [group, nested, child];
  expect(descendants(nodes, new Set([group.id])).size).toBe(3);
  expect(
    moveSelection(nodes, new Set([group.id, child.id]), 10, -5).map((n) => [
      n.x,
      n.y,
    ]),
  ).toEqual([
    [10, -5],
    [20, 15],
    [40, 35],
  ]);
  expect(() =>
    moveSelection(
      [group, { ...child, parent_id: group.id, last_operation_id: "locked" }],
      new Set([group.id]),
      10,
      0,
    ),
  ).toThrow();
});

it("历史时间线保留在文档中但不进入画布渲染或固定节点", () => {
  const text = createResourceNode("text", { x: 0, y: 0 });
  const legacy = { ...text, id: "legacy-timeline", node_type: "timeline" };
  const original = [text, legacy];
  const index = buildRenderIndex(original);
  expect(
    visibleNodes(
      index,
      { x: 0, y: 0, k: 1 },
      { width: 800, height: 600 },
      new Set([legacy.id]),
    ).map((node) => node.id),
  ).toEqual([text.id]);
  expect(original).toHaveLength(2);
});
