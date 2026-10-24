// Stdin/stdout harness used only by the Go isolated-database integration tests.
import * as Y from "yjs";
import { createHash } from "node:crypto";
import {
  nodeTextBinding,
  readCanvasDocument,
  applyCanvasChanges,
} from "../src/model.mjs";
const chunks = [];
for await (const chunk of process.stdin) chunks.push(chunk);
const { state, nodeId, text, evil, edge, title, reference } = JSON.parse(
  Buffer.concat(chunks).toString(),
);
const doc = new Y.Doc();
Y.applyUpdate(doc, Buffer.from(state, "base64"));
const updates = [];
doc.on("update", (value) => updates.push(value));
if (reference) {
  const before = readCanvasDocument(doc),
    after = structuredClone(before),
    node = after.graph.nodes.find((n) => n.id === nodeId);
  node.references ??= [];
  node.references.push({ ...reference, order: node.references.length });
  node.labelState ??= {
    highWater: { image: 0, video: 0, audio: 0, subject: 0 },
    allocations: {},
  };
  const kind = reference.mediaKind;
  node.labelState.allocations[reference.id] = {
    labelKind: kind,
    ordinal: ++node.labelState.highWater[kind],
  };
  node.native.config.composer ??= {
    schemaVersion: 1,
    modelVersion: null,
    mode: null,
    params: {},
  };
  applyCanvasChanges(doc, before, after);
  const mention = new Y.XmlElement("reference");
  mention.setAttribute("referenceId", reference.id);
  const paragraph = nodeTextBinding(doc, nodeId).fragment.get(0);
  paragraph.insert(paragraph.length, [mention]);
} else if (edge) {
  const before = readCanvasDocument(doc);
  applyCanvasChanges(doc, before, {
    ...before,
    graph: {
      ...before.graph,
      connections: [
        ...before.graph.connections,
        {
          id: edge.id,
          fromNodeId: edge.source_node_id,
          toNodeId: edge.target_node_id,
          native: edge,
        },
      ],
    },
  });
} else if (title) {
  const before = readCanvasDocument(doc);
  applyCanvasChanges(doc, before, { ...before, title });
} else if (evil) doc.getMap("unregistered").set("value", true);
else if (text)
  nodeTextBinding(doc, nodeId).fragment.get(0).get(0).insert(1, text);
const update = Y.mergeUpdates(updates);
process.stdout.write(
  JSON.stringify({
    update: Buffer.from(update).toString("base64"),
    payloadHash: createHash("sha256").update(update).digest("hex"),
    snapshot: readCanvasDocument(doc),
  }),
);
doc.destroy();
