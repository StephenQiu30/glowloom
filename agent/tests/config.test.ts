import { describe, expect, it } from 'vitest';
import { loadConfig } from '../src/config.js';

const env = {
  GLOWLOOM_AGENT_HTTP_ADDR: '0.0.0.0:4401',
  GLOWLOOM_AGENT_BUSINESS_ORIGIN: 'http://backend:1123',
  GLOWLOOM_AGENT_SERVICE_TOKEN: 's'.repeat(32),
  GLOWLOOM_AGENT_BUSINESS_TOKEN: 'b'.repeat(32),
  GLOWLOOM_AGENT_DB_DSN: 'postgresql://glowloom_agent:test@pg/agent_test',
};
describe('agent configuration', () => {
  it('loads independent credentials, a restricted role and bounded resources', () => {
    expect(loadConfig(env)).toMatchObject({
      host: '0.0.0.0',
      port: 4401,
      poolMax: 8,
    });
  });
  it.each([
    ['GLOWLOOM_AGENT_SERVICE_TOKEN', ''],
    ['GLOWLOOM_AGENT_SERVICE_TOKEN', 'short'],
    ['GLOWLOOM_AGENT_BUSINESS_TOKEN', env.GLOWLOOM_AGENT_SERVICE_TOKEN],
    ['GLOWLOOM_AGENT_BUSINESS_ORIGIN', 'file:///leak'],
    ['GLOWLOOM_AGENT_BUSINESS_ORIGIN', 'https://internal/api'],
    ['GLOWLOOM_AGENT_DB_DSN', ''],
    ['GLOWLOOM_AGENT_DB_DSN', 'postgresql://postgres:secret@pg/db'],
    [
      'GLOWLOOM_AGENT_DB_DSN',
      'postgresql://glowloom_agent@pg/db?user=postgres',
    ],
    ['GLOWLOOM_AGENT_POOL_MAX', '0'],
    ['GLOWLOOM_AGENT_HTTP_ADDR', 'host:70000'],
  ])('rejects invalid %s without printing the value', (key, value) => {
    expect(() => loadConfig({ ...env, [key!]: value! })).toThrow(key);
  });
});
