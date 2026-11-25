import { loadConfig } from './config.js';
import { createDatabaseBundle } from './database/connection.js';
import { PipelineRegistry } from './mastra/pipeline-engine.js';
import { createMastraRuntime } from './mastra/runtime.js';
import { PromptRegistry } from './observability/langfuse.js';
import { createAgentLogger } from './observability/logger.js';
import { createEventBus } from './sessions/event-bus.js';
import { SessionService } from './sessions/service.js';
import { SessionStore } from './sessions/store.js';
import { SkillRegistry } from './skills/loader.js';
import {
  startAgentGrpcServer,
  type AgentGrpcServer,
} from './transport/grpc-server.js';
import {
  startHealthServer,
  type HealthServer,
} from './transport/health-server.js';

const config = loadConfig(process.env);
const logger = createAgentLogger(config.logLevel);
const dbBundle = createDatabaseBundle(config.dbDsn, config.poolMax);
const eventBus = createEventBus(config.redisUrl, logger);
const prompts = new PromptRegistry(config.langfuse, logger);
const skills = new SkillRegistry();
await skills.reload();
const pipelines = new PipelineRegistry();
await pipelines.reload();

const agentTeam = await createMastraRuntime({
  storage: dbBundle.mastraStore,
  gatewayConfig: {
    businessOrigin: config.businessOrigin,
    businessToken: config.businessToken,
    defaultModel: config.defaultModel,
  },
  prompts,
  skills,
  pipelines,
  logger,
});

const service = new SessionService({
  store: new SessionStore(dbBundle.db),
  eventBus,
  agentTeam,
  mcpConfig: {
    businessOrigin: config.businessOrigin,
    businessToken: config.businessToken,
  },
  skills,
  logger,
  leaseMs: config.leaseMs,
  recoveryPollMs: config.recoveryPollMs,
});

let healthServer: HealthServer | undefined;
let grpcServer: AgentGrpcServer | undefined;
let closing: Promise<void> | undefined;

function close(): Promise<void> {
  closing ??= (async () => {
    logger.info('Shutting down Glowloom Mastra Agent service');
    healthServer?.markDraining();
    const deadline = setTimeout(() => {
      logger.error({ code: 'shutdown_timeout' }, 'Agent shutdown deadline exceeded');
      process.exit(1);
    }, 30_000);
    deadline.unref();
    try {
      await service.stopRecovery();
      await Promise.allSettled([
        grpcServer?.close(),
        healthServer?.close(),
        eventBus.close(),
        prompts.shutdown(),
        dbBundle.close(),
      ]);
    } finally {
      clearTimeout(deadline);
    }
  })();
  return closing;
}

function onSignal(): void {
  void close().catch(() => {
    process.exitCode = 1;
  });
}

process.on('SIGTERM', onSignal);
process.on('SIGINT', onSignal);

try {
  healthServer = await startHealthServer({
    host: config.httpHost,
    port: config.httpPort,
    service,
  });
  grpcServer = await startAgentGrpcServer({
    service,
    address: config.grpcAddr,
    serviceToken: config.serviceToken,
  });
  if (!closing) {
    service.startRecovery();
  }
  logger.info(
    {
      http: healthServer.address,
      grpc: grpcServer.address,
      skills: skills.list().map((s) => s.name),
      pipelines: pipelines.list().map((p) => p.id),
      langfuseEnabled: Boolean(config.langfuse),
      redisEnabled: Boolean(config.redisUrl),
    },
    'Glowloom Mastra Agent service listening',
  );
} catch (error: unknown) {
  logger.error(
    { err: error instanceof Error ? error.message : String(error) },
    'Agent startup failed',
  );
  await close();
  throw error;
}
