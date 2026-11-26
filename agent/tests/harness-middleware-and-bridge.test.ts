import { describe, expect, it, vi } from 'vitest';
import {
  CanvasConflictCircuitOpenError,
  MAX_CANVAS_VIEW_BYTES,
  ToolBudgetExhaustedError,
  deterministicOperationId,
  projectCanvasReadView,
  wrapMcpToolsetWithHarness,
} from '../src/business/canvas-bridge.js';
import { createGatewayModel } from '../src/business/gateway-provider.js';
import {
  ModelBudgetExhaustedError,
  createHarnessModelMiddleware,
  pinPromptCacheAnchor,
} from '../src/business/model-middleware.js';

describe('Harness Model Middleware & Multi-Protocol Gateway', () => {
  it('moves Anthropic cacheControl off volatile trailing context blocks onto the preceding stable block', () => {
    const params = {
      prompt: [
        { role: 'system', content: 'You are Glowloom Producer.' },
        {
          role: 'user',
          content: [
            { type: 'text', text: '帮我把第一集剧本拆成 6 镜分镜。' },
            {
              type: 'text',
              text: '<canvas_changes>\nrevision 4 -> 5: node_2 updated\n</canvas_changes>',
              providerOptions: {
                anthropic: { cacheControl: { type: 'ephemeral' } },
              },
            },
          ],
        },
      ],
    };

    const transformed = pinPromptCacheAnchor(params);
    const userMsg = transformed.prompt[1] as {
      content: Array<{
        providerOptions?: { anthropic?: { cacheControl?: unknown } };
      }>;
    };

    expect(
      userMsg.content[1]?.providerOptions?.anthropic?.cacheControl,
    ).toBeUndefined();
    expect(
      userMsg.content[0]?.providerOptions?.anthropic?.cacheControl,
    ).toEqual({
      type: 'ephemeral',
    });
  });

  it('sets Anthropic session userId affinity and enforces pre-dispatch request budget before calling doGenerate', async () => {
    let used = 0;
    const max = 1;
    const middleware = createHarnessModelMiddleware({
      sessionId: 'session-abc-123',
      reserveModelRequest: async () => {
        if (used >= max) return { allowed: false, used, max };
        used += 1;
        return { allowed: true, used, max };
      },
    });

    const transformed = await middleware.transformParams!({
      type: 'generate',
      params: { prompt: [{ role: 'user', content: 'hello' }] } as never,
      model: {} as never,
    });
    expect(
      (
        transformed as unknown as {
          providerOptions?: { anthropic?: { userId?: string } };
        }
      ).providerOptions?.anthropic?.userId,
    ).toBe('session-abc-123');

    const doGenerate = vi.fn().mockResolvedValue({ text: 'ok' });
    await middleware.wrapGenerate!({
      doGenerate,
      doStream: vi.fn(),
      params: {} as never,
      model: {} as never,
    });
    expect(doGenerate).toHaveBeenCalledTimes(1);

    await expect(
      middleware.wrapGenerate!({
        doGenerate,
        doStream: vi.fn(),
        params: {} as never,
        model: {} as never,
      }),
    ).rejects.toBeInstanceOf(ModelBudgetExhaustedError);
    expect(doGenerate).toHaveBeenCalledTimes(1);
  });

  it('instantiates all supported native Vercel AI SDK protocols cleanly', () => {
    const identity = {
      actorId: 'actor-1',
      orgId: 'org-1',
      scopeId: 'scope-1',
      projectId: 'proj-1',
      sessionId: 'sess-1',
      runId: 'run-1',
    };
    const baseConfig = {
      businessOrigin: 'http://127.0.0.1:1123',
      businessToken: 'x'.repeat(32),
      defaultModel: 'claude-sonnet-4-5',
    };

    for (const protocol of [
      'openai-compatible',
      'openai',
      'openai-responses',
      'anthropic',
      'google',
    ] as const) {
      const model = createGatewayModel(baseConfig, identity, { protocol });
      expect(model).toBeDefined();
    }
  });
});

