import { expect, it } from "vitest";
import {
  rangeForChapter,
  type NarrationManifest,
} from "../../src/narration/manifest";
it("derives chapter bounds only from actual mapped cues", () => {
  const manifest: NarrationManifest = {
    documentId: "d",
    revision: "r",
    scope: "full",
    configDigest: "c",
    precision: "paragraph",
    segments: [
      {
        id: "a",
        src: "a.wav",
        sha256: "a".repeat(64),
        durationMs: 3000,
        cues: [
          {
            blockId: "intro",
            startMs: 0,
            endMs: 800,
            textStart: 0,
            textEnd: 2,
          },
          {
            blockId: "chapter",
            startMs: 800,
            endMs: 1900,
            textStart: 0,
            textEnd: 2,
          },
        ],
      },
      {
        id: "b",
        src: "b.wav",
        sha256: "b".repeat(64),
        durationMs: 2000,
        cues: [
          {
            blockId: "chapter-end",
            startMs: 0,
            endMs: 1000,
            textStart: 0,
            textEnd: 2,
          },
        ],
      },
    ],
  };
  expect(rangeForChapter(manifest, ["chapter", "chapter-end"])).toEqual({
    fromSegment: 0,
    fromMs: 800,
    toSegment: 1,
    toMs: 1000,
  });
  expect(rangeForChapter(manifest, ["unmapped"])).toBeNull();
});
