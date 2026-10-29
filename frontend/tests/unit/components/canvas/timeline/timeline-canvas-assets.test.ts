import { expect, it } from "vitest";
import { timelineCanvasAssets } from "@/components/canvas/timeline/timeline-canvas-assets";
import { timelineResultGraph } from "@/components/canvas/timeline/timeline-delivery";
import type { CanvasNodeData } from "@/components/canvas/types/canvas";
import type { CanvasEditorState } from "@/components/canvas/lib/canvas-document-adapter";
const target = "10000000-0000-4000-8000-000000000001";
const asset = "10000000-0000-4000-8000-000000000002";
const result = "10000000-0000-4000-8000-000000000003";
const video = {
  id: result,
  type: "video",
  title: "成片",
  position: { x: 0, y: 0 },
  width: 420,
  height: 236,
} as CanvasNodeData;
it("includes newly delivered pinned files and saved native media_ref nodes in timeline canvas assets", () => {
  const graph = {
    nodes: [{ ...video, id: target, type: "timeline" }],
    connections: [],
  } as unknown as CanvasEditorState;
  const delivered = timelineResultGraph(graph, target, {
    jobID: result,
    assetID: asset,
    fileName: "成片",
  });
  const expected = [
    { nodeID: result, assetID: asset, title: "成片", kind: "video" },
  ];
  expect(timelineCanvasAssets(delivered.nodes, target)).toEqual(expected);
  expect(
    timelineCanvasAssets(
      [{ ...video, serviceData: { media_ref: { asset_id: asset } } }],
      target,
    ),
  ).toEqual(expected);
  expect(
    timelineCanvasAssets(
      [{ ...video, serviceData: { ref_type: "media_asset", ref_id: asset } }],
      target,
    ),
  ).toEqual(expected);
});
it("rejects removed, ambiguous, local-only, failed and wrong-kind sources while resolving replacements by pinned identity", () => {
  const ref = {
    referenceKey: asset,
    fileId: asset,
    version: "1",
    purpose: "CONTENT",
  };
  const pinned = {
    ...video,
    documentData: { fileReferences: [ref] },
  } as CanvasNodeData;
  expect(timelineCanvasAssets([{ ...pinned, id: target }], target)).toEqual([]);
  expect(
    timelineCanvasAssets(
      [{ ...video, serviceData: { media_ref: { asset_id: "invalid" } } }],
      target,
    ),
  ).toEqual([]);
  expect(
    timelineCanvasAssets(
      [{ ...pinned, metadata: { content: "blob:pending" } }],
      target,
    ),
  ).toEqual([]);
  expect(
    timelineCanvasAssets(
      [{ ...pinned, metadata: { status: "error" } }],
      target,
    ),
  ).toEqual([]);
  expect(timelineCanvasAssets([{ ...pinned, type: "text" }], target)).toEqual(
    [],
  );
  expect(
    timelineCanvasAssets(
      [
        {
          ...pinned,
          documentData: {
            ...pinned.documentData!,
            fileReferences: [
              ref,
              ref,
            ] as CanvasContract.CanvasFileReferenceDTO[],
          },
        },
      ],
      target,
    ),
  ).toEqual([]);
});
