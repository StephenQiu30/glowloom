import assert from "node:assert/strict";
import test from "node:test";
import * as Y from "yjs";
import { validateCanvasRoots } from "../src/canvas-roots.mjs";
import { AdmissionGate } from "../src/admission.mjs";
import { createCanvasDocument, readCanvasDocument } from "../src/model.mjs";
import { MemoryStore, scope } from "./fixtures.mjs";

const empty = () =>
  createCanvasDocument({
    title: "canvas",
    graph: { schemaVersion: 2, nodes: [], connections: [] },
  });
function migration(doc) {
  const client = doc.clientID,
    clock = Y.decodeStateVector(Y.encodeStateVector(doc)).get(client) - 1;
  const name = `__tiptap_position_migrations:item:${client}:${clock}`;
  const entry = {
    version: 1,
    client,
    clock,
    seen: [[client, clock]],
    transfers: [{ source: { client, clock, length: 1 }, target: null }],
  };
  doc.getMap(name).set(`${client}:${clock}`, entry);
  return { name, key: `${client}:${clock}`, entry };
}

test("metadata schema, root names, nested shared types and sequence payloads remain admission failures", async () => {
  const cases = [
    ["unknown root", (doc) => doc.getMap("other").set("x", 1), "INVALID_ROOT"],
    [
      "prefix is not permission",
      (doc) => doc.getMap("__tiptap_position_migrations:other").set("x", 1),
      "INVALID_ROOT",
    ],
    [
      "noncanonical identity",
      (doc) => doc.getMap("__tiptap_position_migrations:item:01:0").set("x", 1),
      "INVALID_ROOT",
    ],
    [
      "unexpected business fields",
      (doc, m) => doc.getMap(m.name).set(m.key, { ...m.entry, nodes: [] }),
    ],
    [
      "nested shared value",
      (doc, m) => doc.getMap(m.name).set(m.key, new Y.Map()),
    ],
    ["wrong record key", (doc, m) => doc.getMap(m.name).set("wrong", m.entry)],
    [
      "unknown causal client",
      (doc, m) =>
        doc
          .getMap(m.name)
          .set(m.key, { ...m.entry, seen: [[Number.MAX_SAFE_INTEGER, 1]] }),
    ],
    [
      "unbounded metadata",
      (doc, m) =>
        doc.getMap(m.name).set(m.key, {
          ...m.entry,
          transfers: Array(20001).fill(m.entry.transfers[0]),
        }),
    ],
    [
      "hidden sequence",
      (doc, m) => {
        const sequence = new Y.Doc();
        try {
          sequence.getArray(m.name).push(["payload"]);
          Y.applyUpdate(doc, Y.encodeStateAsUpdate(sequence));
        } finally {
          sequence.destroy();
        }
      },
    ],
  ];
  for (const [label, mutate, expected = "INVALID_EDITOR_METADATA"] of cases) {
    const initial = empty(),
      store = new MemoryStore(initial),
      doc = new Y.Doc();
    try {
      Y.applyUpdate(doc, Y.encodeStateAsUpdate(initial));
      // A causal item is needed to name a legitimate fragment identity.
      doc.getMap("canvas").set("title", "changed");
      const m = migration(doc);
      mutate(doc, m);
      const gate = new AdmissionGate({
        store,
        project: readCanvasDocument,
        validate: () => {},
        authorize: () => true,
        validateRoots: validateCanvasRoots,
      });
      await assert.rejects(
        gate.submit(scope, {
          epoch: scope.epoch,
          updateId: crypto.randomUUID(),
          update: Y.encodeStateAsUpdate(doc),
        }),
        { code: expected },
        label,
      );
      assert.equal(store.commits.length, 0, label);
      assert.deepEqual(readCanvasDocument(initial).graph, {
        schemaVersion: 2,
        nodes: [],
        connections: [],
      });
    } finally {
      doc.destroy();
      initial.destroy();
    }
  }
});
