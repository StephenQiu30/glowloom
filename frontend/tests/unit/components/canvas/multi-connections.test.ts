import { describe, expect, it } from "vitest";
import { createCanvasNode } from "@/components/canvas/lib/canvas-node-factory";
import {
  connectCanvasDocument,
  connectCanvasDocumentSources,
} from "@/components/canvas/lib/canvas-document-connections";

describe("multi-selection connections", () => {
  function fixture() {
    const nodes = ["text", "text", "image"].map((type) =>
      createCanvasNode(type, { x: 0, y: 0 }),
    );
    return {
      nodes,
      connections: [],
      backgroundMode: "dots" as const,
      showImageInfo: false,
      viewport: { x: 0, y: 0, k: 1 },
    };
  }
  it("connects all selected sources in one immutable result", () => {
    const original = fixture();
    const before = structuredClone(original);
    const result = connectCanvasDocumentSources(
      original,
      original.nodes.slice(0, 2).map((n) => n.id),
      original.nodes[2].id,
    );
    expect(original).toEqual(before);
    expect(result.nodes).toHaveLength(3);
    expect(result.connections).toHaveLength(2);
    expect(result.connections.map((e) => e.fromNodeId)).toEqual(
      original.nodes.slice(0, 2).map((n) => n.id),
    );
    expect(
      result.connections.every((e) => e.toNodeId === original.nodes[2].id),
    ).toBe(true);
  });
  it("does not retain the first edge when a later source is invalid", () => {
    const original = fixture();
    original.nodes[1] = createCanvasNode("group", { x: 0, y: 0 });
    const before = structuredClone(original);
    expect(() =>
      connectCanvasDocumentSources(
        original,
        original.nodes.slice(0, 2).map((n) => n.id),
        original.nodes[2].id,
      ),
    ).toThrow("这两种节点之间没有可用的输入用途");
    expect(original).toEqual(before);
  });
  it("rejects repeated source IDs instead of silently reducing the selection", () => {
    const original = fixture();
    const before = structuredClone(original);
    expect(() =>
      connectCanvasDocumentSources(
        original,
        [original.nodes[0].id, original.nodes[0].id],
        original.nodes[2].id,
      ),
    ).toThrow("来源节点重复");
    expect(original).toEqual(before);
  });
  it("rejects the entire batch when a fresh first source is followed by an existing relation", () => {
    const fixtureState = fixture();
    const original = connectCanvasDocument(
      fixtureState,
      fixtureState.nodes[1].id,
      fixtureState.nodes[2].id,
    );
    const before = structuredClone(original);
    expect(() =>
      connectCanvasDocumentSources(
        original,
        original.nodes.slice(0, 2).map((node) => node.id),
        original.nodes[2].id,
      ),
    ).toThrow("已经关联");
    expect(original).toEqual(before);
    expect(original.connections).toHaveLength(1);
  });
});
