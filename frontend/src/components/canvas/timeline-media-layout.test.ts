import { describe, expect, it } from "vitest";
import { timelineMediaLayout } from "./timeline-media-layout";

describe("时间线裁切预览几何", () => {
  it("按实际原件尺寸裁切，再在输出画幅内等比留黑边", () => {
    const result = timelineMediaLayout(320, 240, "16:9", {
      x: 40,
      y: 20,
      width: 200,
      height: 160,
    });
    expect(result.frameWidthPercent).toBeCloseTo(70.3125);
    expect(result.frameHeightPercent).toBe(100);
    expect(result.mediaWidthPercent).toBe(160);
    expect(result.mediaHeightPercent).toBe(150);
    expect(result.leftPercent).toBe(-20);
    expect(result.topPercent).toBe(-12.5);
  });
  it("未裁切的横图在竖画幅居中，不拉伸原片", () => {
    const result = timelineMediaLayout(320, 240, "9:16", null);
    expect(result.frameWidthPercent).toBe(100);
    expect(result.frameHeightPercent).toBeCloseTo(42.1875);
    expect(result.mediaWidthPercent).toBe(100);
    expect(result.leftPercent).toBe(0);
  });
  it("拒绝超出实际素材的裁切和无效尺寸", () => {
    expect(() =>
      timelineMediaLayout(320, 240, "16:9", {
        x: 250,
        y: 0,
        width: 200,
        height: 160,
      }),
    ).toThrow();
    expect(() => timelineMediaLayout(0, 240, "16:9", null)).toThrow();
  });
});
