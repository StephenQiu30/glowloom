import assert from "node:assert/strict";
import test from "node:test";
import * as Y from "yjs";
import { randomUUID } from "node:crypto";
import { computeCandidate } from "../src/service-runtime.mjs";
import { runtimeServer } from "../src/server.mjs";
import { digest } from "../src/admission.mjs";
import { applyCanvasChanges, readCanvasDocument } from "../src/model.mjs";
const dto = () => ({
  id: randomUUID(),
  project_id: randomUUID(),
  name: "画布",
  schema_version: 1,
  operation_version: 1,
  revision: "1",
  permissions: { read: true, write: true },
  nodes: [],
  edges: [],
});
const request = () => {
  const document = dto();
  return {
    documentId: document.id,
    epoch: randomUUID(),
    writerTerm: "1",
    commitSeq: "0",
    document,
  };
};
test("stateless prepare binds isolated candidate to exact authority, rejects unknown roots without contaminating the baseline", async () => {
  const req = request(),
    initial = await computeCandidate("initialize", req),
    doc = new Y.Doc();
  try {
    Y.applyUpdate(doc, Buffer.from(initial.state, "base64"));
    const before = readCanvasDocument(doc);
    applyCanvasChanges(doc, before, { ...before, title: "共同画布" });
    const bytes = Y.encodeStateAsUpdate(doc),
      update = Buffer.from(bytes).toString("base64");
    const input = {
      ...req,
      state: initial.state,
      update,
      updateId: randomUUID(),
      payloadHash: digest(bytes),
    };
    const result = await computeCandidate("prepare", input);
    assert.equal(result.projection.name, "共同画布");
    assert.equal(result.effect, "VISIBLE_CHANGE");
    assert.equal(result.epoch, req.epoch);
    assert.equal(result.commitSeq, "0");
    doc.getMap("evil").set("bad", true);
    const bad = Y.encodeStateAsUpdate(doc);
    await assert.rejects(
      computeCandidate("prepare", {
        ...input,
        update: Buffer.from(bad).toString("base64"),
        payloadHash: digest(bad),
      }),
    );
    assert.deepEqual(await computeCandidate("prepare", input), result);
  } finally {
    doc.destroy();
  }
});
test("runtime is inaccessible without internal bearer, accepts bounded authenticated compute calls", async () => {
  const token = "x".repeat(32),
    server = runtimeServer({ token });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  try {
    const url = `http://127.0.0.1:${server.address().port}/v1/initialize`;
    assert.equal(
      (await fetch(url, { method: "POST", body: "{}" })).status,
      401,
    );
    const response = await fetch(url, {
      method: "POST",
      headers: { authorization: `Bearer ${token}` },
      body: JSON.stringify(request()),
    });
    assert.equal(response.status, 200);
    assert.ok((await response.json()).state);
  } finally {
    await new Promise((resolve) => server.close(resolve));
  }
});

test("late edits targeting a deleted shared subtree are rejected, while ordinary concurrent character deletion remains mergeable", async () => {
  const req = request(),
    id = randomUUID();
  req.document.nodes = [
    {
      id,
      node_type: "text",
      node_action: "resource",
      title: "正文",
      x: 0,
      y: 0,
      parent_id: null,
      z_index: 1,
      content_version: "1",
      config: { text: "AB", format: "plain", format_version: 1 },
    },
  ];
  const initial = await computeCandidate("initialize", req),
    deleted = new Y.Doc(),
    late = new Y.Doc();
  try {
    Y.applyUpdate(deleted, Buffer.from(initial.state, "base64"));
    Y.applyUpdate(late, Buffer.from(initial.state, "base64"));
    const before = readCanvasDocument(deleted);
    applyCanvasChanges(deleted, before, {
      ...before,
      graph: { ...before.graph, nodes: [] },
    });
    const updates = [];
    late.on("update", (update) => updates.push(update));
    late
      .getMap("canvas")
      .get("nodes")
      .get(id)
      .get("body")
      .get(0)
      .get(0)
      .insert(1, "迟到");
    const bytes = Y.mergeUpdates(updates);
    await assert.rejects(
      computeCandidate("prepare", {
        ...req,
        state: Buffer.from(Y.encodeStateAsUpdate(deleted)).toString("base64"),
        document: { ...req.document, nodes: [] },
        updateId: randomUUID(),
        update: Buffer.from(bytes).toString("base64"),
        payloadHash: digest(bytes),
      }),
      (error) => error.code === "TARGET_DELETED",
    );
  } finally {
    deleted.destroy();
    late.destroy();
  }
});

