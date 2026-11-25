import { randomUUID } from 'node:crypto';
import { createServer } from 'node:http';
import { once } from 'node:events';
import { readFile } from 'node:fs/promises';
import { z } from 'zod';
import { canvasWorkflowSchema } from '../src/mastra/canvas-workflow.js';
import { afterAll, expect, it, vi } from 'vitest';
import { createDatabaseBundle } from '../src/database/connection.js';
import { createMastraRuntime } from '../src/mastra/runtime.js';
import { PipelineRegistry } from '../src/mastra/pipeline-engine.js';
import { PromptRegistry } from '../src/observability/langfuse.js';
import { createAgentLogger } from '../src/observability/logger.js';
import { InMemoryEventBus } from '../src/sessions/event-bus.js';
import { SessionService } from '../src/sessions/service.js';
import { SessionStore } from '../src/sessions/store.js';
import { SkillRegistry, skillWorkspace } from '../src/skills/loader.js';

const dsn = process.env.GLOWLOOM_TEST_SKILL_RUNTIME_DB_DSN;
const closers: Array<() => Promise<void>> = [];
afterAll(async () => {
  await Promise.all(closers.map((close) => close()));
});

it.skipIf(!dsn)(
  'runs a pinned graph through the service, native Skill tools, and restricted PostgreSQL role',
  async () => {
    if (!dsn) throw new Error('Missing isolated database');
    const parsed = new URL(dsn);
    if (
      parsed.username !== 'glowloom_agent' ||
      parsed.hostname !== '127.0.0.1' ||
      !parsed.pathname.endsWith('_test')
    ) {
      throw new Error('Expected a restricted, isolated test database');
    }
    const db = createDatabaseBundle(dsn, 5);
    closers.push(() => db.close());
    const requests: Array<{
      model: string;
      messages: unknown[];
      tools?: Array<{ function: { name: string } }>;
    }> = [];
    const mock = createServer(async (request, response) => {
      if (request.url !== '/internal/agent-gateway/v1/chat/completions') {
        response.writeHead(404).end();
        return;
      }
      const chunks = [];
      for await (const chunk of request) chunks.push(chunk);
      const body = JSON.parse(
        Buffer.concat(chunks).toString(),
      ) as (typeof requests)[number];
      requests.push(body);
      const tool = body.tools?.find(
        (candidate) => candidate.function.name === 'skill_read',
      );
      const invoke =
        tool &&
        JSON.stringify(body.messages).includes('审查第一集') &&
        !body.messages.some(
          (message) =>
            typeof message === 'object' &&
            message !== null &&
            'role' in message &&
            message.role === 'tool',
        );
      const message = invoke
        ? {
            role: 'assistant',
            content: null,
            tool_calls: [
              {
                id: randomUUID(),
                type: 'function',
                function: {
                  name: tool.function.name,
                  arguments: JSON.stringify({
                    skillName: 'short-drama-review',
                    path: 'references/stage-contract.md',
                  }),
                },
              },
            ],
          }
        : {
            role: 'assistant',
            content: '已完成审查并读取固定版本的阶段契约。',
          };
      response.writeHead(200, { 'Content-Type': 'application/json' }).end(
        JSON.stringify({
          id: randomUUID(),
          model: body.model,
          choices: [
            {
              index: 0,
              message,
              finish_reason: invoke ? 'tool_calls' : 'stop',
            },
          ],
          usage: { prompt_tokens: 30, completion_tokens: 20, total_tokens: 50 },
        }),
      );
    }).listen(0, '127.0.0.1');
    closers.push(async () => {
      await new Promise<void>((resolve, reject) =>
        mock.close((error) => (error ? reject(error) : resolve())),
      );
    });
    await once(mock, 'listening');
    const local = mock.address();
    if (!local || typeof local === 'string')
      throw new Error('Missing mock origin');
    const logger = createAgentLogger('error');
    const gateway = {
      businessOrigin: `http://127.0.0.1:${local.port}`,
      businessToken: 'test-token'.repeat(4),
      defaultModel: 'test-model',
    };
    const skills = new SkillRegistry();
    const store = new SessionStore(db.db);
    const eventBus = new InMemoryEventBus();
    const team = await createMastraRuntime({
      storage: db.mastraStore,
      gatewayConfig: gateway,
      prompts: new PromptRegistry(undefined, logger),
      skills,
      pipelines: new PipelineRegistry(),
      logger,
    });
    const service = new SessionService({
      store,
      eventBus,
      agentTeam: team,
      mcpConfig: gateway,
      skills,
      logger,
      leaseMs: 30_000,
      recoveryPollMs: 5_000,
    });
    const identity = { actorId: randomUUID(), projectId: randomUUID() };
    const canvasId = randomUUID();
    const session = await service.createSession({
      identity,
      canvasId,
      requestId: randomUUID(),
      title: 'Skill workflow integration',
    });
    const nodeId = randomUUID();
    const start = {
      identity,
      sessionId: session.id,
      requestId: randomUUID(),
      expectedExecutionRevision: 0,
      goal: '审查第一集',
      manifest: {
        model_key: 'test-model',
        workflow: {
          id: randomUUID(),
          nodes: [
            {
              id: nodeId,
              skill: 'short-drama-review',
              title: '交付审查',
              task: '读取阶段契约，返回审查结果',
              after: [],
            },
          ],
        },
      },
    };
    const accepted = await service.startRun(start);
    const runId = String(accepted.root_run_id);
    await vi.waitFor(
      async () => {
        const run = await service.getRun(identity, runId);
        expect(run.state.status).toBe('completed');
        expect(
          run.messages_page.items.map((item) => item.content).join(' '),
        ).toContain('已完成审查');
        const events = await service.listRunEventsAfter(identity, runId);
        expect(
          events.events.some(
            (event) =>
              event.event === 'workflow.node.completed' &&
              event.data.node_id === nodeId,
          ),
        ).toBe(true);
      },
      { timeout: 16_000, interval: 100 },
    );
    expect(requests).toHaveLength(2);
    expect(requests[1]?.messages).toContainEqual(
      expect.objectContaining({
        role: 'tool',
        content: expect.stringContaining('REV-01'),
      }),
    );
    expect((await service.startRun(start)).root_run_id).toBe(runId);
    const persisted = await new SessionStore(db.db).skillsForRun(runId);
    expect(persisted.map((skill) => skill.name)).toEqual([
      'short-drama-review',
    ]);
    const workspace = await skillWorkspace(persisted);
    expect(
      (
        await workspace.skills?.getAsset(
          'short-drama-review',
          'references/stage-contract.md',
        )
      )?.length,
    ).toBeGreaterThan(0);
    const latest = await service.getRun(identity, runId);
    expect(latest.snapshot_cursor).toMatch(/^\d+:0$/);
    expect(latest.messages_page.items[0]?.sequence).toMatch(/^\d+:0$/);
    const [message] = latest.messages_page.items;
    const history = await service.listMessages(
      identity,
      runId,
      20,
      undefined,
      latest.messages_page.through,
    );
    expect(history.items[0]?.message_id).toBe(message?.message_id);
    expect(
      (
        await service.listRunEventsAfter(
          identity,
          runId,
          latest.snapshot_cursor,
        )
      ).events,
    ).toEqual([]);

    const templateFile = new URL(
      '../../frontend/src/components/canvas/editor/skill-workflow-templates.json',
      import.meta.url,
    );
    const templates = z
      .array(
        z.object({
          title: z.string(),
          nodes: z.array(z.record(z.string(), z.unknown())),
        }),
      )
      .parse(JSON.parse(await readFile(templateFile, 'utf8')));
    for (const [index, template] of templates.entries()) {
      const graph = canvasWorkflowSchema.parse({
        id: randomUUID(),
        nodes: template.nodes.map((node) => ({ ...node, id: node.skill })),
      });
      const receipt = await service.startRun({
        ...start,
        requestId: randomUUID(),
        expectedExecutionRevision: index + 1,
        goal: '按模板改编示例故事',
        manifest: { model_key: 'test-model', workflow: graph },
      });
      const id = String(receipt.root_run_id);
      await vi.waitFor(
        async () => {
          const run = await service.getRun(identity, id);
          expect(run.state.status).toBe('completed');
          expect(
            run.events.filter(
              (event) => event.event_type === 'workflow.node.completed',
            ),
          ).toHaveLength(graph.nodes.length);
        },
        { timeout: 5000, interval: 50 },
      );
      const pins = await store.skillsForRun(id);
      expect(pins).toHaveLength(graph.nodes.length);
    }
    expect(requests).toHaveLength(21);
  },
  25_000,
);
