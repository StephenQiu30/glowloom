// Adapted from basketikun/infinite-canvas @ 9f2bd9f (MIT).
// Copyright (c) 2026 basketikun. See /licenses/infinite-canvas.txt.
import React, { useEffect, useRef, useState } from "react";

import { theme, type CanvasBackgroundMode } from "./theme";
import { canvasWheelViewport } from "./wheel";

import type { ViewportTransform } from "./types";

type InfiniteCanvasProps = {
  containerRef: React.RefObject<HTMLDivElement | null>;
  viewport: ViewportTransform;
  tool: "select" | "pan";
  backgroundMode?: CanvasBackgroundMode;
  onViewportChange: (viewport: ViewportTransform) => void;
  onCanvasMouseDown?: (event: React.PointerEvent<HTMLDivElement>) => void;
  onCanvasDeselect?: () => void;
  onCanvasDoubleClick?: (event: React.MouseEvent<HTMLDivElement>) => void;
  onContextMenu?: (event: React.MouseEvent) => void;
  onDrop?: (event: React.DragEvent<HTMLDivElement>) => void;
  children: React.ReactNode;
};

export function InfiniteCanvas({
  containerRef,
  viewport,
  tool,
  backgroundMode = "dots",
  onViewportChange,
  onCanvasMouseDown,
  onCanvasDeselect,
  onCanvasDoubleClick,
  onContextMenu,
  onDrop,
  children,
}: InfiniteCanvasProps) {
  const panState = useRef({
    isPanning: false,
    pointerId: -1,
    initialK: 1,
    startX: 0,
    startY: 0,
    initialX: 0,
    initialY: 0,
    hasMoved: false,
    startedOnBackground: false,
  });
  const frameRef = useRef<number | null>(null);
  const nextViewportRef = useRef<ViewportTransform | null>(null);
  const [isSpacePressed, setIsSpacePressed] = useState(false);
  const [isControlPressed, setIsControlPressed] = useState(false);
  const [isPanning, setIsPanning] = useState(false);

  useEffect(
    () => () => {
      if (frameRef.current) cancelAnimationFrame(frameRef.current);
    },
    [],
  );

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Control") setIsControlPressed(true);
      if (event.code !== "Space") return;
      const target = event.target instanceof Element ? event.target : null;
      if (
        event.target instanceof HTMLInputElement ||
        event.target instanceof HTMLTextAreaElement ||
        event.target instanceof HTMLSelectElement ||
        target?.closest(
          "[contenteditable='true'],button,[role=menu],[role=listbox],[role=dialog]",
        )
      )
        return;
      event.preventDefault();
      setIsSpacePressed(true);
    };

    const handleKeyUp = (event: KeyboardEvent) => {
      if (event.code === "Space") {
        const target = event.target instanceof Element ? event.target : null;
        if (!(
          event.target instanceof HTMLInputElement ||
          event.target instanceof HTMLTextAreaElement ||
          event.target instanceof HTMLSelectElement ||
          target?.closest(
            "[contenteditable='true'],button,[role=menu],[role=listbox],[role=dialog]",
          )
        ))
          event.preventDefault();
        setIsSpacePressed(false);
      }
      if (event.key === "Control") setIsControlPressed(false);
    };

    const handleBlur = () => {
      setIsSpacePressed(false);
      setIsControlPressed(false);
    };

    window.addEventListener("keydown", handleKeyDown);
    window.addEventListener("keyup", handleKeyUp);
    window.addEventListener("blur", handleBlur);
    return () => {
      window.removeEventListener("keydown", handleKeyDown);
      window.removeEventListener("keyup", handleKeyUp);
      window.removeEventListener("blur", handleBlur);
    };
  }, []);

  const handleWheel = (event: React.WheelEvent<HTMLDivElement>) => {
    if (panState.current.isPanning) return;
    const target = event.target instanceof Element ? event.target : null;
    if (target?.closest("[data-canvas-no-zoom]")) return;

    const rect = containerRef.current?.getBoundingClientRect();
    if (!rect) return;
    onViewportChange(
      canvasWheelViewport(
        viewport,
        event,
        { x: event.clientX - rect.left, y: event.clientY - rect.top },
        rect.height,
      ),
    );
  };

  const handlePointerDown = (event: React.PointerEvent<HTMLDivElement>) => {
    if (panState.current.isPanning) return;
    const target = event.target instanceof Element ? event.target : null;
    if (target?.closest("[data-canvas-no-zoom]")) return;
    if (target?.closest("[data-connection-create-menu]")) return;
    const isBackgroundClick = !target?.closest(
      "[data-node-id],[data-connection-id]",
    );
    const temporaryTool = event.ctrlKey || isSpacePressed;
    const activeTool = temporaryTool
      ? tool === "select"
        ? "pan"
        : "select"
      : tool;
    const shouldPan =
      event.button === 1 || (event.button === 0 && activeTool === "pan");

    if (shouldPan) {
      event.preventDefault();
      event.currentTarget.setPointerCapture(event.pointerId);
      panState.current = {
        isPanning: true,
        pointerId: event.pointerId,
        initialK: viewport.k,
        startX: event.clientX,
        startY: event.clientY,
        initialX: viewport.x,
        initialY: viewport.y,
        hasMoved: false,
        startedOnBackground: isBackgroundClick,
      };
      setIsPanning(true);
      document.body.style.cursor = "grabbing";
      return;
    }

    if (event.button === 0 && isBackgroundClick) {
      event.preventDefault();
      event.currentTarget.setPointerCapture(event.pointerId);
      onCanvasMouseDown?.(event);
    }
  };

  const handleDoubleClick = (event: React.MouseEvent<HTMLDivElement>) => {
    const target = event.target instanceof Element ? event.target : null;
    if (
      target?.closest(
        "[data-canvas-no-zoom],[data-node-id],[data-connection-id]",
      )
    )
      return;
    onCanvasDoubleClick?.(event);
  };

  useEffect(() => {
    const clearFrame = () => {
      if (frameRef.current !== null) cancelAnimationFrame(frameRef.current);
      frameRef.current = null;
      nextViewportRef.current = null;
    };
    const position = (event: PointerEvent) => {
      const pan = panState.current;
      const dx = event.clientX - pan.startX;
      const dy = event.clientY - pan.startY;
      pan.hasMoved ||= Math.abs(dx) > 3 || Math.abs(dy) > 3;
      return { x: pan.initialX + dx, y: pan.initialY + dy, k: pan.initialK };
    };
    const handlePointerMove = (event: PointerEvent) => {
      if (
        !panState.current.isPanning ||
        event.pointerId !== panState.current.pointerId
      )
        return;
      nextViewportRef.current = position(event);
      if (frameRef.current !== null) return;
      frameRef.current = requestAnimationFrame(() => {
        frameRef.current = null;
        if (nextViewportRef.current) onViewportChange(nextViewportRef.current);
      });
    };
    const finish = (event?: PointerEvent) => {
      const pan = panState.current;
      if (!pan.isPanning || (event && event.pointerId !== pan.pointerId))
        return;
      const next =
        event?.type === "pointerup"
          ? position(event)
          : { x: pan.initialX, y: pan.initialY, k: pan.initialK };
      clearFrame();
      pan.isPanning = false;
      const container = containerRef.current;
      if (container?.hasPointerCapture?.(pan.pointerId))
        container.releasePointerCapture(pan.pointerId);
      setIsPanning(false);
      document.body.style.cursor = "";
      onViewportChange(next);
      if (
        event?.type === "pointerup" &&
        !pan.hasMoved &&
        pan.startedOnBackground
      )
        onCanvasDeselect?.();
    };
    const cancel = () => finish();
    const escape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !event.isComposing) finish();
    };
    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", finish);
    window.addEventListener("pointercancel", finish);
    window.addEventListener("lostpointercapture", finish);
    window.addEventListener("blur", cancel);
    window.addEventListener("keydown", escape);
    return () => {
      clearFrame();
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("pointerup", finish);
      window.removeEventListener("pointercancel", finish);
      window.removeEventListener("lostpointercapture", finish);
      window.removeEventListener("blur", cancel);
      window.removeEventListener("keydown", escape);
      document.body.style.cursor = "";
    };
  }, [containerRef, onCanvasDeselect, onViewportChange]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    // Prevent canvas scrolling from moving the page while preserving native scrolling inside overlays and dialogs.
    const preventWheelScroll = (event: WheelEvent) => {
      const target = event.target instanceof Element ? event.target : null;
      if (target?.closest("[data-canvas-no-zoom]")) return;
      event.preventDefault();
    };
    container.addEventListener("wheel", preventWheelScroll, { passive: false });
    return () => container.removeEventListener("wheel", preventWheelScroll);
  }, [containerRef]);

  const temporaryTool = isControlPressed || isSpacePressed;
  const activeTool = temporaryTool
    ? tool === "select"
      ? "pan"
      : "select"
    : tool;
  const cursor = isPanning
    ? "grabbing"
    : activeTool === "pan"
      ? "grab"
      : undefined;

  return (
    <div
      ref={containerRef}
      data-testid="canvas-surface"
      role="group"
      aria-label="画布"
      className="relative h-full w-full touch-none overflow-hidden select-none"
      style={{ background: theme.canvas.background, cursor }}
      onPointerDown={handlePointerDown}
      onDoubleClick={handleDoubleClick}
      onWheel={handleWheel}
      onContextMenu={onContextMenu}
      onDragOver={(event) => event.preventDefault()}
      onDrop={onDrop}
    >
      <CanvasGrid viewport={viewport} mode={backgroundMode} />
      <div
        className="absolute origin-top-left"
        style={{
          transform: `translate(${viewport.x}px, ${viewport.y}px) scale(${viewport.k})`,
        }}
      >
        {children}
      </div>
    </div>
  );
}

function CanvasGrid({
  viewport,
  mode,
}: {
  viewport: ViewportTransform;
  mode: CanvasBackgroundMode;
}) {
  if (mode === "blank") return null;

  const gridSize = 20 * viewport.k;
  const x = viewport.x % gridSize;
  const y = viewport.y % gridSize;
  const dotSize = 0.6;
  const backgroundImage =
    mode === "dots"
      ? `radial-gradient(circle, ${theme.canvas.dot} ${dotSize}px, transparent ${dotSize + 0.2}px)`
      : `linear-gradient(${theme.canvas.line} 1px, transparent 1px), linear-gradient(90deg, ${theme.canvas.line} 1px, transparent 1px)`;

  return (
    <div
      className="pointer-events-none absolute inset-0 opacity-15"
      style={{
        backgroundImage,
        backgroundSize: `${gridSize}px ${gridSize}px`,
        backgroundPosition: `${x}px ${y}px`,
      }}
    />
  );
}
