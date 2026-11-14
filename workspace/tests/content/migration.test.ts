import { mkdtemp, mkdir, readFile, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, it } from "vitest";
import { migrateKnowledge } from "../../src/content/migration";

it("moves each knowledge file once and only rewrites necessary link targets", async () => {
  const root = await mkdtemp(join(tmpdir(), "workspace-move-"));
  try {
    await mkdir(join(root, "docs/design"), { recursive: true });
    await writeFile(
      join(root, "BACKLOG.md"),
      "# 进度\n[设计](docs/design/01.md#方案)",
    );
    await writeFile(
      join(root, "docs/design/01.md"),
      "# 设计\n\n## 方案\n\n[进度](../../BACKLOG.md)\n\n正文保持原样。",
    );
    await writeFile(join(root, "docs/table.csv"), "字段,值\na,1\n");
    const result = await migrateKnowledge(
      root,
      ["docs/design/01.md", "docs/table.csv"],
      ["BACKLOG.md"],
    );
    expect(result).toHaveLength(2);
    expect(
      await readFile(join(root, "workspace/content/design/01.md"), "utf8"),
    ).toBe(
      "# 设计\n\n## 方案\n\n[进度](../../../BACKLOG.md)\n\n正文保持原样。",
    );
    expect(await readFile(join(root, "BACKLOG.md"), "utf8")).toContain(
      "workspace/content/design/01.md#方案",
    );
    expect(
      await readFile(join(root, "workspace/content/table.csv"), "utf8"),
    ).toBe("字段,值\na,1\n");
    await expect(readFile(join(root, "docs/design/01.md"))).rejects.toThrow();
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

it("rewrites the URL rather than an identical link label", async () => {
  const { rewriteMarkdownLinks } = await import("../../src/content/catalog");
  expect(
    rewriteMarkdownLinks("[docs/a.md](docs/a.md)", (url) =>
      url.replace("docs/", "workspace/content/"),
    ),
  ).toBe("[docs/a.md](workspace/content/a.md)");
});

it("keeps a missing historical reference label when explicitly requested", async () => {
  const { rewriteMarkdownLinks } = await import("../../src/content/catalog");
  expect(
    rewriteMarkdownLinks(
      "[验收证据](gone.md)",
      () => null,
      (label) => `${label}（历史引用未归档）`,
    ),
  ).toBe("验收证据（历史引用未归档）");
});
