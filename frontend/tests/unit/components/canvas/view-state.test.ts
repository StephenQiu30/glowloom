import { expect, it } from "vitest";
import {
  canvasViewKey,
  readCanvasView,
  writeCanvasView,
  resizeCanvasViewport,
  type CanvasViewRecord,
} from "@/components/canvas/view-state";

const key = canvasViewKey("user", "space", "project", "canvas");
const record: CanvasViewRecord = {
  schemaVersion: 1,
  updatedAt: 12,
  view: {
    viewport: { x: -200, y: 40, k: 0.5 },
    size: { width: 1000, height: 700 },
    background: "dots",
    showConnections: true,
    miniMap: true,
    snapToGrid: true,
  },
};
const storage = () => {
  const values = new Map<string, string>();
  return {
    getItem: (name: string) => values.get(name) ?? null,
    setItem: (name: string, value: string) => {
      values.set(name, value);
    },
  };
};
it("isolates each identity and strips content from view persistence", () => {
  const store = storage();
  expect(
    writeCanvasView(store, key, {
      ...record,
      secret: "no",
      view: { ...record.view, nodes: ["private"] },
    } as CanvasViewRecord),
  ).toBe(true);
  expect(readCanvasView(store, key)).toEqual(record);
  for (const other of [
    canvasViewKey("other", "space", "project", "canvas"),
    canvasViewKey("user", "other", "project", "canvas"),
    canvasViewKey("user", "space", "other", "canvas"),
    canvasViewKey("user", "space", "project", "other"),
  ])
    expect(readCanvasView(store, other)).toBeUndefined();
  expect(canvasViewKey("a:b", "c", "p", "d")).not.toBe(
    canvasViewKey("a", "b:c", "p", "d"),
  );
});
it("ignores malformed, oversized and out-of-range records and storage failures", () => {
  const store = storage();
  for (const value of [
    "{",
    "x".repeat(4097),
    JSON.stringify({ ...record, view: null }),
    JSON.stringify({
      ...record,
      view: { ...record.view, viewport: { x: 0, y: 0, k: 0 } },
    }),
    JSON.stringify({ ...record, updatedAt: -1 }),
  ]) {
    store.setItem(key, value);
    expect(readCanvasView(store, key)).toBeUndefined();
  }
  const broken = {
    getItem: () => {
      throw new Error("blocked");
    },
    setItem: () => {
      throw new Error("quota");
    },
  };
  expect(readCanvasView(broken, key)).toBeUndefined();
  expect(writeCanvasView(broken, key, record)).toBe(false);
});
it("does not let an older tab overwrite a newer personal view", () => {
  const store = storage();
  expect(writeCanvasView(store, key, record)).toBe(true);
  expect(writeCanvasView(store, key, { ...record, updatedAt: 11 })).toBe(false);
  expect(readCanvasView(store, key)).toEqual(record);
});
it("restores the same world center in a different viewport size", () => {
  const next = resizeCanvasViewport(record.view.viewport, record.view.size, {
    width: 400,
    height: 900,
  });
  expect(next).toEqual({ x: -500, y: 140, k: 0.5 });
  expect((200 - next.x) / next.k).toBe(
    (500 - record.view.viewport.x) / record.view.viewport.k,
  );
});
