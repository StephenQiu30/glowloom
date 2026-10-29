import { afterEach, expect, it, vi } from "vitest";
import { getMediaPreview } from "@/api/media";
import { createCanvasDomainConnector } from "@/components/canvas/lib/canvas-domain-connection";
import { createCanvasNode } from "@/components/canvas/lib/canvas-node-factory";
import type { CanvasNodeData } from "@/components/canvas/types/canvas";
import type { CanvasYjsLiveSession } from "@/components/canvas/yjs-live-session";
import type { CanvasEditorState } from "@/components/canvas/lib/canvas-document-adapter";

vi.mock("@/api/media", () => ({ getMediaPreview: vi.fn() }));
afterEach(() => vi.resetAllMocks());

function fixture() {
  const projectID = crypto.randomUUID();
  const scopeID = crypto.randomUUID();
  const source = createCanvasNode("image", { x: 0, y: 0 });
  const target = createCanvasNode("timeline", { x: 500, y: 0 });
  const asset: API.AssetSummary = {
    id: crypto.randomUUID(),
    project_id: projectID,
    revision: 1,
    kind: "image",
  };
  source.serviceData = {
    id: source.id,
    node_type: "image",
    node_action: "resource",
    ref_id: asset.id,
    config: {},
  };
  let graph: CanvasEditorState = {
    nodes: [source, target],
    connections: [],
    viewport: { x: 0, y: 0, k: 1 },
    backgroundMode: "dots",
    showImageInfo: false,
  };
  let epoch = "epoch-one";
  let writable = true;
  let connectionReady = true;
  const change = vi.fn((next: CanvasEditorState) => {
    graph = next;
  });
  const waitForSaved = vi.fn(async () => ({
    nodes: graph.nodes.map((node) => ({ id: node.id })),
    edges: graph.connections.map((edge) => ({ id: edge.id })),
  }));
  // A controlled coordinator seam; real transport/ACK behavior is covered by the live-session suite.
  const client = {
    getSnapshot: () => ({ document: graph, writable }),
    getDocumentEpoch: () => epoch,
    canAcceptConnectionGraph: () => writable && connectionReady,
    connectionMediaNode: (node: CanvasNodeData) => node,
    change,
    waitForSaved,
  } as unknown as CanvasYjsLiveSession;
  const connector = createCanvasDomainConnector(client, projectID);
  const request = { sourceNodeIds: [source.id], targetNodeId: target.id };
  vi.mocked(getMediaPreview).mockResolvedValue({ asset });
  return {
    connector,
    client,
    scopeID,
    request,
    source,
    target,
    asset,
    change,
    waitForSaved,
    graph: () => graph,
    setGraph: (value: CanvasEditorState) => {
      graph = value;
    },
    setEpoch: () => {
      epoch = "epoch-two";
    },
    markUnknown: () => {
      connectionReady = false;
    },
    revoke: () => {
      writable = false;
    },
  };
}

it("publishes one clip-and-relation graph only after all metadata is resolved and verifies saved identities", async () => {
  const f = fixture();
  expect(await f.connector.connect(f.request)).toBe(f.target.id);
  expect(f.change).toHaveBeenCalledTimes(1);
  expect(f.waitForSaved).toHaveBeenCalledTimes(1);
  expect(f.graph().connections).toHaveLength(1);
  expect(f.graph().nodes[1].serviceData?.config?.timeline?.clips).toHaveLength(
    1,
  );
});

it.each(["source", "target", "epoch", "permission", "dispose"])(
  "rejects %s changes during metadata lookup without publishing a prefix",
  async (kind) => {
    const f = fixture();
    vi.mocked(getMediaPreview).mockImplementationOnce(async () => {
      if (kind === "source")
        f.setGraph({
          ...f.graph(),
          nodes: f.graph().nodes.filter((node) => node.id !== f.source.id),
        });
      if (kind === "target")
        f.setGraph({
          ...f.graph(),
          nodes: f.graph().nodes.filter((node) => node.id !== f.target.id),
        });
      if (kind === "epoch") f.setEpoch();
      if (kind === "permission") f.revoke();
      if (kind === "dispose") f.connector.dispose();
      return { asset: f.asset };
    });
    await expect(f.connector.connect(f.request)).rejects.toThrow();
    expect(f.change).not.toHaveBeenCalled();
  },
);

it("preserves unrelated edits made while metadata is loading", async () => {
  const f = fixture();
  const unrelated = createCanvasNode("text", { x: 900, y: 0 });
  vi.mocked(getMediaPreview).mockImplementationOnce(async () => {
    f.setGraph({ ...f.graph(), nodes: [...f.graph().nodes, unrelated] });
    return { asset: f.asset };
  });
  await f.connector.connect(f.request);
  expect(f.graph().nodes.find((node) => node.id === unrelated.id)).toEqual(
    unrelated,
  );
});

it("does not report success when saved readback omits the created relation", async () => {
  const f = fixture();
  f.waitForSaved.mockResolvedValue({ nodes: [{ id: f.target.id }], edges: [] });
  await expect(f.connector.connect(f.request)).rejects.toThrow("尚未确认");
  expect(f.change).toHaveBeenCalledTimes(1);
});

it("never resubmits a new graph when save confirmation fails", async () => {
  const f = fixture();
  f.waitForSaved.mockRejectedValue(new Error("原提交结果未确认"));
  await expect(f.connector.connect(f.request)).rejects.toThrow(
    "原提交结果未确认",
  );
  expect(f.change).toHaveBeenCalledTimes(1);
});

it("does not create another target while the first connection receipt is unknown", async () => {
  const f = fixture();
  f.waitForSaved.mockImplementationOnce(async () => {
    f.markUnknown();
    throw new Error("保存结果尚未确认");
  });
  await expect(f.connector.connect(f.request)).rejects.toThrow(
    "保存结果尚未确认",
  );
  const nextTarget = createCanvasNode("timeline", { x: 800, y: 0 });
  await expect(
    f.connector.connect({
      sourceNodeIds: [f.source.id],
      newTarget: nextTarget,
    }),
  ).rejects.toThrow();
  expect(f.change).toHaveBeenCalledTimes(1);
  expect(f.graph().nodes.some((node) => node.id === nextTarget.id)).toBe(false);
});

it("freezes the intended source set during asynchronous resolution", async () => {
  const f = fixture();
  vi.mocked(getMediaPreview).mockImplementationOnce(async () => {
    f.request.sourceNodeIds.length = 0;
    return { asset: f.asset };
  });
  await expect(f.connector.connect(f.request)).resolves.toBe(f.target.id);
  expect(f.graph().connections).toHaveLength(1);
});
