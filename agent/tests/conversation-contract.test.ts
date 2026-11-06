import { describe, expect, it } from 'vitest';
import { buildApp } from '../src/transport/http/app.js';

const actor = '00000000-0000-4000-8000-000000000001';
const project = '00000000-0000-4000-8000-000000000002';
const sid = '00000000-0000-4000-8000-000000000003';
const rid = '00000000-0000-4000-8000-000000000004';
const headers = {
  authorization: `Bearer ${'a'.repeat(48)}`,
  'x-agent-audience': 'agent',
};
const identity = `actor_id=${actor}&project_id=${project}`;

function service() {
  return {
    ready: async () => true,
    createSession: async () => ({}),
    updateSession: async () => ({}),
  listSessions: async () => ({ items: [] }),
    readSession: async () => undefined,
    createRun: async () => ({}),
    readRun: async () => undefined,
    command: async () => ({}),
    events: async () => [],
    close: async () => {},
    listRuns: async (
      _sid: string,
      query: { actor_id: string; limit?: number },
    ) => ({
      items:
        query.actor_id === actor
          ? [
              {
                root_run_id: rid,
                session_id: sid,
                accepted_revision: 1,
                created_at: '2026-11-04T00:00:00Z',
                status: 'completed',
                input_excerpt: '测试',
              },
            ]
          : [],
    }),
    readSubmission: async () => undefined,
    currentSubmission: async () => ({}),
    listMessages: async () => ({ items: [], through: '0:0' }),
    readPublicMessage: async () => undefined,
  };
}

describe('conversation public transport contract', () => {
  it('returns bounded history summaries rather than losing all but the latest run', async () => {
    const app = await buildApp({
      service: service(),
      serviceToken: 'a'.repeat(48),
    });
    const response = await app.inject({
      url: `/internal/agent/sessions/${sid}/runs?${identity}`,
      headers,
    });
    expect(response.statusCode).toBe(200);
    expect(response.json()).toEqual({
      items: [
        {
          root_run_id: rid,
          session_id: sid,
          accepted_revision: 1,
          created_at: '2026-11-04T00:00:00Z',
          status: 'completed',
          input_excerpt: '测试',
        },
      ],
    });
    for (const limit of ['0', '51', '1.5']) {
      expect(
        (
          await app.inject({
            url: `/internal/agent/sessions/${sid}/runs?${identity}&limit=${limit}`,
            headers,
          })
        ).statusCode,
      ).toBe(400);
    }
    await app.close();
  });
  it('requires identity on history and original-submission discovery', async () => {
    const app = await buildApp({
      service: service(),
      serviceToken: 'a'.repeat(48),
    });
    for (const path of [
      `sessions/${sid}/runs`,
      `sessions/${sid}/submissions/current`,
      `runs/${rid}/messages?through=0:0`,
    ]) {
      expect(
        (await app.inject({ url: `/internal/agent/${path}`, headers }))
          .statusCode,
      ).toBe(400);
    }
    const original = await app.inject({
      url: `/internal/agent/sessions/${sid}/submissions/${rid}?${identity}`,
      headers,
    });
    expect(original.statusCode).toBe(404);
    expect(original.json().code).toBe('submission_not_found');
    await app.close();
  });
  it('separates the latest message page from replay and rejects unbounded pages', async () => {
    const app = await buildApp({
      service: service(),
      serviceToken: 'a'.repeat(48),
    });
    const path = `/internal/agent/runs/${rid}/messages?${identity}&through=0:0`;
    expect((await app.inject({ url: path, headers })).json()).toEqual({
      items: [],
      through: '0:0',
    });
    expect(
      (await app.inject({ url: path + '&limit=101', headers })).statusCode,
    ).toBe(400);
    expect(
      (await app.inject({ url: path.replace('0:0', 'invalid'), headers }))
        .statusCode,
    ).toBe(400);
    await app.close();
  });
});
it('rejects a new run without a frozen authorized input', async () => {
  const app = await buildApp({
    service: service(),
    serviceToken: 'a'.repeat(48),
  });
  const response = await app.inject({
    method: 'POST',
    url: `/internal/agent/sessions/${sid}/runs`,
    headers,
    payload: {
      actor_id: actor,
      org_id: actor,
      scope_id: actor,
      project_id: project,
      request_id: rid,
      content_hash: 'a'.repeat(64),
      expected_execution_revision: 0,
      goal: 'legacy text',
      manifest: {},
    },
  });
  expect(response.statusCode).toBe(400);
  await app.close();
});
