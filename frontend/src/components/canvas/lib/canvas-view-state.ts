import type { ViewportTransform } from "../types/canvas";
import { CANVAS_MAX_SCALE, CANVAS_MIN_SCALE } from "./canvas-viewport";
import {
  CANVAS_SIDE_PANEL_MIN_WIDTH,
  CANVAS_SIDE_PANEL_MAX_WIDTH,
} from "../store/use-canvas-side-panel-store";

export type CanvasViewState = {
  viewport: ViewportTransform;
  size: { width: number; height: number };
  backgroundMode: "dots" | "lines" | "blank";
  showImageInfo: boolean;
  showConnections: boolean;
  miniMap: boolean;
  snapToGrid: boolean;
  panel: { open: boolean; width: number; tab: "canvas" | "assets" };
};
export type CanvasViewRecord = {
  schemaVersion: 1;
  updatedAt: number;
  view: CanvasViewState;
};
type ViewStorage = Pick<Storage, "getItem" | "setItem">;

export function canvasViewKey(
  identity: string,
  scope: string,
  canvasId: string,
) {
  return `glowloom:canvas-view:v1:${encodeURIComponent(scope)}:${encodeURIComponent(identity)}:${encodeURIComponent(canvasId)}`;
}

function validRecord(value: unknown): value is CanvasViewRecord {
  if (!value || typeof value !== "object") return false;
  const record = value as CanvasViewRecord;
  const view = record.view;
  return (
    record.schemaVersion === 1 &&
    Number.isFinite(record.updatedAt) &&
    !!view &&
    !!view.viewport &&
    [view.viewport.x, view.viewport.y].every(
      (n) => Number.isFinite(n) && Math.abs(n) <= 10_000_000,
    ) &&
    Number.isFinite(view.viewport.k) &&
    view.viewport.k >= CANVAS_MIN_SCALE &&
    view.viewport.k <= CANVAS_MAX_SCALE &&
    !!view.size &&
    [view.size.width, view.size.height].every(
      (n) => Number.isFinite(n) && n > 0 && n <= 100_000,
    ) &&
    ["dots", "lines", "blank"].includes(view.backgroundMode) &&
    [
      view.showImageInfo,
      view.showConnections,
      view.miniMap,
      view.snapToGrid,
    ].every((v) => typeof v === "boolean") &&
    !!view.panel &&
    typeof view.panel.open === "boolean" &&
    Number.isFinite(view.panel.width) &&
    view.panel.width >= CANVAS_SIDE_PANEL_MIN_WIDTH &&
    view.panel.width <= CANVAS_SIDE_PANEL_MAX_WIDTH &&
    ["canvas", "assets"].includes(view.panel.tab)
  );
}

function whitelist(record: CanvasViewRecord): CanvasViewRecord {
  const {
    viewport,
    size,
    backgroundMode,
    showImageInfo,
    showConnections,
    miniMap,
    snapToGrid,
    panel,
  } = record.view;
  return {
    schemaVersion: 1,
    updatedAt: record.updatedAt,
    view: {
      viewport: { x: viewport.x, y: viewport.y, k: viewport.k },
      size: { width: size.width, height: size.height },
      backgroundMode,
      showImageInfo,
      showConnections,
      miniMap,
      snapToGrid,
      panel: { open: panel.open, width: panel.width, tab: panel.tab },
    },
  };
}

export function readCanvasView(
  storage: ViewStorage,
  key: string,
): CanvasViewRecord | undefined {
  try {
    const raw = storage.getItem(key);
    if (!raw || raw.length > 4096) return undefined;
    const parsed: unknown = JSON.parse(raw);
    return validRecord(parsed) ? whitelist(parsed) : undefined;
  } catch {
    return undefined;
  }
}

export function writeCanvasView(
  storage: ViewStorage,
  key: string,
  record: CanvasViewRecord,
) {
  try {
    if (
      !validRecord(record) ||
      (readCanvasView(storage, key)?.updatedAt ?? -1) > record.updatedAt
    )
      return false;
    storage.setItem(key, JSON.stringify(whitelist(record)));
    return true;
  } catch {
    return false;
  }
}

export function resizeCanvasViewport(
  view: ViewportTransform,
  before: CanvasViewState["size"],
  after: CanvasViewState["size"],
) {
  return {
    x: view.x + (after.width - before.width) / 2,
    y: view.y + (after.height - before.height) / 2,
    k: view.k,
  };
}
