import {
  mkdtemp,
  mkdir,
  writeFile,
  readFile,
  rm,
  symlink,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { randomUUID } from "node:crypto";
import { expect, it } from "vitest";
import { applyProposal } from "../../src/server/bridge/proposals";
import { digest } from "../../src/content/parse";
it("applies a revision once and protects later user edits on replay or conflicting proposals", async () => {
  const root = await mkdtemp(join(tmpdir(), "ws-proposal-"));
  try {
    await mkdir(join(root, "workspace/content"), { recursive: true });
    await writeFile(join(root, "workspace/content/a.md"), "# 原文\n");
    await writeFile(
      join(root, "workspace/catalog.json"),
      JSON.stringify([
        { id: "doc-a", path: "content/a.md", visibility: "internal" },
      ]),
    );
    const request = {
      id: randomUUID(),
      documentId: "doc-a",
      baseRevision: digest("# 原文\n"),
      text: "# 修改\n",
      actor: "local-owner",
    };
    const receipt = await applyProposal(root, join(root, "runtime"), request);
    expect(receipt.status).toBe("applied");
    await writeFile(join(root, "workspace/content/a.md"), "# 用户后续编辑\n");
    expect(await applyProposal(root, join(root, "runtime"), request)).toEqual(
      receipt,
    );
    expect(await readFile(join(root, "workspace/content/a.md"), "utf8")).toBe(
      "# 用户后续编辑\n",
    );
    await expect(
      applyProposal(root, join(root, "runtime"), {
        ...request,
        id: randomUUID(),
      }),
    ).rejects.toThrow(/revision_conflict/);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
it("rejects a symlinked document rather than writing outside the knowledge root", async () => {
  const root = await mkdtemp(join(tmpdir(), "ws-proposal-"));
  try {
    await mkdir(join(root, "workspace/content"), { recursive: true });
    await writeFile(join(root, "outside.md"), "# 外部\n");
    await symlink(
      join(root, "outside.md"),
      join(root, "workspace/content/a.md"),
    );
    await writeFile(
      join(root, "workspace/catalog.json"),
      JSON.stringify([
        { id: "doc-a", path: "content/a.md", visibility: "internal" },
      ]),
    );
    await expect(
      applyProposal(root, join(root, "runtime"), {
        id: randomUUID(),
        documentId: "doc-a",
        baseRevision: digest("# 外部\n"),
        text: "# 错误\n",
        actor: "local-owner",
      }),
    ).rejects.toThrow(/path/);
    expect(await readFile(join(root, "outside.md"), "utf8")).toBe("# 外部\n");
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
it("recovers a dead process document lock without clearing a live owner", async () => {
  const root = await mkdtemp(join(tmpdir(), "ws-lock-"));
  try {
    await mkdir(join(root, "workspace/content"), { recursive: true });
    await mkdir(join(root, "runtime/locks"), { recursive: true });
    await writeFile(join(root, "workspace/content/a.md"), "# Original");
    await writeFile(
      join(root, "workspace/catalog.json"),
      JSON.stringify([
        { id: "a", path: "content/a.md", visibility: "internal" },
      ]),
    );
    const lock = join(root, "runtime/locks/a.lock");
    await writeFile(lock, JSON.stringify({ pid: process.pid }));
    const request = {
      id: randomUUID(),
      documentId: "a",
      baseRevision: digest("# Original"),
      text: "# Updated",
      actor: "local-owner",
    };
    await expect(
      applyProposal(root, join(root, "runtime"), request),
    ).rejects.toThrow(/busy/);
    await writeFile(lock, JSON.stringify({ pid: 2147483647 }));
    expect(
      (await applyProposal(root, join(root, "runtime"), request)).status,
    ).toBe("applied");
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
