import { request } from "node:http";
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { randomUUID } from "node:crypto";
import { expect, it } from "vitest";
import { startBridge } from "../../src/server/bridge/service";
import { digest } from "../../src/content/parse";
it("preserves Chinese text across split UTF-8 IPC chunks", async () => {
  const root = await mkdtemp(join(tmpdir(), "ws-ipc-"));
  const socket = "/tmp/ws-ipc-" + randomUUID() + ".sock";
  const token = "fixture-bridge-token-with-at-least-32-characters";
  await mkdir(join(root, "workspace/content"), { recursive: true });
  await writeFile(join(root, "workspace/content/a.md"), "# 原文\n");
  await writeFile(
    join(root, "workspace/catalog.json"),
    JSON.stringify([
      { id: "doc-a", path: "content/a.md", visibility: "internal" },
    ]),
  );
  const bridge = await startBridge({
    repo: root,
    storage: join(root, "runtime"),
    socket,
    token,
    codexBinary: "unused",
  });
  try {
    const text = "# 修改\n\n中文内容。";
    const data = Buffer.from(
      JSON.stringify({
        id: randomUUID(),
        documentId: "doc-a",
        baseRevision: digest("# 原文\n"),
        text,
        actor: "local-owner",
      }),
    );
    const cut = data.indexOf(Buffer.from("中文")) + 1;
    const result = await new Promise<string>((yes, no) => {
      const req = request(
        {
          socketPath: socket,
          path: "/proposal/apply",
          method: "POST",
          headers: {
            authorization: "Bearer " + token,
            "content-type": "application/json",
            "content-length": data.length,
          },
        },
        (res) => {
          let out = "";
          res.setEncoding("utf8");
          res.on("data", (s) => (out += s));
          res.on("end", () => yes(out));
        },
      );
      req.on("error", no);
      req.write(data.subarray(0, cut));
      setTimeout(() => req.end(data.subarray(cut)), 15);
    });
    expect(JSON.parse(result).status).toBe("applied");
    expect(await readFile(join(root, "workspace/content/a.md"), "utf8")).toBe(
      text,
    );
  } finally {
    await bridge.close();
    await rm(root, { recursive: true, force: true });
  }
});
