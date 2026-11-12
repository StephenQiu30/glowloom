import { readFileSync, readdirSync, existsSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { dirname, join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

/** Hash the full executable tree; missing behavioral files cannot escape the pin. */
export function computeDefinitionHash(
  source: string,
  extension: '.ts' | '.js',
  lockfile: string,
): string {
  const files: string[] = [];
  function visit(directory: string) {
    for (const entry of readdirSync(directory, { withFileTypes: true })) {
      const path = join(directory, entry.name);
      if (entry.isDirectory()) visit(path);
      else if (
        entry.isFile() &&
        entry.name.endsWith(extension) &&
        !entry.name.endsWith('.d.ts')
      )
        files.push(path);
    }
  }
  visit(source);
  const hash = createHash('sha256');
  for (const path of files.sort()) {
    hash.update(relative(source, path).replaceAll('\\', '/') + '\0');
    hash.update(readFileSync(path));
    hash.update('\0');
  }
  return hash
    .update('pnpm-lock.yaml\0')
    .update(readFileSync(lockfile))
    .digest('hex');
}

const source = fileURLToPath(new URL('../', import.meta.url));
let packageRoot = dirname(source);
while (!existsSync(join(packageRoot, 'pnpm-lock.yaml'))) {
  const parent = dirname(packageRoot);
  if (parent === packageRoot)
    throw new Error('Pinned Agent dependency lock is unavailable');
  packageRoot = parent;
}
export const DEFINITION_HASH = computeDefinitionHash(
  source,
  import.meta.url.endsWith('.ts') ? '.ts' : '.js',
  join(packageRoot, 'pnpm-lock.yaml'),
);
