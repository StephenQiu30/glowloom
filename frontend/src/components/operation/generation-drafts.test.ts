import { beforeEach, expect, it, vi } from "vitest";
import {
  createGenerationConfig,
  encodeGenerationConfig,
  generationWireSchema,
} from "@/components/canvas/generation-config";
import { loadGenerationDraft, saveGenerationDraft } from "./generation-drafts";

const api = vi.hoisted(() => ({
  listCanvases: vi.fn(),
  getCanvas: vi.fn(),
  createCanvas: vi.fn(),
  applyCanvasCommands: vi.fn(),
}));
vi.mock("@/api/canvases", () => api);
const project = "11111111-1111-4111-8111-111111111111",
  canvas = "22222222-2222-4222-8222-222222222222",
  node = "33333333-3333-4333-8333-333333333333";
const config = {
  ...createGenerationConfig("image.edit"),
  prompt: "窗边的人物",
  inputs: [
    {
      role: "reference_image",
      mediaAssetId: "44444444-4444-4444-8444-444444444444",
    },
  ],
};
const doc = {
  id: canvas,
  project_id: project,
  revision: 8,
  name: "故事",
  nodes: [
    {
      id: node,
      node_type: "generation",
      config: { generation: encodeGenerationConfig(config) },
      x: 120,
      y: 100,
      width: 360,
      height: 260,
      z_index: 2,
    },
  ],
};
beforeEach(() => {
  vi.resetAllMocks();
  api.getCanvas.mockResolvedValue(doc);
});

it("recovers the exact canvas draft and ordered roles within the requested project", async () => {
  const signal = new AbortController().signal;
  const result = await loadGenerationDraft(
    project,
    "image.generate",
    canvas,
    node,
    signal,
  );
  expect(result.nodeId).toBe(node);
  expect(result.config).toEqual(config);
  expect(api.getCanvas).toHaveBeenCalledWith({ id: canvas }, { signal });
  expect(api.listCanvases).not.toHaveBeenCalled();
});
it("rejects cross-project results and never falls back from an explicitly missing node", async () => {
  api.getCanvas.mockResolvedValue({
    ...doc,
    project_id: "55555555-5555-4555-8555-555555555555",
  });
  await expect(
    loadGenerationDraft(project, "image.edit", canvas, node),
  ).rejects.toMatchObject({ status: 502 });
  api.getCanvas.mockResolvedValue(doc);
  await expect(
    loadGenerationDraft(
      project,
      "image.edit",
      canvas,
      "66666666-6666-4666-8666-666666666666",
    ),
  ).rejects.toMatchObject({ status: 404 });
});
it("retries an unresolved save with the same revision, node ID and idempotency key", async () => {
  const request = {
    canvasId: canvas,
    revision: 8,
    nodeId: node,
    config,
    existing: true,
    key: "stable-save",
  };
  api.applyCanvasCommands
    .mockRejectedValueOnce(new Error("connection closed"))
    .mockResolvedValueOnce({ ...doc, revision: 9 });
  await expect(saveGenerationDraft(project, request)).rejects.toThrow(
    "connection closed",
  );
  const result = await saveGenerationDraft(project, request);
  expect(api.applyCanvasCommands.mock.calls[0]).toEqual(
    api.applyCanvasCommands.mock.calls[1],
  );
  expect(api.applyCanvasCommands.mock.calls[1][1].expected_revision).toBe(8);
  expect(
    api.applyCanvasCommands.mock.calls[1][2].headers["Idempotency-Key"],
  ).toBe("stable-save");
  expect(result.document.revision).toBe(9);
  expect(result.config).toEqual(config);
});
it("reports invalid tool fields as safeParse failures without throwing from a transform", () => {
  expect(() =>
    generationWireSchema.safeParse({
      ...encodeGenerationConfig(config),
      output_count: 9,
    }),
  ).not.toThrow();
  expect(
    generationWireSchema.safeParse({
      ...encodeGenerationConfig(config),
      output_count: 9,
    }).success,
  ).toBe(false);
  expect(
    generationWireSchema.safeParse({
      ...encodeGenerationConfig(config),
      params: { operation_id: "forged" },
    }).success,
  ).toBe(false);
});
