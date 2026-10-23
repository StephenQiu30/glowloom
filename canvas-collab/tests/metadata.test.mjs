import assert from "node:assert/strict";
import test from "node:test";
import * as Y from "yjs";
import { prepareTitle } from "../src/metadata.mjs";
import { createCanvasDocument, readCanvasDocument } from "../src/model.mjs";
import { canonicalGraphJson } from "../src/canonical.mjs";
import { digest } from "../src/admission.mjs";

test("resource rename prepares deterministic same-epoch increments without seeding or replacing nodes", () => {
  const doc = createCanvasDocument({
    title: "old",
    graph: {
      schemaVersion: 2,
      nodes: [
        {
          id: "n",
          type: "text",
          nodeTypeVersion: 1,
          title: "text",
          position: { x: 0, y: 0 },
          width: 200,
          height: 100,
        },
      ],
      connections: [],
    },
  });
  const peer = new Y.Doc();
  const checkpoint = Y.encodeStateAsUpdate(doc);
  const state = {
    epoch: crypto.randomUUID(),
    modelVersion: 1,
    lifecycle: "ACTIVE",
    commitSeq: "0",
    checkpointSeq: "0",
    checkpoint: Buffer.from(checkpoint).toString("base64"),
    checkpointHash: digest(checkpoint),
    graphHash: digest(canonicalGraphJson(readCanvasDocument(doc).graph)),
    tail: [],
  };
  try {
    const command = { commandId: crypto.randomUUID(), title: "new", state };
    const first = prepareTitle(command),
      second = prepareTitle(command);
    assert.deepEqual(first, second);
    assert.equal(first.epoch, state.epoch);
    assert.equal(first.expectedCommitSeq, "0");
    assert.equal(first.effect, "VISIBLE_CHANGE");
    Y.applyUpdate(peer, checkpoint);
    peer
      .getMap("canvas")
      .get("nodes")
      .get("n")
      .get("data")
      .set("title", "another window");
    Y.applyUpdate(peer, Buffer.from(first.update, "base64"));
    assert.equal(readCanvasDocument(peer).title, "new");
    assert.equal(
      readCanvasDocument(peer).graph.nodes[0].title,
      "another window",
    );
    Y.applyUpdate(doc, Buffer.from(first.update, "base64"));
    assert.deepEqual(
      Buffer.from(Y.encodeStateAsUpdate(doc)),
      Buffer.from(first.state, "base64"),
    );
    assert.equal(
      prepareTitle({ ...command, title: "old" }).effect,
      "NO_CHANGE",
    );
    assert.throws(
      () =>
        prepareTitle({
          ...command,
          state: { ...state, checkpointHash: "0".repeat(64) },
        }),
      /CORRUPT_CHECKPOINT/,
    );
  } finally {
    doc.destroy();
    peer.destroy();
  }
});
