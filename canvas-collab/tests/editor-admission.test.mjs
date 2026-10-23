import assert from "node:assert/strict";
import test from "node:test";
import { Window } from "happy-dom";
import * as Y from "yjs";
import {
  createCanvasDocument,
  readCanvasDocument,
  nodeTextBinding,
} from "../src/model.mjs";
import { CollaborationHub } from "../src/hub.mjs";
import { MemoryStore, scope, change } from "./fixtures.mjs";
import { digest } from "../src/admission.mjs";
import { canonicalGraphJson } from "../src/canonical.mjs";
import { committedDocument } from "../src/committed-state.mjs";

test("real nested Tiptap structural edits pass the live Hub gate and bootstrap the durable head", async () => {
  const browser = new Window({ url: "http://localhost" });
  for (const key of [
    "window",
    "self",
    "document",
    "navigator",
    "HTMLElement",
    "Node",
    "Text",
    "Element",
    "MutationObserver",
    "DOMParser",
    "getComputedStyle",
    "requestAnimationFrame",
    "cancelAnimationFrame",
  ])
    Object.defineProperty(globalThis, key, {
      configurable: true,
      value:
        /^(getComputedStyle|requestAnimationFrame|cancelAnimationFrame)$/.test(
          key,
        )
          ? browser[key].bind(browser)
          : browser[key],
    });
  const [{ Editor }, { default: StarterKit }, { default: Collaboration }] =
    await Promise.all([
      import("@tiptap/core"),
      import("@tiptap/starter-kit"),
      import("@tiptap/extension-collaboration"),
    ]);
  const initial = createCanvasDocument({
    title: "canvas",
    graph: {
      schemaVersion: 2,
      nodes: [
        {
          id: "n",
          type: "text",
          nodeTypeVersion: 1,
          title: "正文",
          position: { x: 0, y: 0 },
          width: 200,
          height: 100,
          textDocument: {
            schemaVersion: 1,
            content: {
              type: "doc",
              content: [
                { type: "paragraph", content: [{ type: "text", text: "AB" }] },
              ],
            },
          },
        },
      ],
      connections: [],
    },
  });
  const store = new MemoryStore(initial),
    load = store.load.bind(store),
    commit = store.commit.bind(store);
  store.load = async (binding) => {
    const loaded = await load();
    const doc = new Y.Doc();
    Y.applyUpdate(doc, loaded.update);
    const graphHash = digest(canonicalGraphJson(readCanvasDocument(doc).graph));
    doc.destroy();
    return {
      ...loaded,
      binding,
      metadata: {
        epoch: loaded.epoch,
        commitSeq: loaded.commitSeq,
        contentRevision: String(BigInt(loaded.commitSeq) + 1n),
        graphHash,
        lifecycle: "ACTIVE",
      },
    };
  };
  store.commit = async (...args) => {
    const result = await commit(...args);
    if (result.receipt)
      result.receipt.contentRevision = String(
        BigInt(result.receipt.commitSeq) + 1n,
      );
    return result;
  };
  const hub = new CollaborationHub({ store }),
    client = new Y.Doc(),
    late = new Y.Doc(),
    messages = [];
  let editor;
  try {
    const connection = await hub.join(
      scope,
      async (message) => {
        messages.push(message);
        if (message.update)
          Y.applyUpdate(client, Buffer.from(message.update, "base64"));
      },
      () => {},
    );
    editor = new Editor({
      element: document.body.appendChild(document.createElement("div")),
      extensions: [
        StarterKit.configure({ undoRedo: false }),
        Collaboration.configure({
          fragment: nodeTextBinding(client, "n").fragment,
        }),
      ],
    });
    const update = change(client, () =>
      editor
        .chain()
        .setTextSelection({ from: 1, to: 3 })
        .toggleBulletList()
        .run(),
    );
    assert.ok(
      [...client.share.keys()].some((key) =>
        key.startsWith("__tiptap_position_migrations:item:"),
      ),
      "the real plugin must produce position transfer metadata",
    );
    await hub.message(connection, {
      type: "SUBMIT_UPDATE",
      requestId: crypto.randomUUID(),
      updateId: crypto.randomUUID(),
      epoch: scope.epoch,
      payloadHash: digest(update),
      update: Buffer.from(update).toString("base64"),
    });
    assert.equal(messages.at(-1).type, "DURABLE_ACK");
    for (const command of ["undo", "redo"]) {
      const update = change(client, () => editor.commands[command]());
      await hub.message(connection, {
        type: "SUBMIT_UPDATE",
        requestId: crypto.randomUUID(),
        updateId: crypto.randomUUID(),
        epoch: scope.epoch,
        payloadHash: digest(update),
        update: Buffer.from(update).toString("base64"),
      });
      assert.equal(messages.at(-1).type, "DURABLE_ACK", command);
    }
    const durable = store.commits.at(-1),
      rebuilt = committedDocument({
        modelVersion: 1,
        checkpoint: Buffer.from(durable.state).toString("base64"),
        checkpointHash: digest(durable.state),
        checkpointSeq: store.state.commitSeq,
        commitSeq: store.state.commitSeq,
        tail: [],
        graphHash: digest(canonicalGraphJson(durable.graph.graph)),
      });
    try {
      assert.deepEqual(readCanvasDocument(rebuilt), readCanvasDocument(client));
    } finally {
      rebuilt.destroy();
    }
    await hub.join(
      { ...scope, sessionId: "late" },
      async (message) => {
        if (message.update)
          Y.applyUpdate(late, Buffer.from(message.update, "base64"));
      },
      () => {},
    );
    assert.deepEqual(readCanvasDocument(late), readCanvasDocument(client));
    assert.deepEqual(
      [...late.share.keys()].sort(),
      [...client.share.keys()].sort(),
    );
  } finally {
    editor?.destroy();
    hub.close();
    client.destroy();
    late.destroy();
    initial.destroy();
    await browser.happyDOM.close();
  }
});