test("ordinary character tombstones still merge a concurrent insertion into a live paragraph", async () => {
  const req = request(),
    id = randomUUID();
  req.document.nodes = [
    {
      id,
      node_type: "text",
      node_action: "resource",
      title: "正文",
      x: 0,
      y: 0,
      parent_id: null,
      z_index: 1,
      content_version: "1",
      config: { text: "AB", format: "plain", format_version: 1 },
    },
  ];
  const initial = await computeCandidate("initialize", req),
    a = new Y.Doc(),
    b = new Y.Doc();
  try {
    Y.applyUpdate(a, Buffer.from(initial.state, "base64"));
    Y.applyUpdate(b, Buffer.from(initial.state, "base64"));
    const text = (doc) =>
      doc.getMap("canvas").get("nodes").get(id).get("body").get(0).get(0);
    text(a).delete(0, 1);
    const changes = [];
    b.on("update", (u) => changes.push(u));
    text(b).insert(1, "并发");
    const update = Y.mergeUpdates(changes);
    const result = await computeCandidate("prepare", {
      ...req,
      state: Buffer.from(Y.encodeStateAsUpdate(a)).toString("base64"),
      updateId: randomUUID(),
      update: Buffer.from(update).toString("base64"),
      payloadHash: digest(update),
    });
    assert.match(
      JSON.stringify(result.projection.nodes[0].config.rich_text),
      /并发B/,
    );
  } finally {
    a.destroy();
    b.destroy();
  }
});

test("a delta after node deletion may carry already accepted tombstones without rejecting a new title", async () => {
  const req = request(),
    id = randomUUID();
  req.document.nodes = [
    {
      id,
      node_type: "text",
      node_action: "resource",
      title: "正文",
      x: 0,
      y: 0,
      parent_id: null,
      z_index: 1,
      content_version: "1",
      config: { text: "AB", format: "plain", format_version: 1 },
    },
  ];
  const initial = await computeCandidate("initialize", req),
    doc = new Y.Doc();
  try {
    Y.applyUpdate(doc, Buffer.from(initial.state, "base64"));
    const before = readCanvasDocument(doc);
    applyCanvasChanges(doc, before, {
      ...before,
      graph: { ...before.graph, nodes: [] },
    });
    const state = Y.encodeStateAsUpdate(doc),
      vector = Y.encodeStateVector(doc),
      next = readCanvasDocument(doc);
    applyCanvasChanges(doc, next, { ...next, title: "删除后继续编辑" });
    const update = Y.encodeStateAsUpdate(doc, vector);
    const result = await computeCandidate("prepare", {
      ...req,
      document: { ...req.document, nodes: [] },
      state: Buffer.from(state).toString("base64"),
      updateId: randomUUID(),
      update: Buffer.from(update).toString("base64"),
      payloadHash: digest(update),
    });
    assert.equal(result.projection.name, "删除后继续编辑");
  } finally {
    doc.destroy();
  }
});

