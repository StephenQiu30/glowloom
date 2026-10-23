import assert from "node:assert/strict";
import test from "node:test";
import { createCanvasDocument, readCanvasDocument } from "../src/model.mjs";
import * as model from "../src/model.mjs";
import * as Y from "yjs";
import { AdmissionGate } from "../src/admission.mjs";
import { change, MemoryStore, scope } from "./fixtures.mjs";

export const canvas = {
  title: "验收画布",
  graph: {
    schemaVersion: 2,
    nodes: [
      {
        id: "n1",
        type: "text",
        nodeTypeVersion: 1,
        title: "正文",
        position: { x: 10, y: 20 },
        width: 320,
        height: 180,
        zIndex: 0,
        parameters: { mode: "MANUAL" },
        textDocument: {
          schemaVersion: 1,
          content: {
            type: "doc",
            content: [
              {
                type: "paragraph",
                content: [{ type: "text", text: "保护正文" }],
              },
            ],
          },
        },
      },
      {
        id: "n2",
        type: "image",
        nodeTypeVersion: 1,
        title: "固定图片",
        position: { x: 500, y: 20 },
        width: 320,
        height: 240,
        zIndex: 1,
        parameters: { mode: "UPLOAD" },
        fileReferences: [
          {
            referenceKey: "image",
            fileId: "file-fixed",
            version: "version-fixed",
            purpose: "SOURCE",
          },
        ],
      },
    ],
    connections: [
      {
        id: "edge",
        fromNodeId: "n1",
        fromPortId: "text",
        toNodeId: "n2",
        toPortId: "prompt",
        order: 0,
        binding: { kind: "TEXT" },
      },
    ],
    viewport: { x: 3, y: 4, zoom: 0.77 },
    backgroundMode: "dots",
    showImageInfo: false,
  },
};

test("peer projection property order cannot manufacture a protected-field edit", () => {
  const source = {
    title: canvas.title,
    graph: {
      backgroundMode: "dots",
      schemaVersion: 2,
      showImageInfo: false,
      viewport: { x: 0, y: 0, zoom: 1 },
      nodes: canvas.graph.nodes,
      connections: canvas.graph.connections,
    },
  };
  const doc = createCanvasDocument(source);
  try {
    const before = readCanvasDocument(doc);
    const after = {
      title: before.title,
      graph: {
        schemaVersion: 2,
        nodes: before.graph.nodes,
        connections: before.graph.connections,
        viewport: before.graph.viewport,
        backgroundMode: before.graph.backgroundMode,
        showImageInfo: before.graph.showImageInfo,
      },
    };
    let updates = 0;
    doc.on("update", () => updates++);
    model.applyCanvasChanges(doc, before, after);
    assert.equal(updates, 0, "an unchanged peer projection emits no update");
    assert.deepEqual(readCanvasDocument(doc), before);
    after.graph.viewport = { ...after.graph.viewport, x: 90 };
    assert.throws(
      () => model.applyCanvasChanges(doc, before, after),
      /READ_ONLY_PROPERTY/,
    );
    assert.equal(updates, 0, "actual protected changes remain rejected");
  } finally {
    doc.destroy();
  }
});

test("explicit graph undo binds restored containers to the exact epoch and watermark without permitting ordinary replacement", () => {
  const doc = createCanvasDocument(canvas),
    undo = model.canvasUndoManager(doc);
  try {
    const before = model.inspectCanvasDocument(doc);
    model.applyCanvasChanges(doc, canvas, {
      ...canvas,
      graph: { ...canvas.graph, nodes: [], connections: [] },
    });
    const epoch = crypto.randomUUID();
    const restored = model.applyCanvasHistory(doc, undo, "undo", {
      epoch,
      commitSeq: "7",
    });
    assert.deepEqual(
      restored.nodes.toSorted(),
      canvas.graph.nodes.map((node) => node.id).toSorted(),
    );
    const intent = model.validateCanvasTransition(before, doc);
    assert.equal(intent.epoch, epoch);
    assert.equal(intent.sourceCommitSeq, "7");
    assert.deepEqual(readCanvasDocument(doc), canvas);
    doc.getMap("canvas").set("restoration", null);
    assert.throws(
      () => model.validateCanvasTransition(before, doc),
      /REPLACED_SHARED_TYPE/,
    );
  } finally {
    undo.destroy();
    doc.destroy();
  }
});

test("shared canvas round trip preserves stable identity, graph defaults, rich text and fixed references", () => {
  const doc = createCanvasDocument(canvas);
  try {
    assert.deepEqual(readCanvasDocument(doc), canvas);
  } finally {
    doc?.destroy();
  }
});

