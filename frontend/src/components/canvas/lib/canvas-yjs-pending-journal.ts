import type {
  CollaborationScope,
  RecoveryRecord,
} from "@glowloom/canvas-collab/session";
import type { ServiceGraph } from "@glowloom/canvas-collab/service-document";
import { claimCanvasJournal } from "./canvas-journal-lease";

type Record = RecoveryRecord<ServiceGraph>;
export type CanvasPendingIdentity = {
  origin: string;
  actorID: string;
  scopeID: string;
  projectID: string;
  canvasID: string;
};
export type CanvasPendingStore = {
  list(namespace: string): Promise<Record[]>;
  put(namespace: string, record: Record): Promise<void>;
  remove(namespace: string, scope: CollaborationScope): Promise<void>;
};
export type CanvasPendingJournal = {
  sessionId: string;
  record?: Record;
  remaining: number;
  put(record: Record): Promise<void>;
  close(): Promise<void>;
};
export type CanvasPendingJournalFactory = (
  identity: CanvasPendingIdentity,
) => Promise<CanvasPendingJournal>;
const uuid =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const storageKey = (namespace: string, scope: CollaborationScope) =>
  JSON.stringify([namespace, scope.sessionId, scope.epoch]);
export function hasCanvasPendingIntent(record: Record) {
  return !!(
    record.inFlight ||
    record.sealed ||
    record.control ||
    record.pending.length ||
    record.intents.length ||
    record.inFlightIntents.length ||
    record.recoverySources.length ||
    Object.keys(record.forms).length ||
    Object.keys(record.textDrafts).length
  );
}
function validate(
  record: Record,
  identity: CanvasPendingIdentity,
  sessionId?: string,
) {
  if (
    record.version !== 2 ||
    record.scope.origin !== identity.origin ||
    record.scope.actorKey !== `USER:${identity.actorID}` ||
    record.scope.canvasId !== identity.canvasID ||
    (sessionId && record.scope.sessionId !== sessionId) ||
    !uuid.test(record.scope.sessionId) ||
    !uuid.test(record.scope.epoch) ||
    record.confirmed.epoch !== record.scope.epoch ||
    !Number.isSafeInteger(record.generation) ||
    record.generation < 0 ||
    !Array.isArray(record.pending) ||
    !Array.isArray(record.recoverySources) ||
    !Array.isArray(record.intents) ||
    !Array.isArray(record.inFlightIntents)
  )
    throw new Error("本机草稿身份无效");
  if (
    record.inFlight &&
    (record.inFlight.sessionId !== record.scope.sessionId ||
      record.inFlight.epoch !== record.scope.epoch ||
      !uuid.test(record.inFlight.updateId))
  )
    throw new Error("本机草稿提交身份无效");
  const visit = (value: unknown): void => {
    if (
      value === null ||
      value === undefined ||
      ["string", "boolean"].includes(typeof value) ||
      (typeof value === "number" && Number.isFinite(value))
    )
      return;
    if (Array.isArray(value)) {
      value.forEach(visit);
      return;
    }
    if (
      !value ||
      typeof value !== "object" ||
      Object.getPrototypeOf(value) !== Object.prototype
    )
      throw new Error("本机草稿内容无效");
    for (const [key, item] of Object.entries(value)) {
      if (
        /^(cookie|csrf(token)?|authorization|accessToken|refreshToken|providerKey|apiKey|grant|credentials|secret|previewUrl)$/i.test(
          key,
        )
      )
        throw new Error("本机草稿禁止保存凭证");
      visit(item);
    }
  };
  visit(record);
  if (
    new TextEncoder().encode(JSON.stringify(record)).byteLength >
    16 * 1024 * 1024
  )
    throw new Error("本机草稿超过存储上限");
}

