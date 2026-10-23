import * as Y from "yjs";
import { committedDocument } from "./committed-state.mjs";
import { digest } from "./admission.mjs";
import { canonicalGraphJson } from "./canonical.mjs";
import {
  readCanvasDocument,
  inspectCanvasDocument,
  validateCanvasTransition,
} from "./model.mjs";
import { base64 } from "./binary.mjs";

/** Pure preparation for an explicit Java-owned resource command. There is no database or broadcast here. */
export function prepareTitle({ state, title, commandId }) {
  if (
    typeof title !== "string" ||
    !title.length ||
    title.length > 100 ||
    title.trim() !== title ||
    typeof commandId !== "string" ||
    !/^[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}$/.test(commandId) ||
    state.lifecycle !== "ACTIVE"
  )
    throw new Error("INVALID_METADATA_COMMAND");
  const doc = committedDocument(state);
  try {
    const before = inspectCanvasDocument(doc),
      vector = Y.encodeStateVector(doc);
    const oldTitle = readCanvasDocument(doc).title;
    // A retry against exactly the same durable prefix must prepare exactly the same bytes.
    // Resolve the rare collision against every already allocated client, rather than relying on probability.
    const clients = Y.decodeStateVector(vector),
      limit = 2n ** 53n - 1n;
    let identity =
      (BigInt(
        "0x" +
          digest(`canvas-resource:${state.epoch}:${commandId}`).slice(0, 14),
      ) %
        limit) +
      1n;
    for (let attempt = 0; clients.has(Number(identity)); attempt++) {
      if (attempt >= 64) throw new Error("CLIENT_IDENTITY_BUDGET");
      identity = (identity % limit) + 1n;
    }
    doc.clientID = Number(identity);
    if (oldTitle !== title) doc.getMap("canvas").set("title", title);
    validateCanvasTransition(before, doc);
    const snapshot = readCanvasDocument(doc),
      update = Y.encodeStateAsUpdate(doc, vector),
      full = Y.encodeStateAsUpdate(doc);
    if (update.byteLength > 1024 * 1024 || full.byteLength > 8 * 1024 * 1024)
      throw new Error("STATE_TOO_LARGE");
    const graphJson = canonicalGraphJson(snapshot.graph);
    return {
      epoch: state.epoch,
      expectedCommitSeq: state.commitSeq,
      title: snapshot.title,
      graphJson,
      graphHash: digest(graphJson),
      update: base64(update),
      payloadHash: digest(update),
      state: base64(full),
      stateHash: digest(full),
      effect: oldTitle === title ? "NO_CHANGE" : "VISIBLE_CHANGE",
    };
  } finally {
    doc.destroy();
  }
}
