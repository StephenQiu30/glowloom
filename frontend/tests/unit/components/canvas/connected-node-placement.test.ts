import { describe, expect, it } from "vitest";
import {
  createCanvasNode,
  connectedNodeCenter,
} from "@/components/canvas/lib/canvas-node-factory";

describe("connected node placement", () => {
  const source = {
    ...createCanvasNode("text", { x: 0, y: 0 }),
    position: { x: 120, y: 240 },
    width: 480,
    height: 320,
  };
  it("keeps a new empty image square", () => {
    expect(createCanvasNode("image", { x: 0, y: 0 })).toMatchObject({
      width: 320,
      height: 320,
    });
  });
  it.each(["source", "target"] as const)(
    "places a clicked %s port beside the existing node without overlap",
    (handleType) => {
      const center = connectedNodeCenter(
        "video",
        {
          connection: { nodeId: source.id, handleType },
          position: { x: 615, y: 400 },
          placement: "adjacent",
        },
        source,
      );
      const added = createCanvasNode("video", center);
      expect(added.position.y + added.height / 2).toBe(400);
      expect(
        handleType === "source"
          ? added.position.x - (source.position.x + source.width)
          : source.position.x - (added.position.x + added.width),
      ).toBe(60);
    },
  );
  it("preserves a drag release point instead of snapping back to the source", () => {
    const position = { x: -800, y: 1300 };
    expect(
      connectedNodeCenter(
        "image",
        {
          connection: { nodeId: source.id, handleType: "source" },
          position,
        },
        source,
      ),
    ).toEqual(position);
  });
});
