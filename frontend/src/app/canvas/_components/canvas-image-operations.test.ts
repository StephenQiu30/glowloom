import { expect, it } from "vitest";
import {
  imageRegions,
  resizeDimensions,
  dragImageCrop,
} from "./canvas-image-operations";

it("裁剪拖动和尺寸调整夹取到原图，取消时可恢复原范围", () => {
  const crop = { x: 10, y: 20, width: 50, height: 40 };
  expect(dragImageCrop(crop, 90, -50, "move")).toEqual({
    x: 50,
    y: 0,
    width: 50,
    height: 40,
  });
  expect(dragImageCrop(crop, 100, -100, "resize")).toEqual({
    x: 10,
    y: 20,
    width: 90,
    height: 1,
  });
  expect(crop).toEqual({ x: 10, y: 20, width: 50, height: 40 });
});

it("非整除拆图覆盖全部像素且没有重叠或缝隙", () => {
  const pieces = imageRegions(101, 79, { kind: "split", rows: 3, columns: 2 });
  expect(pieces).toHaveLength(6);
  expect(pieces.reduce((total, p) => total + p.width * p.height, 0)).toBe(
    101 * 79,
  );
  expect(pieces.at(-1)).toEqual({ x: 50, y: 52, width: 51, height: 27 });
});
it("裁剪限制在原图内并拒绝无效和过量拆分", () => {
  expect(
    imageRegions(100, 80, {
      kind: "crop",
      x: 0.25,
      y: 0.25,
      width: 0.5,
      height: 0.5,
    }),
  ).toEqual([{ x: 25, y: 20, width: 50, height: 40 }]);
  for (const rows of [0, 7, NaN, 1.5])
    expect(() =>
      imageRegions(100, 80, { kind: "split", rows, columns: 2 }),
    ).toThrow();
  expect(() =>
    imageRegions(10, 10, { kind: "crop", x: 0.9, y: 0, width: 0.5, height: 1 }),
  ).toThrow();
  expect(() =>
    imageRegions(0, 10, { kind: "split", rows: 2, columns: 2 }),
  ).toThrow();
});
it("本地缩放保持比例且限制最长边，拒绝非有限输入", () => {
  expect(resizeDimensions(1600, 900, 4096)).toEqual({
    width: 4096,
    height: 2304,
  });
  expect(resizeDimensions(900, 1600, 2048)).toEqual({
    width: 1152,
    height: 2048,
  });
  expect(() => resizeDimensions(100, 50, 5000)).toThrow();
  expect(() => resizeDimensions(100, 50, NaN)).toThrow();
});
