import { admitCanvasConnectionGraph } from "@/components/canvas/types/canvas-document-integration";
import type { CanvasEditorState } from "@/components/canvas/lib/canvas-document-adapter";
import { expect, it, vi } from "vitest";
import { commitCanvasGraphResult } from "@/components/canvas/lib/canvas-graph-commit";
import type {
  CanvasNodeData,
  CanvasConnection,
} from "@/components/canvas/types/canvas";

const node: CanvasNodeData = {
  id: "source",
  type: "text",
  title: "文本",
  position: { x: 0, y: 0 },
  width: 320,
  height: 320,
};

it("rejects the complete graph without applying nodes when only nodes would fit the admission limit", () => {
  const before = { nodes: [node], connections: [] as CanvasConnection[] };
  const result = {
    nodes: [
      node,
      { ...node, id: "created", metadata: { content: "x".repeat(1200) } },
    ],
    connections: [0, 1].map((index) => ({
      id: String(index) + "x".repeat(400),
      fromNodeId: "source",
      toNodeId: "created",
    })),
  };
  // Same size-based predicate as the live session, using a small bound for this fixture.
  const fits = (graph: typeof before) =>
    new TextEncoder().encode(JSON.stringify(graph)).byteLength < 2048;
  expect(fits({ nodes: result.nodes, connections: before.connections })).toBe(
    true,
  );
  expect(fits(result)).toBe(false);
  let visible = before;
  const commit = vi.fn((graph: typeof before) => {
    visible = graph;
  });
  const admit = vi.fn(fits);
  expect(commitCanvasGraphResult(result, admit, commit)).toBe(false);
  expect(admit).toHaveBeenCalledTimes(1);
  expect(admit).toHaveBeenCalledWith(result);
  expect(commit).not.toHaveBeenCalled();
  expect(visible).toBe(before);
});

it("commits exactly one complete result after acceptance, preserving both collection identities", () => {
  const result = {
    nodes: [node],
    connections: [{ id: "edge", fromNodeId: "source", toNodeId: "target" }],
  };
  const order: string[] = [];
  const admit = vi.fn(() => {
    order.push("admit");
    return true;
  });
  const commit = vi.fn((graph) => {
    order.push("commit");
    expect(graph.nodes).toBe(result.nodes);
    expect(graph.connections).toBe(result.connections);
  });
  expect(commitCanvasGraphResult(result, admit, commit)).toBe(true);
  expect(order).toEqual(["admit", "commit"]);
  expect(commit).toHaveBeenCalledTimes(1);
});

it("does not expose any graph changes when admission throws", () => {
  const result = { nodes: [node], connections: [] };
  const commit = vi.fn();
  expect(() =>
    commitCanvasGraphResult(
      result,
      () => {
        throw Error("admission unavailable");
      },
      commit,
    ),
  ).toThrow("admission unavailable");
  expect(commit).not.toHaveBeenCalled();
});

it("connection integration prioritizes full-candidate admission and never falls back after a rejection", () => {
  const initialState = {
    nodes: [node],
    connections: [],
    backgroundMode: "dots",
    showImageInfo: false,
    viewport: { x: 10, y: 20, k: 0.71 },
  } as CanvasEditorState;
  const result = {
    nodes: [node, { ...node, id: "new-target" }],
    connections: [{ id: "edge", fromNodeId: node.id, toNodeId: "new-target" }],
  };
  const admit = vi.fn(() => true);
  const admitConnectionGraph = vi.fn(() => false);
  const integration = {
    initialState,
    frozen: false,
    admit,
    admitConnectionGraph,
  };
  const commit = vi.fn();
  expect(
    commitCanvasGraphResult(
      result,
      (candidate) => admitCanvasConnectionGraph(integration, candidate),
      commit,
    ),
  ).toBe(false);
  expect(admitConnectionGraph).toHaveBeenCalledWith({
    ...initialState,
    ...result,
  });
  expect(admit).not.toHaveBeenCalled();
  expect(commit).not.toHaveBeenCalled();
  admitConnectionGraph.mockReturnValue(true);
  expect(
    commitCanvasGraphResult(
      result,
      (candidate) => admitCanvasConnectionGraph(integration, candidate),
      commit,
    ),
  ).toBe(true);
  expect(commit).toHaveBeenCalledExactlyOnceWith(result);
});

it("connection integration keeps legacy/standalone behavior while rejecting frozen or invalid candidates", () => {
  const initialState = {
    nodes: [node],
    connections: [],
  } as unknown as CanvasEditorState;
  const result = { nodes: [node], connections: [] };
  const admit = vi.fn(() => true);
  expect(admitCanvasConnectionGraph(undefined, result)).toBe(true);
  expect(
    admitCanvasConnectionGraph({ initialState, frozen: false, admit }, result),
  ).toBe(true);
  expect(admit).toHaveBeenCalledExactlyOnceWith(initialState);
  expect(
    admitCanvasConnectionGraph({ initialState, frozen: true, admit }, result),
  ).toBe(false);
  expect(admit).toHaveBeenCalledTimes(1);
  expect(
    admitCanvasConnectionGraph(
      {
        initialState,
        frozen: false,
        admitConnectionGraph: () => {
          throw Error("invalid projection");
        },
      },
      result,
    ),
  ).toBe(false);
});
