// @vitest-environment node
import { readFileSync, existsSync, readdirSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, resolve, relative, isAbsolute } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";

const require = createRequire(import.meta.url);
const root = resolve(import.meta.dirname, "../../..");

const frontendRoot = resolve(root, "frontend");
const editorRoot = resolve(frontendRoot, "src/components/canvas-editor");
const editorEntry = resolve(editorRoot, "editor/canvas-editor.tsx");

describe("Canvas 编辑器归属与运行时边界", () => {
  it("前端持有唯一编辑器与样式", () => {
    const css = resolve(editorRoot, "canvas.css");
    expect(readFileSync(editorEntry, "utf8")).toContain("function CanvasPage");
    expect(readFileSync(css, "utf8")).toContain("canvas-controls-layout");
    expect(
      existsSync(
        resolve(
          root,
          "frontend/src/components/canvas/editor/canvas-editor.tsx",
        ),
      ),
    ).toBe(false);
  });

  it("协作服务与浏览器编辑器使用分开的编译入口", () => {
    const config = JSON.parse(
      readFileSync(resolve(root, "canvas/tsconfig.json"), "utf8"),
    );
    expect(config.include).toEqual(["src"]);
    expect(require.resolve("@glowloom/canvas/session")).toMatch(
      /collaboration\/session\.js$/,
    );
    expect(require.resolve("@glowloom/canvas/model")).toMatch(
      /document\/model\.js$/,
    );
  });

  it("编辑器与宿主复用 React、Tiptap、ProseMirror 和 Yjs 实例", () => {
    const editorRequire = createRequire(editorEntry);
    for (const dependency of [
      "react",
      "react-dom",
      "@tiptap/core",
      "@tiptap/pm/state",
      "yjs",
    ]) {
      expect(editorRequire.resolve(dependency), dependency).toBe(
        require.resolve(dependency),
      );
    }
  });

  it("浏览器源码只通过宿主边界接入应用，不能导入页面或服务端入口", () => {
    const manifest = JSON.parse(
      readFileSync(resolve(frontendRoot, "package.json"), "utf8"),
    );
    const dependencies = {
      ...manifest.dependencies,
      ...manifest.peerDependencies,
    };
    const files = readdirSync(editorRoot, {
      recursive: true,
      withFileTypes: true,
    })
      .filter((entry) => entry.isFile() && /\.tsx?$/.test(entry.name))
      .map((entry) => resolve(entry.parentPath, entry.name));
    expect(files.length).toBeGreaterThan(200);
    for (const file of files) {
      const source = ts.createSourceFile(
        file,
        readFileSync(file, "utf8"),
        ts.ScriptTarget.Latest,
        true,
      );
      const visit = (node: ts.Node) => {
        const moduleSpecifier =
          ts.isImportDeclaration(node) || ts.isExportDeclaration(node)
            ? node.moduleSpecifier
            : ts.isCallExpression(node) &&
                node.expression.kind === ts.SyntaxKind.ImportKeyword
              ? node.arguments[0]
              : ts.isImportTypeNode(node) && ts.isLiteralTypeNode(node.argument)
                ? node.argument.literal
                : undefined;
        if (moduleSpecifier && ts.isStringLiteral(moduleSpecifier)) {
          const specifier = moduleSpecifier.text;
          if (specifier.startsWith("@public/")) {
            const assetRoot = resolve(frontendRoot, "public");
            const asset = resolve(
              assetRoot,
              specifier.slice("@public/".length),
            );
            expect(relative(assetRoot, asset).startsWith(".."), file).toBe(
              false,
            );
            expect(existsSync(asset), file).toBe(true);
          } else if (
            !specifier.startsWith(".") &&
            !specifier.startsWith("@/")
          ) {
            const dependency = specifier.startsWith("@")
              ? specifier.split("/").slice(0, 2).join("/")
              : specifier.split("/")[0];
            expect(
              dependencies[dependency],
              `${file}: ${dependency}`,
            ).toBeTruthy();
          }
          expect(specifier, file).not.toMatch(
            /^(?:@\/(?:app|server)\/|node:|@nestjs\/|fastify|reflect-metadata|server-only)/,
          );
          if (specifier.startsWith(".")) {
            const path = relative(
              resolve(frontendRoot, "src"),
              resolve(dirname(file), specifier),
            );
            expect(path.startsWith("..") || isAbsolute(path), file).toBe(false);
            expect(path, file).not.toMatch(/^(?:app|server)\//);
          }
          if (specifier.startsWith("@glowloom/canvas/")) {
            expect(specifier, file).toMatch(
              /\/(?:model|rich-text|service-document|session|wire|journal|recovery)$/,
            );
          }
        }
        ts.forEachChild(node, visit);
      };
      visit(source);
    }
  });
});
