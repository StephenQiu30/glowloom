import assert from "node:assert/strict";
import test from "node:test";
import { CollaborationJournal } from "../src/journal.mjs";

function database() {
  let metadata = new Map(),
    records = new Map(),
    queue = Promise.resolve();
  return {
    run(action) {
      const operation = queue
        .catch(() => {})
        .then(async () => {
          const meta = structuredClone(metadata),
            data = structuredClone(records);
          const result = await action({
            meta: async (id) => meta.get(id),
            rows: async (owner) =>
              [...meta.values()].filter((row) => row.owner === owner),
            read: async (id) => data.get(id),
            writeMeta: (row) => meta.set(row.id, row),
            write: (id, value) => data.set(id, structuredClone(value)),
            remove: (id) => data.delete(id),
            removeMeta: (id) => meta.delete(id),
          });
          metadata = meta;
          records = data;
          return result;
        });
      queue = operation;
      return operation;
    },
  };
}
const scope = {
  epoch: "epoch",
  origin: "http://localhost:8123",
  actorKey: "USER:1",
  canvasId: "10",
  sessionId: "6f530c76-6fc6-4f9e-abab-f95f072b9002",
};
const record = {
  version: 2,
  scope,
  generation: 1,
  confirmed: { epoch: "epoch", commitSeq: "1", update: "AAA=" },
  inFlight: { updateId: "original", update: "AAA=" },
  acceptedReceipt: null,
  recoverySources: [],
  recoverySelection: null,
  intents: [],
  inFlightIntents: [],
  automaticReplayAttempts: 0,
  pending: [],
  snapshot: { title: "draft", graph: {} },
  forms: {},
  sealed: null,
  updatedAt: 0,
};
test("a pending control receipt keeps an otherwise clean recovery record", async () => {
  const journal = new CollaborationJournal(database()),
    writer = await journal.open(scope);
  const input = {
    ...record,
    inFlight: null,
    control: {
      operation: "DELETE",
      status: "UNKNOWN",
      body: { commandId: "original" },
    },
  };
  await writer.put(input);
  assert.equal(
    (await journal.list(scope))[0].control.body.commandId,
    "original",
  );
});

test("closing a writer fences late writes to a retained pending record without deleting its private draft", async () => {
  const journal = new CollaborationJournal(database()),
    writer = await journal.open(scope);
  await writer.put(record);
  await writer.close();
  await assert.rejects(
    writer.put({ ...record, generation: record.generation + 1 }),
    /JOURNAL_INVALIDATED/,
  );
  assert.equal((await journal.list(scope))[0].generation, record.generation);
  const reopened = await journal.open(scope);
  await reopened.put({ ...record, generation: record.generation + 1 });
  assert.equal(
    (await journal.list(scope))[0].generation,
    record.generation + 1,
  );
});

test("an atomic recovery handoff reuses its slot at the sixteen-record budget and fences the old epoch writer", async () => {
  const journal = new CollaborationJournal(database());
  let sourceWriter;
  for (let index = 0; index < 16; index++) {
    const itemScope = index
      ? { ...scope, sessionId: crypto.randomUUID() }
      : scope;
    const writer = await journal.open(itemScope);
    if (!index) sourceWriter = writer;
    await writer.put({ ...record, scope: itemScope });
    await writer.close();
  }
  const targetScope = { ...scope, epoch: "new-epoch" };
  const target = {
    ...record,
    scope: targetScope,
    generation: 2,
    textDrafts: {},
    confirmed: { ...record.confirmed, epoch: targetScope.epoch },
    inFlight: null,
    recoverySources: [
      {
        id: crypto.randomUUID(),
        scope,
        confirmed: record.confirmed,
        sealed: {
          reason: "REJECTED",
          snapshot: record.snapshot,
          inFlight: record.inFlight,
          pending: [],
          intents: [],
        },
        forms: {},
        textDrafts: {},
        locations: {},
      },
    ],
  };
  const writer = await journal.transplant(record, target);
  assert.equal((await journal.list(scope)).length, 16);
  assert.ok(
    !(await journal.list(scope)).some(
      (row) =>
        row.scope.epoch === scope.epoch &&
        row.scope.sessionId === scope.sessionId,
    ),
  );
  assert.equal(
    (await journal.list(scope)).find(
      (row) => row.scope.epoch === targetScope.epoch,
    ).recoverySources[0].sealed.snapshot.title,
    "draft",
  );
  await assert.rejects(
    sourceWriter.put({ ...record, generation: 99 }),
    /JOURNAL_INVALIDATED/,
  );
  await writer.put({ ...target, generation: 3 });
});

