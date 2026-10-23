import assert from "node:assert/strict";
import test from "node:test";
import * as Y from "yjs";
import { AdmissionGate } from "../src/admission.mjs";
import {
  change,
  documentFrom,
  MemoryStore,
  project,
  scope,
  validate,
} from "./fixtures.mjs";

function gate(store, options = {}) {
  return new AdmissionGate({
    store,
    project,
    validate,
    authorize: async () => true,
    ...options,
  });
}
function request(update, updateId = "update-1") {
  return { update, updateId, epoch: scope.epoch };
}

test("known candidate rejection is definitive while contention keeps the original outcome unresolved", async () => {
  const store = new MemoryStore(),
    doc = documentFrom(store.state.update);
  try {
    const update = change(doc, (d) => {
      d.getMap("nodes").set("a", {});
      d.getMap("nodes").set("b", {});
    });
    await assert.rejects(
      gate(store, { maxStructs: 1 }).submit(scope, request(update)),
      (error) =>
        error.code === "UPDATE_TOO_COMPLEX" && error.definitive === true,
    );
    store.commit = async () => ({ committed: false });
    await assert.rejects(
      gate(store, { maxRetries: 1 }).submit(scope, request(update)),
      (error) => error.code === "CONTENTION_LIMIT" && error.definitive !== true,
    );
  } finally {
    doc.destroy();
  }
});

test("independent windows converge through durable receipts without replacing the graph", async () => {
  const store = new MemoryStore();
  const engine = gate(store);
  const a = documentFrom(store.state.update),
    b = documentFrom(store.state.update);
  const ua = change(a, (d) => d.getMap("nodes").set("a", { title: "甲" }));
  const ub = change(b, (d) => d.getMap("nodes").set("b", { title: "乙" }));
  const [ra, rb] = await Promise.all([
    engine.submit(scope, request(ua, "a")),
    engine.submit({ ...scope, sessionId: "b" }, request(ub, "b")),
  ]);
  assert.deepEqual([ra.commitSeq, rb.commitSeq].sort(), ["1", "2"]);
  assert.deepEqual(project(documentFrom(store.state.update)).nodes, {
    a: { title: "甲" },
    b: { title: "乙" },
  });
  a.destroy();
  b.destroy();
});

test("same update identity returns original receipt and changed bytes are rejected", async () => {
  const store = new MemoryStore(),
    engine = gate(store),
    doc = documentFrom(store.state.update);
  const first = request(
    change(doc, (d) => d.getText("text").insert(0, "原稿")),
  );
  const receipt = await engine.submit(scope, first);
  assert.deepEqual(await engine.submit(scope, first), receipt);
  assert.equal(store.commits.length, 1);
  await assert.rejects(
    engine.submit(
      scope,
      request(change(doc, (d) => d.getText("text").insert(2, "新"))),
    ),
    { code: "IDENTITY_REUSE" },
  );
  doc.destroy();
});

test("invalid candidate never reaches commit or broadcast", async () => {
  const initial = new Y.Doc();
  initial.getMap("nodes").set("a", {});
  initial.getMap("nodes").set("b", {});
  const store = new MemoryStore(initial),
    messages = [],
    engine = gate(store, { broadcast: async (...m) => messages.push(m) });
  const doc = documentFrom(store.state.update);
  const update = change(doc, (d) => {
    d.getMap("edges").set("ab", { from: "a", to: "b" });
    d.getMap("edges").set("ba", { from: "b", to: "a" });
  });
  await assert.rejects(engine.submit(scope, request(update)), {
    code: "INVALID_GRAPH",
  });
  assert.equal(store.commits.length, 0);
  assert.deepEqual(messages, []);
  assert.deepEqual(project(documentFrom(store.state.update)).edges, {});
  initial.destroy();
  doc.destroy();
});

