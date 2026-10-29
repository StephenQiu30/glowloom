"use client";
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { timelineRulerIntervals } from "./timeline-ruler";

export function timelineTrackGeometry(
  durationMS: number,
  zoom: number,
  compact: boolean,
  viewportWidth: number,
  headroomMS = 0,
) {
  const baseOffset = compact ? 77 : 52;
  const headroom = (Math.max(0, headroomMS) / 1000) * zoom;
  const offset = baseOffset + headroom;
  const trailing = compact ? 0 : 8;
  let natural: number;
  if (compact) {
    const major = timelineRulerIntervals(zoom, true).major;
    const seconds = Math.ceil(Math.max(30, durationMS / 1000) / major) * major;
    natural = Math.max(seconds * zoom, (durationMS / 1000) * zoom + 6);
  } else
    natural =
      Math.max(960, Math.ceil(Math.max(1, durationMS / 1000) * zoom)) + 56;
  const lane = Math.max(natural, viewportWidth - baseOffset - trailing);
  return { offset, lane, content: lane + offset + trailing };
}

export function useTimelineViewport(
  durationMS: number,
  compact: boolean,
  headroomMS = 0,
  trimming = false,
) {
  const [zoom, setZoom] = useState(compact ? 32.1 : 21.4);
  const viewportRef = useRef<HTMLDivElement | null>(null);
  const [viewportVersion, setViewportVersion] = useState(0);
  const scrollRef = useCallback((element: HTMLDivElement | null) => {
    if (viewportRef.current === element) return;
    viewportRef.current = element;
    setViewportVersion((version) => version + 1);
  }, []);
  const [width, setWidth] = useState(0);
  const latestZoom = useRef(zoom);
  const pendingScroll = useRef<number | null>(null);
  const geometry = timelineTrackGeometry(
    durationMS,
    zoom,
    compact,
    width,
    headroomMS,
  );
  const headroom = (Math.max(0, headroomMS) / 1000) * zoom;
  const previousHeadroom = useRef(headroom);
  const changeZoom = useCallback(
    (requested: number, anchorClientX?: number) => {
      const next = Math.max(14, Math.min(180, requested));
      const previous = latestZoom.current;
      if (!Number.isFinite(next) || next === previous) return;
      const viewport = viewportRef.current;
      if (viewport) {
        const rect = viewport.getBoundingClientRect();
        const anchor =
          anchorClientX === undefined || rect.width <= 0
            ? viewport.clientWidth / 2
            : (Math.max(0, Math.min(rect.width, anchorClientX - rect.left)) *
                viewport.clientWidth) /
              rect.width;
        const baseOffset = compact ? 77 : 52;
        const oldOffset =
          baseOffset + (Math.max(0, headroomMS) / 1000) * previous;
        const nextOffset = baseOffset + (Math.max(0, headroomMS) / 1000) * next;
        const current = pendingScroll.current ?? viewport.scrollLeft;
        pendingScroll.current = Math.max(
          0,
          (Math.max(0, current + anchor - oldOffset) / previous) * next +
            nextOffset -
            anchor,
        );
      }
      latestZoom.current = next;
      setZoom(next);
    },
    [compact, headroomMS],
  );
  useLayoutEffect(() => {
    const viewport = viewportRef.current;
    if (!viewport) return;
    const measure = () => setWidth(viewport.clientWidth);
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(viewport);
    window.addEventListener("resize", measure);
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", measure);
    };
  }, [viewportVersion]);
  useLayoutEffect(() => {
    const viewport = viewportRef.current;
    if (!viewport) return;
    const delta = headroom - previousHeadroom.current;
    const target =
      pendingScroll.current ?? viewport.scrollLeft + (trimming ? 0 : delta);
    if (!trimming) previousHeadroom.current = headroom;
    pendingScroll.current = null;
    viewport.scrollLeft = Math.min(
      Math.max(0, geometry.content - viewport.clientWidth),
      Math.max(trimming ? 0 : headroom, target),
    );
  }, [geometry.content, headroom, trimming, viewportVersion, zoom]);
  useEffect(() => {
    const viewport = viewportRef.current;
    if (!viewport) return;
    const guard = () => {
      if (!trimming && viewport.scrollLeft < headroom)
        viewport.scrollLeft = headroom;
    };
    viewport.addEventListener("scroll", guard);
    return () => viewport.removeEventListener("scroll", guard);
  }, [viewportVersion, headroom, trimming]);
  useEffect(() => {
    const viewport = viewportRef.current;
    if (!viewport) return;
    const wheel = (event: WheelEvent) => {
      if (!event.ctrlKey && !event.metaKey) return;
      event.preventDefault();
      event.stopPropagation();
      const sensitivity =
        event.deltaMode === 1 ? 0.05 : event.deltaMode === 2 ? 1 : 0.006;
      changeZoom(
        latestZoom.current * 2 ** (-event.deltaY * sensitivity),
        event.clientX,
      );
    };
    viewport.addEventListener("wheel", wheel, { passive: false });
    return () => viewport.removeEventListener("wheel", wheel);
  }, [viewportVersion, changeZoom]);
  return { zoom, changeZoom, geometry, scrollRef };
}
