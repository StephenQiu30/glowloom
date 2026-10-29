import { expect, it } from "vitest";
import { createCanvasNode } from "@/components/canvas/lib/canvas-node-factory";
import {
  connectCanvasTimelineSources,
  reconcileTimelineConnections,
} from "@/components/canvas/lib/canvas-timeline-connections";
import { removeCanvasReferences } from "@/components/canvas/lib/canvas-reference-state";
import type { CanvasEditorState } from "@/components/canvas/lib/canvas-document-adapter";
import {
  serviceDocumentFromEditor,
  serviceGraph,
} from "@/components/canvas/service-graph";

function fixture() {
  const projectID = crypto.randomUUID();
  const nodes = ["image", "video", "audio"].map((kind) => {
    const node = createCanvasNode(kind as "image" | "video" | "audio", {
      x: 0,
      y: 0,
    });
    const asset: API.AssetSummary = {
      id: crypto.randomUUID(),
      project_id: projectID,
      revision: 2,
      kind,
      file_name: kind,
      duration_ms: kind === "image" ? undefined : 7200,
      width: 1920,
      height: 1080,
    };
    node.documentData = {
      id: node.id,
      type: node.type,
      title: node.title,
      position: node.position,
      width: node.width,
      height: node.height,
      zIndex: 1,
      nodeTypeVersion: 1,
      parameters: { mode: "MANUAL" },
      fileReferences: [
        {
          purpose: "CONTENT",
          referenceKey: asset.id!,
          fileId: asset.id!,
          version: "2",
        },
      ],
    };
    return { node, asset };
  });
  const target = createCanvasNode("timeline", { x: 700, y: 0 });
  const state: CanvasEditorState = {
    nodes: [...nodes.map((n) => n.node), target],
    connections: [],
    backgroundMode: "dots",
    showImageInfo: false,
    viewport: { x: 0, y: 0, k: 1 },
  };
  const options = {
    projectID,
    assets: new Map(nodes.map(({ asset }) => [asset.id!, asset])),
  };
  const ids = nodes.map(({ node }) => node.id);
  return { state, target, options, ids, nodes };
}
it("creates all timeline clips and native relations in one immutable graph and roundtrips service projection", () => {
  const f = fixture(),
    before = structuredClone(f.state);
  const next = connectCanvasTimelineSources(
    f.state,
    f.ids,
    f.target.id,
    f.options,
  );
  const timeline = next.nodes.at(-1)!.serviceData!.config!.timeline!;
  expect(
    timeline.clips!.map((clip) => [clip.node_id, clip.duration_ms]),
  ).toEqual(f.ids.map((id, i) => [id, i === 0 ? 5000 : 7200]));
  expect(next.connections).toHaveLength(3);
  expect(
    next.connections.every(
      (edge) =>
        !edge.documentData && edge.serviceData?.edge_type === "annotation",
    ),
  ).toBe(true);
  expect(f.state).toEqual(before);
  const base: API.DocumentDTO = {
    id: crypto.randomUUID(),
    project_id: f.options.projectID,
    revision: "1",
    schema_version: 1,
    operation_version: 1,
    permissions: { read: true, write: true },
    nodes: [],
    edges: [],
  };
  const dto = serviceDocumentFromEditor(base, next);
  const reopened = serviceGraph(dto, base.id!, base.project_id!);
  expect(reopened.connections.map((edge) => edge.id)).toEqual(
    next.connections.map((edge) => edge.id),
  );
  expect(reopened.nodes.at(-1)?.serviceData?.config?.timeline?.clips).toEqual(
    timeline.clips,
  );
});
it.each([
  "missing",
  "revision",
  "project",
  "duration",
  "pending",
  "loading",
  "error",
  "duplicate",
  "count",
])("rejects entire timeline batch when %s fails", (reason) => {
  const f = fixture();
  if (reason === "missing") f.options.assets.delete(f.nodes[1].asset.id!);
  if (reason === "revision") f.nodes[1].asset.revision = 3;
  if (reason === "project") f.nodes[1].asset.project_id = crypto.randomUUID();
  if (reason === "duration") f.nodes[1].asset.duration_ms = undefined;
  if (reason === "pending")
    f.state.nodes[1].metadata = { content: "blob:pending" };
  if (reason === "loading" || reason === "error")
    f.state.nodes[1].metadata = { status: reason };
  if (reason === "duplicate") f.ids.push(f.ids[0]);
  if (reason === "count")
    f.ids.push(...Array.from({ length: 50 }, () => crypto.randomUUID()));
  const before = structuredClone(f.state);
  expect(() =>
    connectCanvasTimelineSources(f.state, f.ids, f.target.id, f.options),
  ).toThrow();
  expect(f.state).toEqual(before);
});
it("rejects duplicate legacy clip sources even when no visual edge was stored", () => {
  const f = fixture();
  const next = connectCanvasTimelineSources(
    f.state,
    f.ids,
    f.target.id,
    f.options,
  );
  expect(() =>
    connectCanvasTimelineSources(
      { ...next, connections: [] },
      [f.ids[0]],
      f.target.id,
      f.options,
    ),
  ).toThrow();
});
it("disconnecting or deleting a source detaches node bindings while preserving clip media", () => {
  const f = fixture();
  const next = connectCanvasTimelineSources(
    f.state,
    f.ids,
    f.target.id,
    f.options,
  );
  for (const removal of [
    { connectionIds: new Set([next.connections[0].id]) },
    { nodeIds: new Set([f.ids[0]]) },
  ]) {
    const after = removeCanvasReferences(next.nodes, next.connections, removal);
    const clip = after.nodes.find((node) => node.id === f.target.id)!
      .serviceData!.config!.timeline!.clips![0];
    expect(clip.node_id).toBeUndefined();
    expect(clip.asset_id).toBe(f.nodes[0].asset.id);
    expect(after.connections).toHaveLength(2);
  }
});
it("removing the last clip reconciles its visual relation but preserves unrelated annotations", () => {
  const f = fixture(),
    next = connectCanvasTimelineSources(f.state, f.ids, f.target.id, f.options);
  const target = next.nodes.at(-1)!;
  target.serviceData!.config!.timeline!.clips!.shift();
  const after = reconcileTimelineConnections(next);
  expect(after.connections).toHaveLength(2);
  expect(after.connections.some((edge) => edge.fromNodeId === f.ids[0])).toBe(
    false,
  );
});

