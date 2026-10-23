import assert from "node:assert/strict";
import test from "node:test";
import * as Y from "yjs";
import { createCheckpoint, restoreCheckpoint } from "../src/checkpoint.mjs";
import { change, documentFrom } from "./fixtures.mjs";

test("binary checkpoint preserves delete-only changes and later offline merge", () => {
  const doc = new Y.Doc();
  doc.getText("text").insert(0, "旧内容");
  const before = Y.encodeStateAsUpdate(doc),
    offline = documentFrom(before);
  const oldVector = Y.encodeStateVector(doc);
  doc.getText("text").delete(0, 1);
  assert.deepEqual(
    Y.encodeStateVector(doc),
    oldVector,
    "deletion does not advance the struct clock",
  );
  const checkpoint = createCheckpoint(doc, {
    epoch: "epoch-1",
    commitSeq: "8",
  });
  assert.ok(
    checkpoint?.update instanceof Uint8Array,
    "persist binary rather than a JSON projection",
  );
  const restored = restoreCheckpoint(checkpoint);
  assert.equal(restored.getText("text").toString(), "内容");
  const pending = change(offline, (d) => d.getText("text").insert(3, "＋离线"));
  Y.applyUpdate(restored, pending);
  Y.applyUpdate(doc, pending);
  assert.equal(restored.getText("text").toString(), "内容＋离线");
  assert.equal(
    restored.getText("text").toString(),
    doc.getText("text").toString(),
  );
  doc.destroy();
  offline.destroy();
  restored.destroy();
});

test("checkpoint corruption is detected rather than silently opening an empty document", () => {
  const doc = new Y.Doc(),
    checkpoint = createCheckpoint(doc, { epoch: "e", commitSeq: "0" });
  assert.ok(checkpoint);
  assert.throws(
    () => restoreCheckpoint({ ...checkpoint, update: new Uint8Array([255]) }),
    { code: "CORRUPT_CHECKPOINT" },
  );
  doc.destroy();
});
