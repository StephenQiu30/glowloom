import { execFileSync } from 'node:child_process';
import {
  mkdir,
  readFile,
  realpath,
  rename,
  writeFile,
  stat,
} from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { randomUUID } from 'node:crypto';
import { z } from 'zod';
import { parseMarkdown } from './parse';
import { digest } from './identity';

/** Only project governance documents may be imported as external knowledge. */
export const referencePaths = new Set([
  'AGENTS.md',
  'PROJECT.md',
  'DESIGN.md',
  'BACKLOG.md',
  'README.md',
  'backend/AGENTS.md',
  'frontend/AGENTS.md',
  'agent/AGENTS.md',
  'canvas/AGENTS.md',
  'workspace/README.md',
  'workspace/AGENTS.md',
]);
const referenceContract = z.object({
  version: z.literal(1),
  repoCommit: z.string(),
  resources: z.array(z.string()).default([]),
  entries: z.record(
    z.string(),
    z.object({ path: z.string(), source: z.string(), revision: z.string() }),
  ),
});
export type ReferenceSnapshot = z.infer<typeof referenceContract>;

/** Loads a verified read-only input snapshot, independent from sibling source directories. */
export async function loadReferences(
  workspaceRoot: string,
): Promise<ReferenceSnapshot | null> {
  let text: string;
  try {
    text = await readFile(
      resolve(workspaceRoot, '.local/knowledge/references.json'),
      'utf8',
    );
  } catch (error) {
    if (error instanceof Error && 'code' in error && error.code === 'ENOENT') {
      return null;
    }
    throw error;
  }
  const snapshot = referenceContract.parse(JSON.parse(text));
  for (const entry of Object.values(snapshot.entries)) {
    if (
      !referencePaths.has(entry.path) ||
      digest(entry.source) !== entry.revision
    ) {
      throw new Error('Invalid knowledge reference snapshot');
    }
  }
  return snapshot;
}

/** Imports project rules without copying business code or creating writable authorities. */
export async function synchronizeReferences(
  workspaceRoot: string,
  repositoryRoot: string,
) {
  const root = await realpath(repositoryRoot);
  const catalog = z
    .array(z.object({ id: z.string(), path: z.string() }))
    .parse(
      JSON.parse(
        await readFile(resolve(workspaceRoot, 'catalog.json'), 'utf8'),
      ),
    );
  const entries: ReferenceSnapshot['entries'] = {};
  for (const document of catalog.filter((entry) =>
    entry.path.startsWith('../'),
  )) {
    const path = document.path.slice(3);
    if (!referencePaths.has(path)) {
      throw new Error('Disallowed project reference');
    }
    const file = resolve(root, path);
    if ((await realpath(file)) !== file) {
      throw new Error('Project reference symlink escape');
    }
    const source = await readFile(file, 'utf8');
    entries[document.id] = { path, source, revision: digest(source) };
  }
  const resources = new Set<string>();
  for (const document of catalog) {
    const virtualPath = document.path.startsWith('../')
      ? resolve(root, document.path.slice(3))
      : resolve(root, 'workspace', document.path);
    const source =
      entries[document.id]?.source ??
      (await readFile(resolve(workspaceRoot, document.path), 'utf8'));
    for (const href of (await parseMarkdown(source)).links) {
      if (/^(?:[a-z][a-z\d+.-]*:|\/\/|#)/i.test(href)) {
        continue;
      }
      const target = resolve(
        dirname(virtualPath),
        decodeURIComponent(href.split(/[?#]/)[0]),
      );
      if (
        !target.startsWith(root + '/') ||
        target.startsWith(workspaceRoot + '/')
      ) {
        continue;
      }
      try {
        await stat(target);
        resources.add(target.slice(root.length + 1));
      } catch {
        /* Missing links remain errors. */
      }
    }
  }
  let repoCommit = 'unversioned';
  try {
    repoCommit = execFileSync('git', ['rev-parse', 'HEAD'], {
      cwd: root,
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore'],
    }).trim();
  } catch {
    /* Imported filesystem knowledge has no Git metadata. */
  }
  const snapshot: ReferenceSnapshot = {
    version: 1,
    repoCommit,
    entries,
    resources: [...resources].sort(),
  };
  const target = resolve(workspaceRoot, '.local/knowledge/references.json');
  await mkdir(dirname(target), { recursive: true, mode: 0o700 });
  const temporary = target + '.' + randomUUID();
  await writeFile(temporary, JSON.stringify(snapshot), {
    flag: 'wx',
    mode: 0o600,
  });
  await rename(temporary, target);
  return { references: Object.keys(entries).length, repoCommit };
}
