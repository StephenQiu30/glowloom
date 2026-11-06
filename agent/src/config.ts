export interface Config {
  host: string;
  port: number;
  businessOrigin: string;
  serviceToken: string;
  businessToken: string;
  dbDsn: string;
  poolMax: number;
  leaseMs: number;
  swagger: boolean;
}

export function loadConfig(env: NodeJS.ProcessEnv): Config {
  function invalid(key: string): never {
    throw new Error(`Invalid GLOWLOOM_AGENT_${key}`);
  }
  function required(key: string) {
    return env[`GLOWLOOM_AGENT_${key}`]?.trim() || invalid(key);
  }
  function url(key: string, raw: string) {
    try {
      return new URL(raw);
    } catch {
      return invalid(key);
    }
  }
  function integer(key: string, fallback: number, min: number, max: number) {
    const raw = env[`GLOWLOOM_AGENT_${key}`];
    if (raw === undefined) return fallback;
    const n = Number(raw);
    return /^\d+$/.test(raw) && Number.isSafeInteger(n) && n >= min && n <= max
      ? n
      : invalid(key);
  }
  function token(key: string) {
    const value = required(key);
    return /^[!-~]{32,}$/.test(value) ? value : invalid(key);
  }
  const addr = required('HTTP_ADDR').match(
    /^(\[[\da-fA-F:.]+\]|[\w.-]+):(\d+)$/,
  );
  if (!addr || !addr[1] || !addr[2]) invalid('HTTP_ADDR');
  const port = Number(addr[2]);
  if (port < 1 || port > 65535) invalid('HTTP_ADDR');
  const origin = url('BUSINESS_ORIGIN', required('BUSINESS_ORIGIN'));
  if (
    !['http:', 'https:'].includes(origin.protocol) ||
    !origin.host ||
    origin.username ||
    origin.password ||
    origin.pathname !== '/' ||
    origin.search ||
    origin.hash
  )
    invalid('BUSINESS_ORIGIN');
  const dbDsn = required('DB_DSN');
  const db = url('DB_DSN', dbDsn);
  if (
    !['postgres:', 'postgresql:'].includes(db.protocol) ||
    !db.hostname ||
    db.username !== 'glowloom_agent' ||
    db.pathname.length < 2 ||
    db.hash ||
    [...db.searchParams.keys()].some(
      (key) => !['sslmode', 'application_name'].includes(key),
    )
  )
    invalid('DB_DSN');
  const serviceToken = token('SERVICE_TOKEN');
  const businessToken = token('BUSINESS_TOKEN');
  if (serviceToken === businessToken) invalid('BUSINESS_TOKEN');
  const swagger = env.GLOWLOOM_AGENT_SWAGGER_ENABLED;
  if (swagger !== undefined && swagger !== 'true' && swagger !== 'false') {
    invalid('SWAGGER_ENABLED');
  }
  return {
    swagger: swagger === 'true',
    host: addr[1].replace(/^\[|\]$/g, ''),
    port,
    dbDsn,
    serviceToken,
    businessToken,
    businessOrigin: origin.origin,
    poolMax: integer('POOL_MAX', 8, 1, 64),
    leaseMs: integer('LEASE_MS', 30_000, 1000, 300_000),
  };
}
