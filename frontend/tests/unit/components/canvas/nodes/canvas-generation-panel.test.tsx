import type { RichGeneration } from "@/components/generation/rich-generation";
import { cleanup, render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { CanvasGenerationPanel } from "@/components/canvas/nodes/canvas-generation-panel";
import { serviceGraph } from "@/components/canvas/service-graph";
import type { CanvasYjsLiveSession } from "@/components/canvas/yjs-live-session";
const captured = vi.hoisted(() => ({
  props: null as Parameters<typeof RichGeneration>[0] | null,
  get: vi.fn(),
  preview: vi.fn(),
}));
vi.mock("next/dynamic", () => ({
  default: () => (props: Parameters<typeof RichGeneration>[0]) => {
    captured.props = props;
    return null;
  },
}));
vi.mock("@/api/operations", () => ({ getOperation: captured.get }));
vi.mock("@/api/media", () => ({ getMediaPreview: captured.preview }));
const canvasID = "11111111-1111-4111-8111-111111111111";
const projectID = "22222222-2222-4222-8222-222222222222";
const nodeID = "33333333-3333-4333-8333-333333333333";
const operationID = "44444444-4444-4444-8444-444444444444";
const outputID = "55555555-5555-4555-8555-555555555555";
function setup(
  kind = "text",
  configure?: (graph: ReturnType<typeof serviceGraph>) => void,
  onOperationReady?: (nodeID: string, operationID: string) => void,
) {
  const dto: API.DocumentDTO = {
    id: canvasID,
    project_id: projectID,
    revision: "1",
    schema_version: 1,
    operation_version: 1,
    permissions: { read: true, write: true },
    nodes: [
      {
        id: nodeID,
        node_type: kind,
        node_action: "resource",
        x: 0,
        y: 0,
        width: 320,
        height: 320,
        config:
          kind === "text"
            ? { text: "已有正文", format: "plain", format_version: 1 }
            : {},
      },
    ],
    edges: [],
  };
  const state = {
    document: serviceGraph(dto, canvasID, projectID),
    writable: true,
    migration: false,
    inputDrafts: {},
    epoch: "epoch-1",
  };
  configure?.(state.document);
  const changed = vi.fn((graph) => {
    state.document = graph;
  });
  const waitForSaved = vi.fn(async (check?: () => void) => {
    check?.();
    return dto;
  });
  const client = {
    subscribe: () => () => {},
    getDocumentEpoch: () => state.epoch,
    getSnapshot: () => state,
    canAccept: () => state.writable,
    change: changed,
    waitForSaved,
  } as unknown as CanvasYjsLiveSession;
  render(
    <CanvasGenerationPanel
      client={client}
      nodeID={nodeID}
      canvasID={canvasID}
      projectID={projectID}
      onOperationReady={onOperationReady}
    />,
  );
  return { state, changed, waitForSaved };
}

it("正式生成回执绑定当前节点供节点内读取，保留原始操作身份", () => {
  const ready = vi.fn();
  setup("image", undefined, ready);
  captured.props!.onOperationReady?.(operationID, "draft-id");
  expect(ready).toHaveBeenCalledWith(nodeID, operationID);
});
function textResult() {
  return {
    id: operationID,
    project_id: projectID,
    status: "completed",
    capability: "text.generate",
    outputs: [
      {
        id: outputID,
        kind: "json",
        moderation_status: "passed",
        text: "新的生成正文",
      },
    ],
  };
}
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});
it("projects each incoming binding into the composer, including text and unresolved media", () => {
  setup("video", (graph) => {
    const target = graph.nodes[0];
    const source = {
      ...target,
      id: "source",
      type: "video",
      title: "片段",
      documentData: {
        ...target.documentData!,
        fileReferences: [
          {
            fileId: "unrelated",
            referenceKey: "first",
            version: "1",
            purpose: "CONTENT" as const,
          },
          {
            fileId: "linked",
            referenceKey: "second",
            version: "2",
            purpose: "CONTENT" as const,
          },
        ],
      },
    };
    const text = {
      ...target,
      id: "text",
      title: "镜头描述",
      type: "text",
      metadata: { content: "镜头缓慢推进" },
    };
    graph.nodes.push(source, text);
    graph.connections.push(
      {
        id: "fixed",
        fromNodeId: source.id,
        toNodeId: nodeID,
        documentData: {
          id: "fixed",
          fromNodeId: source.id,
          toNodeId: nodeID,
          fromPortId: "video",
          toPortId: "reference_videos",
          purpose: "REFERENCE_VIDEO",
          order: 0,
          binding: { kind: "FILE_REFERENCE", referenceKey: "second" },
        },
      },
      {
        id: "pending",
        fromNodeId: source.id,
        toNodeId: nodeID,
        documentData: {
          id: "pending",
          fromNodeId: source.id,
          toNodeId: nodeID,
          fromPortId: "video",
          toPortId: "reference_videos",
          purpose: "REFERENCE_VIDEO",
          order: 1,
          binding: {
            kind: "RUN_OUTPUT",
            selection: { mode: "ITEM", index: 1 },
          },
        },
      },
      { id: "legacy-text", fromNodeId: text.id, toNodeId: nodeID },
    );
  });
  expect(captured.props?.canvas?.incomingSources).toEqual([
    {
      edgeID: "fixed",
      nodeID: "source",
      title: "片段",
      kind: "video",
      assetID: "linked",
    },
    {
      edgeID: "legacy-text",
      nodeID: "text",
      title: "镜头描述",
      kind: "text",
      text: "镜头缓慢推进",
    },
    { edgeID: "pending", nodeID: "source", title: "片段", kind: "video" },
  ]);
});

