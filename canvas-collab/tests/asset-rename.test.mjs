import assert from "node:assert/strict";
import test from "node:test";
import { randomUUID } from "node:crypto";
import * as Y from "yjs";
import { computeCandidate } from "../src/service-runtime.mjs";
import { readCanvasDocument } from "../src/model.mjs";

test("asset rename uses a title delta without resetting graph or existing Yjs identity", async () => {
  const document = {
    id: randomUUID(),
    project_id: randomUUID(),
    name: "旧名称",
    schema_version: 1,
    operation_version: 1,
    revision: "1",
    permissions: { read: true, write: true },
    nodes: [],
    edges: [],
  };
  const request = {
    documentId: document.id,
    epoch: randomUUID(),
    writerTerm: "1",
    commitSeq: "0",
    document,
  };
  const initial = await computeCandidate("initialize", request);
  const renameRequest = {
    ...request,
    state: initial.state,
    updateId: randomUUID(),
    name: "资产新名称",
  };
  const renamed = await computeCandidate("rename", renameRequest);
  assert.deepEqual(await computeCandidate("rename", renameRequest), renamed);
  assert.equal(renamed.projection.name, "资产新名称");
  assert.equal(renamed.epoch, initial.epoch);
  assert.deepEqual(renamed.projection.nodes, initial.projection.nodes);
  assert.deepEqual(renamed.projection.edges, initial.projection.edges);
  assert.equal(renamed.effect, "VISIBLE_CHANGE");
  const doc = new Y.Doc();
  try {
    Y.applyUpdate(doc, Buffer.from(initial.state, "base64"));
    const originalClocks = Y.decodeStateVector(Y.encodeStateVector(doc));
    Y.applyUpdate(doc, Buffer.from(renamed.appliedUpdate, "base64"));
    assert.equal(readCanvasDocument(doc).title, "资产新名称");
    const nextClocks = Y.decodeStateVector(Y.encodeStateVector(doc));
    for (const [client, clock] of originalClocks)
      assert.ok(nextClocks.get(client) >= clock);
    assert.deepEqual(readCanvasDocument(doc).graph, {
      ...readCanvasDocument(doc).graph,
      nodes: [],
      connections: [],
    });
  } finally {
    doc.destroy();
  }
  for (const name of ["", " x", "x ", "x".repeat(51)])
    await assert.rejects(
      () =>
        computeCandidate("rename", {
          ...request,
          state: initial.state,
          updateId: randomUUID(),
          name,
        }),
      /INVALID_RENAME/,
    );
});
