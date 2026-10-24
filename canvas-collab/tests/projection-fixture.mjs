// Test-only stdin/stdout bridge. Uses the production Yjs candidate runtime; no OT.
import * as Y from "yjs";
import { createHash } from "node:crypto";
import { computeCandidate } from "../src/service-runtime.mjs";
import { serviceSnapshot } from "../src/service-document.mjs";
import {
  readCanvasDocument,
  applyRecoveredCanvasChanges,
  canvasUndoManager,
  applyCanvasHistory,
} from "../src/model.mjs";
const chunks = [];
for await (const c of process.stdin) chunks.push(c);
const { action, input } = JSON.parse(Buffer.concat(chunks).toString());
try {
  let result;
  if (action === "update") {
    const doc = new Y.Doc();
    const decode = (s) => Buffer.from(s, "base64");
    if (input.undo) {
      Y.applyUpdate(doc, decode(input.undo.state));
      const manager = canvasUndoManager(doc);
      // The same tracked origin is used by the production editor.
      Y.applyUpdate(doc, decode(input.undo.update), "local-canvas");
      manager.stopCapturing();
      Y.applyUpdate(doc, decode(input.state.state), "remote");
      const vector = Y.encodeStateVector(doc);
      applyCanvasHistory(doc, manager, "undo", {
        epoch: input.state.epoch,
        commitSeq: input.state.commitSeq,
      });
      const update = Y.encodeStateAsUpdate(doc, vector);
      result = {
        update: Buffer.from(update).toString("base64"),
        payloadHash: createHash("sha256").update(update).digest("hex"),
      };
      manager.destroy();
    } else {
      Y.applyUpdate(doc, decode(input.state.state));
      const vector = Y.encodeStateVector(doc);
      // Semantic fixture replacement uses the real rich-text recovery binding.
      // Concurrent character-merge coverage uses independent Yjs clients elsewhere.
      applyRecoveredCanvasChanges(
        doc,
        readCanvasDocument(doc),
        serviceSnapshot(input.projection),
      );
      const update = Y.encodeStateAsUpdate(doc, vector);
      result = {
        update: Buffer.from(update).toString("base64"),
        payloadHash: createHash("sha256").update(update).digest("hex"),
      };
    }
    doc.destroy();
  } else result = await computeCandidate(action, input);
  process.stdout.write(JSON.stringify(result));
} catch (e) {
  process.stdout.write(
    JSON.stringify({
      fixtureError: e.code ?? e.message,
      definitive: e.definitive === true,
    }),
  );
  process.exitCode = 1;
}