test("stale, active, oversized or different-account handoffs never remove the protected original", async () => {
  const journal = new CollaborationJournal(database()),
    writer = await journal.open(scope);
  await writer.put(record);
  const target = {
    ...record,
    generation: 2,
    scope: { ...scope, epoch: "new-epoch" },
    confirmed: { ...record.confirmed, epoch: "new-epoch" },
  };
  await assert.rejects(
    journal.transplant(record, target),
    /JOURNAL_SOURCE_BUSY/,
  );
  await writer.close();
  await assert.rejects(
    journal.transplant(
      { ...record, snapshot: { title: "stale", graph: {} } },
      target,
    ),
    /JOURNAL_SOURCE_CHANGED/,
  );
  await assert.rejects(
    journal.transplant(record, {
      ...target,
      scope: { ...target.scope, actorKey: "USER:2" },
    }),
    /JOURNAL_SCOPE/,
  );
  await assert.rejects(
    journal.transplant(record, {
      ...target,
      snapshot: { title: "x".repeat(5 * 1024 * 1024), graph: {} },
    }),
    /JOURNAL_BUDGET/,
  );
  assert.deepEqual((await journal.list(scope))[0], record);
});
test("composition text is retained even before a CRDT update exists", async () => {
  const journal = new CollaborationJournal(database()),
    writer = await journal.open(scope);
  await writer.put({
    ...record,
    inFlight: null,
    textDrafts: {
      n: { schemaVersion: 1, content: { type: "doc", content: [] } },
    },
  });
  assert.equal(
    Object.keys((await journal.list(scope))[0].textDrafts).length,
    1,
  );
});
test("discarding one claimed session preserves another window's recovery record", async () => {
  const journal = new CollaborationJournal(database()),
    first = await journal.open(scope);
  const other = { ...scope, sessionId: crypto.randomUUID() },
    second = await journal.open(other);
  await first.put(record);
  await second.put({ ...record, scope: other });
  await journal.removeRecord(scope);
  assert.equal((await journal.list(scope)).length, 1);
  assert.equal((await journal.list(scope))[0].scope.sessionId, other.sessionId);
  await assert.rejects(first.put(record), /JOURNAL_INVALIDATED/);
});

test("account and canvas isolation plus tombstones prevent late writes from resurrecting discarded data", async () => {
  const journal = new CollaborationJournal(database()),
    writer = await journal.open(scope);
  await writer.put(record);
  assert.equal((await journal.list(scope)).length, 1);
  assert.equal(
    (await journal.list({ ...scope, actorKey: "USER:2" })).length,
    0,
  );
  await journal.removeCanvas(scope);
  await assert.rejects(writer.put(record), /JOURNAL_INVALIDATED/);
  assert.equal((await journal.list(scope)).length, 0);
  const fresh = await journal.open(scope);
  await fresh.put(record);
  await journal.removeActor(scope.origin, scope.actorKey);
  await assert.rejects(fresh.put(record), /JOURNAL_INVALIDATED/);
});

test("epochs never overwrite each other's recovery data and closed writers cannot resurrect a clean record", async () => {
  const journal = new CollaborationJournal(database());
  const first = await journal.open(scope);
  await first.put({ ...record, generation: 7 });
  const nextScope = { ...scope, epoch: "next-epoch" };
  const nextRecord = {
    ...record,
    scope: nextScope,
    confirmed: { ...record.confirmed, epoch: "next-epoch" },
  };
  const second = await journal.open(nextScope);
  await second.put(nextRecord);
  assert.equal((await journal.list(scope)).length, 2);
  await second.put({ ...nextRecord, generation: 2, inFlight: null });
  await second.close();
  const third = await journal.open(nextScope);
  await assert.rejects(
    second.put({ ...nextRecord, generation: 3 }),
    /JOURNAL_INVALIDATED/,
  );
  await third.put(nextRecord);
  assert.equal((await journal.list(scope)).length, 2);
});

