import assert from "node:assert/strict";
import test from "node:test";
import * as Y from "yjs";
import { CollaborationSession } from "../src/session.mjs";
import {
  createCanvasDocument,
  readCanvasDocument,
  nodeTextBinding,
} from "../src/model.mjs";
import { trackIntents } from "../src/intents.mjs";
import {
  archiveRecovery,
  recoveryFields,
  planRecovery,
} from "../src/recovery.mjs";

const clone = structuredClone;
const value = () => ({
  title: "canvas",
  graph: {
    schemaVersion: 2,
    connections: [],
    nodes: [
      {
        id: "n",
        type: "text",
        nodeTypeVersion: 1,
        title: "node",
        position: { x: 0, y: 0 },
        width: 200,
        height: 100,
        zIndex: 0,
        textDocument: {
          schemaVersion: 1,
          content: { type: "doc", content: [{ type: "paragraph" }] },
        },
      },
    ],
  },
});
const binary = (doc) =>
  Buffer.from(Y.encodeStateAsUpdate(doc)).toString("base64");

test("same-epoch rejected creations retain their never-confirmed business IDs while cross-epoch copies get fresh IDs", async () => {
  for (const [sameEpoch, originalRejected] of [
    [true, true],
    [false, true],
    [true, false],
  ]) {
    const f = fixture();
    const draft = clone(f.base);
    draft.graph.nodes.push({
      ...clone(draft.graph.nodes[0]),
      id: "pending-node",
    });
    draft.graph.connections.push({
      id: "pending-edge",
      fromNodeId: "n",
      toNodeId: "pending-node",
      fromPortId: "text",
      toPortId: "prompt",
      purpose: "PROMPT",
      order: 0,
      binding: { kind: "SOURCE" },
    });
    const record = clone(f.record);
    record.snapshot = draft;
    record.sealed.snapshot = draft;
    record.sealed.intents = trackIntents([], f.base, draft, 3);
    record.sealed.originalRejected = originalRejected;
    const initial = {
      ...f.initial,
      epoch: sameEpoch ? f.initial.epoch : "next-epoch",
    };
    const recovery = archiveRecovery(record, initial),
      source = recovery.recoverySources[0];
    const session = new CollaborationSession({
      scope: f.scope,
      initial,
      recovery,
      journal: { put: async () => {} },
      transport: {},
    });
    try {
      const fields = recoveryFields(source, session.snapshot).map((field) => ({
        ...field,
        copyNode: field.copyRequired,
      }));
      await session.applyRecovery(source.id, fields);
      const restored = session.snapshot.graph;
      const nodeId = restored.nodes.find((node) => node.id !== "n").id;
      const edge = restored.connections[0];
      assert.equal(nodeId === "pending-node", sameEpoch && originalRejected);
      assert.equal(edge.id === "pending-edge", sameEpoch && originalRejected);
      assert.equal(edge.toNodeId, nodeId);
      assert.deepEqual(restored.nodes[0], f.base.graph.nodes[0]);
    } finally {
      session.doc.destroy();
      session.committed.destroy();
      f.server.destroy();
    }
  }
});
function fixture() {
  const server = createCanvasDocument(value());
  const initial = {
    epoch: "epoch",
    commitSeq: "0",
    contentRevision: "1",
    bindingRevision: "0",
    modelVersion: 1,
    lifecycle: "ACTIVE",
    edit: true,
    graphHash: "hash",
    update: binary(server),
  };
  const scope = {
    origin: "http://localhost:8123",
    actorKey: "USER:1",
    canvasId: "10",
    sessionId: crypto.randomUUID(),
  };
  const base = readCanvasDocument(server),
    draft = clone(base);
  draft.title = "private canvas";
  draft.graph.nodes[0].title = "private node";
  const record = {
    version: 2,
    scope: { ...scope, epoch: initial.epoch },
    confirmed: initial,
    generation: 3,
    snapshot: draft,
    inFlight: null,
    acceptedReceipt: null,
    recoverySources: [],
    recoverySelection: null,
    intents: [],
    inFlightIntents: [],
    automaticReplayAttempts: 0,
    pending: [],
    forms: {},
    textDrafts: {},
    control: null,
    sealed: {
      reason: "REJECTED_UPDATE",
      snapshot: draft,
      inFlight: null,
      pending: [],
      intents: trackIntents([], base, draft, 2),
      originalRejected: true,
    },
    updatedAt: 0,
  };
  return { server, initial, scope, base, draft, record };
}

