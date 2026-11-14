import { expect, it } from "vitest";
import { createServer } from "node:net";
import { mkdtemp, writeFile, rm, stat } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { prepareSocket } from "../../src/server/bridge/socket";
it("refuses live sockets and ordinary files while recovering a refused owned socket", async () => {
  const dir = await mkdtemp(join(tmpdir(), "ws-sock-"));
  const path = join(dir, "s");
  const server = createServer((c) => c.end());
  await new Promise<void>((r) => server.listen(path, r));
  try {
    await expect(prepareSocket(path)).rejects.toThrow(/active/);
    await stat(path);
  } finally {
    await new Promise<void>((r) => server.close(() => r()));
  }
  await writeFile(path, "keep");
  await expect(prepareSocket(path)).rejects.toThrow(/unsafe/);
  await rm(path);
  await prepareSocket(path);
  await rm(dir, { recursive: true });
});
