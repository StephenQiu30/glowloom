import { createRef } from "react";
import { act, cleanup, fireEvent, render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { InfiniteCanvas } from "@/components/canvas/infinite-canvas";
import {
  cropDataUrl,
  resolveUpscaleSize,
} from "@/components/canvas/image-data";
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});
it("平移由单一指针持有，松手消费终点并取消待处理帧", () => {
  vi.useFakeTimers();
  const change = vi.fn();
  const { getByTestId } = render(
    <InfiniteCanvas
      containerRef={createRef()}
      viewport={{ x: 20, y: 30, k: 1 }}
      tool="pan"
      onViewportChange={change}
    >
      {null}
    </InfiniteCanvas>,
  );
  const surface = getByTestId("canvas-surface");
  fireEvent.pointerDown(surface, {
    pointerId: 1,
    button: 0,
    clientX: 100,
    clientY: 100,
  });
  fireEvent.pointerMove(window, { pointerId: 2, clientX: 900, clientY: 900 });
  fireEvent.pointerUp(window, { pointerId: 2, clientX: 900, clientY: 900 });
  act(() => vi.advanceTimersByTime(20));
  expect(change).not.toHaveBeenCalled();
  fireEvent.pointerMove(window, { pointerId: 1, clientX: 120, clientY: 120 });
  fireEvent.pointerUp(window, { pointerId: 1, clientX: 150, clientY: 170 });
  expect(change).toHaveBeenLastCalledWith({ x: 70, y: 100, k: 1 });
  const calls = change.mock.calls.length;
  act(() => vi.advanceTimersByTime(20));
  expect(change).toHaveBeenCalledTimes(calls);
});
it.each(["pointercancel", "lostpointercapture", "blur", "Escape"])(
  "平移 %s 恢复原视口且不执行迟到帧",
  (cancel) => {
    vi.useFakeTimers();
    const change = vi.fn();
    const { getByTestId } = render(
      <InfiniteCanvas
        containerRef={createRef()}
        viewport={{ x: 20, y: 30, k: 1 }}
        tool="pan"
        onViewportChange={change}
      >
        {null}
      </InfiniteCanvas>,
    );
    const surface = getByTestId("canvas-surface");
    fireEvent.pointerDown(surface, {
      pointerId: 1,
      button: 0,
      clientX: 100,
      clientY: 100,
    });
    fireEvent.pointerMove(window, { pointerId: 1, clientX: 150, clientY: 170 });
    if (cancel === "Escape") fireEvent.keyDown(window, { key: "Escape" });
    else if (cancel === "blur") fireEvent.blur(window);
    else
      fireEvent(
        surface,
        new PointerEvent(cancel, { pointerId: 1, bubbles: true }),
      );
    expect(change).toHaveBeenLastCalledWith({ x: 20, y: 30, k: 1 });
    act(() => vi.advanceTimersByTime(20));
    expect(change).toHaveBeenLastCalledWith({ x: 20, y: 30, k: 1 });
  },
);
it("滚轮缩放固定指针下的世界坐标，输入区域不触发缩放", () => {
  const change = vi.fn();
  const { getByTestId, getByRole } = render(
    <InfiniteCanvas
      containerRef={createRef()}
      viewport={{ x: 20, y: 30, k: 1 }}
      tool="select"
      onViewportChange={change}
    >
      <textarea data-canvas-no-zoom />
    </InfiniteCanvas>,
  );
  fireEvent.wheel(getByTestId("canvas-surface"), {
    deltaY: -100,
    ctrlKey: true,
    clientX: 220,
    clientY: 130,
  });
  const next = change.mock.calls[0][0];
  expect((220 - next.x) / next.k).toBeCloseTo(200);
  expect((130 - next.y) / next.k).toBeCloseTo(100);
  change.mockClear();
  fireEvent.wheel(getByRole("textbox"), { deltaY: -100 });
  expect(change).not.toHaveBeenCalled();
});
it("普通触控板平移，Shift滚轮横移，不改缩放比例", () => {
  const change = vi.fn();
  const { getByTestId } = render(
    <InfiniteCanvas
      containerRef={createRef()}
      viewport={{ x: 20, y: 30, k: 2 }}
      tool="select"
      onViewportChange={change}
    >
      {null}
    </InfiniteCanvas>,
  );
  const surface = getByTestId("canvas-surface");
  fireEvent.wheel(surface, { deltaX: 12, deltaY: 24 });
  expect(change).toHaveBeenLastCalledWith({ x: 8, y: 6, k: 2 });
  fireEvent.wheel(surface, { deltaY: 2, deltaMode: 1, shiftKey: true });
  expect(change).toHaveBeenLastCalledWith({ x: -12, y: 30, k: 2 });
});
it("本地缩放保持比例；坏图片明确失败而不永久挂起", async () => {
  expect(resolveUpscaleSize(800, 400, 2048)).toEqual({
    width: 2048,
    height: 1024,
  });
  vi.stubGlobal(
    "Image",
    class {
      onerror?: () => void;
      set src(_s: string) {
        this.onerror?.();
      }
    },
  );
  await expect(cropDataUrl("broken")).rejects.toThrow("图片无法解码");
});
it("菜单和工具按钮的空格操作不被画布平移快捷键吞掉", () => {
  render(
    <InfiniteCanvas
      containerRef={createRef()}
      viewport={{ x: 0, y: 0, k: 1 }}
      tool="select"
      onViewportChange={vi.fn()}
    >
      <button>工具</button>
      <div role="menu">
        <div role="menuitem" tabIndex={0}>
          菜单项
        </div>
      </div>
    </InfiniteCanvas>,
  );
  for (const element of [
    document.querySelector("button")!,
    document.querySelector("[role=menuitem]")!,
  ]) {
    const event = new KeyboardEvent("keydown", {
      key: " ",
      code: "Space",
      bubbles: true,
      cancelable: true,
    });
    element.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(false);
  }
});