test("concurrent references retain both identities and receive distinct durable labels without renumbering tombstones", async () => {
  const req = request();
  req.document.nodes = [
    {
      id: randomUUID(),
      node_type: "text",
      node_action: "manual",
      title: "引用",
      x: 0,
      y: 0,
      z_index: 0,
      parent_id: null,
      config: {
        composer: {
          schemaVersion: 1,
          document: { type: "doc", content: [{ type: "paragraph" }] },
          references: [],
          modelVersion: null,
          mode: null,
          params: {},
          labelState: {
            highWater: { image: 0, video: 0, audio: 0, subject: 0 },
            allocations: {},
          },
        },
      },
    },
  ];
  const initial = await computeCandidate("initialize", req);
  const edits = ["a", "b"].map((id) => {
    const doc = new Y.Doc();
    Y.applyUpdate(doc, Buffer.from(initial.state, "base64"));
    const before = readCanvasDocument(doc),
      after = structuredClone(before);
    after.graph.nodes[0].references = [
      {
        id,
        source: {
          kind: "pending_upload",
          clientUploadId: id,
          uploadAttemptId: id,
        },
        bindingGeneration: 1,
        mediaKind: "image",
        role: "reference",
        order: 0,
      },
    ];
    after.graph.nodes[0].labelState.allocations[id] = {
      labelKind: "image",
      ordinal: 1,
    };
    after.graph.nodes[0].labelState.highWater.image = 1;
    applyCanvasChanges(doc, before, after);
    const update = Y.encodeStateAsUpdate(doc);
    doc.destroy();
    return {
      update: Buffer.from(update).toString("base64"),
      payloadHash: digest(update),
      updateId: randomUUID(),
    };
  });
  const a = await computeCandidate("prepare", {
    ...req,
    state: initial.state,
    ...edits[0],
  });
  const b = await computeCandidate("prepare", {
    ...req,
    commitSeq: "1",
    document: a.projection,
    state: a.state,
    ...edits[1],
  });
  const composer = b.projection.nodes[0].config.composer;
  assert.deepEqual(
    composer.references.map((r) => r.id),
    ["a", "b"],
  );
  assert.deepEqual(composer.labelState.allocations, {
    a: { labelKind: "image", ordinal: 1 },
    b: { labelKind: "image", ordinal: 2 },
  });
  const reopened = new Y.Doc();
  Y.applyUpdate(reopened, Buffer.from(b.state, "base64"));
  assert.deepEqual(
    readCanvasDocument(reopened).graph.nodes[0].labelState,
    composer.labelState,
  );
  const before = readCanvasDocument(reopened),
    after = structuredClone(before);
  after.graph.nodes[0].references = [];
  after.graph.nodes[0].labelState = {
    highWater: { image: 0, video: 0, audio: 0, subject: 0 },
    allocations: {},
  };
  applyCanvasChanges(reopened, before, after);
  const update = Y.encodeStateAsUpdate(reopened);
  reopened.destroy();
  const removed = await computeCandidate("prepare", {
    ...req,
    commitSeq: "2",
    document: b.projection,
    state: b.state,
    update: Buffer.from(update).toString("base64"),
    payloadHash: digest(update),
    updateId: randomUUID(),
  });
  assert.deepEqual(
    removed.projection.nodes[0].config.composer.labelState,
    composer.labelState,
  );
});

test("explicit undo creates a new shared lifetime; late edits cannot touch restored content", async () => {
  const { canvasUndoManager, applyCanvasHistory, nodeTextBinding } =
    await import("../src/model.mjs");
  const req = request(),
    id = randomUUID();
  req.document.nodes = [
    {
      id,
      node_type: "text",
      node_action: "resource",
      title: "body",
      x: 0,
      y: 0,
      z_index: 0,
      parent_id: null,
      content_version: "1",
      config: { text: "AB", format: "plain", format_version: 1 },
    },
  ];
  const initial = await computeCandidate("initialize", req),
    doc = new Y.Doc(),
    late = new Y.Doc();
  Y.applyUpdate(doc, Buffer.from(initial.state, "base64"));
  Y.applyUpdate(late, Buffer.from(initial.state, "base64"));
  const manager = canvasUndoManager(doc),
    before = readCanvasDocument(doc);
  applyCanvasChanges(doc, before, {
    ...before,
    graph: { ...before.graph, nodes: [] },
  });
  const prepare = async (base, d, seq, extra = {}) => {
    const bytes = Y.encodeStateAsUpdate(d);
    return computeCandidate("prepare", {
      ...req,
      state: base.state,
      document: base.projection,
      commitSeq: seq,
      updateId: randomUUID(),
      update: Buffer.from(bytes).toString("base64"),
      payloadHash: digest(bytes),
      ...extra,
    });
  };
  const deleted = await prepare(initial, doc, "0");
  applyCanvasHistory(doc, manager, "undo", {
    epoch: req.epoch,
    commitSeq: "1",
  });
  const restored = await prepare(deleted, doc, "1", {
    restorationNodes: req.document.nodes,
  });
  assert.deepEqual(restored.projection.nodes, req.document.nodes);
  nodeTextBinding(late, id).fragment.get(0).get(0).insert(1, "late");
  await assert.rejects(
    prepare(restored, late, "2"),
    (e) => e.code === "TARGET_DELETED",
  );
  manager.destroy();
  doc.destroy();
  late.destroy();
});

