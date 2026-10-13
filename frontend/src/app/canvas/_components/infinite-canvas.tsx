"use client";
// Adapted from BeefTV a2196d32782be6090beb003590ce11a956a423e1.
// MIT: public/licenses/beeftv.txt; provenance: public/THIRD_PARTY_NOTICES.md.
import React, {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import {
  applyCanvasLiveViewport,
  type ViewportTransform,
  type SelectionArea,
  selectionArea,
  resolveCanvasPointerIntent,
} from "./canvas-viewport";
type CanvasBackgroundMode = "dots" | "lines" | "blank";
type InfiniteCanvasProps = {
  interactive?: boolean;
  containerRef: React.RefObject<HTMLDivElement | null>;
  viewport: ViewportTransform;
  backgroundMode?: CanvasBackgroundMode;
  onViewportChange: (viewport: ViewportTransform) => void;
  onViewportPreviewChange?: (viewport: ViewportTransform) => void;
  onCanvasMouseDown?: (event: React.PointerEvent<HTMLDivElement>) => void;
  onSelectArea?: (area: SelectionArea) => void;
  boxSelectEnabled?: boolean;
  onCanvasDoubleClick?: (event: React.MouseEvent<HTMLDivElement>) => void;
  onCanvasDeselect?: () => void;
  onContextMenu?: (event: React.MouseEvent) => void;
  onDrop?: (event: React.DragEvent<HTMLDivElement>) => void;
  onFileDragEnter?: (event: React.DragEvent<HTMLDivElement>) => void;
  onFileDragLeave?: (event: React.DragEvent<HTMLDivElement>) => void;
  onFileDragOver?: (event: React.DragEvent<HTMLDivElement>) => void;
  children: React.ReactNode;
  graphicsLayer?: React.ReactNode;
};

const CANVAS_WHEEL_IGNORE_SELECTOR =
  "[data-canvas-no-zoom],[data-canvas-wheel-scroll],[role=dialog],[role=alertdialog],[role=menu],[role=listbox],[data-slot=popover-content]";
const CANVAS_POINTER_IGNORE_SELECTOR =
  "input,textarea,select,button,[contenteditable=true],[data-canvas-no-zoom],[data-connection-create-menu],[role=dialog],[role=alertdialog],[role=menu],[role=listbox],[data-slot=popover-content]";
const WHEEL_ZOOM_DELTA = 72;
const TRACKPAD_PINCH_ZOOM_DELTA = 24;

type TouchPoint = { x: number; y: number };

type PinchState = {
  active: boolean;
  pointerIds: [number, number];
  initialDistance: number;
  worldX: number;
  worldY: number;
  initialScale: number;
};

export function InfiniteCanvas({
  interactive = true,
  containerRef,
  viewport,
  backgroundMode = "lines",
  onViewportChange,
  onViewportPreviewChange,
  onCanvasMouseDown,
  onSelectArea,
  boxSelectEnabled = false,
  onCanvasDoubleClick,
  onCanvasDeselect,
  onContextMenu,
  onDrop,
  onFileDragEnter,
  onFileDragLeave,
  onFileDragOver,
  graphicsLayer,
  children,
}: InfiniteCanvasProps) {
  const selection = useRef<{
    id: number;
    start: { x: number; y: number };
    end: { x: number; y: number };
  } | null>(null);
  const selectionElement = useRef<HTMLDivElement>(null);
  const [selecting, setSelecting] = useState(false);
  const panState = useRef({
    isPanning: false,
    pointerId: -1,
    startX: 0,
    startY: 0,
    initialX: 0,
    initialY: 0,
    hasMoved: false,
  });
  const viewportRef = useRef(viewport);
  const scaleRef = useRef(viewport.k);
  const containerRectRef = useRef<DOMRect | null>(null);
  const frameRef = useRef<number | null>(null);
  const nextViewportRef = useRef<ViewportTransform | null>(null);
  const syncTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const interactingRef = useRef(false);
  const touchPointsRef = useRef(new Map<number, TouchPoint>());
  const pinchStateRef = useRef<PinchState>({
    active: false,
    pointerIds: [-1, -1],
    initialDistance: 1,
    worldX: 0,
    worldY: 0,
    initialScale: viewport.k,
  });
  const spacePressedRef = useRef(false);
  const [isSpacePressed, setIsSpacePressed] = useState(false);
  const [isPanning, setIsPanning] = useState(false);

  useLayoutEffect(() => {
    if (interactingRef.current) return;
    viewportRef.current = viewport;
    scaleRef.current = viewport.k;
    applyCanvasLiveViewport(containerRef.current, viewport);
  }, [containerRef, viewport]);

  useEffect(() => {
    const container = containerRef.current;
    const touchPoints = touchPointsRef.current;
    return () => {
      if (frameRef.current) cancelAnimationFrame(frameRef.current);
      if (syncTimerRef.current) clearTimeout(syncTimerRef.current);
      for (const id of [
        panState.current.pointerId,
        selection.current?.id ?? -1,
        ...touchPoints.keys(),
      ]) {
        if (id >= 0 && container?.hasPointerCapture(id))
          container.releasePointerCapture(id);
      }
      delete container?.dataset.canvasViewportInteracting;
    };
  }, [containerRef]);

  const syncViewport = useCallback(() => {
    if (interactive) onViewportChange(viewportRef.current);
  }, [interactive, onViewportChange]);

  const scheduleViewportChange = useCallback(
    (next: ViewportTransform, commitAfterIdle = false) => {
      viewportRef.current = next;
      scaleRef.current = next.k;
      onViewportPreviewChange?.(next);
      const container = containerRef.current;
      if (container) container.dataset.canvasViewportInteracting = "true";
      nextViewportRef.current = next;
      if (!frameRef.current)
        frameRef.current = requestAnimationFrame(() => {
          frameRef.current = null;
          const pending = nextViewportRef.current;
          if (!pending) return;
          applyCanvasLiveViewport(containerRef.current, pending);
        });
      if (!commitAfterIdle) return;
      if (syncTimerRef.current) clearTimeout(syncTimerRef.current);
      syncTimerRef.current = setTimeout(() => {
        interactingRef.current = false;
        delete containerRef.current?.dataset.canvasViewportInteracting;
        syncViewport();
        syncTimerRef.current = null;
      }, 120);
    },
    [containerRef, onViewportPreviewChange, syncViewport],
  );

  useEffect(() => {
    if (!interactive) return;
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.code !== "Space") return;
      if (
        event.target instanceof Element &&
        event.target.closest(CANVAS_POINTER_IGNORE_SELECTOR)
      )
        return;
      event.preventDefault();
      spacePressedRef.current = true;
      setIsSpacePressed(true);
    };

    const handleKeyUp = (event: KeyboardEvent) => {
      if (event.code !== "Space") return;
      spacePressedRef.current = false;
      setIsSpacePressed(false);
    };

    const handleBlur = () => {
      spacePressedRef.current = false;
      setIsSpacePressed(false);
    };

    window.addEventListener("keydown", handleKeyDown);
    window.addEventListener("keyup", handleKeyUp);
    window.addEventListener("blur", handleBlur);
    return () => {
      window.removeEventListener("keydown", handleKeyDown);
      window.removeEventListener("keyup", handleKeyUp);
      window.removeEventListener("blur", handleBlur);
    };
  }, [interactive]);

  const handleWheel = useCallback(
    (event: WheelEvent) => {
      const target = event.target instanceof Element ? event.target : null;
      const deltaX = wheelDeltaToPixels(event.deltaX, event.deltaMode);
      const deltaY = wheelDeltaToPixels(event.deltaY, event.deltaMode);
      const absX = Math.abs(deltaX);
      const absY = Math.abs(deltaY);
      const isPinchZoom = event.ctrlKey || event.metaKey;
      if (target?.closest(CANVAS_WHEEL_IGNORE_SELECTOR) && !isPinchZoom) {
        // 内部区域保留纵向滚动，但横向手势不能泄漏为 macOS 浏览器前进/后退。
        if (event.shiftKey || absX > absY) event.preventDefault();
        return;
      }

      // Ctrl/Meta + 滚轮在画布内始终由画布接管，避免浮层区域触发浏览器页面缩放。
      event.preventDefault();
      interactingRef.current = true;
      const current = viewportRef.current;
      const rawAbsY = Math.abs(event.deltaY);
      const looksLikeMouseWheel =
        event.deltaMode !== 0 ||
        (rawAbsY >= 80 &&
          Math.abs(rawAbsY - Math.round(rawAbsY / 100) * 100) < 1);
      const looksLikeTrackpadPan =
        !isPinchZoom &&
        (event.shiftKey || absX > 0 || (!looksLikeMouseWheel && absY > 0));

      if (looksLikeTrackpadPan) {
        const panX = event.shiftKey && absX < 1 ? deltaY : deltaX;
        scheduleViewportChange(
          {
            x: current.x - panX,
            y: current.y - (event.shiftKey && absX < 1 ? 0 : deltaY),
            k: current.k,
          },
          true,
        );
        return;
      }

      const rect =
        containerRectRef.current ||
        containerRef.current?.getBoundingClientRect();
      if (!rect) return;
      const mouseX = event.clientX - rect.left;
      const mouseY = event.clientY - rect.top;
      const zoomDelta =
        isPinchZoom && !looksLikeMouseWheel
          ? TRACKPAD_PINCH_ZOOM_DELTA
          : WHEEL_ZOOM_DELTA;
      const factor = Math.pow(1.1, -deltaY / zoomDelta);
      const newScale = clampScale(current.k * factor);
      const worldX = (mouseX - current.x) / current.k;
      const worldY = (mouseY - current.y) / current.k;

      scheduleViewportChange(
        {
          x: mouseX - worldX * newScale,
          y: mouseY - worldY * newScale,
          k: newScale,
        },
        true,
      );
    },
    [containerRef, scheduleViewportChange],
  );

  const handlePointerDown = (event: React.PointerEvent<HTMLDivElement>) => {
    if (!interactive) return;
    const target = event.target instanceof Element ? event.target : null;
    // Radix 浮层通过 Portal 渲染到节点 DOM 之外；若不统一排除，会被误判为画布空白并捕获指针。
    if (target?.closest(CANVAS_POINTER_IGNORE_SELECTOR)) return;
    const isBackgroundClick = !target?.closest(
      "[data-node-id],[data-connection-id]",
    );
    const isTouch = event.pointerType === "touch";

    const pointerIntent = resolveCanvasPointerIntent({
      altKey: event.altKey,
      background: isBackgroundClick,
      boxSelectEnabled,
      button: event.button,
      ctrlKey: event.ctrlKey,
      metaKey: event.metaKey,
      pointerType: event.pointerType,
      shiftKey: event.shiftKey,
      spacePressed: spacePressedRef.current,
    });
    if (pointerIntent === "select") {
      event.preventDefault();
      event.currentTarget.setPointerCapture(event.pointerId);
      const rect = event.currentTarget.getBoundingClientRect();
      const start = {
        x: event.clientX - rect.left,
        y: event.clientY - rect.top,
      };
      selection.current = { id: event.pointerId, start, end: start };
      setSelecting(true);
      onCanvasMouseDown?.(event);
      return;
    }

    if (isTouch) {
      touchPointsRef.current.set(event.pointerId, {
        x: event.clientX,
        y: event.clientY,
      });
      if (touchPointsRef.current.size >= 2) {
        const [[firstId, first], [secondId, second]] = Array.from(
          touchPointsRef.current.entries(),
        );
        event.preventDefault();
        event.currentTarget.setPointerCapture(firstId);
        event.currentTarget.setPointerCapture(secondId);
        const rect =
          containerRectRef.current ||
          event.currentTarget.getBoundingClientRect();
        const current = viewportRef.current;
        const centerX = (first.x + second.x) / 2 - rect.left;
        const centerY = (first.y + second.y) / 2 - rect.top;
        pinchStateRef.current = {
          active: true,
          pointerIds: [firstId, secondId],
          initialDistance: Math.max(
            Math.hypot(second.x - first.x, second.y - first.y),
            1,
          ),
          worldX: (centerX - current.x) / current.k,
          worldY: (centerY - current.y) / current.k,
          initialScale: current.k,
        };
        panState.current.isPanning = false;
        interactingRef.current = true;
        return;
      }

      if (!isBackgroundClick) return;
      event.preventDefault();
      event.currentTarget.setPointerCapture(event.pointerId);
      if (!event.isPrimary) return;
      const current = viewportRef.current;
      interactingRef.current = true;
      panState.current = {
        isPanning: true,
        pointerId: event.pointerId,
        startX: event.clientX,
        startY: event.clientY,
        initialX: current.x,
        initialY: current.y,
        hasMoved: false,
      };
      setIsPanning(true);

      return;
    }

    if (pointerIntent === "pan") {
      const current = viewportRef.current;
      event.preventDefault();
      event.currentTarget.setPointerCapture(event.pointerId);
      interactingRef.current = true;
      panState.current = {
        isPanning: true,
        pointerId: event.pointerId,
        startX: event.clientX,
        startY: event.clientY,
        initialX: current.x,
        initialY: current.y,
        hasMoved: false,
      };
      setIsPanning(true);
    }
  };

  useEffect(() => {
    if (!interactive) return;
    const handlePointerMove = (event: PointerEvent) => {
      const area = selection.current;
      if (area?.id === event.pointerId) {
        const rect = containerRef.current?.getBoundingClientRect();
        if (!rect) return;
        area.end = {
          x: event.clientX - rect.left,
          y: event.clientY - rect.top,
        };
        if (selectionElement.current) {
          const box = selectionElement.current;
          box.style.left = `${Math.min(area.start.x, area.end.x)}px`;
          box.style.top = `${Math.min(area.start.y, area.end.y)}px`;
          box.style.width = `${Math.abs(area.end.x - area.start.x)}px`;
          box.style.height = `${Math.abs(area.end.y - area.start.y)}px`;
        }
        return;
      }
      if (
        event.pointerType === "touch" &&
        touchPointsRef.current.has(event.pointerId)
      ) {
        touchPointsRef.current.set(event.pointerId, {
          x: event.clientX,
          y: event.clientY,
        });
        const pinch = pinchStateRef.current;
        if (pinch.active) {
          const first = touchPointsRef.current.get(pinch.pointerIds[0]);
          const second = touchPointsRef.current.get(pinch.pointerIds[1]);
          const rect =
            containerRectRef.current ||
            containerRef.current?.getBoundingClientRect();
          if (!first || !second || !rect) return;
          event.preventDefault();
          const centerX = (first.x + second.x) / 2 - rect.left;
          const centerY = (first.y + second.y) / 2 - rect.top;
          const distance = Math.max(
            Math.hypot(second.x - first.x, second.y - first.y),
            1,
          );
          const scale = clampScale(
            pinch.initialScale * (distance / pinch.initialDistance),
          );
          scheduleViewportChange({
            x: centerX - pinch.worldX * scale,
            y: centerY - pinch.worldY * scale,
            k: scale,
          });
          return;
        }
      }

      if (
        !panState.current.isPanning ||
        panState.current.pointerId !== event.pointerId
      )
        return;

      const dx = event.clientX - panState.current.startX;
      const dy = event.clientY - panState.current.startY;
      if (Math.abs(dx) > 3 || Math.abs(dy) > 3) {
        panState.current.hasMoved = true;
      }

      scheduleViewportChange({
        x: panState.current.initialX + dx,
        y: panState.current.initialY + dy,
        k: scaleRef.current,
      });
    };

    const handlePointerEnd = (event: PointerEvent) => {
      const container = containerRef.current;
      if (container?.hasPointerCapture(event.pointerId))
        container.releasePointerCapture(event.pointerId);
      const area = selection.current;
      if (area?.id === event.pointerId) {
        if (event.type === "pointerup")
          onSelectArea?.(
            selectionArea(area.start, area.end, viewportRef.current),
          );
        selection.current = null;
        setSelecting(false);
        return;
      }
      if (
        event.pointerType === "touch" &&
        pinchStateRef.current.active &&
        pinchStateRef.current.pointerIds.includes(event.pointerId)
      ) {
        pinchStateRef.current.active = false;
        touchPointsRef.current.clear();
        panState.current.isPanning = false;
        panState.current.pointerId = -1;
        interactingRef.current = false;
        if (syncTimerRef.current) clearTimeout(syncTimerRef.current);
        delete containerRef.current?.dataset.canvasViewportInteracting;
        syncViewport();
        setIsPanning(false);

        return;
      }

      if (event.pointerType === "touch")
        touchPointsRef.current.delete(event.pointerId);
      if (
        !panState.current.isPanning ||
        panState.current.pointerId !== event.pointerId
      )
        return;

      if (event.type === "pointerup" && !panState.current.hasMoved) {
        onCanvasDeselect?.();
      }
      panState.current.isPanning = false;
      panState.current.pointerId = -1;
      interactingRef.current = false;
      if (syncTimerRef.current) clearTimeout(syncTimerRef.current);
      delete containerRef.current?.dataset.canvasViewportInteracting;
      syncViewport();
      setIsPanning(false);
    };

    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", handlePointerEnd);
    window.addEventListener("pointercancel", handlePointerEnd);
    return () => {
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("pointerup", handlePointerEnd);
      window.removeEventListener("pointercancel", handlePointerEnd);
    };
  }, [
    interactive,
    containerRef,
    onCanvasDeselect,
    onSelectArea,
    scheduleViewportChange,
    syncViewport,
  ]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container || !interactive) return;
    const updateRect = () => {
      containerRectRef.current = container.getBoundingClientRect();
    };
    updateRect();
    const observer =
      typeof ResizeObserver === "undefined"
        ? undefined
        : new ResizeObserver(updateRect);
    observer?.observe(container);
    window.addEventListener("resize", updateRect);
    container.addEventListener("wheel", handleWheel, {
      passive: false,
      capture: true,
    });
    return () => {
      observer?.disconnect();
      window.removeEventListener("resize", updateRect);
      container.removeEventListener("wheel", handleWheel, { capture: true });
    };
  }, [interactive, containerRef, handleWheel]);

  return (
    <div
      ref={containerRef}
      aria-label="画布工作区"
      data-canvas-viewport
      data-canvas-pan-state={
        isPanning
          ? "grabbing"
          : isSpacePressed || !boxSelectEnabled
            ? "grab"
            : undefined
      }
      className={`relative h-full w-full touch-none overflow-hidden select-none ${isPanning ? "cursor-grabbing" : isSpacePressed || !boxSelectEnabled ? "cursor-grab" : "canvas-cursor-select"}`}
      style={
        {
          background: "var(--canvas-bg)",
          overscrollBehavior: "none",
          "--canvas-live-x": `${viewport.x}px`,
          "--canvas-live-y": `${viewport.y}px`,
          "--canvas-live-scale": viewport.k,
          "--canvas-live-inverse-scale": 1 / Math.max(viewport.k, 0.05),
          "--canvas-committed-scale": viewport.k,
          "--canvas-live-scale-ratio": 1,
        } as React.CSSProperties
      }
      onPointerDown={handlePointerDown}
      onDoubleClick={(event) => {
        const target = event.target instanceof Element ? event.target : null;
        if (
          !target?.closest(
            "[data-node-id],[data-connection-id],[data-canvas-no-zoom]",
          )
        )
          onCanvasDoubleClick?.(event);
      }}
      onContextMenu={onContextMenu}
      onDragEnter={onFileDragEnter}
      onDragLeave={onFileDragLeave}
      onDragOver={(event) => {
        event.preventDefault();
        onFileDragOver?.(event);
      }}
      onDrop={onDrop}
    >
      <CanvasGrid mode={backgroundMode} />
      {graphicsLayer}
      {selecting && (
        <div
          ref={selectionElement}
          className="canvas-selection-box pointer-events-none absolute z-10"
        />
      )}
      <div
        data-canvas-world-layer
        className="canvas-world-layer absolute origin-top-left"
      >
        <div
          data-canvas-world-raster-layer
          className="canvas-world-raster-layer absolute origin-top-left"
        >
          {children}
        </div>
      </div>
    </div>
  );
}

function CanvasGrid({ mode }: { mode: CanvasBackgroundMode }) {
  const gridColor = "var(--canvas-grid)";
  const backgroundImage =
    mode === "dots"
      ? `radial-gradient(circle, ${gridColor} 0.8px, transparent 1px)`
      : `linear-gradient(${gridColor} 1px, transparent 1px), linear-gradient(90deg, ${gridColor} 1px, transparent 1px)`;
  if (mode === "blank") return null;

  return (
    <div
      data-canvas-grid-layer
      className="pointer-events-none absolute"
      style={{
        // 装饰网格固定在屏幕坐标，避免缩放时改变密度或产生亚像素位移闪烁。
        inset: 0,
        backgroundImage,
        backgroundSize: "48px 48px",
        opacity: mode === "dots" ? 0.34 : 0.46,
      }}
    />
  );
}

function wheelDeltaToPixels(delta: number, deltaMode: number) {
  if (deltaMode === 1) return delta * 16;
  if (deltaMode === 2) return delta * 720;
  return delta;
}

function clampScale(scale: number) {
  return Math.min(Math.max(scale, 0.05), 2);
}
