import { describe, expect, it } from "vitest";
import { canvasPortsFor } from "@/components/canvas/constants/canvas-input-ports";
import { createCanvasNode } from "@/components/canvas/lib/canvas-node-factory";
import {
  connectCanvasDocumentSources,
  supportsCanvasConnection,
} from "@/components/canvas/lib/canvas-document-connections";

function graph(types: string[]) {
  return {
    nodes: types.map((type) => createCanvasNode(type, { x: 0, y: 0 })),
    connections: [],
    backgroundMode: "dots" as const,
    showImageInfo: false,
    viewport: { x: 0, y: 0, k: 1 },
  };
}

describe("connection authoring capability", () => {
  it.each([
    ["text", "audio"],
    ["video", "image"],
    ["audio", "image"],
  ])(
    "rejects newly authored %s to %s without invalidating the legacy wire port",
    (source, target) => {
      expect(canvasPortsFor(source, target).length).toBeGreaterThan(0);
      expect(supportsCanvasConnection(source, target)).toBe(false);
      const original = graph([source, target]);
      expect(() =>
        connectCanvasDocumentSources(
          original,
          [original.nodes[0].id],
          original.nodes[1].id,
        ),
      ).toThrow("无法连接这些节点");
      expect(original.connections).toEqual([]);
    },
  );

  it("rejects the complete mixed batch when only one source is compatible", () => {
    const original = graph(["image", "video", "image"]);
    const before = structuredClone(original);
    expect(() =>
      connectCanvasDocumentSources(
        original,
        original.nodes.slice(0, 2).map((node) => node.id),
        original.nodes[2].id,
      ),
    ).toThrow("无法连接这些节点");
    expect(original).toEqual(before);
  });

  it("rejects a batch crossing audio capacity without adding the valid prefix", () => {
    const initial = graph([...Array<string>(8).fill("image"), "audio"]);
    const target = initial.nodes[8].id;
    const six = connectCanvasDocumentSources(
      initial,
      initial.nodes.slice(0, 6).map((node) => node.id),
      target,
    );
    const before = structuredClone(six);
    expect(() =>
      connectCanvasDocumentSources(
        six,
        initial.nodes.slice(6, 8).map((node) => node.id),
        target,
      ),
    ).toThrow("引用数量已达上限");
    expect(six).toEqual(before);
    const seven = connectCanvasDocumentSources(
      six,
      [initial.nodes[6].id],
      target,
    );
    expect(seven.connections).toHaveLength(7);
  });

  it("counts unique sources rather than physical file edges", () => {
    const initial = graph([...Array<string>(7).fill("image"), "audio"]);
    const source = initial.nodes[0];
    source.documentData = {
      id: source.id,
      type: source.type,
      nodeTypeVersion: 1,
      title: source.title,
      position: source.position,
      width: source.width,
      height: source.height,
      zIndex: 0,
      fileReferences: [1, 2].map((index) => ({
        purpose: "CONTENT",
        referenceKey: `file-${index}`,
        fileId: crypto.randomUUID(),
        version: "1",
      })),
    };
    const six = connectCanvasDocumentSources(
      initial,
      initial.nodes.slice(0, 6).map((node) => node.id),
      initial.nodes[7].id,
    );
    expect(six.connections).toHaveLength(7);
    expect(
      connectCanvasDocumentSources(
        six,
        [initial.nodes[6].id],
        initial.nodes[7].id,
      ).connections,
    ).toHaveLength(8);
  });
});
