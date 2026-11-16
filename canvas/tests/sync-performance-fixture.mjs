// Deterministic client workload; generation is outside measured acceptance time.
import * as Y from "yjs";
import { createHash } from "node:crypto";
import {
  applyCanvasChanges,
  nodeTextBinding,
  readCanvasDocument,
} from "../dist/document/model.js";

const chunks = [];
for await (const chunk of process.stdin) chunks.push(chunk);
const { state, nodeId, mode, count, modes } = JSON.parse(
  Buffer.concat(chunks).toString(),
);
const doc = new Y.Doc();
Y.applyUpdate(doc, Buffer.from(state, "base64"));
doc.clientID = 42;
const updates = [];
doc.on("update", (update) => {
  updates.push({
    update: Buffer.from(update).toString("base64"),
    payloadHash: createHash("sha256").update(update).digest("hex"),
  });
});
for (const action of modes ?? Array(count).fill(mode)) {
  if (action === "causal") {
    const text = nodeTextBinding(doc, nodeId).fragment.get(0).get(0);
    doc.transact(() => {
      text.insert(1, "测");
      text.delete(1, 1);
    });
  } else if (action === "text") {
    nodeTextBinding(doc, nodeId).fragment.get(0).get(0).insert(1, "测");
  } else {
    const before = readCanvasDocument(doc);
    const after = structuredClone(before);
    after.graph.nodes.find((node) => node.id === nodeId).position.x += 1;
    applyCanvasChanges(doc, before, after);
  }
}
process.stdout.write(JSON.stringify(updates));
doc.destroy();
