import { existsSync } from 'node:fs';
import { resolve } from 'node:path';
import { startBridge } from '../server/bridge/service';
import {
  localSecret,
  startOnlineServer,
  type OnlineOptions,
} from './online-server';

/** Runs the controlled host bridge independently from the website. */
export async function startLocalBridge(
  workspaceRoot: string,
  repositoryRoot: string,
) {
  const token = await localSecret(workspaceRoot, 'bridge-token');
  const socket = resolve(workspaceRoot, '.local/bridge.sock');
  const appBinary =
    '/Applications/ChatGPT.app/Contents/Resources/codex-cli/CodexCLI.app/Contents/MacOS/codex';
  const bridge = await startBridge({
    repo: repositoryRoot,
    storage: resolve(workspaceRoot, '.local/bridge'),
    socket,
    token,
    codexBinary:
      process.env.GLOWLOOM_WORKSPACE_CODEX_BINARY ??
      (existsSync(appBinary) ? appBinary : 'codex'),
  });
  return { socket, token, close: bridge.close };
}
/** Local composition requires an explicit repository; deployment starts the site alone. */
export async function startDevelopment(
  options: OnlineOptions & { readonly repositoryRoot: string },
) {
  const bridge = await startLocalBridge(
    options.workspaceRoot,
    options.repositoryRoot,
  );
  try {
    const online = await startOnlineServer({ ...options, bridge });
    let stopped = false;
    return {
      process: online.process,
      async close() {
        if (stopped) {
          return;
        }
        stopped = true;
        online.close();
        await bridge.close();
      },
    };
  } catch (error) {
    await bridge.close();
    throw error;
  }
}