test("replacement of a shared collection cannot masquerade as a harmless identical graph", async () => {
  assert.equal(typeof model.inspectCanvasDocument, "function");
  const initial = createCanvasDocument(canvas),
    attacker = new Y.Doc();
  Y.applyUpdate(attacker, Y.encodeStateAsUpdate(initial));
  const store = new MemoryStore(initial);
  const engine = new AdmissionGate({
    store,
    allowedRoots: ["canvas"],
    project: readCanvasDocument,
    inspectDocument: model.inspectCanvasDocument,
    validateTransition: model.validateCanvasTransition,
    authorize: async () => true,
    validate: () => {},
  });
  try {
    const update = change(attacker, (d) => {
      const root = d.getMap("canvas");
      root.set("nodes", root.get("nodes").clone());
    });
    assert.deepEqual(
      readCanvasDocument(attacker),
      canvas,
      "visible content alone cannot detect this destructive replacement",
    );
    await assert.rejects(
      engine.submit(scope, {
        updateId: "replacement",
        epoch: scope.epoch,
        update,
      }),
      { code: "REPLACED_SHARED_TYPE" },
    );
    assert.equal(store.commits.length, 0);
  } finally {
    initial.destroy();
    attacker.destroy();
  }
});

test("different fields on the same node merge without replacing its shared record", () => {
  assert.equal(typeof model.applyCanvasChanges, "function");
  const original = createCanvasDocument(canvas),
    a = new Y.Doc(),
    b = new Y.Doc();
  Y.applyUpdate(a, Y.encodeStateAsUpdate(original));
  Y.applyUpdate(b, Y.encodeStateAsUpdate(original));
  try {
    const ma = structuredClone(canvas),
      mb = structuredClone(canvas);
    ma.graph.nodes[0].title = "新标题";
    mb.graph.nodes[0].position = { x: 90, y: 80 };
    model.applyCanvasChanges(a, canvas, ma);
    model.applyCanvasChanges(b, canvas, mb);
    const ua = Y.encodeStateAsUpdate(a),
      ub = Y.encodeStateAsUpdate(b);
    Y.applyUpdate(a, ub);
    Y.applyUpdate(b, ua);
    const expected = structuredClone(ma);
    expected.graph.nodes[0].position = { x: 90, y: 80 };
    assert.deepEqual(readCanvasDocument(a), expected);
    assert.deepEqual(readCanvasDocument(b), expected);
  } finally {
    original.destroy();
    a.destroy();
    b.destroy();
  }
});

test("independent ordinary parameter keys merge while model configuration stays an atomic value", () => {
  const source = structuredClone(canvas);
  source.graph.nodes[0].parameters = {
    mode: "MANUAL",
    values: { speed: 1, tone: "旧" },
  };
  const original = createCanvasDocument(source),
    a = new Y.Doc(),
    b = new Y.Doc();
  Y.applyUpdate(a, Y.encodeStateAsUpdate(original));
  Y.applyUpdate(b, Y.encodeStateAsUpdate(original));
  try {
    const ma = structuredClone(source),
      mb = structuredClone(source);
    ma.graph.nodes[0].parameters.values.speed = 2;
    mb.graph.nodes[0].parameters.values.tone = "新";
    model.applyCanvasChanges(a, source, ma);
    model.applyCanvasChanges(b, source, mb);
    const ua = Y.encodeStateAsUpdate(a),
      ub = Y.encodeStateAsUpdate(b);
    Y.applyUpdate(a, ub);
    Y.applyUpdate(b, ua);
    assert.deepEqual(readCanvasDocument(a).graph.nodes[0].parameters, {
      mode: "MANUAL",
      values: { speed: 2, tone: "新" },
    });
    assert.deepEqual(readCanvasDocument(a), readCanvasDocument(b));
  } finally {
    original.destroy();
    a.destroy();
    b.destroy();
  }
});

test("removing a parameter dictionary cannot hide another window's concurrently added key", () => {
  const source = structuredClone(canvas);
  source.graph.nodes[0].parameters.values = { old: 1 };
  const original = createCanvasDocument(source),
    a = new Y.Doc(),
    b = new Y.Doc();
  Y.applyUpdate(a, Y.encodeStateAsUpdate(original));
  Y.applyUpdate(b, Y.encodeStateAsUpdate(original));
  try {
    const ma = structuredClone(source),
      mb = structuredClone(source);
    delete ma.graph.nodes[0].parameters.values;
    mb.graph.nodes[0].parameters.values.new = 2;
    model.applyCanvasChanges(a, source, ma);
    model.applyCanvasChanges(b, source, mb);
    const ua = Y.encodeStateAsUpdate(a),
      ub = Y.encodeStateAsUpdate(b);
    Y.applyUpdate(a, ub);
    Y.applyUpdate(b, ua);
    assert.deepEqual(readCanvasDocument(a).graph.nodes[0].parameters.values, {
      new: 2,
    });
    assert.deepEqual(readCanvasDocument(a), readCanvasDocument(b));
  } finally {
    original.destroy();
    a.destroy();
    b.destroy();
  }
});

test("text is visible before an optional presence marker and cannot be hidden by metadata", () => {
  const doc = createCanvasDocument(canvas);
  try {
    const { fragment } = model.nodeTextBinding(doc, "n2");
    doc.transact(() => {
      const text = new Y.XmlText();
      fragment.get(0).insert(0, [text]);
      text.insert(0, "提示词");
    });
    assert.deepEqual(readCanvasDocument(doc).graph.nodes[1].textDocument, {
      schemaVersion: 1,
      content: {
        type: "doc",
        content: [
          { type: "paragraph", content: [{ type: "text", text: "提示词" }] },
        ],
      },
    });
  } finally {
    doc.destroy();
  }
});
