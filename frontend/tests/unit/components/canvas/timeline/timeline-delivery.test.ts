import { expect, it } from "vitest";
import { timelineResultGraph } from "@/components/canvas/timeline/timeline-delivery";
import type { CanvasEditorState } from "@/components/canvas/lib/canvas-document-adapter";
const source = "10000000-0000-4000-8000-000000000001";
const assetID = "10000000-0000-4000-8000-000000000002";
const jobID = "10000000-0000-4000-8000-000000000003";
const graph = {
  nodes: [
    {
      id: source,
      type: "timeline",
      title: "时间线 1",
      position: { x: 0, y: 0 },
      width: 1200,
      height: 220,
    },
  ],
  connections: [],
  backgroundMode: "dots",
  showImageInfo: false,
  viewport: { x: 0, y: 0, k: 1 },
} as CanvasEditorState;
it("publishes one stable video node and retries without duplicates", () => {
  const next = timelineResultGraph(graph, source, {
    jobID,
    assetID,
    fileName: "成片",
  });
  expect(next.nodes).toHaveLength(2);
  expect(next.nodes[1].documentData?.fileReferences?.[0].fileId).toBe(assetID);
  expect(next.nodes[1].position.x).toBeGreaterThan(1200);
  expect(
    timelineResultGraph(next, source, { jobID, assetID, fileName: "成片" }),
  ).toBe(next);
});
it("rejects removed source and job identity collisions", () => {
  expect(() =>
    timelineResultGraph({ ...graph, nodes: [] }, source, {
      jobID,
      assetID,
      fileName: "成片",
    }),
  ).toThrow();
  expect(() =>
    timelineResultGraph(
      { ...graph, nodes: [...graph.nodes, { ...graph.nodes[0], id: jobID }] },
      source,
      { jobID, assetID, fileName: "成片" },
    ),
  ).toThrow();
});
