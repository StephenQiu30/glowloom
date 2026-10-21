import { describe, expect, it } from "vitest";
import {
  createNode,
  duplicateSelection,
  removeSelection,
  parseDocument,
} from "@/components/canvas/document";
import {
  applyGroupSelection,
  applyUngroupSelection,
  getGroupWrapRect,
} from "@/components/canvas/geometry";

describe("独立画布编辑", () => {
  it("分组、复制和删除保持成员与内部连线一致", () => {
    const a = createNode("text", { x: 100, y: 100 });
    const b = createNode("image", { x: 500, y: 100 });
    const group = createNode("group", { x: 0, y: 0 });
    const rect = getGroupWrapRect([a, b]);
    Object.assign(group, {
      position: { x: rect.x, y: rect.y },
      width: rect.width,
      height: rect.height,
    });
    const grouped = applyGroupSelection(
      new Set([a.id, b.id]),
      [a, b],
      [{ id: "edge", fromNodeId: a.id, toNodeId: b.id }],
      group,
    )!;
    const copied = duplicateSelection(grouped, new Set([group.id]));
    expect(copied.nodes).toHaveLength(3);
    const newGroup = copied.nodes.find((n) => n.type === "group")!;
    expect(
      copied.nodes
        .filter((n) => n.type !== "group")
        .every((n) => n.metadata?.groupId === newGroup.id),
    ).toBe(true);
    expect(copied.connections[0].fromNodeId).not.toBe(a.id);
    expect(removeSelection(grouped, new Set([group.id]))).toEqual({
      nodes: [],
      connections: [],
    });
    const ungrouped = applyUngroupSelection(
      new Set([group.id]),
      grouped.nodes,
      grouped.connections,
    )!;
    expect(ungrouped.nodes).toHaveLength(2);
    expect(ungrouped.nodes.every((n) => !n.metadata?.groupId)).toBe(true);
    expect(ungrouped.connections).toHaveLength(1);
  });
  it("导入拒绝坏坐标、悬空边和未知节点，而不静默丢弃", () => {
    const node = createNode("text", { x: 0, y: 0 });
    const doc = {
      format: "glowloom-canvas",
      version: 1,
      nodes: [node],
      connections: [],
      viewport: { x: 0, y: 0, k: 1 },
    };
    expect(parseDocument(JSON.stringify(doc)).nodes[0]).toEqual(node);
    expect(() =>
      parseDocument(
        JSON.stringify({ ...doc, nodes: [{ ...node, width: -1 }] }),
      ),
    ).toThrow();
    expect(() =>
      parseDocument(
        JSON.stringify({ ...doc, nodes: [{ ...node, type: "config" }] }),
      ),
    ).toThrow();
    expect(() =>
      parseDocument(
        JSON.stringify({
          ...doc,
          connections: [{ id: "e", fromNodeId: node.id, toNodeId: "missing" }],
        }),
      ),
    ).toThrow();
    expect(() =>
      parseDocument(
        JSON.stringify({
          ...doc,
          nodes: [
            {
              ...node,
              type: "image",
              metadata: { content: "javascript:alert(1)" },
            },
          ],
        }),
      ),
    ).toThrow();
  });
});
