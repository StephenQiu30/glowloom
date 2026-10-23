import * as Y from "yjs";
import { digest } from "./admission.mjs";

export function createCheckpoint(doc, { epoch, commitSeq }) {
  const update = Y.encodeStateAsUpdate(doc);
  return { format: "yjs-v1", epoch, commitSeq, update, hash: digest(update) };
}

export function restoreCheckpoint(checkpoint) {
  const fail = () =>
    Object.assign(new Error("Invalid binary checkpoint"), {
      code: "CORRUPT_CHECKPOINT",
    });
  if (
    checkpoint.format !== "yjs-v1" ||
    !(checkpoint.update instanceof Uint8Array) ||
    checkpoint.hash !== digest(checkpoint.update)
  )
    throw fail();
  const doc = new Y.Doc();
  try {
    Y.applyUpdate(doc, checkpoint.update);
  } catch {
    doc.destroy();
    throw fail();
  }
  return doc;
}
