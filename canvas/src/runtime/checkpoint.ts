import * as Y from "yjs";
import { digest } from "./admission.js";

export function createCheckpoint(
  doc: Y.Doc,
  { epoch, commitSeq }: { epoch: string; commitSeq: string },
) {
  const update = Y.encodeStateAsUpdate(doc);
  return { format: "yjs-v1", epoch, commitSeq, update, hash: digest(update) };
}

export function restoreCheckpoint(checkpoint: {
  format?: string;
  update?: unknown;
  hash?: string;
}) {
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
