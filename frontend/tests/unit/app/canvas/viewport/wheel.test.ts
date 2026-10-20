import { expect, it } from "vitest";
import { canvasWheelViewport } from "@/app/canvas/viewport/wheel";

const viewport = { x: 140, y: -50, k: 1 };
const anchor = { x: 500, y: 300 };
const wheel = {
  deltaX: 0,
  deltaY: 0,
  deltaMode: 0,
  ctrlKey: false,
  metaKey: false,
  shiftKey: false,
};

it("普通和横向手势平移，Shift将纵向输入转换为横向", () => {
  expect(
    canvasWheelViewport(
      viewport,
      { ...wheel, deltaX: 13, deltaY: 24 },
      anchor,
      800,
    ),
  ).toEqual({ x: 127, y: -74, k: 1 });
  expect(
    canvasWheelViewport(
      viewport,
      { ...wheel, deltaY: 100, shiftKey: true },
      anchor,
      800,
    ),
  ).toEqual({ x: 40, y: -50, k: 1 });
});
it("页滚动按真实画布高度归一化，不固定720px", () => {
  expect(
    canvasWheelViewport(
      viewport,
      { ...wheel, deltaY: 1, deltaMode: 2 },
      anchor,
      844,
    ),
  ).toEqual({ x: 140, y: -894, k: 1 });
  expect(
    canvasWheelViewport(
      viewport,
      { ...wheel, deltaY: 2, deltaMode: 1 },
      anchor,
      844,
    ),
  ).toEqual({ x: 140, y: -82, k: 1 });
});
it.each([
  { deltaY: 100, ctrlKey: true },
  { deltaY: 120, metaKey: true },
  { deltaY: 3, deltaMode: 1, ctrlKey: true },
  { deltaY: 12, ctrlKey: true },
  { deltaY: -12, metaKey: true },
])("鼠标滚轮及捏合缩放保持光标世界坐标：%o", (event) => {
  const next = canvasWheelViewport(
    viewport,
    { ...wheel, ...event },
    anchor,
    800,
  );
  expect(next.k).not.toBe(1);
  expect((anchor.x - next.x) / next.k).toBeCloseTo(
    (anchor.x - viewport.x) / viewport.k,
    10,
  );
  expect((anchor.y - next.y) / next.k).toBeCloseTo(
    (anchor.y - viewport.y) / viewport.k,
    10,
  );
});
it("沿用目标8%到800%的缩放边界，零输入保持不变", () => {
  expect(
    canvasWheelViewport(
      viewport,
      { ...wheel, deltaY: -10000, ctrlKey: true },
      anchor,
      800,
    ).k,
  ).toBe(8);
  expect(
    canvasWheelViewport(
      viewport,
      { ...wheel, deltaY: 10000, ctrlKey: true },
      anchor,
      800,
    ).k,
  ).toBe(0.08);
  expect(canvasWheelViewport(viewport, wheel, anchor, 800)).toEqual(viewport);
});
it("零缩放输入和达到缩放边界时不引入坐标误差或保存", () => {
  const fractional = { x: 0.1, y: 0.2, k: 0.08 };
  expect(
    canvasWheelViewport(fractional, { ...wheel, ctrlKey: true }, anchor, 800),
  ).toBe(fractional);
  expect(
    canvasWheelViewport(
      fractional,
      { ...wheel, ctrlKey: true, deltaY: 100 },
      anchor,
      800,
    ),
  ).toBe(fractional);
});