describe('Harness Canvas Bridge & Hybrid task.wait Wrapper', () => {
  it('bounds a 300-node canvas read view strictly below 40KB UTF-8 with explicit complete:false markers', () => {
    const nodes = Array.from({ length: 300 }, (_, i) => ({
      id: `node_${i + 1}`,
      type: 'text',
      title: `备注 ${i + 1}`,
      content: i === 0 ? 'x'.repeat(4_000) : `内容片段 ${i + 1}`,
    }));
    const projected = projectCanvasReadView({
      revision: 12,
      nodes,
      edges: [{ from: 'node_1', to: 'node_2' }],
    });

    const bytes = Buffer.byteLength(JSON.stringify(projected), 'utf8');
    expect(bytes).toBeLessThan(MAX_CANVAS_VIEW_BYTES);
    expect(projected.revision).toBe(12);
    expect(projected.complete).toBe(false);
    expect((projected.nodeIndex as unknown[]).length).toBe(300);
    const firstNode = (projected.nodes as Array<Record<string, unknown>>)[0]!;
    expect(firstNode.complete).toBe(false);
    expect((firstNode.content as Record<string, unknown>).readRef).toEqual({
      nodeId: 'node_1',
      fieldPath: '/content',
    });
  });

  it('rebases same-turn local canvas writes and trips circuit breaker on external conflict', async () => {
    let revision = 5;
    const seenExpectedRevisions: number[] = [];

    const tools = wrapMcpToolsetWithHarness(
      {
        canvas_read_graph: {
          execute: async () => ({
            revision,
            nodes: [],
            edges: [],
          }),
        },
        canvas_node_update: {
          execute: async (input: Record<string, unknown>) => {
            seenExpectedRevisions.push(Number(input.expectedRevision));
            if (input.simulateExternalConflict) {
              throw new Error('stale_revision: canvas modified externally');
            }
            revision += 1;
            return { ok: true, revision };
          },
        },
      },
      { runId: 'run-rebase-1' },
    );

    await tools.canvas_read_graph.execute!();
    await tools.canvas_node_update.execute!({
      nodeId: 'n1',
      expectedRevision: 5,
    });
    await tools.canvas_node_update.execute!({
      nodeId: 'n2',
      expectedRevision: 5,
    });
    expect(seenExpectedRevisions).toEqual([5, 6]);

    await expect(
      tools.canvas_node_update.execute!({
        nodeId: 'n3',
        expectedRevision: 7,
        simulateExternalConflict: true,
      }),
    ).rejects.toThrow('stale_revision');

    await expect(
      tools.canvas_node_update.execute!({ nodeId: 'n4', expectedRevision: 7 }),
    ).rejects.toBeInstanceOf(CanvasConflictCircuitOpenError);

    // Fresh read resets the conflict circuit breaker
    await tools.canvas_read_graph.execute!();
    const recovered = await tools.canvas_node_update.execute!({
      nodeId: 'n4',
      expectedRevision: 7,
    });
    expect(recovered).toEqual({ ok: true, revision: 8 });
  });

  it('injects deterministic operationId, awaits async media tasks via MCP task_wait, and enforces tool step budget', async () => {
    let waitCalls = 0;
    const progressEvents: unknown[] = [];
    const seenOpIds: string[] = [];
    let stepsUsed = 0;

    const tools = wrapMcpToolsetWithHarness(
      {
        media_submit_image: {
          execute: async (input: Record<string, unknown>) => {
            seenOpIds.push(String(input.operationId));
            return { status: 'running', taskId: 'task-img-1' };
          },
        },
        task_wait: {
          execute: async () => {
            waitCalls += 1;
            return {
              tasks: [
                {
                  id: 'task-img-1',
                  status: waitCalls >= 2 ? 'completed' : 'running',
                  outputUrl: waitCalls >= 2 ? 'cos://img/1.png' : undefined,
                },
              ],
            };
          },
        },
      },
      {
        runId: 'run-media-1',
        reserveToolStep: async () => {
          if (stepsUsed >= 2)
            return { allowed: false, used: stepsUsed, max: 2 };
          stepsUsed += 1;
          return { allowed: true, used: stepsUsed, max: 2 };
        },
        onTaskProgress: (update) => {
          progressEvents.push(update);
        },
        maxTaskWaitMs: 5_000,
        waitSliceMs: 100,
      },
    );

    const res = (await tools.media_submit_image.execute!({
      nodeId: 'shot-1',
      prompt: 'rainy neon street',
    })) as unknown as { status: string; taskIds: string[] };

    expect(res.status).toBe('completed');
    expect(res.taskIds).toEqual(['task-img-1']);
    expect(waitCalls).toBe(2);
    expect(progressEvents.length).toBe(2);
    expect(seenOpIds[0]).toBe(
      deterministicOperationId('run-media-1', 'media_submit_image', {
        nodeId: 'shot-1',
        prompt: 'rainy neon street',
      }),
    );

    // Second tool call succeeds (2/2), third is blocked by durable tool budget
    await tools.media_submit_image.execute!({
      nodeId: 'shot-2',
      prompt: 'sunrise rooftop',
    });
    await expect(
      tools.media_submit_image.execute!({
        nodeId: 'shot-3',
        prompt: 'exceeds budget',
      }),
    ).rejects.toBeInstanceOf(ToolBudgetExhaustedError);
  });
});
