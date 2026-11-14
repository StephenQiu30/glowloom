import { existsSync } from 'node:fs';
import { readFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { expect, it } from 'vitest';

it('owns the Sidebar, theme and build without sibling frontend or generated UI aliases', async () => {
  const root = resolve(import.meta.dirname, '../..');
  expect(existsSync(resolve(root, 'scripts'))).toBe(false);
  expect(existsSync(resolve(root, 'src/components/ui/sidebar.tsx'))).toBe(true);
  const tsconfig = JSON.parse(
    await readFile(resolve(root, 'tsconfig.json'), 'utf8'),
  );
  expect(
    Object.values(tsconfig.compilerOptions.paths)
      .flat()
      .some((path) => String(path).includes('.build')),
  ).toBe(false);
  const build = await readFile(resolve(root, 'src/build/service.ts'), 'utf8');
  expect(build).not.toMatch(/\.\.\/frontend|prepare-ui|shared-ui|shared-hooks/);
});
