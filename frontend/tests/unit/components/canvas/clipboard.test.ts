import { describe, it, expect } from "vitest";
import { createCanvasNode } from "@/components/canvas/lib/canvas-node-factory";
import {
  captureCanvasSelection,
  instantiateCanvasClipboard,
} from "@/components/canvas/lib/canvas-clipboard";

describe("canvas clipboard snapshots", () => {
  it("captures nested descendants and internal edges, then remaps every identity without changing the original", () => {
    const group = createCanvasNode("group", { x: 0, y: 0 });
    const inner = createCanvasNode(
      "group",
      { x: 20, y: 20 },
      { groupId: group.id },
    );
    const child = createCanvasNode(
      "text",
      { x: 40, y: 40 },
      { groupId: inner.id, colorTags: ["cyan"] },
    );
    child.position = { x: 40, y: 40 };
    const external = createCanvasNode("image", { x: 100, y: 0 });
    const edges = [
      { id: "inside", fromNodeId: inner.id, toNodeId: child.id },
      { id: "outside", fromNodeId: external.id, toNodeId: child.id },
    ];
    const snapshot = captureCanvasSelection(
      [group, inner, child, external],
      edges,
      new Set([group.id]),
    );
    expect(snapshot.nodes).toHaveLength(3);
    expect(snapshot.connections).toHaveLength(1);
    child.metadata!.colorTags!.push("purple");
    let sequence = 0;
    const result = instantiateCanvasClipboard(
      snapshot,
      { x: 36, y: 36 },
      () => `new-${++sequence}`,
    );
    expect(result.nodes[1].metadata?.groupId).toBe(result.nodes[0].id);
    expect(result.nodes[2].metadata?.groupId).toBe(result.nodes[1].id);
    expect(result.connections[0]).toMatchObject({
      fromNodeId: result.nodes[1].id,
      toNodeId: result.nodes[2].id,
    });
    expect(result.nodes[2].metadata?.colorTags).toEqual(["cyan"]);
    expect(snapshot.nodes[2].position).toEqual({ x: 40, y: 40 });
    expect(result.nodes[2].position).toEqual({ x: 76, y: 76 });
    expect(snapshot.nodes[0].id).toBe(group.id);
  });
  it("detaches a copied child from an unselected parent and keeps subject pins independent", () => {
    const subject = createCanvasNode(
      "subject",
      { x: 10, y: 20 },
      { groupId: "outside" },
    );
    subject.serviceData!.config!.subject = {
      version: 1,
      subject_id: "fixed",
      version_id: "version",
    };
    const snapshot = captureCanvasSelection(
      [subject],
      [],
      new Set([subject.id]),
    );
    const result = instantiateCanvasClipboard(snapshot, { x: 20, y: 30 });
    expect(result.nodes[0].metadata?.groupId).toBeUndefined();
    expect(result.nodes[0].serviceData?.config?.subject).toEqual(
      subject.serviceData!.config!.subject,
    );
    result.nodes[0].serviceData!.config!.subject!.subject_id = "different";
    expect(snapshot.nodes[0].serviceData?.config?.subject?.subject_id).toBe(
      "fixed",
    );
  });
  it("rebinds copied timeline clips to copied media while retaining external and fixed references", () => {
    const image = createCanvasNode("image", { x: 0, y: 0 });
    const timeline = createCanvasNode("timeline", { x: 400, y: 0 });
    timeline.serviceData!.config!.timeline!.clips = [
      { id: "clip-internal", node_id: image.id, asset_id: "fixed-asset" },
      {
        id: "clip-external",
        node_id: "external-node",
        asset_id: "external-asset",
      },
    ];
    const copy = instantiateCanvasClipboard(
      captureCanvasSelection(
        [image, timeline],
        [],
        new Set([image.id, timeline.id]),
      ),
      { x: 36, y: 36 },
    );
    expect(copy.nodes[1].serviceData!.config!.timeline!.clips).toEqual([
      {
        id: "clip-internal",
        node_id: copy.nodes[0].id,
        asset_id: "fixed-asset",
      },
      {
        id: "clip-external",
        node_id: "external-node",
        asset_id: "external-asset",
      },
    ]);
    expect(timeline.serviceData!.config!.timeline!.clips![0].node_id).toBe(
      image.id,
    );
  });
});
