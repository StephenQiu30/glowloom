import type { Node as PMNode } from "@tiptap/pm/model";
import type { Transaction } from "@tiptap/pm/state";
import type {
  PromptDocument,
  PromptInline,
} from "@/components/generation/prompt-document";
import {
  applySequence,
  sequenceLength,
  type Atom,
  type SequenceEdit,
} from "./sequence";

function unsupported(): never {
  throw new Error("unsupported_editor_step");
}
function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value))
    return unsupported();
  return value as Record<string, unknown>;
}
function only(value: Record<string, unknown>, keys: string[]) {
  if (Object.keys(value).some((key) => !keys.includes(key))) unsupported();
}
export function promptSequence(value: unknown): Atom[] {
  const doc = record(value);
  only(doc, ["type", "content"]);
  if (doc.type !== "doc" || !Array.isArray(doc.content) || !doc.content.length)
    return unsupported();
  const atoms: Atom[] = [];
  for (const [index, item] of doc.content.entries()) {
    const paragraph = record(item);
    only(paragraph, ["type", "content"]);
    if (
      paragraph.type !== "paragraph" ||
      (paragraph.content !== undefined && !Array.isArray(paragraph.content))
    )
      return unsupported();
    if (index) atoms.push({ type: "paragraphBreak" });
    for (const value of (paragraph.content ?? []) as unknown[]) {
      const inline = record(value);
      if (inline.type === "text") {
        only(inline, ["type", "text"]);
        if (typeof inline.text !== "string" || !inline.text)
          return unsupported();
        atoms.push({ type: "text", text: inline.text });
      } else if (inline.type === "hardBreak") {
        only(inline, ["type"]);
        atoms.push({ type: "hardBreak" });
      } else if (inline.type === "reference") {
        only(inline, ["type", "attrs"]);
        const attrs = record(inline.attrs);
        only(attrs, ["referenceId"]);
        if (typeof attrs.referenceId !== "string") return unsupported();
        atoms.push({ type: "reference", reference_id: attrs.referenceId });
      } else return unsupported();
    }
  }
  return applySequence(atoms, []);
}
export function sequencePrompt(atoms: Atom[]): PromptDocument {
  const content: PromptDocument["content"] = [{ type: "paragraph" }];
  for (const atom of applySequence(atoms, [])) {
    if (atom.type === "paragraphBreak") {
      content.push({ type: "paragraph" });
      continue;
    }
    let inline: PromptInline;
    if (atom.type === "text") inline = { type: "text", text: atom.text };
    else if (atom.type === "reference")
      inline = { type: "reference", attrs: { referenceId: atom.reference_id } };
    else inline = { type: "hardBreak" };
    (content[content.length - 1].content ??= []).push(inline);
  }
  return { type: "doc", content };
}
function project(doc: PMNode, plain: boolean) {
  const rich = promptSequence(doc.toJSON());
  const atoms: Atom[] = plain
    ? rich.map((atom) => {
        if (atom.type === "reference") return unsupported();
        return atom.type === "text" ? atom : { type: "text", text: "\n" };
      })
    : rich;
  const boundaries = new Map<number, { left: number; right: number }>();
  let pm = 0,
    position = 0;
  const exact = (key: number, value: number) =>
    boundaries.set(key, { left: value, right: value });
  doc.forEach((paragraph, _offset, index) => {
    if (index) position++;
    boundaries.set(pm, { left: position - Number(index > 0), right: position });
    exact(pm + 1, position);
    paragraph.forEach((inline, offset) => {
      if (inline.marks.length) unsupported();
      for (let i = 0; i <= inline.nodeSize; i++)
        exact(pm + 1 + offset + i, position + i);
      position += inline.nodeSize;
    });
    exact(pm + 1 + paragraph.content.size, position);
    pm += paragraph.nodeSize;
    boundaries.set(pm, {
      left: position,
      right: position + Number(index < doc.childCount - 1),
    });
  });
  exact(0, 0);
  exact(doc.content.size, position);
  return {
    atoms: applySequence(atoms, []),
    at: (pm: number, association = 1) => {
      const entry = boundaries.get(pm);
      if (!entry) return unsupported();
      return association < 0 ? entry.left : entry.right;
    },
  };
}
function fragment(atoms: Atom[], from: number, to: number): Atom[] {
  const edit: SequenceEdit = [];
  if (from) edit.push({ delete: from });
  if (to > from) edit.push({ retain: to - from });
  if (sequenceLength(atoms) > to)
    edit.push({ delete: sequenceLength(atoms) - to });
  return applySequence(atoms, edit);
}

/** Each editor step is mapped from its own pre-step document. No text diff or
 * whole-document replacement is used to guess a user's character contribution. */
export function editorSequenceEdits(
  transaction: Transaction,
  plain = false,
): SequenceEdit[] {
  const result: SequenceEdit[] = [];
  for (const [index, step] of transaction.steps.entries()) {
    const before = transaction.docs[index];
    const applied = step.apply(before);
    if (applied.failed || !applied.doc) return unsupported();
    const from = project(before, plain),
      to = project(applied.doc, plain);
    const ranges: { from: number; end: number; inserted: Atom[] }[] = [];
    step.getMap().forEach((oldStart, oldEnd, newStart, newEnd) => {
      ranges.push({
        from: from.at(oldStart),
        end: from.at(oldEnd),
        inserted: fragment(to.atoms, to.at(newStart), to.at(newEnd)),
      });
    });
    const edit: SequenceEdit = [];
    let cursor = 0;
    for (const range of ranges) {
      if (range.from < cursor || range.end < range.from) return unsupported();
      if (range.from > cursor) edit.push({ retain: range.from - cursor });
      if (range.inserted.length) edit.push({ insert: range.inserted });
      if (range.end > range.from) edit.push({ delete: range.end - range.from });
      cursor = range.end;
    }
    if (
      JSON.stringify(applySequence(from.atoms, edit)) !==
      JSON.stringify(to.atoms)
    )
      return unsupported();
    if (edit.length) result.push(edit);
  }
  return result;
}
