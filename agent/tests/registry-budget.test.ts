import { authorizedText } from './helpers/conversation-input.js';
import { randomUUID } from 'node:crypto';
import { expect, it } from 'vitest';
import { SessionService } from '../src/sessions/service.js';
import {
  BusinessClient,
  type Grant,
  type CallReceipt,
} from '../src/business/client.js';
import { testPool } from './storage/database.js';

it('persists one shared reserve for all 41 core plan tools and rejects the 41st before mutation', async () => {
  const pool = testPool();
  const scope = {
    actor_id: randomUUID(),
    org_id: randomUUID(),
    scope_id: randomUUID(),
    project_id: randomUUID(),
  };
  let calls = 0;
  class Fixture extends BusinessClient {
    constructor() {
      super('http://127.0.0.1:1', 'fixture');
    }
    override async tools() {
      return [];
    }
    override async decisions() {
      return {
        current: null,
        authorization_revision: 0,
        barrier: false,
        terminal: false,
      };
    }
    override async grant(
      input: Parameters<BusinessClient['grant']>[0],
    ): Promise<Grant> {
      return {
        ...input,
        grant_id: randomUUID(),
        available_micros: 1000000,
        expires_at: input.lease_until,
        scope,
      };
    }
    override async validate(grant: Grant) {
      return grant;
    }
    override async snapshot() {
      return { revision: 1 };
    }
    override async findCall() {
      return undefined;
    }
    override async execute(
      ...args: Parameters<BusinessClient['execute']>
    ): Promise<CallReceipt> {
      calls++;
      const [, , grant, call] = args;
      const result =
        calls === 1
          ? {
              output: '计划',
              tool_calls: Array.from({ length: 41 }, (_, i) => ({
                id: `plan-${i + 1}`,
                name: 'plan_update',
                arguments: {
                  items: [
                    {
                      id: `step-${i + 1}`,
                      title: '分析',
                      status: 'pending',
                      depends_on: [],
                      artifacts: [],
                    },
                  ],
                },
              })),
              stop_reason: 'tool_use',
            }
          : { output: 'done', stop_reason: 'stop' };
      return {
        receipt_id: randomUUID(),
        root_run_id: grant.root_run_id,
        submission_id: grant.submission_id,
        call,
        status: 'completed',
        cost_micros: 1,
        result: {
          ...result,
          usage: { input_tokens: 1, output_tokens: 1 },
          model_key: 'test',
        },
      };
    }
  }
  const service = new SessionService(pool, new Fixture(), { leaseMs: 30000 });
  try {
    const session = await service.createSession({
      ...scope,
      request_id: randomUUID(),
      content_hash: 'a'.repeat(64),
      title: '预算',
    });
    const run = await service.createRun(session.id, {
      input: authorizedText('分析'),
      ...scope,
      request_id: randomUUID(),
      content_hash: 'b'.repeat(64),
      goal: '分析',
      expected_execution_revision: 0,
      manifest: { model_key: 'test' },
    });
    await service.waitForRun(run.root_run_id);
    const snapshot = await service.readRun(run.root_run_id, scope);
    expect(snapshot?.state.tool_steps).toBe(40);
    expect((snapshot?.state.plan as { id: string }[])[0]?.id).toBe('step-40');
    const errors = await pool.query(
      "SELECT body FROM agent.runtime_records WHERE session_id=$1 AND record_type='entry' AND body::text LIKE '%budget exceeded%'",
      [session.id],
    );
    expect(errors.rowCount).toBeGreaterThan(0);
  } finally {
    await service.close();
    await pool.end();
  }
});
