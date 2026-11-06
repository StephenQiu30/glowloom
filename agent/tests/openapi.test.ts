import { mkdir, writeFile } from 'node:fs/promises';
import { describe, expect, it } from 'vitest';
import { agentFixture, buildApp, createOpenApi } from './helpers/agent-app.js';

describe('Agent Swagger contract', () => {
  it('derives the Go client input from actual controller and DTO metadata', async () => {
    const app = await buildApp({
      service: agentFixture(),
      serviceToken: 'offline-export-only-not-a-service-credential',
    });
    try {
      const spec = createOpenApi(app);
      expect(spec.openapi).toBe('3.0.3');
      expect(spec.paths['/internal/agent/sessions']?.post?.operationId).toBe(
        'createAgentSession',
      );
      expect(
        spec.paths['/internal/agent/runs/{rid}/events']?.get?.responses?.[
          '200'
        ],
      ).toMatchObject({ content: { 'text/event-stream': {} } });
      expect(spec.components?.schemas?.CreateRunRequest).toMatchObject({
        additionalProperties: false,
        properties: {
          input: { $ref: '#/components/schemas/AuthorizedInput' },
          expected_execution_revision: { type: 'integer', format: 'int64' },
        },
      });
      expect(spec.components?.schemas?.AgentState).toMatchObject({
        properties: {
          pending_proposal: { type: 'string', nullable: true },
        },
      });
      if (process.env.GLOWLOOM_AGENT_EXPORT_OPENAPI === '1') {
        const output = new URL(
          '../node_modules/.cache/openapi/',
          import.meta.url,
        );
        await mkdir(output, { recursive: true });
        await writeFile(
          new URL('agent.json', output),
          JSON.stringify(spec, null, 2) + '\n',
        );
      }
    } finally {
      await app.close();
    }
  });
});