test("update depending on a rejected predecessor cannot be acknowledged", async () => {
  const store = new MemoryStore(),
    engine = gate(store),
    doc = documentFrom(store.state.update);
  const predecessor = change(doc, (d) =>
    d.getText("text").insert(0, "拒绝前驱"),
  );
  const dependent = change(doc, (d) => d.getText("text").insert(4, "依赖后续"));
  await assert.rejects(engine.submit(scope, request(dependent, "dependent")), {
    code: "MISSING_DEPENDENCY",
  });
  assert.equal(store.commits.length, 0);
  const clean = documentFrom(store.state.update);
  await engine.submit(
    { ...scope, sessionId: "recovered" },
    request(
      change(clean, (d) => d.getText("text").insert(0, "重新确认的输入")),
      "recovered",
    ),
  );
  assert.equal(
    project(documentFrom(store.state.update)).text,
    "重新确认的输入",
  );
  assert.ok(predecessor.byteLength > 0);
  doc.destroy();
  clean.destroy();
});

test("deletion with missing target is held despite containing no new structs", async () => {
  const store = new MemoryStore(),
    engine = gate(store),
    doc = documentFrom(store.state.update);
  change(doc, (d) => d.getText("text").insert(0, "未接纳"));
  const deletion = change(doc, (d) => d.getText("text").delete(0, 3));
  await assert.rejects(engine.submit(scope, request(deletion)), {
    code: "MISSING_DEPENDENCY",
  });
  assert.equal(store.commits.length, 0);
  doc.destroy();
});

test("commit failure and expired epoch cannot broadcast or grant durable success", async () => {
  const store = new MemoryStore(),
    messages = [],
    engine = gate(store, { broadcast: async (m) => messages.push(m) });
  const doc = documentFrom(store.state.update),
    update = change(doc, (d) => d.getText("text").insert(0, "保留"));
  await assert.rejects(
    engine.submit(scope, { ...request(update), epoch: "retired" }),
    { code: "EPOCH_CHANGED" },
  );
  store.commit = async () => {
    throw Object.assign(new Error("database unavailable"), {
      code: "STORE_FAILED",
    });
  };
  await assert.rejects(engine.submit(scope, request(update)), {
    code: "STORE_FAILED",
  });
  assert.deepEqual(messages, []);
  doc.destroy();
});

test("broadcast happens after durability and a publish failure preserves original receipt", async () => {
  const store = new MemoryStore();
  let seen;
  const engine = gate(store, {
    broadcast: async () => {
      seen = store.state.commitSeq;
      throw new Error("Redis unavailable");
    },
  });
  const doc = documentFrom(store.state.update),
    r = request(change(doc, (d) => d.getText("text").insert(0, "已提交")));
  const ack = await engine.submit(scope, r);
  assert.equal(seen, "1");
  assert.equal(ack.commitSeq, "1");
  assert.deepEqual(await engine.submit(scope, r), ack);
  assert.equal(store.commits.length, 1);
  doc.destroy();
});

test("unauthorized, malformed and oversized updates fail before persistence", async () => {
  const store = new MemoryStore();
  await assert.rejects(
    gate(store, { authorize: async () => false }).submit(
      scope,
      request(new Uint8Array([0, 0])),
    ),
    { code: "FORBIDDEN" },
  );
  await assert.rejects(
    gate(store).submit(scope, request(new Uint8Array([255]))),
    { code: "INVALID_UPDATE" },
  );
  await assert.rejects(
    gate(store, { maxUpdateBytes: 2 }).submit(
      scope,
      request(new Uint8Array(3)),
    ),
    { code: "UPDATE_TOO_LARGE" },
  );
  assert.equal(store.commits.length, 0);
});

test("an unregistered shared root cannot hide writes outside the business projection", async () => {
  const store = new MemoryStore(),
    engine = gate(store),
    doc = documentFrom(store.state.update);
  const update = change(doc, (d) =>
    d.getMap("permissions").set("role", "owner"),
  );
  await assert.rejects(engine.submit(scope, request(update)), {
    code: "INVALID_ROOT",
  });
  assert.equal(store.commits.length, 0);
  doc.destroy();
});
