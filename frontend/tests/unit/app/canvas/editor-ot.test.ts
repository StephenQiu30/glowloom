import { Editor } from "@tiptap/core";
import { Slice, Fragment } from "@tiptap/pm/model";
import { ReplaceAroundStep } from "@tiptap/pm/transform";
import { afterEach, expect, it } from "vitest";
import { composerEditorOptions } from "@/components/generation/composer-session";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
import {
  editorSequenceEdits,
  promptSequence,
  sequencePrompt,
} from "@/app/canvas/editor-ot";
import { applySequence } from "@/app/canvas/sequence";
const editors: Editor[] = [];
function create(lines: string[]) {
  const draft = emptyPromptDraft();
  draft.document.content = lines.map((text) => ({
    type: "paragraph",
    ...(text ? { content: [{ type: "text" as const, text }] } : {}),
  }));
  const editor = new Editor(
    composerEditorOptions({ contextKey: "ot/test", replicaId: "test", draft }),
  );
  editors.push(editor);
  return editor;
}
afterEach(() => editors.splice(0).forEach((e) => e.destroy()));
it("maps the actual middle character of repeated text", () => {
  const e = create(["aaa"]),
    tr = e.state.tr.delete(2, 3);
  expect(editorSequenceEdits(tr)).toEqual([[{ retain: 1 }, { delete: 1 }]]);
});
it("maps Enter, paragraph join, hard break and paste against each step's own document", () => {
  const e = create(["AB"]);
  const tr = e.state.tr.split(2).insertText("中😀", 4).delete(2, 4);
  let seq = promptSequence(e.state.doc.toJSON());
  const edits = editorSequenceEdits(tr);
  expect(edits).toHaveLength(3);
  for (const edit of edits) seq = applySequence(seq, edit);
  expect(sequencePrompt(seq)).toEqual({
    type: "doc",
    content: [
      { type: "paragraph", content: [{ type: "text", text: "A中😀B" }] },
    ],
  });
  const hard = e.state.tr.insert(2, e.schema.nodes.hardBreak.create());
  expect(editorSequenceEdits(hard)).toEqual([
    [{ retain: 1 }, { insert: [{ type: "hardBreak" }] }],
  ]);
});
it("preserves ReplaceAroundStep gap without inventing a whole-document replacement", () => {
  const e = create(["abc"]);
  const tr = e.state.tr.step(
    new ReplaceAroundStep(
      1,
      4,
      2,
      3,
      new Slice(Fragment.from(e.schema.text("XY")), 0, 0),
      1,
    ),
  );
  const [edit] = editorSequenceEdits(tr);
  expect(applySequence([{ type: "text", text: "abc" }], edit)).toEqual([
    { type: "text", text: "XbY" },
  ]);
  expect(edit).toEqual([
    { insert: [{ type: "text", text: "X" }] },
    { delete: 1 },
    { retain: 1 },
    { insert: [{ type: "text", text: "Y" }] },
    { delete: 1 },
  ]);
});
it("round trips empty paragraphs and repeated reference atoms without lossy marks", () => {
  const doc = {
    type: "doc",
    content: [
      { type: "paragraph" },
      {
        type: "paragraph",
        content: [
          { type: "reference", attrs: { referenceId: "r" } },
          { type: "reference", attrs: { referenceId: "r" } },
          { type: "hardBreak" },
          { type: "text", text: "😀" },
        ],
      },
      { type: "paragraph" },
    ],
  };
  expect(sequencePrompt(promptSequence(doc))).toEqual(doc);
  expect(() =>
    promptSequence({
      type: "doc",
      content: [
        {
          type: "paragraph",
          content: [{ type: "text", text: "a", marks: [{ type: "bold" }] }],
        },
      ],
    }),
  ).toThrow();
});
