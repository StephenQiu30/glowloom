import { describe, expect, it } from "vitest";
import {
  createCanvasNode,
  nextCanvasNodeTitle,
} from "@/components/canvas/lib/canvas-node-factory";
import type { CanvasNodeData } from "@/components/canvas/types/canvas";

describe("canvas node titles", () => {
  it("uses a two-word name and does not append a number", () => {
    const image = createCanvasNode("image", { x: 0, y: 0 });
    const timeline = nextCanvasNodeTitle("时间线", "timeline", [image]);
    expect(image.title).toBe("图片素材");
    expect(timeline).toBe("剪辑时间线");
    expect(image.title).not.toMatch(/\d/);
    expect(timeline).not.toMatch(/\d/);
  });

  it("keeps the same semantic name when another node of that type exists", () => {
    const existing = {
      type: "image",
      title: "图片素材",
    } as CanvasNodeData;
    expect(nextCanvasNodeTitle("图片", "image", [existing])).toBe("图片素材");
    expect(nextCanvasNodeTitle("配色", "cinema_studio", [])).toBe("色彩方案");
  });
});
