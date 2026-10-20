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

it("Go and TS share every finite field policy and seeded causal graph result", async () => {
  const { fieldPolicies } = await import("@/app/canvas/operation-fields");
  const { applyOperations, transformOperationPair } =
    await import("@/app/canvas/operations");
  const root = resolve(import.meta.dirname, "../../../../.."),
    dir = mkdtempSync(resolve(tmpdir(), "glowloom-graph-vectors-")),
    out = resolve(dir, "go.json");
  const oracle = (test: string) => {
    execFileSync(
      "go",
      [
        "test",
        "./tests/canvas/otvectors",
        "-run",
        "^" + test + "$",
        "-count=1",
      ],
      {
        cwd: resolve(root, "backend"),
        env: { ...process.env, LV_OT_VECTOR_OUTPUT: out },
        timeout: 120000,
        stdio: "pipe",
      },
    );
    return JSON.parse(readFileSync(out, "utf8"));
  };
  try {
    const normalized = Object.fromEntries(
      Object.entries(fieldPolicies).map(([k, v]) => [
        k,
        Object.fromEntries(Object.entries(v).filter(([, v]) => v !== false)),
      ]),
    );
    expect(oracle("TestFieldPolicyProtocolVectors")).toEqual(normalized);
    const vectors = oracle("TestGraphProtocolVectors") as {
      name: string;
      base: import("@/app/canvas/document").Canvas;
      A: import("@/app/canvas/operations").Operation[];
      B: import("@/app/canvas/operations").Operation[];
      ap: unknown;
      bp: unknown;
      nodes: unknown;
    }[];
    expect(vectors.length).toBeGreaterThan(200);
    for (const v of vectors) {
      const [ap, bp] = transformOperationPair(v.base, v.A, v.B);
      const wire = (value: unknown) => JSON.parse(JSON.stringify(value));
      expect(wire(ap), v.name + " a'").toEqual(v.ap);
      expect(wire(bp), v.name + " b'").toEqual(v.bp);
      expect(
        wire(applyOperations(applyOperations(v.base, v.A), bp).nodes),
        v.name + " canonical",
      ).toEqual(v.nodes);
    }
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}, 130000);
