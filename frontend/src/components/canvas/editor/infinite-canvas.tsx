import React, { useEffect, useLayoutEffect, useRef, useState } from "react";

import {
  canvasThemes,
  type CanvasBackgroundMode,
} from "@/components/canvas/runtime/lib/theme";
import { CANVAS_GRID_SIZE } from "@/components/canvas/lib/canvas-viewport";
import { canvasWheelViewport } from "@/components/canvas/lib/canvas-wheel";
import {
  hasCanvasOverlay,
  isCanvasKeyboardTarget,
  isCanvasShortcut,
} from "../lib/canvas-keyboard";
import { useThemeStore } from "@/components/canvas/runtime/store/use-theme-store";
import type { ViewportTransform } from "@/components/canvas/types/canvas";

type InfiniteCanvasProps = {
  containerRef: React.RefObject<HTMLDivElement | null>;
  viewport: ViewportTransform;
  tool: "select" | "pan";
  backgroundMode?: CanvasBackgroundMode;
  onViewportChange: (viewport: ViewportTransform) => void;
  onViewportPreview?: (viewport: ViewportTransform) => void;
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
  backgroundMode = "lines",
  onViewportChange,
  onViewportPreview,
  onCanvasMouseDown,
  onCanvasDeselect,
  onCanvasDoubleClick,
  onContextMenu,
  onDrop,
  children,
}: InfiniteCanvasProps) {
  const theme = canvasThemes[useThemeStore((state) => state.theme)];
  const panState = useRef({
    isPanning: false,
    startX: 0,
    startY: 0,
    initialX: 0,
    initialY: 0,
    hasMoved: false,
    startedOnBackground: false,
  });
  const viewportRef = useRef(viewport);
  const changeRef = useRef(onViewportChange);
  const previewRef = useRef(onViewportPreview);
  const deselectRef = useRef(onCanvasDeselect);
  const frameRef = useRef<number | null>(null);
  const nextViewportRef = useRef<ViewportTransform | null>(null);
  const [isSpacePressed, setIsSpacePressed] = useState(false);
  const [isPanning, setIsPanning] = useState(false);

  useLayoutEffect(() => {
    viewportRef.current = viewport;
    changeRef.current = onViewportChange;
    previewRef.current = onViewportPreview;
    deselectRef.current = onCanvasDeselect;
  }, [viewport, onViewportChange, onViewportPreview, onCanvasDeselect]);

  useEffect(
    () => () => {
      if (frameRef.current) cancelAnimationFrame(frameRef.current);
    },
    [],
  );

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (
        !isCanvasShortcut(event) ||
        event.code !== "Space" ||
        event.ctrlKey ||
        event.metaKey ||
        event.altKey
      )
        return;
      event.preventDefault();
      setIsSpacePressed(true);
    };

    const handleKeyUp = (event: KeyboardEvent) => {
      if (event.code === "Space") {
        setIsSpacePressed(false);
      }
    };

    const handleBlur = () => {
      if (frameRef.current) cancelAnimationFrame(frameRef.current);
      frameRef.current = null;
      nextViewportRef.current = null;
      setIsSpacePressed(false);
      panState.current.isPanning = false;
      setIsPanning(false);
      document.body.style.cursor = "";
    };
    const handleFocus = (event: FocusEvent) => {
      if (!isCanvasKeyboardTarget(event.target)) handleBlur();
    };

    window.addEventListener("keydown", handleKeyDown);
    window.addEventListener("keyup", handleKeyUp);
    window.addEventListener("blur", handleBlur);
    window.addEventListener("focusin", handleFocus);
    window.addEventListener("pointercancel", handleBlur);
    return () => {
      window.removeEventListener("keydown", handleKeyDown);
      window.removeEventListener("keyup", handleKeyUp);
      window.removeEventListener("blur", handleBlur);
      window.removeEventListener("focusin", handleFocus);
      window.removeEventListener("pointercancel", handleBlur);
    };
  }, []);

  useEffect(() => {
    if (!isSpacePressed) return;
    const observer = new MutationObserver(() => {
      if (hasCanvasOverlay(document)) {
        if (frameRef.current) cancelAnimationFrame(frameRef.current);
        frameRef.current = null;
        nextViewportRef.current = null;
        setIsSpacePressed(false);
        panState.current.isPanning = false;
        setIsPanning(false);
        document.body.style.cursor = "";
      }
    });
    observer.observe(document.body, { childList: true, subtree: true });
    return () => observer.disconnect();
  }, [isSpacePressed]);

  const handlePointerDown = (event: React.PointerEvent<HTMLDivElement>) => {
    const target = event.target instanceof Element ? event.target : null;
    // React portals retain ancestry; their controls and overlays are outside the canvas DOM.
    if (!target || !event.currentTarget.contains(target)) return;
    if (target?.closest("[data-canvas-no-zoom],[data-canvas-no-pan]")) return;
    if (target?.closest("[data-connection-create-menu]")) return;
    const isBackgroundClick = !target?.closest(
      "[data-node-id],[data-connection-id]",
    );
    if (
      target.closest(
        "input,textarea,select,button,a,[contenteditable='true']",
      ) ||
      hasCanvasOverlay(document)
    )
      return;
    event.currentTarget.focus({ preventScroll: true });
    const activeTool = isSpacePressed ? "pan" : tool;
    const shouldPan =
      event.button === 1 || (event.button === 0 && activeTool === "pan");

    if (shouldPan) {
      event.preventDefault();
      event.stopPropagation();
      event.currentTarget.setPointerCapture(event.pointerId);
      panState.current = {
        isPanning: true,
        startX: event.clientX,
        startY: event.clientY,
        initialX: viewportRef.current.x,
        initialY: viewportRef.current.y,
        hasMoved: false,
        startedOnBackground: isBackgroundClick,
      };
      setIsPanning(true);
      document.body.style.cursor = "grabbing";
      return;
    }

    if (event.eventPhase === 1) return;
    if (event.button === 0 && isBackgroundClick) {
      event.preventDefault();
      event.currentTarget.setPointerCapture(event.pointerId);
      onCanvasMouseDown?.(event);
    }
  };

  const handleDoubleClick = (event: React.MouseEvent<HTMLDivElement>) => {
    if (tool === "pan" || isSpacePressed || hasCanvasOverlay(document)) return;
    const target = event.target instanceof Element ? event.target : null;
    if (!target || !event.currentTarget.contains(target)) return;
    if (
      target?.closest(
        "[data-canvas-no-zoom],[data-node-id],[data-connection-id]",
      )
    )
      return;
    onCanvasDoubleClick?.(event);
  };

  useEffect(() => {
    const handlePointerMove = (event: PointerEvent) => {
      if (!panState.current.isPanning) return;

      const dx = event.clientX - panState.current.startX;
      const dy = event.clientY - panState.current.startY;
      if (Math.abs(dx) > 3 || Math.abs(dy) > 3) {
        panState.current.hasMoved = true;
      }

      nextViewportRef.current = {
        x: panState.current.initialX + dx,
        y: panState.current.initialY + dy,
        k: viewportRef.current.k,
      };
      previewRef.current?.(nextViewportRef.current);
      if (frameRef.current) return;
      frameRef.current = requestAnimationFrame(() => {
        frameRef.current = null;
        if (nextViewportRef.current) {
          viewportRef.current = nextViewportRef.current;
          changeRef.current(nextViewportRef.current);
          nextViewportRef.current = null;
        }
      });
    };

    const handlePointerUp = (event: PointerEvent) => {
      if (!panState.current.isPanning) return;
      if (frameRef.current) cancelAnimationFrame(frameRef.current);
      frameRef.current = null;
      nextViewportRef.current = null;
      if (event.type !== "pointercancel") {
        const next = {
          x:
            panState.current.initialX + event.clientX - panState.current.startX,
          y:
            panState.current.initialY + event.clientY - panState.current.startY,
          k: viewportRef.current.k,
        };
        viewportRef.current = next;
        changeRef.current(next);
      }
      if (!panState.current.hasMoved && panState.current.startedOnBackground) {
        deselectRef.current?.();
      }
      panState.current.isPanning = false;
      setIsPanning(false);
      document.body.style.cursor = "";
    };

    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", handlePointerUp);
    window.addEventListener("pointercancel", handlePointerUp);
    return () => {
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("pointerup", handlePointerUp);
      window.removeEventListener("pointercancel", handlePointerUp);
      document.body.style.cursor = "";
    };
  }, []);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const handleWheel = (event: WheelEvent) => {
      const target = event.target instanceof Element ? event.target : null;
      if (
        hasCanvasOverlay(document) ||
        target?.closest(
          '[data-canvas-no-zoom],[data-canvas-wheel-scroll],[data-slot="dialog-content"],[data-slot="popover-content"],[data-slot="dropdown-menu-content"],[data-slot="select-content"]',
        )
      )
        return;
      event.preventDefault();
      if (panState.current.isPanning || (!event.deltaX && !event.deltaY))
        return;
      const rect = container.getBoundingClientRect();
      const next = canvasWheelViewport(
        viewportRef.current,
        event,
        { x: event.clientX - rect.left, y: event.clientY - rect.top },
        rect.height,
      );
      // Accumulate events immediately, but commit the viewport at most once per frame.
      viewportRef.current = next;
      nextViewportRef.current = next;
      previewRef.current?.(next);
      if (frameRef.current !== null) return;
      frameRef.current = requestAnimationFrame(() => {
        frameRef.current = null;
        if (nextViewportRef.current) {
          changeRef.current(nextViewportRef.current);
          nextViewportRef.current = null;
        }
      });
    };
    container.addEventListener("wheel", handleWheel, { passive: false });
    return () => container.removeEventListener("wheel", handleWheel);
  }, [containerRef]);

  const activeTool = isSpacePressed ? "pan" : tool;
  const cursor = isPanning
    ? "grabbing"
    : activeTool === "pan"
      ? "grab"
      : undefined;

  return (
    <div
      ref={containerRef}
      className="relative h-full w-full touch-none overflow-hidden outline-none select-none focus-visible:ring-1 focus-visible:ring-ring/40 focus-visible:ring-inset"
      data-canvas-viewport
      tabIndex={0}
      aria-label="创作画布"
      style={{ background: theme.canvas.background, cursor }}
      onPointerDownCapture={(event) => {
        const target = event.target instanceof Element ? event.target : null;
        if (
          !target?.closest(
            "input,textarea,select,button,[contenteditable='true'],[data-canvas-no-zoom]",
          )
        )
          handlePointerDown(event);
      }}
      onPointerDown={handlePointerDown}
      onDoubleClick={handleDoubleClick}
      onContextMenu={onContextMenu}
      onDragOver={(event) => event.preventDefault()}
      onDrop={onDrop}
    >
      <CanvasGrid viewport={viewport} mode={backgroundMode} />
      <div
        className="absolute origin-top-left"
        data-canvas-world
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
  const theme = canvasThemes[useThemeStore((state) => state.theme)];
  if (mode === "blank") return null;

  const baseSize = (mode === "dots" ? 20 : CANVAS_GRID_SIZE) * viewport.k;
  // Skip minor grid marks when zoomed out so the canvas stays legible instead of forming a moiré pattern.
  const gridSize =
    baseSize * Math.pow(2, Math.max(0, Math.ceil(Math.log2(12 / baseSize))));
  const x = viewport.x % gridSize;
  const y = viewport.y % gridSize;
  const dotSize = 0.65;
  const backgroundImage =
    mode === "dots"
      ? `radial-gradient(circle at 0.5px 0.5px, ${theme.canvas.dot} ${dotSize}px, transparent ${dotSize + 0.2}px)`
      : `linear-gradient(${theme.canvas.line} 1px, transparent 1px), linear-gradient(90deg, ${theme.canvas.line} 1px, transparent 1px)`;

  return (
    <div
      className="pointer-events-none absolute inset-0"
      style={{
        backgroundImage,
        opacity: mode === "dots" ? 0.6 : 0.4,
        backgroundSize: `${gridSize}px ${gridSize}px`,
        backgroundPosition: `${x}px ${y}px`,
      }}
    />
  );
}
