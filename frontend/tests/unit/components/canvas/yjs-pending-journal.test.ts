import { expect, it } from "vitest";
import {
  openCanvasPendingJournal,
  type CanvasPendingStore,
} from "@/components/canvas/lib/canvas-yjs-pending-journal";
import type { RecoveryRecord } from "@glowloom/canvas-collab/session";
import type { ServiceGraph } from "@glowloom/canvas-collab/service-document";

const identity = () => ({
  origin: "https://canvas.example",
  actorID: crypto.randomUUID(),
  scopeID: crypto.randomUUID(),
  projectID: crypto.randomUUID(),
  canvasID: crypto.randomUUID(),
});
function fixture() {
  const rows = new Map<string, RecoveryRecord<ServiceGraph>>();
  const locks = new Set<string>();
  const store: CanvasPendingStore = {
    list: async (namespace) =>
      [...rows]
        .filter(([key]) => key.startsWith(namespace + ":"))
        .map(([, record]) => structuredClone(record)),
    put: async (namespace, record) => {
      rows.set(
        namespace + ":" + record.scope.sessionId + ":" + record.scope.epoch,
        structuredClone(record),
      );
    },
    remove: async (namespace, scope) => {
      rows.delete(namespace + ":" + scope.sessionId + ":" + scope.epoch);
    },
  };
  const claim = async (scope: { sessionId: string }) => {
    if (locks.has(scope.sessionId)) return null;
    locks.add(scope.sessionId);
    return async () => {
      locks.delete(scope.sessionId);
    };
  };
  return { rows, store, claim };
}
function record(
  id: ReturnType<typeof identity>,
  sessionId: string,
): RecoveryRecord<ServiceGraph> {
  const epoch = crypto.randomUUID();
  return {
    version: 2,
    generation: 1,
    scope: {
      origin: id.origin,
      actorKey: `USER:${id.actorID}`,
      canvasId: id.canvasID,
      sessionId,
      epoch,
    },
    confirmed: {
      epoch,
      commitSeq: "1",
      contentRevision: "2",
      graphHash: "hash",
      update: "",
      bindingRevision: "2",
      modelVersion: 1,
      lifecycle: "ACTIVE",
      edit: true,
    },
    inFlight: {
      epoch,
      sessionId,
      updateId: crypto.randomUUID(),
      update: "AQID",
      payloadHash: "original-hash",
    },
    acceptedReceipt: null,
    recoverySources: [],
    recoverySelection: null,
    intents: [],
    inFlightIntents: [],
    automaticReplayAttempts: 0,
    pending: [],
    snapshot: {
      title: "",
      graph: {
        schemaVersion: 1,
        serviceVersion: 1,
        documentId: id.canvasID,
        projectId: id.projectID,
        nodes: [],
        connections: [],
      },
    },
    sealed: null,
    forms: {},
    textDrafts: {},
    control: null,
    updatedAt: Date.now(),
  };
}
it("retains the exact pending identity across handles and isolates account, scope, project and canvas", async () => {
  const f = fixture(),
    id = identity();
  const first = await openCanvasPendingJournal(id, f);
  const original = record(id, first.sessionId);
  await first.put(original);
  await first.close();
  const reopened = await openCanvasPendingJournal(id, f);
  expect(reopened.record).toEqual(original);
  expect(reopened.sessionId).toBe(first.sessionId);
  await reopened.close();
  for (const key of ["actorID", "scopeID", "projectID", "canvasID"] as const) {
    const other = await openCanvasPendingJournal(
      { ...id, [key]: crypto.randomUUID() },
      f,
    );
    expect(other.record).toBeUndefined();
    await other.close();
  }
});
it("does not steal a live session lease or silently lose an older epoch record", async () => {
  const f = fixture(),
    id = identity(),
    first = await openCanvasPendingJournal(id, f);
  const original = record(id, first.sessionId);
  await first.put(original);
  const second = await openCanvasPendingJournal(id, f);
  expect(second.sessionId).not.toBe(first.sessionId);
  expect(second.record).toBeUndefined();
  await second.close();
  await first.close();
  const reopened = await openCanvasPendingJournal(id, f);
  expect(reopened.record?.scope.epoch).toBe(original.scope.epoch);
  await reopened.close();
});
it("rejects credentials, cross-scope writes and storage failure without losing the prior record", async () => {
  const f = fixture(),
    id = identity(),
    handle = await openCanvasPendingJournal(id, f),
    original = record(id, handle.sessionId);
  await handle.put(original);
  await expect(
    handle.put({ ...original, forms: { accessToken: "must-not-persist" } }),
  ).rejects.toThrow();
  await expect(
    handle.put({
      ...original,
      scope: { ...original.scope, actorKey: `USER:${crypto.randomUUID()}` },
    }),
  ).rejects.toThrow();
  f.store.put = async () => {
    throw new Error("QuotaExceededError");
  };
  await expect(handle.put({ ...original, generation: 2 })).rejects.toThrow(
    "QuotaExceeded",
  );
  await handle.close();
  expect([...f.rows.values()]).toEqual([original]);
});

it("preserves every abandoned session and exposes remaining recovery work without stealing a lease", async () => {
  const f = fixture(),
    id = identity();
  const first = await openCanvasPendingJournal(id, f);
  const original = record(id, first.sessionId);
  await first.put(original);
  const second = await openCanvasPendingJournal(id, f);
  const other = record(id, second.sessionId);
  await second.put(other);
  expect(second.remaining).toBe(1);
  await first.close();
  await second.close();
  const reopened = await openCanvasPendingJournal(id, f);
  expect(reopened.record).toEqual(original);
  expect(reopened.remaining).toBe(1);
  expect([...f.rows.values()]).toEqual([original, other]);
  await reopened.close();
});

it("preserves optional undefined fields supported by structured clone but rejects executable or binary record shapes", async () => {
  const f = fixture(),
    id = identity(),
    handle = await openCanvasPendingJournal(id, f);
  const original = record(id, handle.sessionId);
  original.forms = { input: { mode: undefined, text: "draft" } };
  await handle.put(original);
  expect([...f.rows.values()][0]).toEqual(original);
  for (const invalid of [new Map(), new Uint8Array([1]), () => {}]) {
    await expect(
      handle.put({ ...original, forms: { input: invalid } }),
    ).rejects.toThrow("内容无效");
  }
  await handle.close();
});