it("uses legacy asset identity without confusing node content revision with asset revision", () => {
  const f = fixture();
  f.state.nodes[0].documentData!.fileReferences![0].version = "91";
  f.state.nodes[0].serviceData = {
    id: f.ids[0],
    node_type: "image",
    ref_id: f.nodes[0].asset.id,
    content_version: "91",
  };
  expect(
    connectCanvasTimelineSources(f.state, [f.ids[0]], f.target.id, f.options)
      .connections,
  ).toHaveLength(1);
});
it("rejects ambiguous outputs, vanished sources and locked timeline lanes without changing the graph", () => {
  const f = fixture();
  f.state.nodes[0].documentData!.fileReferences!.push({
    ...f.state.nodes[0].documentData!.fileReferences![0],
    referenceKey: "other",
    fileId: crypto.randomUUID(),
  });
  expect(() =>
    connectCanvasTimelineSources(f.state, f.ids, f.target.id, f.options),
  ).toThrow();
  expect(() =>
    connectCanvasTimelineSources(
      f.state,
      [crypto.randomUUID()],
      f.target.id,
      f.options,
    ),
  ).toThrow();
  f.state.nodes[0].documentData!.fileReferences!.pop();
  f.target.serviceData!.config!.timeline!.tracks![0].locked = true;
  const before = structuredClone(f.state);
  expect(() =>
    connectCanvasTimelineSources(f.state, f.ids, f.target.id, f.options),
  ).toThrow("锁定");
  expect(f.state).toEqual(before);
});
it("detaches a legacy node-only clip on source deletion and preserves its source asset", () => {
  const f = fixture(),
    next = connectCanvasTimelineSources(f.state, f.ids, f.target.id, f.options);
  const clip = next.nodes.at(-1)!.serviceData!.config!.timeline!.clips![0];
  delete clip.asset_id;
  next.connections = [];
  const after = removeCanvasReferences(next.nodes, [], {
    nodeIds: new Set([f.ids[0]]),
  });
  const detached = after.nodes.at(-1)!.serviceData!.config!.timeline!.clips![0];
  expect(detached.node_id).toBeUndefined();
  expect(detached.asset_id).toBe(f.nodes[0].asset.id);
});
