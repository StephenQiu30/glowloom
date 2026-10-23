// Attribution and source revision are recorded in /licenses/canvas-sources.md.
// See /licenses/canvas-sources.md.
import type { ViewportTransform } from "./types";
export type CanvasViewState = {
  viewport: ViewportTransform;
  size: { width: number; height: number };
  background: "dots" | "lines" | "blank";
  showConnections: boolean;
  miniMap: boolean;
  snapToGrid: boolean;
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
  projectId: string,
  canvasId: string,
) {
  return `glowloom:canvas-view:v1:${[identity, scope, projectId, canvasId].map(encodeURIComponent).join(":")}`;
}

function validRecord(value: unknown): value is CanvasViewRecord {
  if (!value || typeof value !== "object") return false;
  const record = value as CanvasViewRecord;
  const view = record.view;
  return (
    record.schemaVersion === 1 &&
    Number.isSafeInteger(record.updatedAt) &&
    record.updatedAt >= 0 &&
    !!view &&
    !!view.viewport &&
    [view.viewport.x, view.viewport.y].every(
      (n) => Number.isFinite(n) && Math.abs(n) <= 10_000_000,
    ) &&
    Number.isFinite(view.viewport.k) &&
    view.viewport.k >= 0.05 &&
    view.viewport.k <= 5 &&
    !!view.size &&
    [view.size.width, view.size.height].every(
      (n) => Number.isFinite(n) && n > 0 && n <= 100_000,
    ) &&
    ["dots", "lines", "blank"].includes(view.background) &&
    [view.showConnections, view.miniMap, view.snapToGrid].every(
      (v) => typeof v === "boolean",
    )
  );
}

function whitelist(record: CanvasViewRecord): CanvasViewRecord {
  const { viewport, size, background, showConnections, miniMap, snapToGrid } =
    record.view;
  return {
    schemaVersion: 1,
    updatedAt: record.updatedAt,
    view: {
      viewport: { x: viewport.x, y: viewport.y, k: viewport.k },
      size: { width: size.width, height: size.height },
      background,
      showConnections,
      miniMap,
      snapToGrid,
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