type Stored = { id: string; namespace: string; record: Record };
async function database() {
  if (!globalThis.indexedDB) throw new Error("本机草稿存储不可用");
  return new Promise<IDBDatabase>((resolve, reject) => {
    const request = indexedDB.open("glowloom-service-canvas-recovery", 1);
    request.onupgradeneeded = () =>
      request.result
        .createObjectStore("pending", { keyPath: "id" })
        .createIndex("namespace", "namespace");
    request.onerror = () =>
      reject(request.error ?? new Error("本机草稿存储不可用"));
    request.onblocked = () => reject(new Error("本机草稿存储被其他页面占用"));
    request.onsuccess = () => resolve(request.result);
  });
}
async function transact<T>(
  mode: IDBTransactionMode,
  action: (store: IDBObjectStore) => IDBRequest<T>,
): Promise<T> {
  const db = await database();
  try {
    return await new Promise<T>((resolve, reject) => {
      const tx = db.transaction("pending", mode);
      let result: T;
      const request = action(tx.objectStore("pending"));
      request.onsuccess = () => {
        result = request.result;
      };
      tx.oncomplete = () => resolve(result);
      tx.onerror = () =>
        reject(tx.error ?? request.error ?? new Error("本机草稿写入失败"));
      tx.onabort = () => reject(tx.error ?? new Error("本机草稿写入被中止"));
    });
  } finally {
    db.close();
  }
}
const indexedStore: CanvasPendingStore = {
  async list(namespace) {
    return (
      await transact<Stored[]>("readonly", (store) =>
        store.index("namespace").getAll(namespace),
      )
    ).map((row) => row.record);
  },
  async put(namespace, record) {
    await transact("readwrite", (store) =>
      store.put({ id: storageKey(namespace, record.scope), namespace, record }),
    );
  },
  async remove(namespace, scope) {
    await transact("readwrite", (store) =>
      store.delete(storageKey(namespace, scope)),
    );
  },
};

/** Own one abandoned session lease before restoring its original frozen submission. */
export async function openCanvasPendingJournal(
  identity: CanvasPendingIdentity,
  options: {
    store?: CanvasPendingStore;
    claim?: (
      scope: Omit<CollaborationScope, "epoch">,
    ) => Promise<(() => Promise<void>) | null>;
  } = {},
): Promise<CanvasPendingJournal> {
  if (
    ![
      identity.actorID,
      identity.scopeID,
      identity.projectID,
      identity.canvasID,
    ].every((value) => uuid.test(value))
  )
    throw new Error("本机草稿作用域无效");
  const namespace = JSON.stringify([
    identity.origin,
    identity.actorID,
    identity.scopeID,
    identity.projectID,
    identity.canvasID,
  ]);
  const store = options.store ?? indexedStore,
    claim = options.claim ?? claimCanvasJournal;
  const rows = await store.list(namespace);
  rows.forEach((record) => validate(record, identity));
  const pending = rows
    .filter(hasCanvasPendingIntent)
    .sort((a, b) => a.updatedAt - b.updatedAt);
  let record: Record | undefined;
  let release: (() => Promise<void>) | null = null;
  for (const previous of pending) {
    release = await claim(previous.scope);
    if (!release) continue;
    try {
      record = (await store.list(namespace)).find(
        (value) =>
          value.scope.sessionId === previous.scope.sessionId &&
          value.scope.epoch === previous.scope.epoch,
      );
      if (record) {
        validate(record, identity);
        break;
      }
    } catch (error) {
      await release();
      throw error;
    }
    await release();
    release = null;
  }
  const sessionId = record?.scope.sessionId ?? crypto.randomUUID();
  release ??= await claim({
    origin: identity.origin,
    actorKey: `USER:${identity.actorID}`,
    canvasId: identity.canvasID,
    sessionId,
  });
  if (!release) throw new Error("本机草稿正由其他页面处理");
  let closed = false,
    queue = Promise.resolve();
  let previousScope = record?.scope;
  const owned = release;
  return {
    sessionId,
    record: record && structuredClone(record),
    remaining: pending.length - (record ? 1 : 0),
    async put(value) {
      if (closed) return Promise.reject(new Error("本机草稿会话已关闭"));
      validate(value, identity, sessionId);
      const copy = structuredClone(value);
      const write = queue
        .catch(() => {})
        .then(async () => {
          if (hasCanvasPendingIntent(copy)) await store.put(namespace, copy);
          else await store.remove(namespace, copy.scope);
          if (previousScope && previousScope.epoch !== copy.scope.epoch)
            await store.remove(namespace, previousScope);
          previousScope = copy.scope;
        });
      queue = write;
      return write;
    },
    async close() {
      if (closed) return;
      closed = true;
      await queue.catch(() => {});
      await owned();
    },
  };
}
