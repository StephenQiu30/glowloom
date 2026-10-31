import { expect, it } from "vitest";
import { classifyCanvasRemoteApplication } from "@/components/canvas/lib/canvas-remote-application";
import { changeCanvasColorTags } from "@/components/canvas/lib/canvas-color-tags";
import { createCanvasNode } from "@/components/canvas/lib/canvas-node-factory";

it("does not keep swallowing color changes when a preview replaced the remote array before effects settle", () => {
  const previous = {
    nodes: [createCanvasNode("text", { x: 0, y: 0 })],
    connections: [],
  };
  const remote = {
    nodes: previous.nodes.map((n) => ({ ...n, title: "Remote title" })),
    connections: previous.connections,
    previous,
  };
  expect(classifyCanvasRemoteApplication(remote, previous)).toBe("waiting");
  const preview = {
    nodes: remote.nodes.map((n) => ({
      ...n,
      metadata: { ...n.metadata, status: "success" as const },
    })),
    connections: remote.connections,
  };
  const selected = new Set([preview.nodes[0]!.id]);
  const edited = {
    ...preview,
    nodes: changeCanvasColorTags(preview.nodes, selected, "cyan"),
  };
  expect(classifyCanvasRemoteApplication(remote, edited)).toBe("local");
  expect(edited.nodes[0]?.title).toBe("Remote title");
  expect(edited.nodes[0]?.metadata?.colorTags).toEqual(["cyan"]);
});
it("suppresses an exact remote echo but admits a local connection edit", () => {
  const previous = {
    nodes: [createCanvasNode("text", { x: 0, y: 0 })],
    connections: [],
  };
  const remote = { ...previous, nodes: [...previous.nodes], previous };
  expect(classifyCanvasRemoteApplication(remote, remote)).toBe("echo");
  expect(
    classifyCanvasRemoteApplication(remote, {
      nodes: remote.nodes,
      connections: [...remote.connections],
    }),
  ).toBe("local");
});
