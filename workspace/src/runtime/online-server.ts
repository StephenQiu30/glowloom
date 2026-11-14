import { spawn } from 'node:child_process';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { randomBytes } from 'node:crypto';
import { resolve } from 'node:path';
import { verifyRelease } from '../build/release';

export interface OnlineOptions {
  readonly workspaceRoot: string;
  readonly port: number;
  readonly host?: string;
  readonly bridge?: { socket: string; token: string };
}
/** Returns a private local credential without printing it or replacing existing data. */
export async function localSecret(
  workspaceRoot: string,
  name: 'access-token' | 'bridge-token',
) {
  const local = resolve(workspaceRoot, '.local');
  await mkdir(local, { recursive: true, mode: 0o700 });
  const path = resolve(local, name);
  try {
    return (await readFile(path, 'utf8')).trim();
  } catch (error) {
    if (
      !(error instanceof Error) ||
      !('code' in error) ||
      error.code !== 'ENOENT'
    ) {
      throw error;
    }
    const token = randomBytes(32).toString('hex');
    await writeFile(path, token + '\n', { flag: 'wx', mode: 0o600 });
    return token;
  }
}
/** Starts only the deployable site. Repository execution is an explicitly connected capability. */
export async function startOnlineServer(options: OnlineOptions) {
  const { releaseId } = JSON.parse(
    await readFile(
      resolve(options.workspaceRoot, 'dist/online/current.json'),
      'utf8',
    ),
  );
  if (typeof releaseId !== 'string' || !/^[\w-]+$/.test(releaseId)) {
    throw new Error('Invalid selected release');
  }
  const releaseRoot = resolve(options.workspaceRoot, 'dist/online', releaseId);
  const release = await verifyRelease(releaseRoot, 'online');
  if (typeof release.entry !== 'string' || release.entry.includes('..')) {
    throw new Error('Invalid online entry');
  }
  const origin =
    process.env.GLOWLOOM_WORKSPACE_ORIGIN ?? `http://127.0.0.1:${options.port}`;
  const secret =
    process.env.GLOWLOOM_WORKSPACE_SESSION_SECRET ??
    (await localSecret(options.workspaceRoot, 'access-token'));
  const child = spawn(process.execPath, [resolve(releaseRoot, release.entry)], {
    cwd: releaseRoot,
    stdio: 'inherit',
    env: {
      ...process.env,
      NODE_ENV: 'production',
      PORT: String(options.port),
      HOSTNAME: options.host ?? '127.0.0.1',
      GLOWLOOM_WORKSPACE_ORIGIN: origin,
      GLOWLOOM_WORKSPACE_SESSION_SECRET: secret,
      GLOWLOOM_WORKSPACE_BRIDGE_SOCKET:
        options.bridge?.socket ??
        process.env.GLOWLOOM_WORKSPACE_BRIDGE_SOCKET ??
        '',
      GLOWLOOM_WORKSPACE_BRIDGE_TOKEN:
        options.bridge?.token ??
        process.env.GLOWLOOM_WORKSPACE_BRIDGE_TOKEN ??
        '',
    },
  });
  console.log(
    `Online workspace: ${origin}${release.basePath}/; real TTS disabled`,
  );
  console.log(
    `Local credential: ${resolve(options.workspaceRoot, '.local/access-token')} (not printed)`,
  );
  return {
    process: child,
    close() {
      child.kill('SIGTERM');
    },
  };
}
