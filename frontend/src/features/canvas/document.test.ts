import { describe, expect, it } from "vitest";
import {
  applyCommands,
  inverseCommands,
  diffNodes,
  createNode,
  prepareEdit,
} from "./document";
import { CanvasNodeType, type CanvasDocument } from "./model";

const id = (value: number) =>
  `00000000-0000-4000-8000-${String(value).padStart(12, "0")}`;
const document = (): CanvasDocument => ({
  id: id(100),
  projectId: id(101),
  name: "创作画布",
  revision: 1,
  scope: {},
  viewport: { x: 0, y: 0, k: 1 },
  nodes: [
    createNode(CanvasNodeType.Text, { x: 0, y: 0 }, id(1)),
    createNode(CanvasNodeType.Image, { x: 400, y: 0 }, id(2), {
      assetId: id(50),
    }),
  ],
  connections: [],
});
describe("正式画布文档命令", () => {
  it("删除组只解组且保持world坐标，撤销恢复关系", () => {
    const group = createNode(CanvasNodeType.Frame, { x: -20, y: -50 }, id(3));
    const before = document();
    before.nodes.push(group);
    before.nodes[0].parentId = group.id;
    const commands = [{ type: "DeleteNodes" as const, ids: [group.id] }];
    const after = applyCommands(before, commands);
    expect(after.nodes[0].parentId).toBeUndefined();
    expect(after.nodes[0].position).toEqual({ x: 0, y: 0 });
    const restored = applyCommands(after, inverseCommands(before, commands));
    expect(restored.nodes.find((node) => node.id === id(1))?.parentId).toBe(
      group.id,
    );
  });
  it("大量连接删除与撤销采用批量typed命令，不拆分非原子请求", () => {
    const before = document();
    before.connections = Array.from({ length: 150 }, (_, n) => ({
      id: id(n + 1000),
      fromNodeId: id(1),
      toNodeId: id(2),
    }));
    const commands = [{ type: "DeleteNodes" as const, ids: [id(1)] }];
    const inverse = inverseCommands(before, commands);
    expect(inverse.length).toBeLessThan(5);
    expect(
      applyCommands(applyCommands(before, commands), inverse).connections,
    ).toEqual(before.connections);
  });
  it("2,001 个连接恢复分成两个同请求命令且完整撤销", () => {
    const before = document();
    before.connections = Array.from({ length: 2001 }, (_, n) => ({
      id: id(n + 1000),
      fromNodeId: id(1),
      toNodeId: id(2),
    }));
    const commands = [{ type: "DeleteNodes" as const, ids: [id(1)] }];
    const { inverse } = prepareEdit(before, commands);
    const connections = inverse.filter((command) => command.type === "Connect");
    expect(connections.map((command) => command.edges.length)).toEqual([
      2000, 1,
    ]);
    expect(
      applyCommands(applyCommands(before, commands), inverse).connections,
    ).toEqual(before.connections);
  });
  it("保存前拒绝合法 forward 却超过 100 条逆命令的操作", () => {
    const before = document();
    const group = createNode(CanvasNodeType.Frame, { x: 0, y: 0 }, id(3));
    before.nodes = [
      group,
      ...Array.from({ length: 99 }, (_, n) => ({
        ...createNode(CanvasNodeType.Text, { x: n, y: 0 }, id(n + 10)),
        parentId: group.id,
      })),
    ];
    const commands = [
      ...before.nodes.slice(1).map((node) => ({
        type: "UpdateNodeConfig" as const,
        id: node.id,
        config: { text: "new" },
      })),
      { type: "DeleteNodes" as const, ids: [group.id] },
    ];
    expect(applyCommands(before, commands).nodes).toHaveLength(99);
    expect(() => prepareEdit(before, commands)).toThrow(/100.*撤销/);
  });
  it("本地边界遵守命令、单批 item 和最终连接数量限制", () => {
    const before = document();
    expect(() =>
      applyCommands(
        before,
        Array.from({ length: 101 }, () => ({
          type: "SetViewport" as const,
          viewport: before.viewport,
        })),
      ),
    ).toThrow(/100/);
    expect(() =>
      applyCommands(before, [
        {
          type: "MoveNodes",
          moves: Array.from({ length: 2001 }, () => ({
            id: id(1),
            x: 0,
            y: 0,
          })),
        },
      ]),
    ).toThrow(/2,000/);
    before.connections = Array.from({ length: 4000 }, (_, n) => ({
      id: id(n + 1000),
      fromNodeId: id(1),
      toNodeId: id(2),
    }));
    expect(() =>
      applyCommands(before, [
        {
          type: "Connect",
          edges: [{ id: id(6000), fromNodeId: id(1), toNodeId: id(2) }],
        },
      ]),
    ).toThrow(/4,000/);
  });
  it("拒绝越界层次和随后被修正的中间分组循环", () => {
    const before = document();
    expect(() =>
      prepareEdit(before, [
        { type: "SetNodeZIndex", z_indices: [{ id: id(1), z_index: 1000001 }] },
      ]),
    ).toThrow(/层次/);
    const outer = createNode(CanvasNodeType.Frame, { x: 0, y: 0 }, id(3)),
      inner = createNode(CanvasNodeType.Frame, { x: 0, y: 0 }, id(4));
    before.nodes.push(outer, inner);
    expect(() =>
      prepareEdit(before, [
        {
          type: "SetNodeParents",
          parents: [{ id: outer.id, parent_id: inner.id }],
        },
        {
          type: "SetNodeParents",
          parents: [{ id: inner.id, parent_id: outer.id }],
        },
        {
          type: "SetNodeParents",
          parents: [{ id: outer.id, parent_id: null }],
        },
      ]),
    ).toThrow(/循环/);
  });
  it("diff包含移动、改尺寸、改名、分组和层次，服务端revision不由本地递增", () => {
    const before = document();
    const nodes = structuredClone(before.nodes);
    nodes[0] = {
      ...nodes[0],
      title: "新名字",
      position: { x: 60, y: 30 },
      width: 600,
      zIndex: 12,
    };
    const commands = diffNodes(before.nodes, nodes);
    expect(commands.map((command) => command.type)).toEqual(
      expect.arrayContaining([
        "MoveNodes",
        "ResizeNodes",
        "RenameNodes",
        "SetNodeZIndex",
      ]),
    );
    const after = applyCommands(before, commands);
    expect(after.nodes[0]).toMatchObject(nodes[0]);
    expect(after.revision).toBe(1);
  });
  it("拒绝自连接、非真实media asset、越界坐标和分组循环", () => {
    const before = document();
    expect(() =>
      applyCommands(before, [
        {
          type: "Connect",
          edges: [{ id: id(10), fromNodeId: id(1), toNodeId: id(1) }],
        },
      ]),
    ).toThrow();
    expect(() =>
      applyCommands(before, [
        {
          type: "AddNodes",
          nodes: [createNode(CanvasNodeType.Video, { x: 0, y: 0 }, id(4))],
        },
      ]),
    ).toThrow();
    expect(() =>
      applyCommands(before, [
        { type: "MoveNodes", moves: [{ id: id(1), x: Infinity, y: 0 }] },
      ]),
    ).toThrow();
  });
});
