import { Pool } from 'pg';
import pino from 'pino';
import { loadConfig } from './config.js';
import { BusinessClient } from './integrations/go-business-api/client.js';
import { SessionService } from './sessions/service.js';
import { buildApp } from './transport/http/app.js';
const config = loadConfig(process.env);
const logger = pino({
  name: 'agent',
  level: process.env.GLOWLOOM_LOG_LEVEL ?? 'info',
  base: { service: 'agent' },
  redact: {
    paths: [
      'req.headers.authorization',
      'authorization',
      'serviceToken',
      'businessToken',
      'dbDsn',
      'messages',
      'goal',
      'params',
      'manifest',
    ],
    remove: true,
  },
});
const pool = new Pool({
  connectionString: config.dbDsn,
  max: config.poolMax,
  connectionTimeoutMillis: 5000,
  idleTimeoutMillis: 30000,
  statement_timeout: 10000,
});
pool.on('error', () =>
  logger.error(
    { code: 'postgres_connection_error' },
    'Agent database connection failed',
  ),
);
const service = new SessionService(
  pool,
  new BusinessClient(config.businessOrigin, config.businessToken),
  { leaseMs: config.leaseMs },
);
const app = await buildApp({
  service,
  serviceToken: config.serviceToken,
  logger,
});
await app.listen({ host: config.host, port: config.port });
service.startRecovery();
let closing = false;
async function close() {
  if (closing) return;
  closing = true;
  logger.info('Closing agent');
  const deadline = setTimeout(() => {
    logger.error(
      { code: 'shutdown_timeout' },
      'Agent shutdown deadline exceeded',
    );
    process.exit(1);
  }, 30000);
  deadline.unref();
  try {
    await app.close();
    await pool.end();
    clearTimeout(deadline);
  } catch {
    logger.error({ code: 'shutdown_failed' }, 'Agent shutdown failed');
    process.exitCode = 1;
  }
}
process.once('SIGTERM', () => {
  void close();
});
process.once('SIGINT', () => {
  void close();
});