test("actual editor node-reference labels survive concurrent connections and removals", async () => {
  const req = request(),
    target = randomUUID(),
    x = randomUUID(),
    y = randomUUID();
  req.document.nodes = [target, x, y].map((id) => ({
    id,
    node_type: "text",
    node_action: "resource",
    title: id,
    x: 0,
    y: 0,
    z_index: 0,
    parent_id: null,
    config: { text: "AB", format: "plain", format_version: 1 },
  }));
  const initial = await computeCandidate("initialize", req);
  const changes = [x, y].map((source) => {
    const doc = new Y.Doc();
    Y.applyUpdate(doc, Buffer.from(initial.state, "base64"));
    const before = readCanvasDocument(doc),
      after = structuredClone(before),
      id = randomUUID();
    const targetNode = after.graph.nodes.find((n) => n.id === target);
    targetNode.parameters = { mode: "GENERATE" };
    targetNode.native.config.editor = { schemaVersion: 1, metadata: {} };
    after.graph.connections.push({
      id,
      fromNodeId: source,
      toNodeId: target,
      native: {
        id,
        edge_type: "annotation",
        source_node_id: source,
        target_node_id: target,
      },
    });
    applyCanvasChanges(doc, before, after);
    const update = Y.encodeStateAsUpdate(doc);
    doc.destroy();
    return {
      updateId: randomUUID(),
      update: Buffer.from(update).toString("base64"),
      payloadHash: digest(update),
    };
  });
  const a = await computeCandidate("prepare", {
    ...req,
    state: initial.state,
    ...changes[0],
  });
  const b = await computeCandidate("prepare", {
    ...req,
    state: a.state,
    document: a.projection,
    commitSeq: "1",
    ...changes[1],
  });
  const labels = b.projection.nodes.find((n) => n.id === target).config.editor
    .metadata.referenceLabels;
  assert.equal(labels[x].ordinal, 1);
  assert.equal(labels[y].ordinal, 2);
  const doc = new Y.Doc();
  Y.applyUpdate(doc, Buffer.from(b.state, "base64"));
  const before = readCanvasDocument(doc),
    after = structuredClone(before);
  after.graph.connections = [];
  applyCanvasChanges(doc, before, after);
  const update = Y.encodeStateAsUpdate(doc);
  const c = await computeCandidate("prepare", {
    ...req,
    state: b.state,
    document: b.projection,
    commitSeq: "2",
    updateId: randomUUID(),
    update: Buffer.from(update).toString("base64"),
    payloadHash: digest(update),
  });
  assert.deepEqual(
    c.projection.nodes.find((n) => n.id === target).config.editor.metadata
      .referenceLabels,
    labels,
  );
  doc.destroy();
});

test("clients cannot mint reference ordinals without an accepted connection", async () => {
  const req = request(),
    id = randomUUID(),
    source = randomUUID();
  req.document.nodes = [
    {
      id,
      node_type: "text",
      node_action: "resource",
      title: "T",
      x: 0,
      y: 0,
      z_index: 0,
      parent_id: null,
      config: { text: "AB", format: "plain", format_version: 1 },
    },
  ];
  const initial = await computeCandidate("initialize", req),
    doc = new Y.Doc();
  Y.applyUpdate(doc, Buffer.from(initial.state, "base64"));
  doc
    .getMap("canvas")
    .get("nodes")
    .get(id)
    .get("data")
    .set("referenceLabels", { [source]: { kind: "text", ordinal: 999 } });
  const bytes = Y.encodeStateAsUpdate(doc);
  const accepted = await computeCandidate("prepare", {
    ...req,
    state: initial.state,
    updateId: randomUUID(),
    update: Buffer.from(bytes).toString("base64"),
    payloadHash: digest(bytes),
  });
  assert.deepEqual(
    accepted.projection.nodes[0].config.editor?.metadata?.referenceLabels ?? {},
    {},
  );
  doc.destroy();
});
