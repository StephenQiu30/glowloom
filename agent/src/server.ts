import { Pool } from 'pg';
import { disconnectDatabase } from './database/connection.js';
import { loadConfig } from './config.js';
import { BusinessClient } from './business/client.js';
import { SessionService } from './sessions/service.js';
import { buildApp } from './transport/app.js';
import {
  startAgentGrpcServer,
  type AgentGrpcServer,
} from './transport/grpc-server.js';
import { createAgentLogger } from './observability/logger.js';
import { setSpanSink } from './observability/otel.js';

const config = loadConfig(process.env);
const logger = createAgentLogger();
setSpanSink((span) => {
  logger.debug({ span }, 'agent.span');
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
    onRecoveryEvent: (event) =>
      logger.warn({ recovery: event }, 'Agent recovery requires attention'),
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
  await disconnectDatabase(pool).finally(() => pool.end());
  throw error;
});

let grpcServer: AgentGrpcServer | undefined;
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
      await (grpcServer ? grpcServer.close() : Promise.resolve())
        .finally(() => app.close())
        .finally(() => disconnectDatabase(pool).finally(() => pool.end()));
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
  const grpcAddr = process.env.GLOWLOOM_AGENT_GRPC_ADDR ?? '0.0.0.0:4402';
  grpcServer = await startAgentGrpcServer({
    app,
    service,
    address: grpcAddr,
    serviceToken: config.serviceToken,
  });
  if (!closing) service.startRecovery();
} catch (error) {
  await close();
  throw error;
}
