import assert from "node:assert/strict";
import { mkdtemp, mkdir, rm, symlink, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { afterEach, test } from "node:test";
import type { Root } from "mdast";
import { compileMdx } from "nextra/compile";
import { readAttachment } from "../lib/source-files.mjs";
import remarkSourceLinks, {
  resolveSourceLink,
} from "../lib/remark-source-links.mjs";

const fixtures: string[] = [];
afterEach(async () => {
  await Promise.all(
    fixtures
      .splice(0)
      .map((root) => rm(root, { recursive: true, force: true })),
  );
});
async function fixture() {
  const root = await mkdtemp(path.join(tmpdir(), "lanverse-attachments-"));
  fixtures.push(root);
  await mkdir(path.join(root, "workspace/content/licenses"), {
    recursive: true,
  });
  return root;
}

test("仅白名单原文可读，LICENSE字面文本和保存修改保留", async () => {
  const root = await fixture();
  const original = "Copyright <year>\r\n# 许可原文\r\n";
  await writeFile(path.join(root, "LICENSE"), original);
  assert.equal(await readAttachment("LICENSE", root), original);
  await writeFile(path.join(root, "LICENSE"), "Updated\n");
  assert.equal(await readAttachment("LICENSE", root), "Updated\n");
  await writeFile(path.join(root, "private.md"), "不得读取");
  for (const source of [
    "private.md",
    "../LICENSE",
    "/etc/passwd",
    "workspace/../LICENSE",
    "LICENSE/",
    "%2e%2e/LICENSE",
    "backend/unknown.go",
  ])
    assert.equal(await readAttachment(source, root), undefined);
});

test("白名单中的符号链接及目录符号链接被拒绝", async () => {
  const root = await fixture();
  const outside = await fixture();
  await writeFile(path.join(outside, "LICENSE"), "不得跟随");
  await symlink(path.join(outside, "LICENSE"), path.join(root, "LICENSE"));
  assert.equal(await readAttachment("LICENSE", root), undefined);
  await writeFile(path.join(outside, "beeftv.txt"), "不得跟随目录读取");
  await rm(path.join(root, "workspace/content/licenses"), { recursive: true });
  await symlink(outside, path.join(root, "workspace/content/licenses"));
  assert.equal(
    await readAttachment("workspace/content/licenses/beeftv.txt", root),
    undefined,
  );
});

test("中文与百分号Markdown路径按真实文件位置转换，index目录与hash保留", () => {
  const repositoryRoot = "/repo";
  const file = "/repo/workspace/content/design/当前.md";
  const resolve = (href: string) =>
    resolveSourceLink(file, href, { repositoryRoot });
  assert.equal(
    resolve("../operation/中文%20文件%25.md#章节"),
    "/operation/%E4%B8%AD%E6%96%87%20%E6%96%87%E4%BB%B6%25#章节",
  );
  assert.equal(resolve("../index.md#入口"), "/#入口");
  assert.equal(resolve("../licenses/"), "/licenses");
  assert.equal(resolve("#本节"), "#本节");
  assert.equal(resolve("https://example.com/page"), "https://example.com/page");
});

test("根规范和许可可读，历史Schema、CI和归档链接拒绝发布", () => {
  const file = "/repo/workspace/content/design/当前.md";
  const resolve = (href: string) =>
    resolveSourceLink(file, href, { repositoryRoot: "/repo" });
  assert.equal(
    resolve("../../../AGENTS.md#前端工程规范"),
    "/files/AGENTS%2Emd#前端工程规范",
  );
  assert.equal(resolve("../../../LICENSE"), "/files/LICENSE");
  assert.equal(
    resolve("../licenses/video-depth-anything.txt"),
    "/files/workspace/content/licenses/video-depth-anything.txt",
  );
  for (const href of [
    "../../../backend/db/schema.sql",
    "../../../.github/workflows/ci.yml",
    "../../history/engineering/PROJECT.md",
  ])
    assert.throws(() => resolve(href), /白名单/);
  assert.throws(() => resolve("../../../.env"), /白名单/);
  assert.throws(() => resolve("../../../../private.md"), /越出仓库/);
});

test("remark仅改链接与引用定义，不改正文代码或作者文本", () => {
  const tree: Root = {
    type: "root",
    children: [
      {
        type: "paragraph",
        children: [
          {
            type: "link",
            url: "../index.md",
            children: [{ type: "text", value: "入口" }],
          },
          { type: "inlineCode", value: "[样例](../../../.env)" },
        ],
      },
      { type: "definition", identifier: "source", url: "../../../BACKLOG.md" },
    ],
  };
  remarkSourceLinks({ repositoryRoot: "/repo" })(tree, {
    path: "/repo/workspace/content/design/当前.md",
  });
  assert.equal(
    (tree.children[0] as { children: { url?: string }[] }).children[0].url,
    "/",
  );
  assert.equal(
    (tree.children[1] as { url: string }).url,
    "/files/BACKLOG%2Emd",
  );
  assert.match(JSON.stringify(tree), /\[样例\]\(\.\.\/\.\.\/\.\.\/\.env\)/);
});

test("原生Nextra编译后附件Markdown扩展名仍指向原文件", async () => {
  const compiled = await compileMdx("[规范](../../../AGENTS.md#前端工程规范)", {
    filePath: "/repo/workspace/content/design/current.md",
    mdxOptions: {
      format: "md",
      remarkPlugins: [[remarkSourceLinks, { repositoryRoot: "/repo" }]],
    },
  });
  assert.match(compiled, /href: "\/files\/AGENTS%2Emd#/);
});

test("已存在的历史Schema、CI与归档原文也不提供附件读取", async () => {
  const root = await fixture();
  for (const source of [
    "backend/db/schema.sql",
    ".github/workflows/ci.yml",
    "workspace/history/engineering/PROJECT.md",
  ]) {
    const file = path.join(root, source);
    await mkdir(path.dirname(file), { recursive: true });
    await writeFile(file, "历史内容不应发布");
    assert.equal(await readAttachment(source, root), undefined);
  }
});
