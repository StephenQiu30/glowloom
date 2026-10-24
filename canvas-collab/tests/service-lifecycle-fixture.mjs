import * as Y from "yjs";
import { digest } from "../src/admission.mjs";
import {
  readCanvasDocument,
  applyCanvasChanges,
  canvasUndoManager,
  applyCanvasHistory,
} from "../src/model.mjs";
const chunks = [];
for await (const c of process.stdin) chunks.push(c);
const input = JSON.parse(Buffer.concat(chunks)),
  doc = new Y.Doc();
Y.applyUpdate(doc, Buffer.from(input.state, "base64"));
const manager = canvasUndoManager(doc),
  before = readCanvasDocument(doc),
  updates = [];
doc.on("update", (u) => updates.push(u));
const encode = () => {
  const update = Y.mergeUpdates(updates);
  updates.length = 0;
  return {
    update: Buffer.from(update).toString("base64"),
    payloadHash: digest(update),
  };
};
applyCanvasChanges(doc, before, {
  ...before,
  graph: { ...before.graph, nodes: [], connections: [] },
});
const deletion = encode();
applyCanvasHistory(doc, manager, "undo", {
  epoch: input.epoch,
  commitSeq: String(BigInt(input.commitSeq) + 1n),
});
const restoration = encode();
process.stdout.write(JSON.stringify({ deletion, restoration }));
manager.destroy();
doc.destroy();
