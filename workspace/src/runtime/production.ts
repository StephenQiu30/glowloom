import { readFile } from 'node:fs/promises';
import { resolve, dirname } from 'node:path';
import { pathToFileURL } from 'node:url';

/** The production process loads only its sealed release and compiled Next.js entry. */
const release: unknown = JSON.parse(await readFile('release.json', 'utf8'));
if (
  !release ||
  typeof release !== 'object' ||
  !('mode' in release) ||
  release.mode !== 'online' ||
  !('entry' in release) ||
  typeof release.entry !== 'string' ||
  release.entry.includes('..') ||
  release.entry.startsWith('/')
) {
  throw new Error('Invalid online release');
}
const entry = resolve(release.entry);
process.chdir(dirname(entry));
await import(pathToFileURL(entry).href);
