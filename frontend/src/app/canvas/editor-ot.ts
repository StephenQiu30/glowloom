import { Slice, type Node as PMNode } from "@tiptap/pm/model";
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
  let source = doc.toJSON();
  if (plain) {
    const content: PromptDocument["content"] = [];
    doc.forEach((paragraph) => {
      if (
        paragraph.type.name !== "paragraph" ||
        Object.entries(paragraph.attrs).some(
          ([key, value]) =>
            key !== "textAlign" || (value !== null && value !== "left"),
        )
      )
        unsupported();
      const children: PromptInline[] = [];
      paragraph.forEach((inline) => {
        if (inline.marks.length) unsupported();
        if (inline.isText) children.push({ type: "text", text: inline.text! });
        else if (inline.type.name === "hardBreak")
          children.push({ type: "hardBreak" });
        else unsupported();
      });
      content.push({
        type: "paragraph",
        ...(children.length ? { content: children } : {}),
      });
    });
    source = { type: "doc", content };
  }
  const rich = promptSequence(source);
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

/** Applies a canonical sequence as local PM replacements. PM maps the existing
 * selection through these steps; no full-document overwrite or text diff occurs. */
export function applySequenceTransaction(
  transaction: Transaction,
  edit: SequenceEdit,
  plain = false,
): Transaction {
  const before = project(transaction.doc, plain);
  const expected = applySequence(before.atoms, edit);
  const positions = new Map<number, number>();
  let position = 0;
  transaction.doc.forEach((paragraph, offset, index) => {
    if (index) position++;
    positions.set(position, offset + 1);
    paragraph.forEach((inline, childOffset) => {
      for (let i = 0; i <= inline.nodeSize; i++)
        positions.set(position + i, offset + 1 + childOffset + i);
      position += inline.nodeSize;
    });
    positions.set(position, offset + 1 + paragraph.content.size);
  });
  const spans: { from: number; to: number; insert: Atom[] }[] = [];
  let cursor = 0;
  for (const part of edit) {
    if (part.retain) cursor += part.retain;
    else if (part.insert)
      spans.push({ from: cursor, to: cursor, insert: part.insert });
    else if (part.delete) {
      spans.push({ from: cursor, to: cursor + part.delete, insert: [] });
      cursor += part.delete;
    }
  }
  for (const span of spans.reverse()) {
    const from = positions.get(span.from),
      to = positions.get(span.to);
    if (from === undefined || to === undefined) return unsupported();
    let atoms = span.insert;
    if (plain) {
      atoms = [];
      for (const atom of span.insert) {
        if (atom.type !== "text") return unsupported();
        atom.text.split("\n").forEach((line, i) => {
          if (i) atoms.push({ type: "paragraphBreak" });
          if (line) atoms.push({ type: "text", text: line });
        });
      }
    }
    const slice = atoms.length
      ? new Slice(
          transaction.doc.type.schema.nodeFromJSON(sequencePrompt(atoms))
            .content,
          1,
          1,
        )
      : Slice.empty;
    transaction.replaceRange(from, to, slice);
  }
  if (
    JSON.stringify(project(transaction.doc, plain).atoms) !==
    JSON.stringify(expected)
  )
    return unsupported();
  return transaction;
}

export function plainEditorText(document: PMNode): string {
  return project(document, true)
    .atoms.map((atom) => (atom.type === "text" ? atom.text : ""))
    .join("");
}
