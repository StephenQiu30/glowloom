import { expect, it } from "vitest";
import { nodePanelPosition } from "./panel-position";

it("提示词面板始终位于选中节点下方，移动与缩放使用同一视口", () => {
  const node = { x: 200, y: 480, width: 288, height: 200 };
  const first = nodePanelPosition(node, { x: 0, y: 0, k: 1 }, 1280);
  expect(first.top).toBe(700);
  const moved = nodePanelPosition(
    { ...node, y: 580 },
    { x: 40, y: -120, k: 1 },
    1280,
  );
  expect(moved.top).toBe(680);
  const zoomed = nodePanelPosition(node, { x: 20, y: -100, k: 0.5 }, 1280);
  expect(zoomed.top).toBe(260);
  expect(first.width).toBe(680);
  expect(zoomed.width).toBe(680);
});
it("手机仅收缩面板宽度，纵向锚点不会固定到屏幕底部", () => {
  const node = { x: 200, y: 700, width: 288, height: 200 };
  const result = nodePanelPosition(node, { x: 0, y: 0, k: 1 }, 390);
  expect(result.width).toBe(358);
  expect(result.left).toBe(16);
  expect(result.top).toBe(920);
});
it("下方空间不足时翻转到节点上方，矮视口内保持可访问", () => {
  const node = { x: 500, y: 620, width: 320, height: 180 };
  const above = nodePanelPosition(node, { x: 0, y: 0, k: 1 }, 1460, 900, 216);
  expect(above.top).toBe(384);
  expect(above.left).toBeGreaterThanOrEqual(76);
  const short = nodePanelPosition(node, { x: 0, y: -580, k: 1 }, 390, 300, 216);
  expect(short.top).toBe(132);
  expect(short.top + short.maxHeight).toBe(284);
});
