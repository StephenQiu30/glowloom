import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { CanvasConnectionHandle } from "@/components/canvas/nodes/canvas-connection-handle";
import {
  advanceConnectionHandle,
  connectionHandleCounterScale,
  connectionHandleRest,
  connectionHandleTarget,
  connectionHandleTransform,
} from "@/components/canvas/nodes/canvas-connection-handle-geometry";

let frames: Map<number, FrameRequestCallback>;
let nextFrame: number;
let motion: MediaQueryList;
let motionListener: (() => void) | undefined;
beforeEach(() => {
  frames = new Map();
  nextFrame = 0;
  vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => {
    frames.set(++nextFrame, cb);
    return nextFrame;
  });
  vi.stubGlobal("cancelAnimationFrame", (id: number) => frames.delete(id));
  vi.stubGlobal("PointerEvent", MouseEvent);
  motion = {
    matches: false,
    addEventListener: vi.fn((_event, listener) => {
      motionListener = listener;
    }),
    removeEventListener: vi.fn(),
  } as unknown as MediaQueryList;
  vi.stubGlobal("matchMedia", () => motion);
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  motionListener = undefined;
});
function frame(time: number) {
  act(() => {
    const callbacks = [...frames.values()];
    frames.clear();
    callbacks.forEach((callback) => callback(time));
  });
}
function handle() {
  const button = screen.getByRole("button", { name: "连接输出节点" });
  const field = button.parentElement!;
  vi.spyOn(field, "getBoundingClientRect").mockReturnValue({
    left: 100,
    top: 200,
    width: 60,
    height: 120,
  } as DOMRect);
  return { button, field };
}
it("counter-scales chrome only down to 50%, and keeps the logical center separate", () => {
  expect(connectionHandleCounterScale(0.25)).toBe(2);
  expect(connectionHandleCounterScale(NaN)).toBe(1);
  expect(connectionHandleCounterScale(0)).toBe(1);
  expect(connectionHandleRest("right", 1)).toEqual({ x: 52, y: 60 });
  expect(connectionHandleRest("left", 0.25)).toEqual({ x: -14, y: 60 });
  expect(connectionHandleRest("right", 0.71).x).toBeCloseTo(60.98591549);
});
it("normalizes a static world-space box, clamps independently, and rejects invalid layout", () => {
  const rect = { left: 100, top: 200, width: 30, height: 60 };
  expect(connectionHandleTarget("right", { x: 200, y: 100 }, rect)).toEqual({
    x: 90,
    y: 0,
  });
  expect(connectionHandleTarget("left", { x: 50, y: 400 }, rect)).toEqual({
    x: -30,
    y: 120,
  });
  expect(connectionHandleTarget("left", { x: 115, y: 230 }, rect)).toEqual({
    x: 30,
    y: 60,
  });
  expect(
    connectionHandleTarget("right", { x: 0, y: 0 }, { ...rect, width: 0 }),
  ).toBeNull();
});
it("uses a first 60Hz step, 80ms exponential response, a 64ms cap, and exact convergence", () => {
  const origin = { x: 52, y: 60 },
    target = { x: 90, y: 120 };
  expect(advanceConnectionHandle(origin, target, null).point.x).toBeCloseTo(
    52 + 38 * (1 - Math.exp(-(1000 / 60) / 80)),
  );
  expect(advanceConnectionHandle(origin, target, 1000)).toEqual(
    advanceConnectionHandle(origin, target, 64),
  );
  expect(advanceConnectionHandle(origin, target, -20).point).toEqual(origin);
  expect(advanceConnectionHandle({ x: 89.96, y: 119.96 }, target, 16)).toEqual({
    point: target,
    settled: true,
  });
  expect(connectionHandleTransform({ x: 12.3456, y: 60 }, 2)).toContain(
    "12.35px, 60px",
  );
});
it("wakes from an invisible outer field, follows both axes on one RAF, and resets on leave", () => {
  render(
    <CanvasConnectionHandle
      scale={1}
      side="right"
      visible={false}
      onMouseDown={vi.fn()}
    />,
  );
  const { button, field } = handle();
  expect(field.dataset.visible).toBe("false");
  expect(button.tabIndex).toBe(-1);
  fireEvent.pointerEnter(field, { clientX: 180, clientY: 300 });
  fireEvent.pointerMove(field, { clientX: 190, clientY: 320 });
  expect(field.dataset.visible).toBe("true");
  expect(button.tabIndex).toBe(0);
  expect(frames.size).toBe(1);
  frame(100);
  expect(button.style.transform).toBe(
    connectionHandleTransform(
      advanceConnectionHandle({ x: 52, y: 60 }, { x: 90, y: 120 }, null).point,
      1,
    ),
  );
  frame(164);
  expect(frames.size).toBe(1);
  fireEvent.pointerLeave(field);
  expect(button.style.transform).toBe(
    connectionHandleTransform({ x: 52, y: 60 }, 1),
  );
  expect(field.dataset.canvasConnectionGlyphTracking).toBeUndefined();
  expect(field.dataset.visible).toBe("false");
  expect(frames.size).toBe(0);
});
it("retains selected visibility across leave and routes mouse and keyboard without double click dispatch", () => {
  const start = vi.fn();
  render(
    <CanvasConnectionHandle
      scale={1}
      side="right"
      visible
      onMouseDown={start}
    />,
  );
  const { button, field } = handle();
  fireEvent.mouseDown(field, { button: 2 });
  expect(start).not.toHaveBeenCalled();
  fireEvent.mouseDown(field, { button: 0 });
  fireEvent.click(button, { detail: 1 });
  expect(start).toHaveBeenCalledTimes(1);
  fireEvent.click(button, { detail: 0 });
  expect(start).toHaveBeenCalledTimes(2);
  fireEvent.pointerEnter(field, { clientX: 180, clientY: 300 });
  fireEvent.pointerLeave(field);
  expect(field.dataset.visible).toBe("true");
  expect(button.tabIndex).toBe(0);
});
it("snaps immediately for reduced motion and responds to a live preference change", () => {
  render(
    <CanvasConnectionHandle
      scale={1}
      side="right"
      visible
      onMouseDown={vi.fn()}
    />,
  );
  const { button, field } = handle();
  fireEvent.pointerMove(field, { clientX: 180, clientY: 300 });
  frame(100);
  act(() => {
    Object.defineProperty(motion, "matches", {
      value: true,
      configurable: true,
    });
    motionListener?.();
  });
  expect(button.style.transform).toBe(
    connectionHandleTransform({ x: 80, y: 100 }, 1),
  );
  expect(frames.size).toBe(0);
  fireEvent.pointerMove(field, { clientX: 190, clientY: 310 });
  frame(120);
  expect(button.style.transform).toBe(
    connectionHandleTransform({ x: 90, y: 110 }, 1),
  );
  expect(frames.size).toBe(0);
});
it("cancels work on zoom, disable and unmount; disabled handles cannot activate", () => {
  const start = vi.fn();
  const view = render(
    <CanvasConnectionHandle
      scale={1}
      side="right"
      visible
      onMouseDown={start}
    />,
  );
  const { button, field } = handle();
  fireEvent.pointerMove(field, { clientX: 180, clientY: 300 });
  view.rerender(
    <CanvasConnectionHandle
      scale={0.25}
      side="right"
      visible
      onMouseDown={start}
    />,
  );
  expect(frames.size).toBe(0);
  expect(button.style.transform).toBe(
    connectionHandleTransform({ x: 74, y: 60 }, 2),
  );
  fireEvent.pointerMove(field, { clientX: 180, clientY: 300 });
  view.rerender(
    <CanvasConnectionHandle
      scale={0.25}
      side="right"
      visible
      disabled
      onMouseDown={start}
    />,
  );
  fireEvent.mouseDown(field, { button: 0 });
  fireEvent.click(button, { detail: 0 });
  expect(start).not.toHaveBeenCalled();
  expect(frames.size).toBe(0);
  expect(button.tabIndex).toBe(-1);
  view.rerender(
    <CanvasConnectionHandle
      scale={1}
      side="right"
      visible
      onMouseDown={start}
    />,
  );
  fireEvent.pointerMove(field, { clientX: 180, clientY: 300 });
  view.unmount();
  expect(frames.size).toBe(0);
  expect(motion.removeEventListener).toHaveBeenCalled();
});
it("moves the aggregate glyph from its centered rest point and preserves fixed chrome across zoom", () => {
  const view = render(
    <CanvasConnectionHandle
      variant="aggregate"
      scale={0.71}
      side="right"
      visible
      onMouseDown={vi.fn()}
    />,
  );
  const button = screen.getByRole("button", { name: "为选中节点创建后接节点" });
  const field = button.parentElement!;
  const glyph = field.querySelector<HTMLElement>(
    "[data-canvas-connection-glyph]",
  )!;
  vi.spyOn(field, "getBoundingClientRect").mockReturnValue({
    left: 100,
    top: 200,
    width: 60,
    height: 120,
  } as DOMRect);
  fireEvent.pointerMove(field, { clientX: 170, clientY: 300 });
  frame(100);
  expect(glyph.style.transform).toBe(
    connectionHandleTransform(
      advanceConnectionHandle({ x: 30, y: 60 }, { x: 70, y: 100 }, null).point,
      1,
    ),
  );
  fireEvent.pointerLeave(field);
  expect(glyph.style.transform).toBe(
    connectionHandleTransform({ x: 30, y: 60 }, 1),
  );
  expect(frames.size).toBe(0);
  view.rerender(
    <CanvasConnectionHandle
      variant="aggregate"
      scale={0.25}
      side="right"
      visible
      onMouseDown={vi.fn()}
    />,
  );
  expect(glyph.style.transform).toBe(
    connectionHandleTransform({ x: 30, y: 60 }, 1),
  );
  expect(field.style.getPropertyValue("--canvas-handle-counter-scale")).toBe(
    "1",
  );
  view.unmount();
  expect(frames.size).toBe(0);
});
