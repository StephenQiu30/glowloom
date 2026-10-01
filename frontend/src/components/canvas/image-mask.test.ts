import { describe, expect, it } from "vitest";
import { editMaskPixels, maskDimensions } from "./image-mask";

describe("局部重绘蒙版", () => {
  it("编辑区透明、未选区白色不透明，保持源选择数据", () => {
    const selection = new Uint8ClampedArray([0, 0, 0, 0, 0, 0, 0, 128]);
    expect(Array.from(editMaskPixels(selection, 2, 1))).toEqual([
      255, 255, 255, 255, 255, 255, 255, 0,
    ]);
    expect(selection[7]).toBe(128);
  });
  it("拒绝空编辑区、无效尺寸和不一致像素预算", () => {
    expect(() => editMaskPixels(new Uint8ClampedArray(8), 2, 1)).toThrow(
      "涂抹",
    );
    expect(() => editMaskPixels(new Uint8ClampedArray(4), 2, 1)).toThrow();
    expect(() => maskDimensions(8192, 8192)).toThrow();
    expect(() => maskDimensions(Number.NaN, 200)).toThrow();
    expect(() => maskDimensions(0, 200)).toThrow();
  });
});
