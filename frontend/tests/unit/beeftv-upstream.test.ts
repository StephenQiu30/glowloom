// @vitest-environment node
import { createHash } from "node:crypto";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import path from "node:path";
import ts from "typescript";
import { expect, it } from "vitest";

const root = path.resolve(
  import.meta.dirname,
  "../../../third_party/beeftv-canvas",
);
type Manifest = {
  repository: string;
  commit: string;
  roots: string[];
  tests: string[];
  files: Record<string, string>;
};
const manifest = () =>
  JSON.parse(
    readFileSync(path.join(root, "upstream.json"), "utf8"),
  ) as Manifest;

it("固定提交的四个画布目录完整迁入，包含全部405个文件", () => {
  const source = manifest();
  const expected = {
    "web/src/pages/canvas": 80,
    "web/src/components/canvas": 128,
    "web/src/lib/canvas": 192,
    "web/src/stores/canvas": 5,
  };
  expect(source.roots.slice().sort()).toEqual(Object.keys(expected).sort());
  for (const [directory, count] of Object.entries(expected)) {
    const absolute = path.join(root, directory);
    const files = readdirSync(absolute, {
      recursive: true,
      withFileTypes: true,
    })
      .filter((entry) => entry.isFile())
      .map((entry) =>
        path.relative(root, path.join(entry.parentPath, entry.name)),
      );
    expect(files, directory).toHaveLength(count);
    expect(files.slice().sort(), directory).toEqual(
      Object.keys(source.files)
        .filter((file) => file.startsWith(directory + "/"))
        .sort(),
    );
  }
});

it("完整画布基线保留固定上游源码、测试、依赖锁与许可原件", () => {
  const source = manifest();
  expect(source.repository).toBe("https://github.com/glanderness/BeefTV");
  expect(source.commit).toBe("ddaf5f9abc64b16567c28ea5399d8f2069c47832");
  for (const required of [
    "LICENSE",
    "NOTICE",
    "THIRD_PARTY_NOTICES.md",
    "web/bun.lock",
    "web/package.json",
    "web/src/pages/canvas/project.tsx",
    "web/src/components/canvas/infinite-canvas.tsx",
    "web/src/stores/canvas/use-canvas-store.ts",
  ])
    expect(source.files[required], required).toMatch(/^[a-f0-9]{64}$/);
  for (const [file, digest] of Object.entries(source.files)) {
    const absolute = path.resolve(root, file);
    expect(absolute.startsWith(root + path.sep), file).toBe(true);
    expect(path.basename(file), file).not.toMatch(/^\.env(?:\.|$)/);
    expect(
      createHash("sha256").update(readFileSync(absolute)).digest("hex"),
      file,
    ).toBe(digest);
  }
  expect(source.tests).toHaveLength(35);
  for (const file of source.tests)
    expect(source.files[file], file).toBeDefined();
});

it("迁入原件的所有静态本地模块依赖都存在，不用占位模块补齐", () => {
  const source = manifest();
  const missing: string[] = [];
  for (const file of Object.keys(source.files)) {
    if (!/\.[cm]?[jt]sx?$/.test(file)) continue;
    const absolute = path.join(root, file);
    for (const dependency of ts.preProcessFile(
      readFileSync(absolute, "utf8"),
      true,
      true,
    ).importedFiles) {
      const specifier = dependency.fileName;
      const target = specifier.startsWith("@/")
        ? path.join(root, "web/src", specifier.slice(2))
        : specifier.startsWith(".")
          ? path.resolve(path.dirname(absolute), specifier)
          : null;
      if (!target) continue;
      if (
        ![
          target,
          ...[".ts", ".tsx", ".js", ".json"].map(
            (extension) => target + extension,
          ),
          path.join(target, "index.ts"),
          path.join(target, "index.tsx"),
        ].some(existsSync)
      ) {
        missing.push(`${file}: ${specifier}`);
      }
    }
  }
  expect(missing).toEqual([]);
});
