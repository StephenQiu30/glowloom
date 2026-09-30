import { describe, expect, it } from "vitest";

import {
  applyCommands,
  createEditor,
  edit,
  redo,
  resetEditor,
  undo,
  type CanvasDocument,
} from "./live-state";

const document: CanvasDocument = {
  id: "canvas-1",
  project_id: "project-1",
  name: "备注画布",
  scope: {},
  revision: 1,
  viewport: { x: 0, y: 0, zoom: 1 },
  nodes: [
    {
      id: "a",
      node_type: "text",
      node_action: "resource",
      config: { text: "原文" },
      x: 0,
      y: 0,
    },
    {
      id: "b",
      node_type: "text",
      node_action: "resource",
      config: { text: "备注" },
      x: 200,
      y: 0,
    },
  ],
  edges: [
    {
      id: "ab",
      edge_type: "annotation",
      source_node_id: "a",
      target_node_id: "b",
    },
  ],
};

describe("真实画布本页命令", () => {
  it("删除后撤销恢复备注和关联注释线，重做再次删除", () => {
    const removed = edit(createEditor(document), [
      { type: "DeleteNodes", ids: ["a"] },
    ]);
    expect(removed.document.nodes.map((node) => node.id)).toEqual(["b"]);
    expect(removed.document.edges).toEqual([]);
    const restored = undo(removed);
    expect(restored.document.nodes).toHaveLength(2);
    expect(restored.document.edges).toEqual(document.edges);
    expect(redo(restored).document.nodes.map((node) => node.id)).toEqual(["b"]);
    expect(document.nodes).toHaveLength(2);
  });

  it("移动和视口撤销形成反向命令，保留服务端 revision", () => {
    const moved = edit(createEditor(document), [
      { type: "MoveNodes", moves: [{ id: "a", x: 20, y: 30 }] },
      { type: "SetViewport", viewport: { x: 50, y: 60, zoom: 0.5 } },
    ]);
    const restored = undo(moved);
    expect(restored.document.nodes[0].x).toBe(0);
    expect(restored.document.viewport).toEqual(document.viewport);
    expect(restored.document.revision).toBe(1);
    expect(restored.future).toHaveLength(1);
  });

  it("冲突后读取最新文档并清撤销历史，保护其他用户的新备注", () => {
    const pending = edit(createEditor(document), [
      { type: "UpdateNodeConfig", id: "a", config: { text: "本页修改" } },
    ]);
    const latest = {
      ...document,
      revision: 4,
      nodes: [...document.nodes, { ...document.nodes[1], id: "c" }],
    };
    const rebased = resetEditor(pending, latest);
    expect(rebased.document.revision).toBe(4);
    expect(rebased.document.nodes).toHaveLength(3);
    expect(rebased.document.nodes[0].config.text).toBe("原文");
    expect(rebased.past).toHaveLength(0);
  });

  it("目标已被删除的修改拒绝重放，不伪造保存成功", () => {
    const pending = edit(createEditor(document), [
      { type: "MoveNodes", moves: [{ id: "a", x: 2, y: 3 }] },
    ]);
    expect(() =>
      applyCommands(
        { ...document, revision: 2, nodes: [document.nodes[1]], edges: [] },
        pending.past[0].forward,
      ),
    ).toThrow("备注已不存在");
  });

  it("拒绝自连、重复ID和不合法坐标", () => {
    expect(() =>
      applyCommands(document, [
        {
          type: "Connect",
          edge: {
            id: "self",
            edge_type: "annotation",
            source_node_id: "a",
            target_node_id: "a",
          },
        },
      ]),
    ).toThrow();
    expect(() =>
      applyCommands(document, [
        { type: "AddNodes", nodes: [document.nodes[0]] },
      ]),
    ).toThrow();
    expect(() =>
      applyCommands(document, [
        { type: "MoveNodes", moves: [{ id: "a", x: Number.NaN, y: 0 }] },
      ]),
    ).toThrow();
  });
});
