import * as Y from "yjs";
import { digest } from "./admission.mjs";
import { unbase64 } from "./binary.mjs";
import { canonicalGraphJson } from "./canonical.mjs";
import { readCanvasDocument } from "./model.mjs";
import { validateCanvasRoots } from "./canvas-roots.mjs";

const fail = (code) => Object.assign(new Error(code), { code });

/** Rebuilds only verified durable state. The caller owns and destroys the returned document. */
export function committedDocument(state) {
  if (state.modelVersion !== 1) throw fail("INVALID_CANVAS_MODEL");
  const checkpoint = unbase64(state.checkpoint);
  if (checkpoint.byteLength > 8 * 1024 * 1024) throw fail("STATE_TOO_LARGE");
  if (digest(checkpoint) !== state.checkpointHash)
    throw fail("CORRUPT_CHECKPOINT");
  const doc = new Y.Doc();
  try {
    Y.applyUpdate(doc, checkpoint);
    let sequence = BigInt(state.checkpointSeq);
    for (const update of state.tail) {
      if (BigInt(update.commitSeq) !== ++sequence) throw fail("COMMIT_GAP");
      const bytes = unbase64(update.update);
      if (bytes.byteLength > 1024 * 1024) throw fail("UPDATE_TOO_LARGE");
      if (digest(bytes) !== update.payloadHash) throw fail("CORRUPT_UPDATE");
      Y.applyUpdate(doc, bytes);
    }
    if (sequence !== BigInt(state.commitSeq)) throw fail("COMMIT_GAP");
    if (Y.encodeStateAsUpdate(doc).byteLength > 8 * 1024 * 1024)
      throw fail("STATE_TOO_LARGE");
    validateCanvasRoots(doc);
    if (
      digest(canonicalGraphJson(readCanvasDocument(doc).graph)) !==
      state.graphHash
    )
      throw fail("PROJECTION_MISMATCH");
    return doc;
  } catch (error) {
    doc.destroy();
    throw error;
  }
}
