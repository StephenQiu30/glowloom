// Entity patch history adapted from BeefTV use-canvas-history.ts 193–249 (MIT).
import type {
  CanvasDocument,
  CanvasNodeData,
  CanvasConnection,
} from "../model";
type EntityPatch<T> = {
  changes: { id: string; before?: T; after?: T }[];
  beforeOrder?: string[];
  afterOrder?: string[];
};
export type CanvasHistoryPatch = {
  nodes?: EntityPatch<CanvasNodeData>;
  connections?: EntityPatch<CanvasConnection>;
  viewport?: {
    before: CanvasDocument["viewport"];
    after: CanvasDocument["viewport"];
  };
};
function createEntityPatch<T extends { id: string }>(
  before: T[],
  after: T[],
): EntityPatch<T> | undefined {
  const old = new Map(before.map((item) => [item.id, item])),
    next = new Map(after.map((item) => [item.id, item]));
  const changes: EntityPatch<T>["changes"] = [];
  new Set([...old.keys(), ...next.keys()]).forEach((id) => {
    if (old.get(id) !== next.get(id))
      changes.push({ id, before: old.get(id), after: next.get(id) });
  });
  const beforeOrder = before.map((item) => item.id),
    afterOrder = after.map((item) => item.id);
  const orderChanged =
    beforeOrder.length !== afterOrder.length ||
    beforeOrder.some((id, index) => id !== afterOrder[index]);
  return changes.length || orderChanged
    ? {
        changes,
        beforeOrder: orderChanged ? beforeOrder : undefined,
        afterOrder: orderChanged ? afterOrder : undefined,
      }
    : undefined;
}
function applyEntityPatch<T extends { id: string }>(
  current: T[],
  patch: EntityPatch<T>,
  side: "before" | "after",
) {
  const byId = new Map(current.map((item) => [item.id, item]));
  patch.changes.forEach((change) => {
    const value = change[side];
    if (value) byId.set(change.id, value);
    else byId.delete(change.id);
  });
  const order = side === "before" ? patch.beforeOrder : patch.afterOrder;
  return (order ?? current.map((item) => item.id))
    .map((id) => byId.get(id))
    .filter((item): item is T => Boolean(item));
}
export function createCanvasHistoryPatch(
  before: CanvasDocument,
  after: CanvasDocument,
): CanvasHistoryPatch {
  return {
    nodes: createEntityPatch(before.nodes, after.nodes),
    connections: createEntityPatch(before.connections, after.connections),
    viewport:
      JSON.stringify(before.viewport) === JSON.stringify(after.viewport)
        ? undefined
        : { before: before.viewport, after: after.viewport },
  };
}
export function applyCanvasHistoryPatch(
  snapshot: CanvasDocument,
  patch: CanvasHistoryPatch,
  side: "before" | "after",
): CanvasDocument {
  return {
    ...snapshot,
    nodes: patch.nodes
      ? applyEntityPatch(snapshot.nodes, patch.nodes, side)
      : snapshot.nodes,
    connections: patch.connections
      ? applyEntityPatch(snapshot.connections, patch.connections, side)
      : snapshot.connections,
    viewport: patch.viewport?.[side] ?? snapshot.viewport,
  };
}
