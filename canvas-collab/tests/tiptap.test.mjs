import assert from "node:assert/strict";
import test from "node:test";
import { Window } from "happy-dom";
import * as Y from "yjs";
import { AdmissionGate } from "../src/admission.mjs";
import { MemoryStore, scope } from "./fixtures.mjs";

test("actual Tiptap bindings preserve simultaneous text, marks, mentions and personal undo", async () => {
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
        typeof browser[key] === "function" &&
        /^(getComputedStyle|requestAnimationFrame|cancelAnimationFrame)$/.test(
          key,
        )
          ? browser[key].bind(browser)
          : browser[key],
    });
  const [
    { Editor, getSchema },
    { default: StarterKit },
    { default: Mention },
    { default: Collaboration },
    { prosemirrorJSONToYDoc, yDocToProsemirrorJSON },
  ] = await Promise.all([
    import("@tiptap/core"),
    import("@tiptap/starter-kit"),
    import("@tiptap/extension-mention"),
    import("@tiptap/extension-collaboration"),
    import("@tiptap/y-tiptap"),
  ]);
  const reference = Mention.extend({
    addAttributes() {
      return { ...this.parent?.(), token: { default: null } };
    },
  }).configure({
    renderText: ({ node }) => String(node.attrs.token ?? node.attrs.label),
  });
  const baseExtensions = [StarterKit.configure({ undoRedo: false }), reference];
  const initial = prosemirrorJSONToYDoc(
    getSchema(baseExtensions),
    {
      type: "doc",
      content: [
        {
          type: "paragraph",
          content: [
            { type: "text", text: "AB" },
            {
              type: "mention",
              attrs: { id: "fixed-file", label: "图", token: "@图" },
            },
            { type: "text", text: "Z" },
          ],
        },
      ],
    },
    "body",
  );
  const a = new Y.Doc(),
    b = new Y.Doc();
  const store = new MemoryStore(initial);
  const schema = getSchema(baseExtensions);
  const engine = new AdmissionGate({
    store,
    allowedRoots: ["body"],
    authorize: async () => true,
    project: (doc) => yDocToProsemirrorJSON(doc, "body"),
    validate: (json) => schema.nodeFromJSON(json).check(),
  });
  Y.applyUpdate(a, Y.encodeStateAsUpdate(initial));
  Y.applyUpdate(b, Y.encodeStateAsUpdate(initial));
  const mount = (doc) =>
    new Editor({
      element: document.body.appendChild(document.createElement("div")),
      extensions: [
        ...baseExtensions,
        Collaboration.configure({ document: doc, field: "body" }),
      ],
    });
  const ea = mount(a),
    eb = mount(b);
  const capture = (doc, run) => {
    const updates = [];
    const listener = (u) => updates.push(u);
    doc.on("update", listener);
    try {
      run();
    } finally {
      doc.off("update", listener);
    }
    return Y.mergeUpdates(updates);
  };
  try {
    assert.equal(ea.getText(), "AB@图Z");
    assert.equal(eb.getText(), "AB@图Z");
    const ua = capture(a, () =>
      ea.chain().setTextSelection(1).toggleBold().insertContent("甲").run(),
    );
    const ub = capture(b, () => eb.commands.insertContentAt(5, "乙"));
    await engine.submit(scope, {
      epoch: scope.epoch,
      updateId: "editor-a",
      update: ua,
    });
    await engine.submit(
      { ...scope, sessionId: "b" },
      { epoch: scope.epoch, updateId: "editor-b", update: ub },
    );
    Y.applyUpdate(a, ub, "remote");
    Y.applyUpdate(b, ua, "remote");
    assert.equal(ea.getText(), "甲AB@图Z乙");
    assert.equal(eb.getText(), "甲AB@图Z乙");
    assert.deepEqual(ea.getJSON(), eb.getJSON());
    const nodes = ea.getJSON().content[0].content;
    assert.deepEqual(nodes[0], {
      type: "text",
      marks: [{ type: "bold" }],
      text: "甲",
    });
    assert.equal(
      nodes.find((n) => n.type === "mention").attrs.id,
      "fixed-file",
    );
    assert.equal(nodes.find((n) => n.type === "mention").attrs.token, "@图");
    const undo = capture(a, () => assert.equal(ea.commands.undo(), true));
    await engine.submit(scope, {
      epoch: scope.epoch,
      updateId: "editor-undo",
      update: undo,
    });
    Y.applyUpdate(b, undo, "remote");
    assert.equal(ea.getText(), "AB@图Z乙", "A's undo preserves B's insertion");
    assert.equal(eb.getText(), "AB@图Z乙");
    const persisted = store.state.update,
      reopened = new Y.Doc();
    Y.applyUpdate(reopened, persisted);
    const ec = mount(reopened);
    try {
      assert.equal(
        ec.getText(),
        "AB@图Z乙",
        "mounting does not insert the initial JSON again",
      );
    } finally {
      ec.destroy();
      reopened.destroy();
    }
  } finally {
    ea.destroy();
    eb.destroy();
    a.destroy();
    b.destroy();
    initial.destroy();
    await browser.happyDOM.close();
  }
});
