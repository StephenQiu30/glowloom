import { expect, it } from "vitest";
import { isCanvasMediaCandidate } from "@/components/canvas/lib/canvas-media-selection";
import type { CanvasNodeData } from "@/components/canvas/types/canvas";
const node = {
  id: "image-node",
  type: "image",
  serviceData: {
    media_ref: { asset_id: "11111111-1111-4111-8111-111111111111" },
  },
} as CanvasNodeData;
it("direct selection only admits a different node with a formal media identity and allowed role", () => {
  expect(isCanvasMediaCandidate(node, "subject-node", ["image", "video"])).toBe(
    true,
  );
  expect(isCanvasMediaCandidate(node, node.id, ["image"])).toBe(false);
  expect(isCanvasMediaCandidate(node, "subject-node", ["audio"])).toBe(false);
  expect(
    isCanvasMediaCandidate(
      { ...node, serviceData: undefined },
      "subject-node",
      ["image"],
    ),
  ).toBe(false);
  expect(
    isCanvasMediaCandidate(
      { ...node, serviceData: { media_ref: { asset_id: "blob:preview" } } },
      "subject-node",
      ["image"],
    ),
  ).toBe(false);
});
