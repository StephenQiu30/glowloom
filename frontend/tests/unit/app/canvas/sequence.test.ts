import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { expect, it } from "vitest";
import {
  applySequence,
  transformSequencePair,
  sequenceLength,
  type Atom,
  type SequenceEdit,
} from "@/app/canvas/sequence";
const vectors: {
  name: string;
  before: Atom[];
  a: SequenceEdit;
  b: SequenceEdit;
  want: Atom[];
}[] = JSON.parse(
  readFileSync(
    resolve(
      import.meta.dirname,
      "../../../../../backend/tests/canvas/testdata/sequence.json",
    ),
    "utf8",
  ),
);
vectors.push(
  ...JSON.parse(
    readFileSync(
      resolve(
        import.meta.dirname,
        "../../../../../backend/tests/canvas/testdata/sequence-random.json",
      ),
      "utf8",
    ),
  ),
);
for (const v of vectors)
  it(v.name, () => {
    const [ap, bp] = transformSequencePair(v.a, v.b, sequenceLength(v.before));
    expect(applySequence(applySequence(v.before, v.a), bp)).toEqual(v.want);
    expect(applySequence(applySequence(v.before, v.b), ap)).toEqual(v.want);
  });
it("rejects splitting an original surrogate pair and malformed operations", () => {
  for (const edit of [
    [{ retain: 1 }, { delete: 1 }],
    [{ retain: 3 }],
    [{ delete: 1 }],
    [{ retain: -1 }],
    [{ retain: 1, delete: 1 }],
    [{ insert: [{ type: "unknown" }] }],
  ]) {
    expect(() =>
      applySequence([{ type: "text", text: "😀" }], edit as SequenceEdit),
    ).toThrow();
  }
});
