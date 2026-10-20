import { expect, it } from "vitest";
import { fitNodes, screenToWorld, zoomAt } from "./frame";

const view = { x: 100, y: 40, zoom: 2 };

it("屏幕点换算到世界坐标，缩放保持指针锚点", () => {
  expect(screenToWorld({ x: 300, y: 140 }, view)).toEqual({ x: 100, y: 50 });
  expect(zoomAt(view, { x: 300, y: 140 }, 4)).toEqual({
    x: -100,
    y: -60,
    zoom: 4,
  });
  expect(zoomAt(view, { x: 300, y: 140 }, 20).zoom).toBe(8);
  expect(zoomAt(view, { x: 300, y: 140 }, 0.01).zoom).toBe(0.08);
});

it("空图回到原点，全图适配不超过50%，选中适配不超过100%", () => {
  const frame = { left: 12, top: 40, width: 1000, height: 800 };
  expect(fitNodes([], frame)).toEqual({ x: 0, y: 0, zoom: 1 });
  const node = { x: 0, y: 0, width: 200, height: 100 };
  expect(fitNodes([node], frame).zoom).toBe(0.5);
  expect(fitNodes([node], frame, true).zoom).toBe(1);
  const fitted = fitNodes([node], frame, true);
  expect(fitted.x).toBe(12 + 500 - 100);
  expect(fitted.y).toBe(40 + 400 - 50);
});
