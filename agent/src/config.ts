import { findPackageJSON } from 'node:module';
import path from 'node:path';
import { z } from 'zod';

export function resourceDirectory(name: string): string {
  const manifest = findPackageJSON('.', import.meta.url);
  if (!manifest) throw new Error('Cannot locate Agent package');
  return path.join(path.dirname(manifest), name);
}

const envSchema = z.object({
  GLOWLOOM_AGENT_HTTP_ADDR: z.string().default('0.0.0.0:4401'),
  GLOWLOOM_AGENT_GRPC_ADDR: z.string().default('0.0.0.0:4402'),
  GLOWLOOM_AGENT_DB_DSN: z.string().min(1, 'GLOWLOOM_AGENT_DB_DSN is required'),
  GLOWLOOM_AGENT_SERVICE_TOKEN: z
    .string()
    .min(32, 'GLOWLOOM_AGENT_SERVICE_TOKEN must be at least 32 chars'),
  GLOWLOOM_AGENT_BUSINESS_ORIGIN: z.string().url(),
  GLOWLOOM_AGENT_BUSINESS_TOKEN: z
    .string()
    .min(32, 'GLOWLOOM_AGENT_BUSINESS_TOKEN must be at least 32 chars'),
  GLOWLOOM_AGENT_POOL_MAX: z.coerce.number().int().min(1).max(64).default(12),
  GLOWLOOM_AGENT_LEASE_MS: z.coerce
    .number()
    .int()
    .min(5000)
    .max(300000)
    .default(30000),
  GLOWLOOM_AGENT_RECOVERY_POLL_MS: z.coerce
    .number()
    .int()
    .min(500)
    .max(60000)
    .default(5000),
  GLOWLOOM_AGENT_DEFAULT_MODEL: z.string().default('gpt-4o-mini'),
  GLOWLOOM_REDIS_URL: z.string().optional(),
  LANGFUSE_BASE_URL: z.preprocess(
    (value) =>
      typeof value === 'string' && value.trim() === '' ? undefined : value,
    z.string().url().optional(),
  ),
  LANGFUSE_PUBLIC_KEY: z.string().optional(),
  LANGFUSE_SECRET_KEY: z.string().optional(),
  GLOWLOOM_LOG_LEVEL: z
    .enum(['trace', 'debug', 'info', 'warn', 'error', 'fatal'])
    .default('info'),
});

export interface AgentConfig {
  httpHost: string;
  httpPort: number;
  grpcAddr: string;
  dbDsn: string;
  serviceToken: string;
  businessOrigin: string;
  businessToken: string;
  poolMax: number;
  leaseMs: number;
  recoveryPollMs: number;
  defaultModel: string;
  redisUrl?: string;
  langfuse?: {
    baseUrl: string;
    publicKey: string;
    secretKey: string;
  };
  logLevel: 'trace' | 'debug' | 'info' | 'warn' | 'error' | 'fatal';
}

function parseHostPort(addr: string): { host: string; port: number } {
  const idx = addr.lastIndexOf(':');
  if (idx <= 0) {
    throw new Error(`Invalid listen address: ${addr}`);
  }
  const host = addr.slice(0, idx);
  const port = Number.parseInt(addr.slice(idx + 1), 10);
  if (!Number.isInteger(port) || port < 0 || port > 65535) {
    throw new Error(`Invalid listen port in address: ${addr}`);
  }
  return { host, port };
}

export function loadConfig(env: NodeJS.ProcessEnv): AgentConfig {
  const parsed = envSchema.parse(env);
  const { host: httpHost, port: httpPort } = parseHostPort(
    parsed.GLOWLOOM_AGENT_HTTP_ADDR,
  );
  const hasLangfuse =
    Boolean(parsed.LANGFUSE_BASE_URL) &&
    Boolean(parsed.LANGFUSE_PUBLIC_KEY) &&
    Boolean(parsed.LANGFUSE_SECRET_KEY);

  return {
    httpHost,
    httpPort,
    grpcAddr: parsed.GLOWLOOM_AGENT_GRPC_ADDR,
    dbDsn: parsed.GLOWLOOM_AGENT_DB_DSN,
    serviceToken: parsed.GLOWLOOM_AGENT_SERVICE_TOKEN,
    businessOrigin: parsed.GLOWLOOM_AGENT_BUSINESS_ORIGIN.replace(/\/+$/, ''),
    businessToken: parsed.GLOWLOOM_AGENT_BUSINESS_TOKEN,
    poolMax: parsed.GLOWLOOM_AGENT_POOL_MAX,
    leaseMs: parsed.GLOWLOOM_AGENT_LEASE_MS,
    recoveryPollMs: parsed.GLOWLOOM_AGENT_RECOVERY_POLL_MS,
    defaultModel: parsed.GLOWLOOM_AGENT_DEFAULT_MODEL,
    ...(parsed.GLOWLOOM_REDIS_URL
      ? { redisUrl: parsed.GLOWLOOM_REDIS_URL }
      : {}),
    ...(hasLangfuse
      ? {
          langfuse: {
            baseUrl: parsed.LANGFUSE_BASE_URL!,
            publicKey: parsed.LANGFUSE_PUBLIC_KEY!,
            secretKey: parsed.LANGFUSE_SECRET_KEY!,
          },
        }
      : {}),
    logLevel: parsed.GLOWLOOM_LOG_LEVEL,
  };
}
