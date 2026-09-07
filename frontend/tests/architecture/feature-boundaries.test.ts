import { readdirSync, readFileSync, existsSync } from "node:fs";
import { dirname, relative, resolve } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";

const sourceRoot = resolve(import.meta.dirname, "../../src");

function sourceFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = resolve(directory, entry.name);
    return entry.isDirectory() ? sourceFiles(path) : /\.tsx?$/.test(entry.name) && !entry.name.endsWith(".d.ts") ? [path] : [];
  });
}

function dependencies(file: string): string[] {
  const source = ts.createSourceFile(file, readFileSync(file, "utf8"), ts.ScriptTarget.Latest, true);
  return source.statements.flatMap((statement) => {
    if ((!ts.isImportDeclaration(statement) && !ts.isExportDeclaration(statement)) || !statement.moduleSpecifier || !ts.isStringLiteral(statement.moduleSpecifier)) return [];
    const name = statement.moduleSpecifier.text;
    const base = name.startsWith("@/") ? resolve(sourceRoot, name.slice(2)) : name.startsWith(".") ? resolve(dirname(file), name) : undefined;
    if (!base) return [];
    const path = [base, `${base}.ts`, `${base}.tsx`, `${base}/index.ts`, `${base}/index.tsx`].find((candidate) => existsSync(candidate) && /\.tsx?$/.test(candidate));
    return path && !path.endsWith(".d.ts") ? [path] : [];
  });
}

describe("feature ownership", () => {
  it("keeps business implementations outside Next.js routes", () => {
    const permitted = new Set([
      "page.tsx", "layout.tsx", "providers.tsx", "not-found.tsx", "loading.tsx",
      "error.tsx", "global-error.tsx", "template.tsx", "default.tsx", "route.ts",
    ]);
    const misplaced = sourceFiles(resolve(sourceRoot, "app")).filter((file) => !permitted.has(file.split("/").at(-1)!));
    expect(misplaced.map((file) => relative(sourceRoot, file))).toEqual([]);
  });

  it("keeps shared state independent and cross-feature imports on public boundaries", () => {
    const violations: string[] = [];
    for (const file of sourceFiles(sourceRoot)) {
      const owner = relative(sourceRoot, file);
      for (const dependency of dependencies(file)) {
        const target = relative(sourceRoot, dependency);
        const sourceFeature = owner.startsWith("features/") ? owner.split("/")[1] : undefined;
        const targetFeature = target.startsWith("features/") ? target.split("/")[1] : undefined;
        if (owner.startsWith("lib/") && targetFeature) violations.push(`${owner} -> ${target}`);
        if (sourceFeature && target.startsWith("app/")) violations.push(`${owner} -> ${target}`);
        if (sourceFeature && targetFeature && sourceFeature !== targetFeature && !/^features\/[^/]+\/(endpoints\.ts|[a-z-]+-workspace\.tsx)$/.test(target)) violations.push(`${owner} -> ${target}`);
        if (owner.startsWith("components/ui/") && (targetFeature || target.startsWith("api/"))) violations.push(`${owner} -> ${target}`);
        if (owner === "lib/server-state.ts" && target.startsWith("api/")) violations.push(`${owner} -> ${target}`);
      }
    }
    expect(violations).toEqual([]);
  });

  it("has no local import cycles after endpoint injection", () => {
    const graph = new Map(sourceFiles(sourceRoot).map((file) => [file, dependencies(file)]));
    const complete = new Set<string>();
    const active: string[] = [];
    const cycles: string[] = [];
    function visit(file: string) {
      if (active.includes(file)) {
        cycles.push([...active.slice(active.indexOf(file)), file].map((path) => relative(sourceRoot, path)).join(" -> "));
        return;
      }
      if (complete.has(file)) return;
      active.push(file);
      for (const target of graph.get(file) ?? []) visit(target);
      active.pop();
      complete.add(file);
    }
    for (const file of graph.keys()) visit(file);
    expect(cycles).toEqual([]);
  });
});
