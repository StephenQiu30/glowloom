import { expect, it } from "vitest";
import {
  validateManifest,
  cueAt,
  planSegments,
  type NarrationManifest,
} from "../../src/narration/manifest";
const manifest: NarrationManifest = {
  documentId: "design-43",
  revision: "rev1",
  scope: "full",
  configDigest: "cfg",
  precision: "paragraph",
  segments: [
    {
      id: "s1",
      src: "s1.wav",
      sha256: "a".repeat(64),
      durationMs: 2000,
      cues: [
        { startMs: 500, endMs: 1500, blockId: "b1", textStart: 0, textEnd: 4 },
      ],
    },
  ],
};
it("uses actual media seconds to locate cues, including seek and pause positions", () => {
  expect(cueAt(manifest.segments[0], 0.6)?.blockId).toBe("b1");
  expect(cueAt(manifest.segments[0], 0.1)).toBeNull();
  expect(cueAt(manifest.segments[0], 1.8)).toBeNull();
});
it("rejects stale sources and out-of-bounds cues without inventing timings", () => {
  expect(() =>
    validateManifest(manifest, {
      id: "design-43",
      revision: "rev2",
      blocks: [{ id: "b1", text: "原文段落" }],
    }),
  ).toThrow(/stale|过期/);
  expect(() =>
    validateManifest(
      {
        ...manifest,
        segments: [
          {
            ...manifest.segments[0],
            cues: [
              {
                startMs: 0,
                endMs: 4000,
                blockId: "b1",
                textStart: 0,
                textEnd: 4,
              },
            ],
          },
        ],
      },
      {
        id: "design-43",
        revision: "rev1",
        blocks: [{ id: "b1", text: "原文段落" }],
      },
    ),
  ).toThrow(/cue/);
});
it("reuses only matching text and configuration and never resubmits unknown segments", () => {
  const jobs = planSegments(
    [
      { id: "a", textDigest: "t1" },
      { id: "b", textDigest: "t2" },
      { id: "c", textDigest: "t3" },
    ],
    "cfg",
    [
      { id: "a", textDigest: "t1", configDigest: "cfg", status: "ready" },
      { id: "b", textDigest: "t2", configDigest: "cfg", status: "unknown" },
    ],
  );
  expect(jobs).toEqual([
    { id: "a", action: "reuse" },
    { id: "b", action: "reconcile" },
    { id: "c", action: "generate" },
  ]);
  expect(
    planSegments([{ id: "a", textDigest: "changed" }], "cfg", [
      { id: "a", textDigest: "t1", configDigest: "cfg", status: "ready" },
    ]),
  ).toEqual([{ id: "a", action: "generate" }]);
});
