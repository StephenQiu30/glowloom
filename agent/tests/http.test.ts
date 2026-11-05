import { describe, expect, it } from 'vitest';
import { buildApp } from '../src/transport/http/app.js';

const scope = {
  actor_id: '00000000-0000-4000-8000-000000000001',
  org_id: '00000000-0000-4000-8000-000000000002',
  scope_id: '00000000-0000-4000-8000-000000000003',
  project_id: '00000000-0000-4000-8000-000000000004',
};
const token = 'a'.repeat(48);
const headers = {
  authorization: `Bearer ${token}`,
  'x-agent-audience': 'agent',
};
const service = {
  async ready() {
    return true;
  },
  async createSession(input: unknown) {
    return {
      id: '00000000-0000-4000-8000-000000000005',
      ...scope,
      title: '创作',
      execution_revision: 0,
      input,
    };
  },
  async listSessions() {
    return [];
  },
  async readSession() {
    return undefined;
  },
  async createRun() {
    throw new Error('not called');
  },
  async readRun() {
    return undefined;
  },
  async command() {
    throw new Error('not called');
  },
  async events() {
    return [];
  },
  async close() {},
  listRuns: async () => ({ items: [] }),
  readSubmission: async () => undefined,
  currentSubmission: async () => ({}),
  listMessages: async () => ({ items: [], through: '0:0' }),
  readPublicMessage: async () => undefined,
};

describe('agent internal HTTP boundary', () => {
  it('health does not require business availability; all internal routes require directional identity', async () => {
    const app = await buildApp({ service, serviceToken: token });
    expect((await app.inject('/healthz')).statusCode).toBe(200);
    expect((await app.inject('/internal/agent/sessions')).statusCode).toBe(401);
    expect(
      (
        await app.inject({
          url: '/internal/agent/sessions',
          headers: {
            authorization: `Bearer ${token}`,
            'x-agent-audience': 'agent-business',
          },
        })
      ).statusCode,
    ).toBe(401);
    await app.close();
  });
  it('validates scope and request identities before admission and rejects obsolete routes', async () => {
    const app = await buildApp({ service, serviceToken: token });
    const url = '/internal/agent/sessions';
    expect(
      (
        await app.inject({
          method: 'POST',
          url,
          headers,
          payload: { title: '创作' },
        })
      ).statusCode,
    ).toBe(400);
    const response = await app.inject({
      method: 'POST',
      url,
      headers,
      payload: {
        ...scope,
        request_id: '00000000-0000-4000-8000-000000000006',
        content_hash: 'b'.repeat(64),
        title: '创作',
      },
    });
    expect(response.statusCode).toBe(201);
    expect(
      (
        await app.inject({
          method: 'POST',
          url: '/internal/agent/sessions/old/advance',
          headers,
        })
      ).statusCode,
    ).toBe(404);
    await app.close();
  });
  it('exports its TypeBox contract without opening a listener or database', async () => {
    const app = await buildApp({ service, serviceToken: token });
    const spec = app.swagger();
    expect(spec.paths?.['/internal/agent/sessions']?.post?.operationId).toBe(
      'createAgentSession',
    );
    expect(JSON.stringify(spec)).not.toContain(token);
    await app.close();
  });
});
