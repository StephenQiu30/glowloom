import assert from "node:assert/strict";
import test from "node:test";
import * as Y from "yjs";
import {
  serviceSnapshot,
  serviceProjection,
} from "../src/service-document.mjs";
import {
  createCanvasDocument,
  canvasGraphHash,
  readCanvasDocument,
  nodeTextBinding,
  applyCanvasChanges,
} from "../src/model.mjs";
const source = () => ({
  id: "00000000-0000-4000-8000-000000000001",
  project_id: "00000000-0000-4000-8000-000000000002",
  name: "协作",
  revision: "9007199254740994",
  schema_version: 1,
  operation_version: 1,
  permissions: { read: true, write: true },
  scope: { type: "independent" },
  nodes: [
    {
      id: "00000000-0000-4000-8000-000000000003",
      node_type: "text",
      node_action: "manual",
      title: "正文",
      x: 10,
      y: 20,
      width: 300,
      height: 200,
      parent_id: null,
      z_index: 1,
      content_version: "8",
      config: {
        text: "你好🌏",
        format: "plain",
        format_version: 1,
        editor: {
          version: 1,
          parameters: { mode: "MANUAL", values: { seed: 12, strength: 0.4 } },
          metadata: { fontSize: 20 },
          files: [
            {
              fileId: "asset",
              referenceKey: "asset",
              version: "2",
              purpose: "CONTENT",
            },
          ],
        },
      },
    },
  ],
  edges: [],
});
test("native DTO round trip preserves fields and server-only revision without numeric conversion", () => {
  const dto = source();
  const d = createCanvasDocument(serviceSnapshot(dto));
  try {
    assert.deepEqual(serviceProjection(readCanvasDocument(d), dto), dto);
  } finally {
    d.destroy();
  }
});
test("two users share one paragraph, independent parameter keys, and a third page reloads accepted bytes", () => {
  const dto = source();
  const initial = createCanvasDocument(serviceSnapshot(dto));
  const bytes = Y.encodeStateAsUpdate(initial);
  const a = new Y.Doc(),
    b = new Y.Doc(),
    c = new Y.Doc();
  try {
    Y.applyUpdate(a, bytes);
    Y.applyUpdate(b, bytes);
    const ta = nodeTextBinding(a, dto.nodes[0].id).fragment.get(0).get(0);
    const tb = nodeTextBinding(b, dto.nodes[0].id).fragment.get(0).get(0);
    ta.insert(2, "甲");
    tb.insert(2, "乙");
    const pa = readCanvasDocument(a),
      pb = readCanvasDocument(b),
      na = structuredClone(pa),
      nb = structuredClone(pb);
    na.graph.nodes[0].parameters.values.seed = 99;
    nb.graph.nodes[0].parameters.values.strength = 0.9;
    applyCanvasChanges(a, pa, na);
    applyCanvasChanges(b, pb, nb);
    const au = Y.encodeStateAsUpdate(a),
      bu = Y.encodeStateAsUpdate(b);
    Y.applyUpdate(a, bu);
    Y.applyUpdate(b, au);
    Y.applyUpdate(c, Y.encodeStateAsUpdate(a));
    assert.deepEqual(readCanvasDocument(a), readCanvasDocument(b));
    assert.deepEqual(readCanvasDocument(a), readCanvasDocument(c));
    const projected = serviceProjection(readCanvasDocument(c), dto);
    assert.deepEqual(projected.nodes[0].config.editor.parameters.values, {
      seed: 99,
      strength: 0.9,
    });
    assert.match(JSON.stringify(projected.nodes[0].config.rich_text), /甲/);
    assert.match(JSON.stringify(projected.nodes[0].config.rich_text), /乙/);
    assert.equal(
      projected.nodes[0].config.composer.labelState.highWater.image,
      0,
    );
  } finally {
    for (const d of [initial, a, b, c]) d.destroy();
  }
});
test("unknown DTO node/config fields survive mapping so Go can reject them rather than silently dropping data", () => {
  const dto = source();
  dto.nodes[0].config.future = { value: 42 };
  dto.nodes[0].future = "kept";
  assert.deepEqual(serviceProjection(serviceSnapshot(dto), dto), dto);
});

test("editor optional parameter fields never enter shared canonical JSON as undefined", () => {
  const dto = source();
  dto.nodes[0].config.editor = {
    schemaVersion: 1,
    parameters: { mode: "GENERATE", modelSelection: undefined },
    metadata: {},
  };
  dto.edges.push({
    id: "00000000-0000-4000-8000-000000000004",
    source_node_id: dto.nodes[0].id,
    target_node_id: "00000000-0000-4000-8000-000000000005",
    edge_type: "annotation",
    binding: { schemaVersion: 1, order: 0, optional: undefined },
  });
  const snapshot = serviceSnapshot(dto);
  const doc = createCanvasDocument(snapshot);
  assert.doesNotThrow(() => canvasGraphHash(readCanvasDocument(doc).graph));
  assert.equal(
    Object.hasOwn(snapshot.graph.nodes[0].parameters, "modelSelection"),
    false,
  );
  assert.equal(
    Object.hasOwn(snapshot.graph.connections[0].native.binding, "optional"),
    false,
  );
  doc.destroy();
});

test("reprojecting an accepted rich-text state preserves its derived composer for restoration", () => {
  const dto = source(),
    doc = createCanvasDocument(serviceSnapshot(dto));
  nodeTextBinding(doc, dto.nodes[0].id)
    .fragment.get(0)
    .get(0)
    .insert(0, "新正文");
  const snapshot = readCanvasDocument(doc),
    accepted = serviceProjection(snapshot, dto);
  assert.ok(accepted.nodes[0].config.composer);
  assert.deepEqual(serviceProjection(snapshot, accepted), accepted);
  doc.destroy();
});

test("graph edits after plain-text promotion retain the derived composer registry", () => {
  const dto = source(),
    doc = createCanvasDocument(serviceSnapshot(dto));
  try {
    nodeTextBinding(doc, dto.nodes[0].id)
      .fragment.get(0)
      .get(0)
      .insert(0, "中文");
    const accepted = serviceProjection(readCanvasDocument(doc), dto);
    const before = serviceSnapshot(accepted),
      after = structuredClone(before);
    // The editor serializes a composer shell on its first graph edit. Its
    // unchanged derived registry is absent from the original shared document.
    after.graph.nodes[0].native.config.composer.params.seed = 42;
    applyCanvasChanges(doc, before, after);
    const projected = serviceProjection(readCanvasDocument(doc), accepted);
    assert.deepEqual(projected.nodes[0].config.composer.references, []);
    assert.deepEqual(
      projected.nodes[0].config.composer.labelState,
      accepted.nodes[0].config.composer.labelState,
    );
    assert.equal(projected.nodes[0].config.composer.params.seed, 42);
    nodeTextBinding(doc, dto.nodes[0].id)
      .fragment.get(0)
      .get(0)
      .insert(0, "再次");
    const edited = serviceProjection(readCanvasDocument(doc), projected);
    assert.deepEqual(edited.nodes[0].config.composer.references, []);
    assert.deepEqual(
      edited.nodes[0].config.composer.labelState,
      accepted.nodes[0].config.composer.labelState,
    );
  } finally {
    doc.destroy();
  }
});
