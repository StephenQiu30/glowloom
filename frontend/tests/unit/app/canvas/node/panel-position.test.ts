import { expect, it } from "vitest";
import { nodePanelPosition } from "@/app/canvas/node/panel-position";

it("空间足够时提示词面板位于节点下方，移动与缩放使用同一视口", () => {
  const node = { x: 200, y: 480, width: 288, height: 200 };
  const first = nodePanelPosition(node, { x: 0, y: 0, k: 1 }, 1280);
  expect(first.top).toBe(696);
  const moved = nodePanelPosition(
    { ...node, y: 580 },
    { x: 40, y: -120, k: 1 },
    1280,
  );
  expect(moved.top).toBe(676);
  const zoomed = nodePanelPosition(node, { x: 20, y: -100, k: 0.5 }, 1280);
  expect(zoomed.top).toBe(256);
  expect(first.width).toBe(680);
  expect(zoomed.width).toBe(680);
});
it("手机仅收缩面板宽度，纵向锚点不会固定到屏幕底部", () => {
  const node = { x: 200, y: 700, width: 288, height: 200 };
  const result = nodePanelPosition(node, { x: 0, y: 0, k: 1 }, 390);
  expect(result.width).toBe(358);
  expect(result.left).toBe(16);
  expect(result.top).toBe(916);
});
it("下方空间不足时放到节点上方，并避开标题", () => {
  const node = { x: 500, y: 620, width: 320, height: 180 };
  const result = nodePanelPosition(node, { x: 0, y: 0, k: 1 }, 1460, 900);
  expect(result.top).toBe(348);
  expect(node.y).toBe(620);
  expect(result.left).toBeGreaterThanOrEqual(76);
  expect(result.top + 224).toBeLessThanOrEqual(node.y - 48);
});

it("上下都放不下时选择空间较大一侧滚动，不能覆盖节点", () => {
  const node = { x: 200, y: 300, width: 320, height: 260 };
  const result = nodePanelPosition(node, { x: 0, y: 0, k: 1 }, 1280, 700, 320);
  expect(result.top).toBe(64);
  expect(result.maxHeight).toBe(188);
  expect(result.top + result.maxHeight).toBe(node.y - 48);
});
it("使用实际弹层高度及缩放后节点边界避让", () => {
  const node = { x: 200, y: 600, width: 320, height: 200 };
  const viewport = { x: 0, y: 100, k: 0.75 };
  const result = nodePanelPosition(node, viewport, 1280, 800, 280);
  expect(result.top + 280).toBe(502);
  expect(result.top + 280).toBeLessThan(node.y * viewport.k + viewport.y);
  expect(viewport).toEqual({ x: 0, y: 100, k: 0.75 });
});
it("窄屏下方不足时缩短上方面板，而不是压住节点", () => {
  const result = nodePanelPosition(
    { x: 20, y: 200, width: 320, height: 400 },
    { x: 0, y: 0, k: 1 },
    390,
    680,
    320,
  );
  expect(result.width).toBe(358);
  expect(result.top).toBe(64);
  expect(result.maxHeight).toBe(88);
});
