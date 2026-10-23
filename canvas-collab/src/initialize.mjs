import * as Y from "yjs";
import { createCanvasDocument, readCanvasDocument } from "./model.mjs";
import { canonicalGraphJson, parseStoredGraph } from "./canonical.mjs";
import { digest } from "./admission.mjs";
import { base64 } from "./binary.mjs";
import { normalizeRichText } from "./rich-text-schema.mjs";

/** Pure initialization for explicit create/restore/maintenance commands, never a lazy connection migration. */
export function initializeDocument({ title, graphJson }) {
  if (
    typeof title !== "string" ||
    !title.length ||
    title.length > 100 ||
    title.trim() !== title ||
    typeof graphJson !== "string" ||
    Buffer.byteLength(graphJson) > 1024 * 1024
  )
    throw new Error("INVALID_INITIAL_DOCUMENT");
  const source = parseStoredGraph(graphJson),
    expected = structuredClone(source);
  for (const node of expected.nodes)
    if (node.textDocument)
      node.textDocument.content = normalizeRichText(node.textDocument.content);
  const doc = createCanvasDocument({ title, graph: source });
  try {
    const projection = readCanvasDocument(doc),
      update = Y.encodeStateAsUpdate(doc);
    if (canonicalGraphJson(expected) !== canonicalGraphJson(projection.graph))
      throw new Error("PROJECTION_MISMATCH");
    if (update.byteLength > 8 * 1024 * 1024) throw new Error("STATE_TOO_LARGE");
    return {
      update: base64(update),
      stateHash: digest(update),
      graphHash: digest(canonicalGraphJson(projection.graph)),
      graphJson: canonicalGraphJson(projection.graph),
      sourceGraphHash: digest(graphJson),
      title: projection.title,
      modelVersion: 1,
    };
  } finally {
    doc.destroy();
  }
}
