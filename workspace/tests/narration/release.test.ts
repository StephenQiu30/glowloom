import { expect, it } from "vitest";
import { mkdtemp, mkdir, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { freezeNarration } from "../../src/narration/release";
import { digest } from "../../src/content/parse";
it("freezes audio identity and requires separate public scope", async () => {
  const root = await mkdtemp(join(tmpdir(), "ws-narr-"));
  const doc = { id: "d", revision: "r", blocks: [] };
  try {
    const a = await freezeNarration(root, [doc], "internal");
    await mkdir(join(root, "d"));
    const audio = Buffer.from("owned fixture");
    await writeFile(join(root, "d/a.wav"), audio);
    const m = {
      documentId: "d",
      revision: "r",
      scope: "internal",
      configDigest: "cfg",
      precision: "none",
      segments: [
        {
          id: "a",
          src: "a.wav",
          sha256: digest(audio),
          durationMs: 1,
          cues: [],
        },
      ],
    };
    await writeFile(join(root, "d/manifest.json"), JSON.stringify(m));
    const b = await freezeNarration(root, [doc], "internal");
    expect(b.digest).not.toBe(a.digest);
    expect((await freezeNarration(root, [doc], "public")).items).toHaveLength(
      0,
    );
    m.scope = "public";
    await writeFile(join(root, "d/manifest.json"), JSON.stringify(m));
    expect((await freezeNarration(root, [doc], "public")).items).toHaveLength(
      1,
    );
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