test("archiving makes a fresh authoritative document while retaining flat private sources and the original causal proof", () => {
  const f = fixture();
  try {
    const pending = clone(f.record);
    pending.inFlight = { updateId: "unknown" };
    assert.throws(
      () => archiveRecovery(pending, f.initial),
      /ORIGINAL_UNCONFIRMED/,
    );
    const next = archiveRecovery(f.record, { ...f.initial, epoch: "next" });
    assert.equal(next.scope.sessionId, f.scope.sessionId);
    assert.equal(next.scope.epoch, "next");
    assert.deepEqual(next.snapshot, f.base);
    assert.equal(next.pending.length, 0);
    assert.deepEqual(next.recoverySources[0].sealed, f.record.sealed);
    assert.equal(next.recoverySources[0].confirmed.update, f.initial.update);
    assert.equal(next.recoverySources[0].scope.epoch, "epoch");
    assert.deepEqual(
      archiveRecovery(next, f.initial).recoverySources,
      next.recoverySources,
    );
    assert.equal(JSON.stringify(next).includes('"recoverySources":[]'), false);
  } finally {
    f.server.destroy();
  }
});

test("field selection retains unrelated peer changes and rejects a stale review only for selected logical fields", () => {
  const f = fixture();
  try {
    const record = archiveRecovery(f.record, f.initial),
      source = record.recoverySources[0];
    const fields = recoveryFields(source, f.base),
      title = fields.find(
        (field) => field.field === "title" && field.collection === "nodes",
      );
    const current = clone(f.base);
    current.graph.nodes[0].position.x = 42;
    const plan = planRecovery(source, current, [title]);
    assert.equal(plan.snapshot.title, "canvas");
    assert.equal(plan.snapshot.graph.nodes[0].title, "private node");
    assert.equal(plan.snapshot.graph.nodes[0].position.x, 42);
    current.graph.nodes[0].title = "new peer value";
    assert.throws(
      () => planRecovery(source, current, [title]),
      /RECOVERY_REVIEW_CHANGED/,
    );
    const altered = { ...title, generation: title.generation - 1 };
    assert.throws(
      () => planRecovery(source, f.base, [altered]),
      /RECOVERY_REVIEW_CHANGED/,
    );
  } finally {
    f.server.destroy();
  }
});

test("deleted nodes require an explicit fresh copy and selected references are remapped without reviving lifetime IDs", () => {
  const f = fixture();
  try {
    f.record.textDrafts.n = {
      schemaVersion: 1,
      content: {
        type: "doc",
        content: [
          {
            type: "paragraph",
            content: [{ type: "text", text: "private body" }],
          },
        ],
      },
    };
    const source = archiveRecovery(f.record, f.initial).recoverySources[0],
      empty = clone(f.base);
    empty.graph.nodes = [];
    const fields = recoveryFields(source, empty),
      title = fields.find(
        (field) => field.id === "n" && field.field === "title",
      );
    assert.throws(
      () => planRecovery(source, empty, [title]),
      /RECOVERY_COPY_REQUIRED/,
    );
    const plan = planRecovery(
      source,
      empty,
      fields
        .filter((field) => field.id === "n")
        .map((field) => ({ ...field, copyNode: true })),
    );
    assert.equal(plan.snapshot.graph.nodes.length, 1);
    const node = plan.snapshot.graph.nodes[0];
    assert.notEqual(node.id, "n");
    assert.equal(node.title, "private node");
    assert.equal(
      node.textDocument.content.content[0].content[0].text,
      "private body",
    );
    assert.equal(plan.selection.locations.n, node.id);
    assert.ok(
      plan.selection.mappings.every(
        (mapping) =>
          mapping.targetKey === JSON.stringify(["nodes", node.id, "entity"]),
      ),
    );
  } finally {
    f.server.destroy();
  }
});

