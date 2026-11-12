import { Pool } from 'pg';
import pino from 'pino';
import { loadConfig } from './config.js';
import { BusinessClient } from './business/client.js';
import { SessionService } from './sessions/service.js';
import { buildApp } from './http/app.js';
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
  {
    leaseMs: config.leaseMs,
    deliveryPollMs: config.deliveryPollMs,
    deliveryTimeoutMs: config.deliveryTimeoutMs,
  },
);
const app = await buildApp({
  service,
  serviceToken: config.serviceToken,
  logger,
  swagger: config.swagger,
}).catch(async (error: unknown) => {
  await pool.end();
  throw error;
});

let closing: Promise<void> | undefined;
function close(): Promise<void> {
  closing ??= (async () => {
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
      await app.close().finally(() => pool.end());
    } finally {
      clearTimeout(deadline);
    }
  })();
  return closing;
}
function onSignal() {
  void close().catch(() => {
    logger.error({ code: 'shutdown_failed' }, 'Agent shutdown failed');
    process.exitCode = 1;
  });
}
process.on('SIGTERM', onSignal);
process.on('SIGINT', onSignal);

try {
  await app.listen(config.port, config.host);
  if (!closing) service.startRecovery();
} catch (error) {
  await close();
  throw error;
}
