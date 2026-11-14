import { readFile, readdir, stat } from "node:fs/promises";
import { resolve, dirname, extname } from "node:path";
import ts from "typescript";

export async function assertStaticBoundary(
  workspaceRoot: string,
): Promise<void> {
  const seen = new Set<string>();
  async function inspect(file: string): Promise<void> {
    if (seen.has(file)) return;
    seen.add(file);
    const source = await readFile(file, "utf8");
    const tree = ts.createSourceFile(
      file,
      source,
      ts.ScriptTarget.Latest,
      true,
      extname(file) === ".tsx" ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
    );
    for (const node of tree.statements) {
      if (
        ts.isExpressionStatement(node) &&
        ts.isStringLiteral(node.expression) &&
        node.expression.text === "use server"
      )
        throw new Error(`Static online boundary: ${file}`);
      if (
        !ts.isImportDeclaration(node) ||
        !ts.isStringLiteral(node.moduleSpecifier) ||
        node.importClause?.isTypeOnly
      )
        continue;
      const name = node.moduleSpecifier.text;
      if (
        /^(?:pg(?:\/|$)|next\/(?:headers|server)$|@\/server\/|@\/online\/)/.test(
          name,
        ) ||
        name.includes("/server/")
      )
        throw new Error(`Static online dependency in ${file}: ${name}`);
      if (!name.startsWith(".") && !name.startsWith("@/")) continue;
      if (extname(name) && !/[.]tsx?$/.test(name)) continue;
      const target = name.startsWith("@/")
        ? resolve(workspaceRoot, "src", name.slice(2))
        : resolve(dirname(file), name);
      for (const suffix of extname(target)
        ? [""]
        : [".ts", ".tsx", "/index.ts", "/index.tsx"]) {
        try {
          if ((await stat(target + suffix)).isFile()) {
            await inspect(target + suffix);
            break;
          }
        } catch (error) {
          if (
            !["ENOENT", "ENOTDIR"].includes(
              (error as NodeJS.ErrnoException).code ?? "",
            )
          )
            throw error;
        }
      }
    }
  }
  async function walk(dir: string): Promise<void> {
    for (const entry of await readdir(dir, { withFileTypes: true })) {
      const p = resolve(dir, entry.name);
      if (entry.isDirectory()) await walk(p);
      else if (/\.tsx?$/.test(p)) await inspect(p);
    }
  }
  for (const dir of ["src/app", "src/components"]) {
    try {
      await walk(resolve(workspaceRoot, dir));
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error;
    }
  }
}
