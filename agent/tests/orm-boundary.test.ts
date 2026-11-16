import { readdir, readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { join, relative } from 'node:path';
import ts from 'typescript';
import { expect, it } from 'vitest';

const sourceRoot = fileURLToPath(new URL('../src/', import.meta.url));

async function sourceFiles(root: string): Promise<string[]> {
  const entries = await readdir(root, { withFileTypes: true });
  const nested = await Promise.all(
    entries.map((entry) => {
      const path = join(root, entry.name);
      return entry.isDirectory()
        ? sourceFiles(path)
        : Promise.resolve(entry.name.endsWith('.ts') ? [path] : []);
    }),
  );
  return nested.flat();
}

it('keeps application database operations in ORM builders without schema executors', async () => {
  const violations: string[] = [];
  const statement =
    /^(?:select\s+|with(?:\s+recursive)?\s+\w+\s+as\s*\(|insert\s+into\s+|update\s+[\w".]+\s+set\s+|delete\s+from\s+|(?:create|alter|drop)\s+(?:table|schema|index)\b|(?:begin|commit|rollback)\s*;?\s*$|(?:savepoint|release\s+savepoint)\s+\w+\s*;?\s*$)/i;
  for (const path of await sourceFiles(sourceRoot)) {
    const body = await readFile(path, 'utf8');
    const source = ts.createSourceFile(
      path,
      body,
      ts.ScriptTarget.Latest,
      true,
    );
    const reject = (node: ts.Node, reason: string) => {
      const line =
        source.getLineAndCharacterOfPosition(node.getStart()).line + 1;
      violations.push(`${relative(sourceRoot, path)}:${line}: ${reason}`);
    };
    const visit = (node: ts.Node) => {
      if (
        ts.isImportDeclaration(node) &&
        ts.isStringLiteral(node.moduleSpecifier)
      ) {
        if (/drizzle-kit|\/migrator(?:\/|$)/.test(node.moduleSpecifier.text))
          reject(node, 'runtime schema/migration executor');
      }
      if (
        ts.isStringLiteral(node) ||
        ts.isNoSubstitutionTemplateLiteral(node) ||
        ts.isTemplateHead(node)
      ) {
        if (statement.test(node.text.trimStart()))
          reject(node, 'handwritten SQL statement');
      }
      if (
        ts.isCallExpression(node) &&
        ts.isPropertyAccessExpression(node.expression)
      ) {
        if (node.expression.name.text === 'query')
          reject(node, 'native query escape hatch');
        if (
          node.expression.name.text === 'raw' &&
          node.expression.expression.getText(source) === 'sql'
        )
          reject(node, 'unparameterized SQL escape hatch');
      }
      ts.forEachChild(node, visit);
    };
    visit(source);
  }
  expect(violations).toEqual([]);
});
