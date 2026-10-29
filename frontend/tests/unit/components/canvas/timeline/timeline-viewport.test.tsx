import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  timelineTrackGeometry,
  useTimelineViewport,
} from "@/components/canvas/timeline/use-timeline-viewport";
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
function viewport(width = 1438, scale = 1) {
  const element = document.createElement("div");
  Object.defineProperty(element, "clientWidth", { value: width });
  vi.spyOn(element, "getBoundingClientRect").mockReturnValue({
    left: 100,
    width: width * scale,
  } as DOMRect);
  return element;
}
it("matches the 16-second fullscreen content width and fills the viewport at low zoom", () => {
  expect(timelineTrackGeometry(16000, 180, false, 1438)).toEqual({
    offset: 52,
    lane: 2936,
    content: 2996,
  });
  expect(timelineTrackGeometry(16000, 14, false, 1438)).toEqual({
    offset: 52,
    lane: 1378,
    content: 1438,
  });
  expect(timelineTrackGeometry(9000, 180, false, 1438).content).toBe(1736);
});
it("uses the compact node's 30-second ruler, 32.1 density and 77px origin", () => {
  const h = renderHook(() => useTimelineViewport(4000, true));
  expect(h.result.current.zoom).toBe(32.1);
  expect(h.result.current.geometry).toEqual({
    offset: 77,
    lane: 963,
    content: 1040,
  });
  expect(timelineTrackGeometry(60000, 32.1, true, 1000)).toEqual({
    offset: 77,
    lane: 1932,
    content: 2009,
  });
});
it("anchors slider/button zoom to the viewport center and clamps the scroll at content end", () => {
  const h = renderHook(() => useTimelineViewport(16000, false));
  const element = viewport();
  act(() => h.result.current.scrollRef(element));
  act(() => h.result.current.changeZoom(180));
  expect(h.result.current.geometry.content).toBe(2996);
  expect(element.scrollLeft).toBe(1558);
  act(() => h.result.current.changeZoom(14));
  expect(element.scrollLeft).toBe(0);
});
it("preserves the center time when no end clamp is required", () => {
  const h = renderHook(() => useTimelineViewport(60000, false));
  const element = viewport(1000);
  act(() => h.result.current.scrollRef(element));
  element.scrollLeft = 100;
  act(() => h.result.current.changeZoom(42.8));
  expect(element.scrollLeft).toBeCloseTo(648);
});
it("anchors modifier wheel zoom to pointer position in scaled canvas coordinates", () => {
  const h = renderHook(() => useTimelineViewport(60000, false));
  const element = viewport(1000, 2);
  act(() => h.result.current.scrollRef(element));
  element.scrollLeft = 100;
  const event = new WheelEvent("wheel", {
    ctrlKey: true,
    deltaY: -1 / 0.006,
    clientX: 500,
    cancelable: true,
  });
  act(() => element.dispatchEvent(event));
  expect(event.defaultPrevented).toBe(true);
  expect(h.result.current.zoom).toBeCloseTo(42.8);
  expect(element.scrollLeft).toBeCloseTo(348);
  const normal = new WheelEvent("wheel", { deltaY: -10, cancelable: true });
  act(() => element.dispatchEvent(normal));
  expect(normal.defaultPrevented).toBe(false);
  expect(h.result.current.zoom).toBeCloseTo(42.8);
});
it("clamps scrolling when a trim reduces content length", () => {
  const h = renderHook(({ duration }) => useTimelineViewport(duration, false), {
    initialProps: { duration: 60000 },
  });
  const element = viewport();
  act(() => h.result.current.scrollRef(element));
  act(() => h.result.current.changeZoom(180));
  element.scrollLeft = 9000;
  h.rerender({ duration: 16000 });
  expect(element.scrollLeft).toBe(1558);
});
it("accounts for trimmed first-video headroom and enforces normal scroll minimum", () => {
  const h = renderHook(
    ({ headroom, trimming }) =>
      useTimelineViewport(16000, false, headroom, trimming),
    { initialProps: { headroom: 1000, trimming: false } },
  );
  const element = viewport();
  act(() => h.result.current.scrollRef(element));
  expect(element.scrollLeft).toBeCloseTo(21.4);
  expect(h.result.current.geometry.offset).toBeCloseTo(73.4);
  element.scrollLeft = 0;
  act(() => element.dispatchEvent(new Event("scroll")));
  expect(element.scrollLeft).toBeCloseTo(21.4);
  h.rerender({ headroom: 500, trimming: true });
  element.scrollLeft = 0;
  act(() => element.dispatchEvent(new Event("scroll")));
  expect(element.scrollLeft).toBe(0);
  h.rerender({ headroom: 500, trimming: false });
  expect(element.scrollLeft).toBeCloseTo(10.7);
  h.rerender({ headroom: 0, trimming: false });
  expect(element.scrollLeft).toBe(0);
});
it("scales headroom along with center-anchored zoom", () => {
  const h = renderHook(() => useTimelineViewport(60000, false, 1000));
  const element = viewport(1000);
  act(() => h.result.current.scrollRef(element));
  element.scrollLeft = 121.4;
  act(() => h.result.current.changeZoom(42.8));
  expect(element.scrollLeft).toBeCloseTo(690.8);
  expect(h.result.current.geometry.offset).toBeCloseTo(94.8);
});