it("adopts a checked text result into the same resource only after durable save", async () => {
  captured.get.mockResolvedValue(textResult());
  const { state, changed, waitForSaved } = setup();
  await captured.props!.onAdoptText!(operationID, outputID);
  expect(captured.get).toHaveBeenCalledWith({
    id: operationID,
    project_id: projectID,
  });
  expect(changed).toHaveBeenCalledOnce();
  expect(waitForSaved).toHaveBeenCalledOnce();
  expect(state.document.nodes[0]).toMatchObject({
    id: nodeID,
    type: "text",
    metadata: { content: "新的生成正文" },
  });
});
it("does not overwrite edits made while the result is being revalidated", async () => {
  let resolve!: (value: unknown) => void;
  captured.get.mockReturnValue(
    new Promise((done) => {
      resolve = done;
    }),
  );
  const { state, changed } = setup();
  const adoption = captured.props!.onAdoptText!(operationID, outputID);
  state.document.nodes[0].metadata!.richText!.content = {
    type: "doc",
    content: [
      { type: "paragraph", content: [{ type: "text", text: "协作者新正文" }] },
    ],
  };
  resolve(textResult());
  await expect(adoption).rejects.toThrow("节点结果已变化");
  expect(changed).not.toHaveBeenCalled();
});
it("rejects unmoderated and foreign-project text results", async () => {
  const { changed } = setup();
  captured.get.mockResolvedValue({ ...textResult(), project_id: canvasID });
  await expect(
    captured.props!.onAdoptText!(operationID, outputID),
  ).rejects.toThrow();
  captured.get.mockResolvedValue({
    ...textResult(),
    outputs: [
      {
        ...textResult().outputs[0],
        moderation_status: "pending",
        text: undefined,
      },
    ],
  });
  await expect(
    captured.props!.onAdoptText!(operationID, outputID),
  ).rejects.toThrow("尚未就绪");
  expect(changed).not.toHaveBeenCalled();
});
it("requires a fixed-version same-project media result", async () => {
  const { changed } = setup("image");
  captured.preview.mockResolvedValue({
    asset: { id: outputID, project_id: canvasID, kind: "image", revision: 1 },
    url: "/preview",
  });
  await expect(captured.props!.onAdopt!(outputID, "image")).rejects.toThrow(
    "不可用",
  );
  expect(changed).not.toHaveBeenCalled();
  captured.preview.mockResolvedValue({
    asset: {
      id: outputID,
      project_id: projectID,
      kind: "image",
      revision: 3,
      width: 640,
      height: 480,
    },
    url: "/preview",
  });
  await captured.props!.onAdopt!(outputID, "image");
  expect(changed).toHaveBeenCalledOnce();
  expect(
    changed.mock.calls[0][0].nodes[0].documentData.fileReferences[0],
  ).toMatchObject({ fileId: outputID, version: "3" });
});

it("rejects an operation receipt arriving after a same-node history restore", async () => {
  let resolve!: (value: unknown) => void;
  captured.get.mockReturnValue(
    new Promise((done) => {
      resolve = done;
    }),
  );
  const { state, changed } = setup();
  const adoption = captured.props!.onAdoptText!(operationID, outputID);
  state.epoch = "epoch-restored";
  resolve(textResult());
  await expect(adoption).rejects.toThrow("节点结果已变化");
  expect(changed).not.toHaveBeenCalled();
});
