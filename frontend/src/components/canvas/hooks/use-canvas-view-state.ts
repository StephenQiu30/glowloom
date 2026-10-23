"use client";
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type SetStateAction,
} from "react";
import type { ViewportTransform } from "../types/canvas";
import {
  readCanvasView,
  writeCanvasView,
  resizeCanvasViewport,
  type CanvasViewState,
} from "../lib/canvas-view-state";
import {
  CANVAS_SIDE_PANEL_DEFAULT_WIDTH,
  useCanvasSidePanelStore,
} from "../store/use-canvas-side-panel-store";

export function useCanvasViewState({
  key,
  initial,
  size,
}: {
  key?: string;
  initial: Pick<
    CanvasViewState,
    "viewport" | "backgroundMode" | "showImageInfo"
  >;
  size: CanvasViewState["size"];
}) {
  const [loaded] = useState(() => {
    try {
      return key ? readCanvasView(window.localStorage, key) : undefined;
    } catch {
      return undefined;
    }
  });
  const [view, setView] = useState<CanvasViewState>(
    () =>
      loaded?.view ?? {
        viewport: initial.viewport,
        backgroundMode: initial.backgroundMode,
        showImageInfo: initial.showImageInfo,
        size: { width: 0, height: 0 },
        showConnections: true,
        miniMap: false,
        snapToGrid: false,
        panel: {
          open: false,
          width: CANVAS_SIDE_PANEL_DEFAULT_WIDTH,
          tab: "canvas",
        },
      },
  );
  const latest = useRef(view);
  const dirtyAt = useRef<number | null>(null);
  const updatedAt = useRef(loaded?.updatedAt ?? 0);
  const previousSize = useRef(view.size);
  const markDirty = useCallback(() => {
    dirtyAt.current ??= Date.now();
    updatedAt.current = Math.max(Date.now(), updatedAt.current + 1);
  }, []);
  const updateView = useCallback(
    (patch: Partial<CanvasViewState>) => {
      latest.current = { ...latest.current, ...patch };
      markDirty();
      setView(latest.current);
    },
    [markDirty],
  );
  const setViewport = useCallback(
    (next: SetStateAction<ViewportTransform>) => {
      updateView({
        viewport:
          typeof next === "function" ? next(latest.current.viewport) : next,
      });
    },
    [updateView],
  );
  // Capture the last wheel/pan event even if the tab closes before its RAF renders.
  const previewViewport = useCallback(
    (viewport: ViewportTransform) => {
      latest.current = { ...latest.current, viewport };
      markDirty();
    },
    [markDirty],
  );
  const flush = useCallback(() => {
    if (
      !key ||
      dirtyAt.current === null ||
      !latest.current.size.width ||
      !latest.current.size.height
    )
      return;
    try {
      if (
        writeCanvasView(window.localStorage, key, {
          schemaVersion: 1,
          updatedAt: updatedAt.current,
          view: latest.current,
        })
      )
        dirtyAt.current = null;
    } catch {
      /* View persistence must never block editing or content saves. */
    }
  }, [key]);
  useLayoutEffect(() => {
    if (size.width <= 0 || size.height <= 0) return;
    const before = previousSize.current;
    if (before.width === size.width && before.height === size.height) return;
    previousSize.current = size;
    const viewport =
      before.width && before.height
        ? resizeCanvasViewport(latest.current.viewport, before, size)
        : latest.current.viewport;
    if (before.width && before.height) updateView({ viewport, size });
    else {
      latest.current = { ...latest.current, size };
      setView(latest.current);
    }
  }, [size.width, size.height, size, updateView]);
  useLayoutEffect(() => {
    const panel = latest.current.panel;
    useCanvasSidePanelStore.setState({
      width: panel.width,
      tab: panel.tab,
      panelOpen: panel.open,
      panelMounted: panel.open,
      panelClosing: false,
    });
    const unsubscribe = useCanvasSidePanelStore.subscribe((next, previous) => {
      if (
        next.width !== previous.width ||
        next.panelOpen !== previous.panelOpen ||
        next.tab !== previous.tab
      )
        updateView({
          panel: { width: next.width, open: next.panelOpen, tab: next.tab },
        });
    });
    return () => {
      unsubscribe();
      flush();
    };
  }, [updateView, flush]);
  useEffect(() => {
    if (dirtyAt.current === null) return;
    const timer = setTimeout(
      flush,
      Math.max(0, Math.min(200, 1000 - (Date.now() - dirtyAt.current))),
    );
    return () => clearTimeout(timer);
  }, [view, flush]);
  useEffect(() => {
    const hidden = () => {
      if (document.visibilityState === "hidden") flush();
    };
    window.addEventListener("pagehide", flush);
    document.addEventListener("visibilitychange", hidden);
    return () => {
      window.removeEventListener("pagehide", flush);
      document.removeEventListener("visibilitychange", hidden);
    };
  }, [flush]);
  return { view, updateView, setViewport, previewViewport };
}
