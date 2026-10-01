import { describe, expect, it } from "vitest";
import {
  imageCropPixels,
  imageGridCells,
  imageResizeSize,
  imageToolFileName,
  type ImageCrop,
} from "./image-tools";

describe("图片工具几何合同", () => {
  it("裁切归一化区域覆盖非整数边缘并保持图像边界", () => {
    expect(
      imageCropPixels(101, 61, { x: 0.25, y: 0, width: 0.5, height: 1 }),
    ).toEqual({ x: 25, y: 0, width: 51, height: 61 });
    expect(
      imageCropPixels(100, 60, { x: 0.9, y: 0.8, width: 0.5, height: 0.5 }),
    ).toEqual({ x: 90, y: 48, width: 10, height: 12 });
  });

  it("拒绝空裁切、非有限坐标和无效媒体尺寸", () => {
    for (const crop of [
      { x: 0, y: 0, width: 0, height: 1 },
      { x: Number.NaN, y: 0, width: 1, height: 1 },
      { x: 1, y: 0, width: 1, height: 1 },
    ] satisfies ImageCrop[])
      expect(() => imageCropPixels(100, 60, crop)).toThrow();
    expect(() =>
      imageCropPixels(0, 60, { x: 0, y: 0, width: 1, height: 1 }),
    ).toThrow();
  });

  it("宫格切片恰好覆盖全部像素且不重复", () => {
    const cells = imageGridCells(101, 61, 2, 3);
    expect(cells).toHaveLength(6);
    expect(cells.reduce((sum, cell) => sum + cell.width * cell.height, 0)).toBe(
      101 * 61,
    );
    expect(cells[5]).toEqual({
      row: 1,
      column: 2,
      x: 67,
      y: 30,
      width: 34,
      height: 31,
    });
    expect(() => imageGridCells(101, 61, 6, 2)).toThrow();
    expect(() => imageGridCells(1, 1, 2, 2)).toThrow();
  });

  it("放大保持比例并限制4096长边", () => {
    expect(imageResizeSize(640, 360, 2560)).toEqual({
      width: 2560,
      height: 1440,
    });
    expect(imageResizeSize(360, 640, 8192)).toEqual({
      width: 2304,
      height: 4096,
    });
    expect(() => imageResizeSize(360, 640, Number.NaN)).toThrow();
  });

  it("派生文件只保留基本名、明确工具和PNG格式", () => {
    expect(imageToolFileName("folder\\人物参考.jpg", "split", 2)).toBe(
      "人物参考-宫格-3.png",
    );
    expect(imageToolFileName("../角色.webp", "crop")).toBe("角色-裁切.png");
  });
});
