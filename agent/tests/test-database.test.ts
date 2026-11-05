import { afterEach, expect, it, vi } from 'vitest';
import { testPool } from './storage/database.js';
afterEach(() => vi.unstubAllEnvs());
it('allows the isolated conversation test login only when the connection assumes the restricted role', async () => {
  vi.stubEnv(
    'GLOWLOOM_AGENT_TEST_DB_DSN',
    'postgres://glowloom_agent_conversation@127.0.0.1/glowloom_conversation_20261104_test?options=-c%20role%3Dglowloom_agent',
  );
  const pool = testPool();
  await pool.end();
});
it.each([
  'postgres://postgres@127.0.0.1/glowloom_conversation_20261104_test?options=-c%20role%3Dglowloom_agent',
  'postgres://glowloom_agent_conversation@127.0.0.1/glowloom_conversation_20261104_test',
  'postgres://glowloom_agent_conversation@127.0.0.1/business?options=-c%20role%3Dglowloom_agent',
])('rejects an unsafe fixture connection', (dsn) => {
  vi.stubEnv('GLOWLOOM_AGENT_TEST_DB_DSN', dsn);
  expect(() => testPool()).toThrow();
});
