import { createRef } from "react";
import { act, cleanup, fireEvent, render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { InfiniteCanvas } from "@/components/canvas/editor/infinite-canvas";
vi.mock("@/components/canvas/runtime/store/use-theme-store", () => ({
  useThemeStore: (select: (state: { theme: string }) => unknown) =>
    select({ theme: "dark" }),
}));
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});
it("suspends wheel motion until 150ms idle plus the next frame and cancels on unmount", () => {
  vi.useFakeTimers();
  vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) =>
    setTimeout(() => cb(0), 16),
  );
  vi.stubGlobal("cancelAnimationFrame", (id: number) => clearTimeout(id));
  const moving = vi.fn();
  const view = render(
    <InfiniteCanvas
      containerRef={createRef()}
      viewport={{ x: 0, y: 0, k: 1 }}
      tool="select"
      onViewportChange={vi.fn()}
      onViewportMotionChange={moving}
    >
      {null}
    </InfiniteCanvas>,
  );
  const canvas = view.getByLabelText("创作画布");
  fireEvent.wheel(canvas, {
    deltaY: 100,
    ctrlKey: true,
    clientX: 100,
    clientY: 100,
  });
  expect(moving).toHaveBeenLastCalledWith(true);
  act(() => vi.advanceTimersByTime(150));
  expect(moving).toHaveBeenLastCalledWith(true);
  act(() => vi.advanceTimersByTime(16));
  expect(moving).toHaveBeenLastCalledWith(false);
  fireEvent.wheel(canvas, { deltaY: 100, ctrlKey: true });
  view.unmount();
  expect(vi.getTimerCount()).toBe(0);
});
it.each([0.5, 1, 2])("keeps a one-screen-pixel line at zoom %s", (k) => {
  const view = render(
    <InfiniteCanvas
      containerRef={createRef()}
      viewport={{ x: 0, y: 0, k }}
      tool="select"
      onViewportChange={vi.fn()}
    >
      {null}
    </InfiniteCanvas>,
  );
  const root = view.getByLabelText("创作画布");
  expect(
    parseFloat(
      root.style.getPropertyValue("--canvas-connection-stroke-world-width"),
    ) * k,
  ).toBe(1);
});
