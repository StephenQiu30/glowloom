import {
  readFile,
  realpath,
  mkdir,
  cp,
  stat,
  rename,
  rm,
  writeFile,
} from 'node:fs/promises';
import { resolve, dirname } from 'node:path';
import { execFileSync } from 'node:child_process';
import { loadSnapshot } from '../content/catalog';
import { digest } from '../content/identity';
import {
  validateManifest,
  type NarrationManifest,
} from '../narration/manifest';
/** Imports authorized existing audio after validating source, bytes and measured duration. */
export async function importNarration(options: {
  workspaceRoot: string;
  manifestPath: string;
  authorized: boolean;
}) {
  const workspace = options.workspaceRoot;
  if (!options.authorized) {
    throw new Error(
      'Confirm audio rights with --authorized; synthesis stays disabled',
    );
  }
  const input = await realpath(resolve(options.manifestPath));
  const manifest = JSON.parse(
    await readFile(input, 'utf8'),
  ) as NarrationManifest;
  const snapshot = await loadSnapshot({
    workspaceRoot: workspace,
    scope: 'internal',
  });
  const doc = snapshot.documents.find((d) => d.id === manifest.documentId);
  if (!doc) {
    throw new Error('Unknown document');
  }
  validateManifest(manifest, doc);
  const final = resolve(workspace, '.local/narration', doc.id);
  const output = final + '.import-' + process.pid;
  await mkdir(output, { recursive: true, mode: 0o700 });
  for (const segment of manifest.segments) {
    const path = resolve(dirname(input), segment.src);
    if ((await realpath(path)) !== path) {
      throw new Error('Audio path escape');
    }
    if ((await stat(path)).size > 100_000_000) {
      throw new Error('Audio too large');
    }
    const bytes = await readFile(path);
    if (bytes.byteLength > 100_000_000 || digest(bytes) !== segment.sha256) {
      throw new Error('Audio size or digest mismatch');
    }
    const duration =
      Number(
        execFileSync(
          'ffprobe',
          [
            '-protocol_whitelist',
            'file,pipe',
            '-v',
            'error',
            '-show_entries',
            'format=duration',
            '-of',
            'default=noprint_wrappers=1:nokey=1',
            path,
          ],
          { encoding: 'utf8' },
        ).trim(),
      ) * 1000;
    if (
      !Number.isFinite(duration) ||
      Math.abs(duration - segment.durationMs) > 150
    ) {
      throw new Error('Declared duration does not match actual audio');
    }
    await mkdir(dirname(resolve(output, segment.src)), { recursive: true });
    await cp(path, resolve(output, segment.src));
  }
  await writeFile(
    resolve(output, 'manifest.json'),
    JSON.stringify(manifest, null, 2),
    { flag: 'wx', mode: 0o600 },
  );
  const previous = final + '.previous-' + process.pid;
  try {
    await rename(final, previous);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== 'ENOENT') {
      throw error;
    }
  }
  try {
    await rename(output, final);
  } catch (error) {
    await rename(previous, final).catch(() => {});
    throw error;
  }
  await rm(previous, { recursive: true, force: true });
  console.log(
    'Imported authorized existing narration. Rebuild the matching release to publish it; no TTS called.',
  );

  return { documentId: doc.id, directory: final };
}
