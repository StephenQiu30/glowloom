import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { resolve } from "node:path";
import { expect, it } from "vitest";
import {
  sequenceLength,
  transformSequencePair,
  type Atom,
  type SequenceEdit,
} from "@/app/canvas/sequence";
it("Go and TS produce identical wire operations for all literal and seeded vectors", () => {
  const root = resolve(import.meta.dirname, "../../../../.."),
    dir = mkdtempSync(resolve(tmpdir(), "glowloom-ot-vectors-")),
    out = resolve(dir, "go.json");
  try {
    execFileSync(
      "go",
      [
        "test",
        "./tests/canvas/otvectors",
        "-run",
        "^TestSequenceProtocolVectors$",
        "-count=1",
      ],
      {
        cwd: resolve(root, "backend"),
        env: { ...process.env, LV_OT_VECTOR_OUTPUT: out },
        timeout: 120000,
        stdio: "pipe",
      },
    );
    const vectors: {
      name: string;
      before: Atom[];
      a: SequenceEdit;
      b: SequenceEdit;
    }[] = ["sequence.json", "sequence-random.json"].flatMap((name) =>
      JSON.parse(
        readFileSync(
          resolve(root, "backend/tests/canvas/testdata", name),
          "utf8",
        ),
      ),
    );
    const go = JSON.parse(readFileSync(out, "utf8"));
    const ts = vectors.map((v) => {
      const [ap, bp] = transformSequencePair(
        v.a,
        v.b,
        sequenceLength(v.before),
      );
      return { name: v.name, ap, bp };
    });
    expect(go).toEqual(ts);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}, 130000);
