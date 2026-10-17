import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { createRef } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { InfiniteCanvas } from "./infinite-canvas";

beforeEach(() => {
  vi.useFakeTimers();
  vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) =>
    setTimeout(() => callback(0), 16),
  );
  vi.stubGlobal("cancelAnimationFrame", (id: number) => clearTimeout(id));
  vi.stubGlobal(
    "PointerEvent",
    class extends MouseEvent {
      pointerId: number;
      pointerType: string;
      isPrimary: boolean;
      constructor(type: string, init: PointerEventInit = {}) {
        super(type, init);
        this.pointerId = init.pointerId ?? 1;
        this.pointerType = init.pointerType ?? "mouse";
        this.isPrimary = init.isPrimary ?? true;
      }
    },
  );
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});
function canvas() {
  const change = vi.fn(),
    select = vi.fn();
  const container = createRef<HTMLDivElement>();
  const result = render(
    <InfiniteCanvas
      containerRef={container}
      viewport={{ x: 0, y: 0, k: 1 }}
      onViewportChange={change}
      boxSelectEnabled
      onSelectArea={select}
    >
      <textarea aria-label="内部编辑" data-canvas-no-zoom />
    </InfiniteCanvas>,
  );
  const el = container.current!;
  el.setPointerCapture = vi.fn();
  el.hasPointerCapture = vi.fn().mockReturnValue(false);
  el.releasePointerCapture = vi.fn();
  return { ...result, el, change, select };
}
it("滚轮以光标为锚点缩放，编辑区滚动不改变视口", () => {
  const { el, change } = canvas();
  fireEvent.wheel(el, {
    clientX: 80,
    clientY: 60,
    deltaY: -100,
    ctrlKey: true,
  });
  act(() => vi.advanceTimersByTime(140));
  const viewport = change.mock.calls[0][0];
  expect(viewport.k).toBeGreaterThan(1);
  expect((80 - viewport.x) / viewport.k).toBeCloseTo(80);
  expect((60 - viewport.y) / viewport.k).toBeCloseTo(60);
  change.mockClear();
  fireEvent.wheel(screen.getByLabelText("内部编辑"), { deltaY: 100 });
  act(() => vi.advanceTimersByTime(140));
  expect(change).not.toHaveBeenCalled();
});
it("框选提交世界坐标，取消框选不提交", () => {
  const { el, select } = canvas();
  fireEvent.pointerDown(el, { button: 0, clientX: 40, clientY: 50 });
  fireEvent.pointerMove(window, { clientX: 200, clientY: 180 });
  fireEvent.pointerUp(window);
  expect(select).toHaveBeenCalledWith({
    x: 40,
    y: 50,
    width: 160,
    height: 130,
  });
  select.mockClear();
  fireEvent.pointerDown(el, { button: 0, clientX: 40, clientY: 50 });
  fireEvent.pointerMove(window, { clientX: 200, clientY: 180 });
  fireEvent.pointerCancel(window);
  expect(select).not.toHaveBeenCalled();
});
it("中键平移结束提交一次，卸载取消待确认视口和事件", () => {
  const { el, change, unmount } = canvas();
  fireEvent.pointerDown(el, { button: 1, clientX: 40, clientY: 50 });
  fireEvent.pointerMove(window, { clientX: 100, clientY: 90 });
  fireEvent.pointerUp(window);
  expect(change).toHaveBeenCalledWith({ x: 60, y: 40, k: 1 });
  change.mockClear();
  fireEvent.wheel(el, { clientX: 80, clientY: 60, deltaY: -100 });
  unmount();
  act(() => vi.advanceTimersByTime(500));
  expect(change).not.toHaveBeenCalled();
  expect(vi.getTimerCount()).toBe(0);
});
it("连续滚轮在最后一次输入闲置后才提交视口", () => {
  const { el, change } = canvas();
  fireEvent.wheel(el, { deltaY: -100 });
  act(() => vi.advanceTimersByTime(100));
  fireEvent.wheel(el, { deltaY: -100 });
  act(() => vi.advanceTimersByTime(10));
  fireEvent.wheel(el, { deltaY: -100 });
  act(() => vi.advanceTimersByTime(111));
  expect(change).not.toHaveBeenCalled();
  act(() => vi.advanceTimersByTime(10));
  expect(change).toHaveBeenCalledTimes(1);
});

it("双指缩放保持世界锚点，取消手势提交最终视口并清理捕获", () => {
  const { el, change } = canvas();
  fireEvent.pointerDown(el, {
    pointerType: "touch",
    pointerId: 1,
    clientX: 100,
    clientY: 100,
  });
  fireEvent.pointerDown(el, {
    pointerType: "touch",
    pointerId: 2,
    clientX: 200,
    clientY: 100,
    isPrimary: false,
  });
  fireEvent.pointerMove(window, {
    pointerType: "touch",
    pointerId: 2,
    clientX: 300,
    clientY: 100,
  });
  act(() => vi.advanceTimersByTime(20));
  fireEvent.pointerCancel(window, { pointerType: "touch", pointerId: 2 });
  expect(change).toHaveBeenCalledExactlyOnceWith({ x: -100, y: -100, k: 2 });
  fireEvent.pointerUp(window, { pointerType: "touch", pointerId: 1 });
  expect(change).toHaveBeenCalledTimes(1);
});
it("窗口失焦结束平移并释放状态，后续指针移动不继续拖画布", () => {
  const { el, change } = canvas();
  fireEvent.pointerDown(el, { button: 1, clientX: 10, clientY: 10 });
  fireEvent.pointerMove(window, { clientX: 60, clientY: 30 });
  act(() => vi.advanceTimersByTime(20));
  fireEvent.blur(window);
  expect(el.dataset.canvasPanState).not.toBe("grabbing");
  expect(change).toHaveBeenCalledExactlyOnceWith({ x: 50, y: 20, k: 1 });
  fireEvent.pointerMove(window, { clientX: 90, clientY: 50 });
  fireEvent.pointerUp(window);
  expect(change).toHaveBeenCalledTimes(1);
});
