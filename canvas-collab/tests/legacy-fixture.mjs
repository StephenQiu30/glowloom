// Isolated acceptance helper: reproduce the experimental missing-container model.
import * as Y from "yjs";
import { digest } from "../dist/admission.js";
import { canonicalJson } from "../dist/canonical.js";
import { readCanvasDocument, applyCanvasChanges } from "../dist/model.js";
const chunks = [];
for await (const chunk of process.stdin) chunks.push(chunk);
const input = JSON.parse(Buffer.concat(chunks)),
  doc = new Y.Doc();
Y.applyUpdate(doc, Buffer.from(input.state, "base64"));
const before = readCanvasDocument(doc);
if (input.legacy) {
  for (const n of doc.getMap("canvas").get("nodes").values())
    if (n.get("parameters")?.get("present") === false) n.delete("parameters");
  if (canonicalJson(before) !== canonicalJson(readCanvasDocument(doc)))
    throw Error("legacy reproduction changed projection");
  process.stdout.write(
    JSON.stringify({
      state: Buffer.from(Y.encodeStateAsUpdate(doc)).toString("base64"),
      graphHash: digest(canonicalJson(before)),
    }),
  );
} else {
  const updates = [];
  doc.on("update", (u) => updates.push(u));
  const after = structuredClone(before),
    node = after.graph.nodes.find((n) => n.id === input.nodeId);
  node.parameters = { mode: "MANUAL", values: { [input.key]: input.value } };
  applyCanvasChanges(doc, before, after);
  const bytes = Y.mergeUpdates(updates);
  process.stdout.write(
    JSON.stringify({
      update: Buffer.from(bytes).toString("base64"),
      payloadHash: digest(bytes),
      snapshot: readCanvasDocument(doc),
    }),
  );
}
doc.destroy();
