import assert from "node:assert/strict";
import test from "node:test";
import * as Y from "yjs";
import { prosemirrorJSONToYDoc, yDocToProsemirrorJSON } from "@tiptap/y-tiptap";
import {
  richTextSchema,
  normalizeRichText,
  persistentRichText,
} from "../src/rich-text-schema.mjs";

test("three alignments survive normalized Yjs bytes and reopen without changing content", () => {
  const input = {
    type: "doc",
    content: ["left", "center", "right"].map((textAlign, i) => ({
      type: i === 1 ? "heading" : "paragraph",
      attrs: { ...(i === 1 ? { level: 2 } : {}), textAlign },
      content: [{ type: "text", text: textAlign, marks: [{ type: "bold" }] }],
    })),
  };
  const normalized = normalizeRichText(input);
  const doc = prosemirrorJSONToYDoc(richTextSchema, normalized, "body");
  const reopened = new Y.Doc();
  try {
    Y.applyUpdate(reopened, Y.encodeStateAsUpdate(doc));
    assert.deepEqual(
      normalizeRichText(yDocToProsemirrorJSON(reopened, "body")),
      normalized,
    );
    assert.deepEqual(
      normalized.content.map((n) => n.attrs.textAlign),
      ["left", "center", "right"],
    );
  } finally {
    doc.destroy();
    reopened.destroy();
  }
});

test("default alignment does not change old document hashes and arbitrary values are rejected", () => {
  const old = {
    type: "doc",
    content: [{ type: "paragraph", content: [{ type: "text", text: "old" }] }],
  };
  assert.deepEqual(normalizeRichText(old), old);
  assert.deepEqual(
    persistentRichText({ type: "paragraph", attrs: { textAlign: null } }),
    { type: "paragraph" },
  );
  for (const textAlign of ["justify", "url(x)", "", 1, {}])
    assert.throws(() =>
      normalizeRichText({
        type: "doc",
        content: [{ type: "paragraph", attrs: { textAlign } }],
      }),
    );
  assert.throws(() =>
    normalizeRichText({
      type: "doc",
      attrs: { textAlign: "left" },
      content: [{ type: "paragraph" }],
    }),
  );
});
