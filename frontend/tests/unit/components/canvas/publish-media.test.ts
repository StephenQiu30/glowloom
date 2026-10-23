import { afterEach, describe, expect, it, vi } from "vitest";
import { publishCanvasMedia } from "@/components/canvas/publish-media";
import { serviceGraph } from "@/components/canvas/service-graph";
const upload = vi.hoisted(() => vi.fn());
vi.mock("@/api/media", () => ({ uploadMediaAsset: upload }));
vi.mock("@/components/media/library", () => ({
  validateUpload: () => undefined,
}));
afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});
describe("画布本地素材发布", () => {
  it("uploads a derived replacement despite the old file reference, preserving the binding key and upload receipt", async () => {
    const project = "22222222-2222-4222-8222-222222222222",
      canvas = "11111111-1111-4111-8111-111111111111";
    const state = serviceGraph(
      {
        id: canvas,
        project_id: project,
        revision: "1",
        schema_version: 1,
        operation_version: 1,
        permissions: { read: true, write: true },
        nodes: [
          {
            id: canvas,
            node_type: "image",
            node_action: "resource",
            title: "素材",
            x: 0,
            y: 0,
            width: 320,
            height: 240,
            config: {},
            media_ref: { asset_id: project },
            content_version: "1",
          },
        ],
        edges: [],
      },
      canvas,
      project,
    );
    state.nodes[0].metadata!.content = "blob:derived-image";
    const fetch = vi
      .fn()
      .mockResolvedValue({
        blob: async () => new Blob(["test"], { type: "image/png" }),
      });
    vi.stubGlobal("fetch", fetch);
    upload.mockResolvedValue({
      asset: { id: canvas, project_id: project, kind: "image", revision: 2 },
    });
    const receipts = new Map();
    const signal = new AbortController().signal;
    const result = await publishCanvasMedia(state, project, receipts, signal);
    expect(result.nodes[0].documentData!.fileReferences).toEqual([
      {
        referenceKey: project,
        fileId: canvas,
        version: "2",
        purpose: "CONTENT",
      },
    ]);
    await publishCanvasMedia(state, project, receipts, signal);
    expect(upload).toHaveBeenCalledTimes(1);
    expect(fetch).toHaveBeenCalledTimes(1);
  });
});
