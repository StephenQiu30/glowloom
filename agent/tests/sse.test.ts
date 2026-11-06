import { describe, expect, it, vi } from 'vitest';
import { AgentError, agentFixture, buildApp } from './helpers/agent-app.js';

const runId = '00000000-0000-4000-8000-000000000001';
const actorId = '00000000-0000-4000-8000-000000000002';
const projectId = '00000000-0000-4000-8000-000000000003';
const serviceToken = 's'.repeat(40);
const headers = {
  Authorization: `Bearer ${serviceToken}`,
  'X-Agent-Audience': 'agent',
  'Last-Event-ID': '10:2',
};
const path = `/internal/agent/runs/${runId}/events?actor_id=${actorId}&project_id=${projectId}`;

describe('Nest Agent streaming over a real HTTP socket', () => {
  it('returns a cursor failure as JSON before opening the SSE response', async () => {
    const app = await buildApp({
      service: agentFixture({
        events: async () => {
          throw new AgentError('cursor_expired');
        },
      }),
      serviceToken,
    });
    try {
      await app.listen(0, '127.0.0.1');
      const response = await fetch(new URL(path, await app.getUrl()), {
        headers,
      });
      expect(response.status).toBe(410);
      expect(response.headers.get('content-type')).toContain(
        'application/json',
      );
      await expect(response.json()).resolves.toMatchObject({
        code: 'cursor_expired',
      });
    } finally {
      await app.close();
    }
  });

  it('resumes committed events and drains an open stream before disposing the service', async () => {
    const events = vi.fn(async () => [
      { cursor: '10:3', type: 'agent.state', payload: { status: 'running' } },
    ]);
    const actions: string[] = [];
    const app = await buildApp({
      service: agentFixture({
        events,
        stopAccepting: () => {
          actions.push('stop');
        },
        close: async () => {
          actions.push('service');
        },
      }),
      serviceToken,
    });
    try {
      await app.listen(0, '127.0.0.1');
      const response = await fetch(new URL(path, await app.getUrl()), {
        headers,
      });
      expect(response.status).toBe(200);
      expect(response.headers.get('content-type')).toBe('text/event-stream');
      expect(response.headers.get('x-accel-buffering')).toBe('no');
      const reader = response.body?.getReader();
      expect(reader).toBeDefined();
      const first = await reader?.read();
      expect(new TextDecoder().decode(first?.value)).toBe(
        'id: 10:3\nevent: agent.state\ndata: {"status":"running"}\n\n',
      );
      expect(events).toHaveBeenCalledWith(
        runId,
        { actor_id: actorId, project_id: projectId },
        '10:2',
      );
      expect(actions).toEqual([]);
      await app.close();
      expect(actions).toEqual(['stop', 'service']);
    } finally {
      if (actions.length === 0) await app.close();
    }
  });
});
