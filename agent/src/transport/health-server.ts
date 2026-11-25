import { createServer, type Server } from 'node:http';
import type { AgentApplication } from '../sessions/types.js';

export interface HealthServer {
  address: string;
  markDraining(): void;
  close(): Promise<void>;
}

export async function startHealthServer(options: {
  host: string;
  port: number;
  service: Pick<AgentApplication, 'ready'>;
}): Promise<HealthServer> {
  let draining = false;
  const server: Server = createServer((req, res) => {
    const pathname = (req.url ?? '').split('?', 1)[0];
    if (req.method === 'GET' && pathname === '/healthz') {
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ status: 'ok', service: 'agent' }));
      return;
    }
    if (req.method === 'GET' && pathname === '/readyz') {
      void (async () => {
        const ready = !draining && (await options.service.ready());
        res.writeHead(ready ? 200 : 503, {
          'Content-Type': 'application/json',
        });
        res.end(JSON.stringify({ ready }));
      })();
      return;
    }
    res.writeHead(404, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ code: 'not_found' }));
  });

  await new Promise<void>((resolve, reject) => {
    server.once('error', reject);
    server.listen(options.port, options.host, () => resolve());
  });

  const bound = server.address();
  const address =
    typeof bound === 'object' && bound
      ? `${bound.address}:${bound.port}`
      : `${options.host}:${options.port}`;

  return {
    address,
    markDraining() {
      draining = true;
    },
    close() {
      return new Promise<void>((resolve) => {
        server.close(() => resolve());
      });
    },
  };
}
