import { parseArgs } from 'node:util';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const workspaceRoot = resolve(fileURLToPath(new URL('..', import.meta.url)));
const { positionals, values } = parseArgs({
  allowPositionals: true,
  options: {
    public: { type: 'boolean' },
    authorized: { type: 'boolean' },
    repository: { type: 'string' },
    port: { type: 'string' },
    host: { type: 'string' },
  },
});
const [command, argument, identity] = positionals;
const port = Number(values.port ?? process.env.WORKSPACE_PORT ?? '1130');
if (!Number.isInteger(port) || port < 1 || port > 65535) {
  throw new Error('Invalid port');
}
async function main() {
  switch (command) {
    case 'build': {
      if (argument !== 'static' && argument !== 'online') {
        throw new Error('Expected static or online');
      }
      const { buildWorkspace } = await import('./build/service');
      await buildWorkspace({
        workspaceRoot,
        mode: argument,
        scope: values.public ? 'public' : 'internal',
        basePath: process.env.WORKSPACE_BASE_PATH,
      });
      return;
    }
    case 'check': {
      const { validateContent } = await import('./content/validation');
      console.log(
        await validateContent(
          workspaceRoot,
          values.public ? 'public' : 'internal',
        ),
      );
      return;
    }
    case 'knowledge': {
      if (!values.repository) {
        throw new Error('Knowledge sync requires --repository <path>');
      }
      const { synchronizeReferences } = await import('./content/references');
      console.log(
        await synchronizeReferences(workspaceRoot, resolve(values.repository)),
      );
      return;
    }
    case 'select': {
      if ((argument !== 'static' && argument !== 'online') || !identity) {
        throw new Error('Expected static|online <release-id>');
      }
      const { selectRelease } = await import('./build/release');
      await selectRelease(
        resolve(workspaceRoot, 'dist', argument),
        identity,
        argument,
      );
      return;
    }
    case 'narration': {
      if (!argument) {
        throw new Error('Expected manifest path');
      }
      const { importNarration } = await import('./narration/import');
      console.log(
        await importNarration({
          workspaceRoot,
          manifestPath: resolve(argument),
          authorized: values.authorized ?? false,
        }),
      );
      return;
    }
    case 'search': {
      const { synchronizeSearch } = await import('./search/sync');
      console.log(await synchronizeSearch(workspaceRoot));
      return;
    }
    case 'serve': {
      const { startStaticServer } = await import('./runtime/static-server');
      const server = await startStaticServer({
        workspaceRoot,
        port,
        host: values.host,
      });
      process.once('SIGTERM', () => server.close());
      process.once('SIGINT', () => server.close());
      return;
    }
    case 'online':
    case 'dev': {
      const service =
        command === 'dev' && values.repository
          ? await (
              await import('./runtime/development')
            ).startDevelopment({
              workspaceRoot,
              port,
              host: values.host,
              repositoryRoot: resolve(values.repository),
            })
          : await (
              await import('./runtime/online-server')
            ).startOnlineServer({ workspaceRoot, port, host: values.host });
      process.once('SIGTERM', () => void service.close());
      process.once('SIGINT', () => void service.close());
      service.process.once('exit', (code, signal) => {
        process.exitCode =
          code ?? (signal === 'SIGTERM' || signal === 'SIGINT' ? 0 : 1);
        void service.close();
      });
      return;
    }
    case 'bridge': {
      if (!values.repository) {
        throw new Error('Bridge requires --repository <path>');
      }
      const { startLocalBridge } = await import('./runtime/development');
      const service = await startLocalBridge(
        workspaceRoot,
        resolve(values.repository),
      );
      process.once('SIGTERM', () => void service.close());
      process.once('SIGINT', () => void service.close());
      return;
    }
    default:
      throw new Error(
        'Expected build, check, knowledge, select, narration, serve, online, dev or bridge',
      );
  }
}
await main();
