import assert from "node:assert/strict";
import test from "node:test";
import * as Y from "yjs";
import { createCanvasDocument, readCanvasDocument, nodeTextBinding, applyCanvasChanges } from "../src/model.mjs";
import { canonicalGraphJson, parseStoredGraph } from "../src/canonical.mjs";

const id = "10000000-0000-4000-8000-000000000001";
const modelID = "20000000-0000-4000-8000-000000000001";
const seed = () => ({ title: "引用协作迁入", graph: { schemaVersion: 2, nodes: [{
  id, type: "text", nodeTypeVersion: 1, title: "正文", position: { x: 0, y: 0 }, width: 320, height: 320,
  references: [{ id: "image-one", source: { kind: "media", sourceId: id, sourceVersion: "1" }, bindingGeneration: 1, mediaKind: "image", role: "reference_image", order: 0 }],
  labelState: { highWater: { image: 3, video: 0, audio: 0, subject: 0 }, allocations: { "image-one": { labelKind: "image", ordinal: 3 } } },
  textDocument: { schemaVersion: 1, content: { type: "doc", content: [{ type: "paragraph", content: [
    { type: "text", text: "参考😀", marks: [{ type: "bold" }] },
    { type: "reference", attrs: { referenceId: "image-one" } },
    { type: "reference", attrs: { referenceId: "image-one" } },
  ] }] } },
}], connections: [] } });

test("Glowloom reference atoms and stable labels survive three peers, reordered replay and binary reopen", () => {
  const initial = seed();
  const source = createCanvasDocument(initial);
  const a = new Y.Doc(), b = new Y.Doc(), c = new Y.Doc(), reopened = new Y.Doc();
  try {
    const baseline = Y.encodeStateAsUpdate(source);
    for (const peer of [a,b,c]) Y.applyUpdate(peer, baseline);
    const textA = nodeTextBinding(a, id).fragment.get(0).get(0);
    const textB = nodeTextBinding(b, id).fragment.get(0).get(0);
    textA.insert(0, "甲"); textB.insert(0, "乙");
    const before = readCanvasDocument(c), after = structuredClone(before);
    after.graph.nodes[0].position = { x: 50, y: 70 };
    applyCanvasChanges(c, before, after);
    const updates = [a,b,c].map(peer => Y.encodeStateAsUpdate(peer, Y.encodeStateVector(source)));
    for (const peer of [a,b,c]) for (const update of [...updates].reverse()) { Y.applyUpdate(peer, update); Y.applyUpdate(peer, update); }
    assert.deepEqual(readCanvasDocument(a), readCanvasDocument(b));
    assert.deepEqual(readCanvasDocument(b), readCanvasDocument(c));
    Y.applyUpdate(reopened, Y.encodeStateAsUpdate(a));
    const node = readCanvasDocument(reopened).graph.nodes[0];
    assert.deepEqual(node.position, { x: 50, y: 70 });
    assert.deepEqual(node.references, initial.graph.nodes[0].references);
    assert.deepEqual(node.labelState, initial.graph.nodes[0].labelState);
    const content = node.textDocument.content.content[0].content;
    assert.equal(content.filter(n => n.type === "reference" && n.attrs.referenceId === "image-one").length, 2);
    const text = content.filter(n => n.type === "text").map(n=>n.text).join("");
    for (const value of ["甲", "乙", "参考😀"]) assert.ok(text.includes(value));
    assert.ok(content.some(n => n.marks?.some(mark=>mark.type === "bold")));
    assert.deepEqual(readCanvasDocument(reopened), readCanvasDocument(a));
  } finally { for (const doc of [source,a,b,c,reopened]) doc.destroy(); }
});

test("Glowloom model UUIDs retain their string identity in canonical storage", () => {
  const value = seed().graph;
  value.nodes[0].parameters = { modelSelection: { connectionId: modelID } };
  const stored = canonicalGraphJson(value);
  assert.deepEqual(parseStoredGraph(stored), value);
  assert.match(stored, new RegExp('"connectionId":"' + modelID + '"'));
});
