import { mkdtemp, mkdir, writeFile, symlink, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, expect, it } from "vitest";
import { loadSnapshot, validateLinks } from "../../src/content/catalog";

const roots: string[] = [];
async function fixture() {
  const root = await mkdtemp(join(tmpdir(), "workspace-content-"));
  roots.push(root);
  await mkdir(join(root, "workspace/content"), { recursive: true });
  await writeFile(
    join(root, "workspace/content/a.md"),
    "# 需求\n\n[设计](b.md#方案)\n\nFR-WS-01 内容。",
  );
  await writeFile(
    join(root, "workspace/content/b.md"),
    "# 设计\n\n## 方案\n\n唯一来源：[需求](a.md)",
  );
  await writeFile(
    join(root, "workspace/catalog.json"),
    JSON.stringify([
      {
        id: "requirement-1",
        path: "content/a.md",
        visibility: "public",
        aliases: ["docs/a.md"],
      },
      {
        id: "design-1",
        path: "content/b.md",
        visibility: "internal",
        aliases: ["docs/b.md"],
      },
    ]),
  );
  return root;
}
afterEach(async () => {
  await Promise.all(
    roots.splice(0).map((p) => rm(p, { recursive: true, force: true })),
  );
});

it("freezes stable identities, resolves local links and detects stale source revisions", async () => {
  const root = await fixture();
  const first = await loadSnapshot({
    workspaceRoot: join(root, "workspace"),
    scope: "internal",
  });
  expect(first.documents.map((d) => d.id)).toEqual([
    "requirement-1",
    "design-1",
  ]);
  expect(first.documents[0].html).toContain(
    "/read/design-1/#%E6%96%B9%E6%A1%88",
  );
  expect(await validateLinks(first)).toEqual([]);
  await writeFile(
    join(root, "workspace/content/b.md"),
    "# 修改\n\n## 方案\n内容改变",
  );
  const next = await loadSnapshot({
    workspaceRoot: join(root, "workspace"),
    scope: "internal",
  });
  expect(next.contentDigest).not.toBe(first.contentDigest);
  expect(next.documents[0].revision).toBe(first.documents[0].revision);
});
it("filters private documents before reading or parsing them for a public release", async () => {
  const root = await fixture();
  await rm(join(root, "workspace/content/b.md"));
  const snapshot = await loadSnapshot({
    workspaceRoot: join(root, "workspace"),
    scope: "public",
  });
  expect(snapshot.documents).toHaveLength(1);
  expect(JSON.stringify(snapshot)).not.toContain("design-1");
  expect(snapshot.documents[0].html).not.toContain("b.md");
});
it("rejects duplicate identities and symlink escapes rather than copying private files", async () => {
  const root = await fixture();
  await writeFile(join(root, "private.md"), "PRIVATE");
  await rm(join(root, "workspace/content/a.md"));
  await symlink(join(root, "private.md"), join(root, "workspace/content/a.md"));
  await expect(
    loadSnapshot({ workspaceRoot: join(root, "workspace"), scope: "internal" }),
  ).rejects.toThrow(/path|路径/);
});
it("reports a missing local anchor at the originating document", async () => {
  const root = await fixture();
  await writeFile(
    join(root, "workspace/content/a.md"),
    "# 需求\n[设计](b.md#missing)",
  );
  const snapshot = await loadSnapshot({
    workspaceRoot: join(root, "workspace"),
    scope: "internal",
  });
  expect(await validateLinks(snapshot)).toEqual([
    expect.objectContaining({
      documentId: "requirement-1",
      target: "b.md#missing",
      reason: "anchor_missing",
    }),
  ]);
});

it("accepts an existing engineering directory reference without exporting its source", async () => {
  const root = await fixture();
  await mkdir(join(root, "frontend"));
  await writeFile(
    join(root, "workspace/content/a.md"),
    "# 入口\n[工程](../../frontend/)",
  );
  const snapshot = await loadSnapshot({
    workspaceRoot: join(root, "workspace"),
    scope: "internal",
  });
  expect(await validateLinks(snapshot)).toEqual([]);
  expect(snapshot.documents[0].html).toContain("/engineering/?path=frontend");
});
it("removes restricted reference labels and refuses raw HTML in public documents", async () => {
  const root = await fixture();
  const p = join(root, "workspace/content/a.md");
  await writeFile(
    p,
    "# Public\n\n[Confidential-title][secret]\n\n[secret]: b.md\n",
  );
  const s = await loadSnapshot({
    workspaceRoot: join(root, "workspace"),
    scope: "public",
  });
  expect(JSON.stringify(s)).not.toContain("Confidential-title");
  expect(JSON.stringify(s)).not.toContain("b.md");
  await writeFile(p, '# Public\n\n<a href="b.md">Confidential-title</a>');
  await expect(
    loadSnapshot({ workspaceRoot: join(root, "workspace"), scope: "public" }),
  ).rejects.toThrow(/public.*HTML/i);
});
it("invalidates the snapshot when a document alias changes without changing its text", async () => {
  const root = await fixture(),
    path = join(root, "workspace/catalog.json");
  const first = await loadSnapshot({
    workspaceRoot: join(root, "workspace"),
    scope: "internal",
  });
  const rows = JSON.parse(
    await (await import("node:fs/promises")).readFile(path, "utf8"),
  );
  rows[0].aliases.push("docs/new.md");
  await writeFile(path, JSON.stringify(rows));
  expect(
    (
      await loadSnapshot({
        workspaceRoot: join(root, "workspace"),
        scope: "internal",
      })
    ).contentDigest,
  ).not.toBe(first.contentDigest);
});
it("rejects machine-absolute knowledge links even when the file exists locally", async () => {
  const root = await fixture();
  const absolute = join(root, "workspace/content/b.md");
  await writeFile(
    join(root, "workspace/content/a.md"),
    "# Entry\n\n[Local evidence](" + absolute + ")",
  );
  const snapshot = await loadSnapshot({
    workspaceRoot: join(root, "workspace"),
    scope: "internal",
  });
  expect(await validateLinks(snapshot)).toEqual([
    expect.objectContaining({
      documentId: "requirement-1",
      target: absolute,
      reason: "nonportable_local_link",
    }),
  ]);
});
