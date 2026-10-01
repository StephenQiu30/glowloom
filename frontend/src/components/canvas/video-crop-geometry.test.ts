import { describe, expect, it } from "vitest";
import {
  moveVideoCrop,
  normalizeVideoCropForEncoding,
  resizeVideoCrop,
} from "./video-crop-geometry";
describe("视频裁切像素几何", () => {
  it("H264裁切为偶数源坐标与尺寸且不超过奇数原件边界", () => {
    expect(
      normalizeVideoCropForEncoding(
        { x: 3, y: 5, width: 99, height: 57 },
        { width: 101, height: 61 },
      ),
    ).toEqual({ x: 2, y: 4, width: 98, height: 56 });
    expect(
      normalizeVideoCropForEncoding(
        { x: -10, y: 20, width: 500, height: 300 },
        { width: 100, height: 60 },
      ),
    ).toEqual({ x: 0, y: 20, width: 100, height: 40 });
  });
  it("移动与边缘拉伸保持源边界及最小选区", () => {
    const crop = { x: 10, y: 10, width: 50, height: 30 },
      source = { width: 100, height: 60 };
    expect(moveVideoCrop(crop, source, 100, -50)).toEqual({
      ...crop,
      x: 50,
      y: 0,
    });
    expect(resizeVideoCrop(crop, source, "nw", 100, 100)).toEqual({
      x: 44,
      y: 24,
      width: 16,
      height: 16,
    });
  });
  it("拒绝非有限值与无效源尺寸而不制造NaN选区", () => {
    expect(() =>
      normalizeVideoCropForEncoding(
        { x: 0, y: 0, width: NaN, height: 20 },
        { width: 100, height: 60 },
      ),
    ).toThrow();
    expect(() =>
      moveVideoCrop(
        { x: 0, y: 0, width: 20, height: 20 },
        { width: 0, height: 60 },
        3,
        1,
      ),
    ).toThrow();
  });
});