test("copying a retained creation preserves an already alive original identity", () => {
  const f = fixture();
  try {
    const before = clone(f.base);
    before.graph.nodes = [];
    f.record.sealed.intents = trackIntents([], before, f.draft, 3);
    const source = archiveRecovery(f.record, f.initial).recoverySources[0];
    const entity = recoveryFields(source, f.base).find(
      (field) => field.collection === "nodes" && field.field === "entity",
    );
    const plan = planRecovery(source, f.base, [{ ...entity, copyNode: true }], {
      n: "fresh-copy",
    });
    assert.deepEqual(
      plan.snapshot.graph.nodes.map((node) => node.id),
      ["n", "fresh-copy"],
    );
    assert.deepEqual(plan.snapshot.graph.nodes[0], f.base.graph.nodes[0]);
    assert.equal(plan.snapshot.graph.nodes[1].title, "private node");
    const materialized = createCanvasDocument(plan.snapshot);
    materialized.destroy();
  } finally {
    f.server.destroy();
  }
});

test("a selected recovery uses the existing text container and clears only its exact accepted source generations", async () => {
  const f = fixture();
  let session;
  try {
    f.record.textDrafts.n = {
      schemaVersion: 1,
      content: {
        type: "doc",
        content: [
          {
            type: "paragraph",
            content: [{ type: "text", text: "retained  text" }],
          },
        ],
      },
    };
    const recovery = archiveRecovery(f.record, f.initial),
      records = [];
    let release;
    const gate = new Promise((resolve) => {
      release = resolve;
    });
    session = new CollaborationSession({
      scope: f.scope,
      initial: f.initial,
      recovery,
      journal: { put: async (record) => records.push(record) },
      transport: {
        query: async () => null,
        submit: async (request) => {
          await gate;
          Y.applyUpdate(f.server, Buffer.from(request.update, "base64"));
          return {
            ...request,
            commitSeq: "1",
            contentRevision: "2",
            graphHash: "next",
            effect: "VISIBLE_CHANGE",
          };
        },
      },
    });
    const source = session.recoverySources[0],
      fields = recoveryFields(source, session.snapshot),
      selected = fields.filter((field) => field.id === "n");
    const fragment = nodeTextBinding(session.doc, "n").fragment;
    await session.applyRecovery(source.id, selected);
    assert.equal(nodeTextBinding(session.doc, "n").fragment, fragment);
    assert.equal(
      session.snapshot.graph.nodes[0].textDocument.content.content[0].content[0]
        .text,
      "retained  text",
    );
    const sending = session.flush();
    await new Promise((resolve) => setImmediate(resolve));
    const newTail = {
      schemaVersion: 1,
      content: {
        type: "doc",
        content: [
          {
            type: "paragraph",
            content: [{ type: "text", text: "late composition" }],
          },
        ],
      },
    };
    session.retainSourceText(
      f.record.scope,
      "n",
      newTail,
      f.draft.graph.nodes[0],
    );
    release();
    await sending;
    assert.equal(
      session.recoverySources[0].sealed.intents.some(
        (intent) => intent.id === "n" && intent.field === "title",
      ),
      false,
    );
    assert.equal(
      session.recoverySources[0].sealed.intents.some(
        (intent) => intent.collection === "canvas",
      ),
      true,
    );
    assert.deepEqual(session.recoverySources[0].textDrafts.n, newTail);
    assert.equal(session.recoverySelection, null);
    assert.ok(records.some((record) => record.recoverySelection?.updateId));
    assert.equal(session.status, "CLEAN");
  } finally {
    session?.destroy();
    f.server.destroy();
  }
});

