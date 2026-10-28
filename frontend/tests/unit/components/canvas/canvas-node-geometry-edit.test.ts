import { describe, expect, it } from "vitest";
import { resizeCanvasNode } from "@/components/canvas/lib/canvas-node-resize";

const rect = { x: 40, y: 60, width: 320, height: 200 };
describe("节点八方向调整", () => {
  it("左侧调整保持右边界及高度，顶部调整保持底边界及宽度", () => {
    expect(resizeCanvasNode(rect, "left", 20, 50)).toEqual({
      x: 60,
      y: 60,
      width: 300,
      height: 200,
    });
    expect(resizeCanvasNode(rect, "top", 50, -20)).toEqual({
      x: 40,
      y: 40,
      width: 320,
      height: 220,
    });
  });
  it("越过最小尺寸时仍保留对边锚点", () => {
    expect(resizeCanvasNode(rect, "top-left", 1000, 1000)).toEqual({
      x: 140,
      y: 100,
      width: 220,
      height: 160,
    });
  });
  it("媒体在侧边调整时维持比例并围绕另一轴中心缩放", () => {
    expect(resizeCanvasNode(rect, "right", 32, 100, 1.6)).toEqual({
      x: 40,
      y: 50,
      width: 352,
      height: 220,
    });
  });
  it("角调整保持对角锚点和自然比例", () => {
    expect(resizeCanvasNode(rect, "top-left", -32, -10, 1.6)).toEqual({
      x: 8,
      y: 40,
      width: 352,
      height: 220,
    });
  });
});
