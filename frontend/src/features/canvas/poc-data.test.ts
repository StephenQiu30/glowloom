import { expect, it } from "vitest";

import { makePocGraph } from "./poc-data";

it.each([
  [500, 600],
  [500, 800],
  [2000, 3200],
])("builds a stable %i-node, %i-edge graph", (nodeCount, edgeCount) => {
  const first = makePocGraph(nodeCount, edgeCount);
  const second = makePocGraph(nodeCount, edgeCount);

  expect(first).toEqual(second);
  expect(first.nodes).toHaveLength(nodeCount);
  expect(first.edges).toHaveLength(edgeCount);
  expect(new Set(first.nodes.map((node) => node.id)).size).toBe(nodeCount);
  const nodeIDs = new Set(first.nodes.map((node) => node.id));
  expect(
    first.edges.every(
      (edge) => nodeIDs.has(edge.source) && nodeIDs.has(edge.target),
    ),
  ).toBe(true);
  expect(first.nodes.filter((node) => node.data.kind === "image")).toHaveLength(
    Math.round(nodeCount * 0.6),
  );
  expect(first.nodes.filter((node) => node.data.kind === "video")).toHaveLength(
    Math.round(nodeCount * 0.08),
  );
  expect(first.nodes.filter((node) => node.data.playing)).toHaveLength(3);
});
