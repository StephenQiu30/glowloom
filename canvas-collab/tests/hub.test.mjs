import assert from "node:assert/strict";
import test from "node:test";
import * as Y from "yjs";
import { createCanvasDocument, readCanvasDocument } from "../src/model.mjs";
import { CollaborationHub } from "../src/hub.mjs";
import { MemoryStore, scope, change } from "./fixtures.mjs";
import { digest } from "../src/admission.mjs";
import { canonicalGraphJson } from "../src/canonical.mjs";

async function fixture() {
  const initial = createCanvasDocument({
    title: "原稿",
    graph: { schemaVersion: 2, nodes: [], connections: [] },
  });
  const store = new MemoryStore(initial),
    base = store.load.bind(store),
    commit = store.commit.bind(store);
  const denied = new Set();
  store.load = async (binding) => {
    if (denied.has(binding.sessionId))
      throw Object.assign(new Error("revoked"), {
        code: 40300,
        definitive: true,
      });
    const loaded = await base();
    const doc = new Y.Doc();
    try {
      Y.applyUpdate(doc, loaded.update);
      return {
        ...loaded,
        binding,
        metadata: {
          epoch: loaded.epoch,
          commitSeq: loaded.commitSeq,
          contentRevision: String(BigInt(loaded.commitSeq) + 1n),
          graphHash: digest(canonicalGraphJson(readCanvasDocument(doc).graph)),
          lifecycle: "ACTIVE",
        },
      };
    } finally {
      doc.destroy();
    }
  };
  store.commit = async (binding, candidate) => {
    if (candidate.graph.title === "invalid")
      throw Object.assign(new Error("invalid"), { definitive: true });
    const result = await commit(binding, candidate);
    if (result.receipt)
      result.receipt.contentRevision = String(
        BigInt(result.receipt.commitSeq) + 1n,
      );
    return result;
  };
  const hub = new CollaborationHub({ store });
  const attach = async (id) => {
    const doc = new Y.Doc(),
      messages = [];
    const session = await hub.join(
      { ...scope, sessionId: id },
      async (message) => {
        messages.push(message);
        if (message.update)
          Y.applyUpdate(doc, Buffer.from(message.update, "base64"));
      },
      () => {},
    );
    return { doc, messages, session };
  };
  return { store, hub, initial, denied, attach };
}

test("two authenticated sessions see only committed updates and a late session bootstraps the durable head", async () => {
  const f = await fixture(),
    a = await f.attach("a"),
    b = await f.attach("b");
  let c;
  try {
    const update = change(a.doc, (doc) =>
      doc.getMap("canvas").set("title", "窗口甲"),
    );
    await f.hub.message(a.session, {
      type: "SUBMIT_UPDATE",
      requestId: crypto.randomUUID(),
      updateId: crypto.randomUUID(),
      epoch: scope.epoch,
      payloadHash: digest(update),
      update: Buffer.from(update).toString("base64"),
    });
    await f.hub.idle();
    assert.equal(readCanvasDocument(b.doc).title, "窗口甲");
    assert.equal(b.messages.at(-1).type, "COMMITTED_UPDATE");
    assert.equal(a.messages.at(-1).type, "DURABLE_ACK");
    assert.equal(f.store.commits.length, 1);
    c = await f.attach("c");
    assert.equal(readCanvasDocument(c.doc).title, "窗口甲");
    assert.equal(c.messages[0].commitSeq, "1");
  } finally {
    f.hub.close();
    [a, b, c].forEach((peer) => peer?.doc.destroy());
    f.initial.destroy();
  }
});

test("business rejection never reaches another window, while a revoked downstream session is stopped", async () => {
  const f = await fixture(),
    a = await f.attach("a"),
    b = await f.attach("b");
  try {
    const invalid = change(a.doc, (doc) =>
      doc.getMap("canvas").set("title", "invalid"),
    );
    await assert.rejects(
      f.hub.message(a.session, {
        type: "SUBMIT_UPDATE",
        requestId: crypto.randomUUID(),
        updateId: crypto.randomUUID(),
        epoch: scope.epoch,
        payloadHash: digest(invalid),
        update: Buffer.from(invalid).toString("base64"),
      }),
      /invalid/,
    );
    assert.equal(b.messages.length, 1);
    f.denied.add("b");
    await f.hub.wake(scope.canvasId, scope.epoch);
    assert.equal(b.session.closed, true);
    assert.equal(b.messages.length, 1);
  } finally {
    f.hub.close();
    [a, b].forEach((peer) => peer.doc.destroy());
    f.initial.destroy();
  }
});

test("delivery authorization loss after durable commit remains UNKNOWN and never becomes a business rejection", async () => {
  const f = await fixture(),
    a = await f.attach("a"),
    b = await f.attach("b");
  const commit = f.store.commit.bind(f.store);
  f.store.commit = async (...args) => {
    const result = await commit(...args);
    f.denied.add("a");
    return result;
  };
  try {
    const update = change(a.doc, (doc) =>
      doc.getMap("canvas").set("title", "已持久化"),
    );
    await assert.rejects(
      f.hub.message(a.session, {
        type: "SUBMIT_UPDATE",
        requestId: crypto.randomUUID(),
        updateId: crypto.randomUUID(),
        epoch: scope.epoch,
        payloadHash: digest(update),
        update: Buffer.from(update).toString("base64"),
      }),
      (error) =>
        error.definitive === false && error.code === "DELIVERY_UNCONFIRMED",
    );
    assert.equal(f.store.commits.length, 1);
    await f.hub.idle();
    assert.equal(readCanvasDocument(b.doc).title, "已持久化");
  } finally {
    f.hub.close();
    [a, b].forEach((peer) => peer.doc.destroy());
    f.initial.destroy();
  }
});