test("stale records, credential fields and capacity failure never replace a protected original", async () => {
  const journal = new CollaborationJournal(database()),
    writer = await journal.open(scope);
  await writer.put({ ...record, generation: 3 });
  await assert.rejects(writer.put(record), /JOURNAL_SEQUENCE/);
  await assert.rejects(
    writer.put({ ...record, generation: 4, grant: "secret" }),
    /JOURNAL_FIELD/,
  );
  await assert.rejects(
    writer.put({
      ...record,
      generation: 4,
      snapshot: { text: "a".repeat(4 * 1024 * 1024) },
    }),
    /JOURNAL_BUDGET/,
  );
  assert.equal((await journal.list(scope))[0].generation, 3);
});

test("durably clean journals release payload space while preserving an active writer and unapplied forms", async () => {
  const journal = new CollaborationJournal(database()),
    writer = await journal.open(scope);
  await writer.put(record);
  await writer.put({ ...record, generation: 2, inFlight: null });
  assert.equal((await journal.list(scope)).length, 0);
  await writer.put({
    ...record,
    generation: 3,
    inFlight: null,
    forms: { node: { title: "unapplied" } },
  });
  assert.equal((await journal.list(scope)).length, 1);
  await writer.put({ ...record, generation: 4 });
  assert.equal((await journal.list(scope))[0].generation, 4);
});

test("invalid formats and nested credentials never replace the original while mention display tokens remain legal", async () => {
  const journal = new CollaborationJournal(database()),
    writer = await journal.open(scope);
  await writer.put(record);
  for (const change of [
    { version: 1 },
    { forms: { node: { authorization: "private" } } },
    { forms: { node: new Date() } },
    { forms: { node: new Blob(["private"]) } },
    { forms: { node: { providerKey: "private" } } },
  ]) {
    await assert.rejects(
      writer.put({ ...record, generation: 2, ...change }),
      /JOURNAL_(FORMAT|FIELD)/,
    );
    assert.deepEqual((await journal.list(scope))[0], record);
  }
  await writer.put({
    ...record,
    generation: 2,
    forms: { node: { mention: { token: "@image1" } } },
  });
  assert.equal(
    (await journal.list(scope))[0].forms.node.mention.token,
    "@image1",
  );
});

test("separate sessions keep their writers across log instances and selected-canvas cleanup fences only that canvas", async () => {
  const db = database(),
    first = new CollaborationJournal(db),
    second = new CollaborationJournal(db);
  const otherScope = { ...scope, sessionId: crypto.randomUUID() },
    unrelated = { ...scope, canvasId: "11" };
  const a = await first.open(scope),
    b = await second.open(otherScope),
    c = await second.open(unrelated);
  await a.put(record);
  await b.put({ ...record, scope: otherScope });
  await c.put({ ...record, scope: unrelated });
  await a.put({ ...record, generation: 2 });
  assert.equal((await second.list(scope)).length, 2);
  await first.removeCanvas(scope);
  await assert.rejects(
    a.put({ ...record, generation: 3 }),
    /JOURNAL_INVALIDATED/,
  );
  await assert.rejects(
    b.put({ ...record, scope: otherScope, generation: 3 }),
    /JOURNAL_INVALIDATED/,
  );
  await c.put({ ...record, scope: unrelated, generation: 2 });
  assert.equal((await second.list(unrelated))[0].generation, 2);
  assert.equal(await first.countActor(scope.origin, scope.actorKey), 1);
});

test("the default journal is page-local and never opens IndexedDB", async () => {
  const previous = Object.getOwnPropertyDescriptor(globalThis, "indexedDB");
  Object.defineProperty(globalThis, "indexedDB", {
    configurable: true,
    get() {
      throw new Error("IndexedDB forbidden");
    },
  });
  try {
    const page = new CollaborationJournal(),
      writer = await page.open(scope);
    await writer.put(record);
    await writer.close();
    assert.equal((await page.list(scope)).length, 1);
    assert.equal((await new CollaborationJournal().list(scope)).length, 0);
  } finally {
    if (previous) Object.defineProperty(globalThis, "indexedDB", previous);
    else delete globalThis.indexedDB;
  }
});
