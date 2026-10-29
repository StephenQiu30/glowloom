import { describe, expect, it } from "vitest";
import { snapshotCanvasConnectionSelection } from "@/components/canvas/lib/canvas-connection-selection";

describe("connection gesture selection snapshot", () => {
  it.each([
    [1, "a"],
    [2, "a"],
    [3, "b"],
    [4, "b"],
    [5, "c"],
  ])("anchors %i eligible sources at the lower median %s", (count, anchor) => {
    const nodes = ["a", "b", "c", "d", "e"]
      .slice(0, Number(count))
      .map((id) => ({ id, type: "text" }));
    const snapshot = snapshotCanvasConnectionSelection(
      nodes,
      new Set(nodes.map((node) => node.id).reverse()),
    );
    expect(snapshot?.anchorNodeId).toBe(anchor);
    expect(snapshot?.sourceNodeIds).toEqual(nodes.map((node) => node.id));
  });

  it("keeps unsupported selected sources while choosing the anchor only among eligible outputs", () => {
    const nodes = [
      { id: "timeline", type: "timeline" },
      { id: "a", type: "text" },
      { id: "director", type: "director" },
      { id: "b", type: "image" },
      { id: "group", type: "group" },
      { id: "c", type: "video" },
    ];
    expect(
      snapshotCanvasConnectionSelection(
        nodes,
        new Set(nodes.map((node) => node.id)),
      ),
    ).toEqual({
      anchorNodeId: "b",
      sourceNodeIds: nodes.map((node) => node.id),
    });
  });

  it("uses render order rather than selection insertion order and excludes unselected nodes", () => {
    const nodes = [
      { id: "a", type: "text" },
      { id: "b", type: "image" },
      { id: "c", type: "video" },
      { id: "d", type: "text" },
    ];
    expect(
      snapshotCanvasConnectionSelection(
        nodes,
        new Set(["d", "missing", "b", "a"]),
      ),
    ).toEqual({ anchorNodeId: "b", sourceNodeIds: ["a", "b", "d"] });
  });

  it("has no usable gesture when nothing selected exposes an output", () => {
    expect(snapshotCanvasConnectionSelection([], new Set())).toBeNull();
    expect(
      snapshotCanvasConnectionSelection([{ id: "a", type: "text" }], new Set()),
    ).toBeNull();
    expect(
      snapshotCanvasConnectionSelection(
        [
          { id: "a", type: "timeline" },
          { id: "b", type: "unknown" },
        ],
        new Set(["a", "b"]),
      ),
    ).toBeNull();
  });

  it("detaches the source snapshot from later selection and node mutations", () => {
    const nodes = [
      { id: "a", type: "text" },
      { id: "b", type: "video" },
    ];
    const selected = new Set(["a", "b"]);
    const snapshot = snapshotCanvasConnectionSelection(nodes, selected);
    selected.clear();
    nodes.reverse();
    nodes[0].id = "changed";
    expect(snapshot).toEqual({ anchorNodeId: "a", sourceNodeIds: ["a", "b"] });
  });
});
