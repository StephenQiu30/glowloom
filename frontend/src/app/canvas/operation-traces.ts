import type { Editor } from "@tiptap/core";
import type { Transaction } from "@tiptap/pm/state";
import type { TextInputEdit } from "@/components/generation/text-input-intent";
import { editorSequenceEdits } from "./editor-ot";
import type { EditTrace } from "./operation-adapters";
import type { SequenceEdit } from "./sequence";

export function nativeSequenceEdit(edit: TextInputEdit): SequenceEdit {
  const result: SequenceEdit = [];
  if (edit.from) result.push({ retain: edit.from });
  if (edit.text) result.push({ insert: [{ type: "text", text: edit.text }] });
  if (edit.to > edit.from) result.push({ delete: edit.to - edit.from });
  return result;
}
export function composerTrace(nodeID: string, tr?: Transaction): EditTrace {
  if (!tr) return {};
  const text: NonNullable<EditTrace["text"]> = [];
  if (tr.docChanged)
    text.push({
      node_id: nodeID,
      path: ["config", "draft", "composer", "document"],
      edits: editorSequenceEdits(tr),
    });
  const param = tr.getMeta("composer-text-parameter") as
    (TextInputEdit & { field: string }) | undefined;
  if (param) {
    if (param.field !== "negative_prompt") throw new Error("unsupported_field");
    text.push({
      node_id: nodeID,
      path: ["config", "draft", "composer", "params", param.field],
      edits: [nativeSequenceEdit(param)],
    });
  }
  return { text };
}
export function plainEditorTrace(
  nodeID: string,
  path: string[],
  tr: Transaction,
): EditTrace {
  return {
    text: [{ node_id: nodeID, path, edits: editorSequenceEdits(tr, true) }],
  };
}
export function markdownEditorTrace(
  nodeID: string,
  editor: Editor,
  tr: Transaction,
): EditTrace {
  const plain =
    tr.before.childCount === 1 &&
    tr.doc.childCount === 1 &&
    editor.markdown?.serialize(tr.before.toJSON()) === tr.before.textContent &&
    editor.markdown?.serialize(tr.doc.toJSON()) === tr.doc.textContent;
  if (!plain) return { markdownSource: new Set([nodeID]) };
  return plainEditorTrace(nodeID, ["config", "text"], tr);
}
