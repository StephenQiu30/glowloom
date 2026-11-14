import { mkdtemp, mkdir, writeFile, symlink, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { execFileSync } from "node:child_process";
import { expect, it } from "vitest";
import { copyPortableTree } from "../../src/build/package";
it("runs copied dependencies after the original build directory is removed", async () => {
  const root = await mkdtemp(join(tmpdir(), "ws-package-"));
  try {
    const src = join(root, "build"),
      dst = join(root, "release");
    await mkdir(join(src, "packages/pkg"), { recursive: true });
    await mkdir(join(src, "node_modules"));
    await writeFile(
      join(src, "packages/pkg/index.js"),
      'module.exports="portable";',
    );
    await symlink("../packages/pkg", join(src, "node_modules/pkg"));
    await writeFile(join(src, "server.cjs"), 'console.log(require("pkg"));');
    await copyPortableTree(src, dst);
    await rm(src, { recursive: true, force: true });
    expect(
      execFileSync(process.execPath, [join(dst, "server.cjs")], {
        encoding: "utf8",
        stdio: ["ignore", "pipe", "pipe"],
      }).trim(),
    ).toBe("portable");
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
