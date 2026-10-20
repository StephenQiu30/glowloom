export type Atom =
  | { type: "text"; text: string }
  | { type: "paragraphBreak" | "hardBreak" }
  | { type: "reference"; reference_id: string };
export type SequenceComponent = {
  retain?: number;
  insert?: Atom[];
  delete?: number;
};
export type SequenceEdit = SequenceComponent[];
const limit = 1 << 20;
function invalid(): never {
  throw new Error("invalid_sequence");
}
function wellFormed(text: string) {
  for (const point of text) {
    const unit = point.charCodeAt(0);
    if (point.length === 1 && unit >= 0xd800 && unit <= 0xdfff) return false;
  }
  return true;
}
function atomLength(a: Atom): number {
  if (!a || typeof a !== "object") return invalid();
  const keys = Object.keys(a);
  if (a.type === "text") {
    if (
      keys.some((key) => !["type", "text"].includes(key)) ||
      typeof a.text !== "string" ||
      !a.text ||
      !wellFormed(a.text)
    )
      return invalid();
    return a.text.length;
  }
  if (a.type === "reference") {
    if (
      keys.some((key) => !["type", "reference_id"].includes(key)) ||
      typeof a.reference_id !== "string" ||
      !a.reference_id ||
      new TextEncoder().encode(a.reference_id).length > 256 ||
      !wellFormed(a.reference_id)
    )
      return invalid();
    return 1;
  }
  if (
    (a.type === "paragraphBreak" || a.type === "hardBreak") &&
    keys.length === 1
  )
    return 1;
  return invalid();
}
export function sequenceLength(sequence: Atom[]): number {
  if (!Array.isArray(sequence)) return invalid();
  let length = 0;
  for (const atom of sequence) {
    length += atomLength(atom);
    if (length > limit) return invalid();
  }
  return length;
}
function compact(atoms: Atom[]): Atom[] {
  const result: Atom[] = [];
  let text: string[] = [];
  const flush = () => {
    if (text.length) result.push({ type: "text", text: text.join("") });
    text = [];
  };
  for (const atom of atoms) {
    if (atom.type === "text") text.push(atom.text);
    else {
      flush();
      result.push({ ...atom });
    }
  }
  flush();
  return result;
}
function push(out: SequenceEdit, c: SequenceComponent) {
  if (!(c.retain || c.delete || c.insert?.length)) return;
  const last = out.at(-1);
  if (last?.retain && c.retain) last.retain += c.retain;
  else if (last?.delete && c.delete) last.delete += c.delete;
  else if (last?.insert && c.insert)
    last.insert = compact([...last.insert, ...c.insert]);
  else out.push(c.insert ? { insert: compact(c.insert) } : { ...c });
}
function complete(edit: SequenceEdit, length: number): SequenceEdit {
  if (
    !Number.isSafeInteger(length) ||
    length < 0 ||
    length > limit ||
    !Array.isArray(edit) ||
    edit.length > 10000
  )
    return invalid();
  const result: SequenceEdit = [];
  let consumed = 0;
  for (const c of edit) {
    const keys = Object.keys(c);
    const retain = c.retain ?? 0,
      remove = c.delete ?? 0;
    if (
      keys.some((key) => !["retain", "delete", "insert"].includes(key)) ||
      Number(retain !== 0) +
        Number(remove !== 0) +
        Number(c.insert !== undefined) !==
        1 ||
      !Number.isSafeInteger(retain) ||
      !Number.isSafeInteger(remove) ||
      retain < 0 ||
      remove < 0 ||
      (c.insert !== undefined &&
        (!c.insert.length || sequenceLength(c.insert) < 0)) ||
      retain > length - consumed ||
      remove > length - consumed
    )
      return invalid();
    consumed += retain + remove;
    push(result, c);
  }
  push(result, { retain: length - consumed });
  return result;
}
export function applySequence(before: Atom[], edit: SequenceEdit): Atom[] {
  const operations = complete(edit, sequenceLength(before));
  const units: Atom[] = before.flatMap((atom) =>
    atom.type === "text"
      ? [...atom.text].map((text): Atom => ({ type: "text", text }))
      : [atom],
  );
  const result: Atom[] = [];
  let index = 0;
  for (const c of operations) {
    if (c.insert) {
      result.push(...c.insert);
      continue;
    }
    let n = (c.retain ?? 0) + (c.delete ?? 0);
    while (n > 0) {
      const unit = units[index++];
      if (!unit || atomLength(unit) > n) return invalid();
      n -= atomLength(unit);
      if (c.retain) result.push(unit);
    }
  }
  sequenceLength(result);
  return compact(result);
}
/** A is first in server order. Both edits must start at the same base. */
export function transformSequencePair(
  a: SequenceEdit,
  b: SequenceEdit,
  length: number,
): [SequenceEdit, SequenceEdit] {
  const left = complete(a, length),
    right = complete(b, length);
  const ap: SequenceEdit = [],
    bp: SequenceEdit = [];
  let i = 0,
    j = 0;
  while (i < left.length || j < right.length) {
    const x = left[i],
      y = right[j];
    if (x?.insert) {
      push(ap, x);
      push(bp, { retain: sequenceLength(x.insert) });
      i++;
      continue;
    }
    if (y?.insert) {
      push(bp, y);
      push(ap, { retain: sequenceLength(y.insert) });
      j++;
      continue;
    }
    if (!x || !y) return invalid();
    const n = Math.min(
      (x.retain ?? 0) + (x.delete ?? 0),
      (y.retain ?? 0) + (y.delete ?? 0),
    );
    if (x.retain && y.retain) {
      push(ap, { retain: n });
      push(bp, { retain: n });
    } else if (x.delete && y.retain) push(ap, { delete: n });
    else if (x.retain && y.delete) push(bp, { delete: n });
    if (x.retain) x.retain -= n;
    else x.delete! -= n;
    if (y.retain) y.retain -= n;
    else y.delete! -= n;
    if (!(x.retain || x.delete)) i++;
    if (!(y.retain || y.delete)) j++;
  }
  return [ap, bp];
}
