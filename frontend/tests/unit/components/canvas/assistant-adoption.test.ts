import { beforeEach, describe, expect, it, vi } from "vitest";
import { createCanvasAssistantAdoption } from "@/components/canvas/assistant/canvas-assistant-adoption";
import type { CanvasEditorState } from "@/components/canvas/lib/canvas-document-adapter";
import type { CanvasYjsLiveSession } from "@/components/canvas/yjs-live-session";
const api = vi.hoisted(() => ({ operation: vi.fn(), media: vi.fn() }));
vi.mock("@/api/operations", () => ({ getOperation: api.operation }));
vi.mock("@/api/media", () => ({ getMediaPreview: api.media }));
const project = "e4647966-7348-4a34-a677-bcb46cc23403",
  operation = "e4647966-7348-4a34-a677-bcb46cc23404",
  output = "e4647966-7348-4a34-a677-bcb46cc23405";
function client() {
  let graph: CanvasEditorState = {
    nodes: [],
    connections: [],
    viewport: { x: 0, y: 0, k: 1 },
    backgroundMode: "dots",
    showImageInfo: false,
  };
  let epoch = "epoch-1";
  const port = {
    getSnapshot: () => ({ document: graph }),
    getDocumentEpoch: () => epoch,
    canAccept: vi.fn(() => true),
    change: vi.fn((next: CanvasEditorState) => {
      graph = next;
    }),
    waitForSaved: vi.fn(async (check?: () => void) => {
      check?.();
      return {};
    }),
  };
  return {
    port: port as unknown as CanvasYjsLiveSession,
    mocks: port,
    graph: () => graph,
    epoch: (value: string) => {
      epoch = value;
    },
  };
}
beforeEach(() => {
  vi.clearAllMocks();
  api.operation.mockResolvedValue({
    id: operation,
    project_id: project,
    status: "completed",
    capability: "text.generate",
    outputs: [
      {
        id: output,
        kind: "json",
        moderation_status: "passed",
        text: "Verified text",
      },
    ],
  });
});
describe("assistant adoption preserves save and epoch ownership", () => {
  it("adopts reviewed text into one new node and waits for persistence", async () => {
    const c = client(),
      adopt = createCanvasAssistantAdoption(c.port, project);
    await adopt.adoptText(operation, output);
    expect(c.graph().nodes).toHaveLength(1);
    expect(c.graph().nodes[0]?.metadata?.content).toBe("Verified text");
    expect(c.mocks.waitForSaved).toHaveBeenCalledOnce();
  });
  it("keeps one node when an unknown save is retried", async () => {
    const c = client(),
      adopt = createCanvasAssistantAdoption(c.port, project);
    c.mocks.waitForSaved.mockRejectedValueOnce(new Error("unknown"));
    await expect(adopt.adoptText(operation, output)).rejects.toThrow("unknown");
    const id = c.graph().nodes[0]?.id;
    await adopt.adoptText(operation, output);
    expect(c.graph().nodes).toHaveLength(1);
    expect(c.graph().nodes[0]?.id).toBe(id);
    expect(api.operation).toHaveBeenCalledOnce();
    expect(c.mocks.change).toHaveBeenCalledOnce();
  });
  it("rejects an old asynchronous result after epoch restoration", async () => {
    const c = client(),
      adopt = createCanvasAssistantAdoption(c.port, project);
    let resolve: (value: unknown) => void = () => {};
    api.operation.mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }),
    );
    const pending = adopt.adoptText(operation, output);
    c.epoch("epoch-2");
    resolve({
      id: operation,
      project_id: project,
      status: "completed",
      capability: "text.generate",
      outputs: [
        {
          id: output,
          kind: "json",
          moderation_status: "passed",
          text: "Old response",
        },
      ],
    });
    await expect(pending).rejects.toThrow("画布已变化");
    expect(c.mocks.change).not.toHaveBeenCalled();
  });
  it("revalidates media project and fixed version before inserting", async () => {
    const c = client(),
      adopt = createCanvasAssistantAdoption(c.port, project);
    api.media.mockResolvedValue({
      asset: { id: output, project_id: operation, kind: "image", revision: 3 },
      url: "https://example.invalid/asset.png",
    });
    await expect(adopt.adoptMedia(output, "image")).rejects.toThrow();
    expect(c.graph().nodes).toHaveLength(0);
    api.media.mockResolvedValue({
      asset: {
        id: output,
        project_id: project,
        kind: "image",
        revision: 3,
        width: 1024,
        height: 1024,
      },
      url: "https://example.invalid/asset.png",
    });
    await adopt.adoptMedia(output, "image");
    expect(c.graph().nodes[0]?.documentData?.fileReferences?.[0]?.version).toBe(
      "3",
    );
  });
});
