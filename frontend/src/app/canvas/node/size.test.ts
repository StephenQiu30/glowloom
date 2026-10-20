import { expect, it } from "vitest";
import { fitNodeSize, nodeSizeFromRatio } from "./size";

it("过大的素材缩进节点上限，小图保持原尺寸", () => {
  expect(fitNodeSize(1280, 720)).toEqual({ width: 640, height: 360 });
  expect(fitNodeSize(320, 180)).toEqual({ width: 320, height: 180 });
});

it("比例决定节点宽高，极端比例退回基准尺寸", () => {
  expect(nodeSizeFromRatio("16:9", 320, 320)).toEqual({
    width: 320,
    height: 180,
  });
  expect(nodeSizeFromRatio("9:16", 320, 320)).toEqual({
    width: 180,
    height: 320,
  });
  expect(nodeSizeFromRatio("8:1", 320, 200)).toEqual({
    width: 320,
    height: 200,
  });
  expect(nodeSizeFromRatio("auto", 320, 200)).toBeNull();
});
