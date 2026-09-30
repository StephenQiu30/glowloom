import { expect, it } from "vitest";

import { makePocGraph, mediaSelectionError } from "./poc-data";

it("rejects unsupported or oversized local media before creating URLs", () => {
  expect(mediaSelectionError([{ type: "image/png", size: 100 }])).toBeNull();
  expect(
    mediaSelectionError([{ type: "application/pdf", size: 100 }]),
  ).not.toBeNull();
  expect(
    mediaSelectionError(
      Array.from({ length: 41 }, () => ({ type: "image/png", size: 100 })),
    ),
  ).not.toBeNull();
  expect(
    mediaSelectionError([{ type: "video/mp4", size: 101 * 1024 * 1024 }]),
  ).not.toBeNull();
  expect(
    mediaSelectionError(
      Array.from({ length: 4 }, () => ({
        type: "video/mp4",
        size: 80 * 1024 * 1024,
      })),
    ),
  ).not.toBeNull();
});

it("uses distinct selected media without changing graph composition", () => {
  const media = [
    { kind: "image" as const, url: "blob:image-a" },
    { kind: "image" as const, url: "blob:image-b" },
    { kind: "video" as const, url: "blob:video-a" },
    { kind: "video" as const, url: "blob:video-b" },
  ];
  const graph = makePocGraph(500, 800, media);
  expect(
    new Set(
      graph.nodes
        .filter((node) => node.data.kind === "image")
        .map((node) => node.data.src),
    ),
  ).toEqual(new Set(["blob:image-a", "blob:image-b"]));
  expect(
    new Set(
      graph.nodes
        .filter((node) => node.data.kind === "video")
        .map((node) => node.data.src),
    ),
  ).toEqual(new Set(["blob:video-a", "blob:video-b"]));
  expect(graph.nodes.filter((node) => node.data.playing)).toHaveLength(3);
});

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
