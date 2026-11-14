import { expect, it } from "vitest";
import { mkdtemp, writeFile, readFile, rm, mkdir } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { sealRelease, selectRelease } from "../../src/build/release";
it("verifies release assets before selecting or rolling back the mode pointer", async () => {
  const root = await mkdtemp(join(tmpdir(), "ws-release-"));
  try {
    const a = join(root, "a");
    await mkdir(a);
    await writeFile(join(a, "index.html"), "safe");
    await sealRelease(a, { mode: "static", scope: "public", releaseId: "a" });
    await selectRelease(root, "a", "static");
    expect(
      JSON.parse(await readFile(join(root, "current.json"), "utf8")).releaseId,
    ).toBe("a");
    await writeFile(join(a, "index.html"), "changed");
    await expect(selectRelease(root, "a", "static")).rejects.toThrow(
      /integrity/,
    );
    await expect(selectRelease(root, "a", "online")).rejects.toThrow(/mode/);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
