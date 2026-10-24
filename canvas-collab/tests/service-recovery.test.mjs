import assert from "node:assert/strict";
import test from "node:test";
import * as Y from "yjs";
import { createCanvasDocument, readCanvasDocument } from "../src/model.mjs";
import {
  serviceSnapshot,
  serviceProjection,
} from "../src/service-document.mjs";
import { CollaborationSession } from "../src/session.mjs";
import { archiveRecovery, recoveryFields } from "../src/recovery.mjs";

const id = (suffix) => "00000000-0000-4000-8000-" + suffix.padStart(12, "0");
const original = id("11"),
  replacement = id("12"),
  peerAsset = id("13");
const source = () => ({
  id: id("1"),
  project_id: id("2"),
  name: "media",
  revision: "1",
  schema_version: 1,
  operation_version: 1,
  permissions: { read: true, write: true },
  nodes: [
    {
      id: id("3"),
      title: "image",
      node_type: "image",
      node_action: "resource",
      x: 10,
      y: 20,
      width: 320,
      height: 180,
      z_index: 1,
      content_version: "8",
      input_revision: "7",
      last_operation_id: id("4"),
      media_ref: { asset_id: original },
      config: {
        editor: {
          schemaVersion: 1,
          parameters: { mode: "MANUAL" },
          metadata: { freeResize: false },
          files: [
            {
              referenceKey: "primary",
              purpose: "CONTENT",
              fileId: original,
              version: "1",
            },
          ],
        },
      },
    },
  ],
  edges: [],
});
const encode = (doc) =>
  Buffer.from(Y.encodeStateAsUpdate(doc)).toString("base64");
function changeAsset(snapshot, asset) {
  const next = structuredClone(snapshot),
    native = next.graph.nodes[0].native;
  native.media_ref.asset_id = asset;
  native.config.editor.files[0].fileId = asset;
  return next;
}
function fixture() {
  const dto = source(),
    server = createCanvasDocument(serviceSnapshot(dto)),
    sessions = [],
    requests = [];
  let seq = 0;
  const scope = {
    origin: "https://canvas.test",
    actorKey: "USER:a",
    canvasId: dto.id,
    sessionId: crypto.randomUUID(),
  };
  const state = () => ({
    epoch: id("5"),
    commitSeq: String(seq),
    contentRevision: String(seq + 1),
    bindingRevision: "1",
    modelVersion: 1,
    lifecycle: "ACTIVE",
    edit: true,
    graphHash: "a".repeat(64),
    update: encode(server),
  });
  const transport = {
    query: async () => null,
    submit: async (request) => {
      requests.push(structuredClone(request));
      // A recovered reference must still go through authoritative acceptance.
      throw Object.assign(new Error("ASSET_FORBIDDEN"), {
        definitive: true,
        code: "ASSET_FORBIDDEN",
      });
    },
  };
  const open = (recovery) => {
    const session = new CollaborationSession({
      scope,
      initial: state(),
      storage: "memory",
      transport,
      recovery,
    });
    sessions.push(session);
    return session;
  };
  const advance = (asset, title) => {
    const node = server
      .getMap("canvas")
      .get("nodes")
      .get(dto.nodes[0].id)
      .get("data");
    const next = changeAsset(readCanvasDocument(server), asset);
    server.transact(() => {
      node.set("native", next.graph.nodes[0].native);
      node.set("title", title);
    });
    seq++;
    return state();
  };
  return {
    dto,
    server,
    open,
    advance,
    state,
    transport,
    requests,
    close() {
      sessions.forEach((s) => s.destroy());
      server.destroy();
    },
  };
}

test("formal media reference intent survives rejection, rechecks a stale review, and cannot bypass asset authorization on recovery", async () => {
  const f = fixture();
  try {
    const session = f.open();
    session.change(changeAsset(session.snapshot, replacement));
    await assert.rejects(session.flush(), /ASSET_FORBIDDEN/);
    assert.equal(session.sealed.intents.length, 1);
    assert.equal(session.sealed.intents[0].field, "native");
    const native = session.sealed.intents[0].after.value;
    assert.equal(native.media_ref.asset_id, replacement);
    assert.equal(native.config.editor.files[0].fileId, replacement);
    for (const key of [
      "content_version",
      "input_revision",
      "last_operation_id",
    ])
      assert.equal(Object.hasOwn(native, key), false);

    const current = f.advance(peerAsset, "peer title");
    const restored = f.open(archiveRecovery(session.record(), current));
    const source = restored.recoverySources[0];
    const reviewed = recoveryFields(source, restored.snapshot);
    const later = f.advance(original, "later peer title");
    await restored.receive({ ...later, snapshot: true });
    await assert.rejects(
      restored.applyRecovery(source.id, reviewed),
      /RECOVERY_REVIEW_CHANGED/,
    );
    assert.equal(
      restored.snapshot.graph.nodes[0].native.media_ref.asset_id,
      original,
    );
    assert.equal(restored.pending.length, 0);

    const rereviewed = recoveryFields(source, restored.snapshot);
    await restored.applyRecovery(source.id, rereviewed);
    const projected = serviceProjection(restored.snapshot, f.dto);
    assert.equal(projected.nodes[0].title, "later peer title");
    assert.equal(projected.nodes[0].media_ref.asset_id, replacement);
    assert.equal(projected.nodes[0].content_version, "8");
    await assert.rejects(restored.flush(), /ASSET_FORBIDDEN/);
    assert.equal(f.requests.length, 2);
    assert.notEqual(f.requests[0].updateId, f.requests[1].updateId);
    assert.equal(f.requests[1].reviewSeq, later.commitSeq);
    assert.equal(
      readCanvasDocument(f.server).graph.nodes[0].native.media_ref.asset_id,
      original,
    );
    assert.equal(
      recoveryFields(restored.recoverySources[0], restored.snapshot)[0].after
        .value.media_ref.asset_id,
      replacement,
    );
  } finally {
    f.close();
  }
});

test("accepted formal native bytes that lose to a concurrent asset replacement remain recoverable", async () => {
  const f = fixture();
  try {
    const session = f.open();
    session.doc.clientID = 1;
    f.server.clientID = 4294967295;
    session.change(changeAsset(session.snapshot, replacement));
    const peer = f.advance(peerAsset, "image");
    await session.receive({ ...peer, snapshot: true });
    f.transport.submit = async (request) => {
      Y.applyUpdate(f.server, Buffer.from(request.update, "base64"));
      return {
        ...request,
        commitSeq: "2",
        contentRevision: "2",
        graphHash: "a".repeat(64),
        effect: "CAUSAL_CHANGE",
      };
    };
    await session.flush();
    assert.equal(session.inFlight, null);
    assert.equal(session.sealed?.originalOutcome, "COMMITTED");
    assert.equal(session.sealed?.reason, "UNAPPLIED_FIELDS");
    assert.equal(session.sealed.intents[0].field, "native");
    assert.equal(
      session.sealed.intents[0].after.value.media_ref.asset_id,
      replacement,
    );
    assert.equal(
      session.snapshot.graph.nodes[0].native.media_ref.asset_id,
      peerAsset,
    );
  } finally {
    f.close();
  }
});