test("failed local protection cannot submit a recovery or release its source", async () => {
  const f = fixture();
  let session,
    submissions = 0;
  try {
    session = new CollaborationSession({
      scope: f.scope,
      initial: f.initial,
      recovery: archiveRecovery(f.record, f.initial),
      journal: {
        put: async () => {
          throw new Error("quota");
        },
      },
      transport: {
        query: async () => null,
        submit: async () => {
          submissions++;
        },
      },
    });
    const source = session.recoverySources[0];
    await assert.rejects(
      session.applyRecovery(
        source.id,
        recoveryFields(source, session.snapshot),
      ),
      /quota/,
    );
    assert.equal(submissions, 0);
    assert.deepEqual(session.snapshot, f.base);
    assert.deepEqual(session.recoverySources[0].sealed, f.record.sealed);
  } finally {
    session?.destroy();
    f.server.destroy();
  }
});

test("copy allocation is frozen across device writes and selected invalid parameter buffers remain private until node acceptance", async () => {
  const f = fixture();
  let session;
  try {
    f.record.forms.n = { width: "-" };
    const empty = value();
    empty.graph.nodes = [];
    const server = createCanvasDocument(empty),
      initial = { ...f.initial, epoch: "new", update: binary(server) };
    const recovery = archiveRecovery(f.record, initial),
      source = recovery.recoverySources[0];
    const selected = recoveryFields(source, empty)
      .filter((field) => field.id === "n")
      .map((field) => ({ ...field, copyNode: true }));
    const first = planRecovery(source, empty, selected),
      second = planRecovery(source, empty, selected, first.selection.locations);
    assert.deepEqual(second, first);
    let release;
    const gate = new Promise((resolve) => {
      release = resolve;
    });
    session = new CollaborationSession({
      scope: f.scope,
      initial,
      recovery,
      journal: { put: async () => {} },
      transport: {
        query: async () => null,
        submit: async (request) => {
          await gate;
          Y.applyUpdate(server, Buffer.from(request.update, "base64"));
          return {
            ...request,
            commitSeq: "1",
            contentRevision: "2",
            graphHash: "hash",
            effect: "VISIBLE_CHANGE",
          };
        },
      },
    });
    await session.applyRecovery(source.id, selected);
    const copyId = session.snapshot.graph.nodes[0].id;
    assert.equal(session.recoverySelection.locations.n, copyId);
    assert.deepEqual(session.forms, {});
    assert.deepEqual(session.recoverySources[0].forms.n, { width: "-" });
    const sending = session.flush();
    await new Promise((resolve) => setImmediate(resolve));
    release();
    await sending;
    assert.deepEqual(session.forms[copyId], { width: "-" });
    assert.deepEqual(session.recoverySources[0].forms, {});
    assert.equal(
      readCanvasDocument(server).graph.nodes[0].parameters,
      undefined,
    );
    server.destroy();
  } finally {
    session?.destroy();
    f.server.destroy();
  }
});

test("repeated rejected recovery retains one source and one bounded last attempt instead of nested archive copies", async () => {
  const f = fixture();
  let session;
  try {
    let recovery = archiveRecovery(f.record, f.initial);
    for (let attempt = 0; attempt < 20; attempt++) {
      session = new CollaborationSession({
        scope: f.scope,
        initial: f.initial,
        recovery,
        journal: { put: async () => {} },
        transport: {
          query: async () => null,
          submit: async () => {
            throw Object.assign(new Error("FORBIDDEN"), {
              code: "FORBIDDEN",
              definitive: true,
            });
          },
        },
      });
      const source = session.recoverySources[0];
      await session.applyRecovery(
        source.id,
        recoveryFields(source, session.snapshot),
      );
      await assert.rejects(session.flush(), /FORBIDDEN/);
      recovery = archiveRecovery(session.record(), f.initial);
      session.destroy();
      session = null;
      assert.equal(recovery.recoverySources.length, 1);
      assert.equal(
        recovery.recoverySources[0].lastAttempt.sealed.reason,
        "FORBIDDEN",
      );
      assert.deepEqual(recovery.recoverySources[0].sealed, f.record.sealed);
    }
  } finally {
    session?.destroy();
    f.server.destroy();
  }
});
