import { describe, expect, it } from "vitest";
import { canvasWorkspaceRect } from "@/app/canvas/viewport/workspace-rect";

describe("画布可用屏幕矩形", () => {
  it("1512×900关闭面板时保持完整视口与原中心", () => {
    expect(canvasWorkspaceRect(1512, 900)).toEqual({
      left: 0,
      top: 0,
      width: 1512,
      height: 900,
      centerX: 756,
      centerY: 450,
    });
  });
  it("AI展开时预留侧栏与工具安全区", () => {
    expect(canvasWorkspaceRect(1512, 900, 400)).toEqual({
      left: 76,
      top: 64,
      width: 1012,
      height: 836,
      centerX: 582,
      centerY: 482,
    });
    expect(canvasWorkspaceRect(1512, 900, 520).width).toBe(892);
  });
  it("390×844关闭面板保持手机原中心", () => {
    expect(canvasWorkspaceRect(390, 844)).toEqual({
      left: 0,
      top: 0,
      width: 390,
      height: 844,
      centerX: 195,
      centerY: 422,
    });
  });
  it.each([
    [390, 844, 400],
    [390, 180, 360],
    [40, 40, 400],
    [1, 1, 400],
  ])("%i×%i面板挤满视口时仍保留可达区域", (width, height, ai) => {
    const rect = canvasWorkspaceRect(width, height, ai);
    for (const value of Object.values(rect))
      expect(Number.isFinite(value)).toBe(true);
    expect(rect.width).toBeGreaterThan(0);
    expect(rect.height).toBeGreaterThan(0);
    expect(rect.left).toBeGreaterThanOrEqual(0);
    expect(rect.top).toBeGreaterThanOrEqual(0);
    expect(rect.left + rect.width).toBeLessThanOrEqual(width);
    expect(rect.top + rect.height).toBeLessThanOrEqual(height);
    expect(rect.centerX).toBe(rect.left + rect.width / 2);
    expect(rect.centerY).toBe(rect.top + rect.height / 2);
    expect(rect.width).toBeGreaterThanOrEqual(Math.min(96, width));
    expect(rect.height).toBeGreaterThanOrEqual(Math.min(96, height));
  });
  it("测量未就绪或参数无效时返回有限正值，负面板尺寸等同关闭", () => {
    expect(canvasWorkspaceRect(390, 844, -1)).toEqual(
      canvasWorkspaceRect(390, 844),
    );
    const rect = canvasWorkspaceRect(Number.NaN, Number.POSITIVE_INFINITY, 400);
    expect(rect).toEqual({
      left: 0,
      top: 0,
      width: 1,
      height: 1,
      centerX: 0.5,
      centerY: 0.5,
    });
    expect(canvasWorkspaceRect(0, -100).width).toBe(1);
    for (const value of Object.values(
      canvasWorkspaceRect(Number.MAX_VALUE, Number.MAX_VALUE, Number.MAX_VALUE),
    ))
      expect(Number.isFinite(value)).toBe(true);
  });
});
