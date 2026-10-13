import { expect, it } from "vitest";
import { nodePanelPosition } from "./canvas-panel-position";

it("提示词面板始终位于选中节点下方，移动与缩放使用同一视口", () => {
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
  expect(first.width).toBe(660);
});
it("手机仅收缩面板宽度，纵向锚点不会固定到屏幕底部", () => {
  const node = { x: 200, y: 700, width: 288, height: 200 };
  const result = nodePanelPosition(node, { x: 0, y: 0, k: 1 }, 390);
  expect(result.width).toBe(358);
  expect(result.left).toBe(16);
  expect(result.top).toBe(916);
});
