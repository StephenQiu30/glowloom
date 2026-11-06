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

it('creates a real task-owned specialist conversation and returns its committed answer to the producer', async () => {
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
      const [, , grant, call] = args;
      calls++;
      const result =
        calls === 1
          ? {
              output: '委派编剧',
              tool_calls: [
                {
                  id: 'delegate-script',
                  name: 'delegate',
                  arguments: {
                    role: 'script',
                    task: '从目标中提取人物和场景，给出来源',
                  },
                },
              ],
              stop_reason: 'tool_use',
            }
          : calls === 2
            ? { output: '人物A；场景1；来源：用户目标', stop_reason: 'stop' }
            : { output: '已回收编剧的来源分析', stop_reason: 'stop' };
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
      title: '子会话',
    });
    const run = await service.createRun(session.id, {
      input: authorizedText('长剧本分析'),
      ...scope,
      request_id: randomUUID(),
      content_hash: 'b'.repeat(64),
      goal: '长剧本分析',
      expected_execution_revision: 0,
      manifest: { model_key: 'test' },
    });
    await service.waitForRun(run.root_run_id);
    expect(calls).toBe(3);
    const owned = await pool.query(
      "SELECT body FROM agent.runtime_records WHERE session_id=$1 AND record_type='conversation' AND owner_task_id IS NOT NULL",
      [session.id],
    );
    expect(owned.rowCount).toBe(1);
    expect((await service.readRun(run.root_run_id, scope))?.state.status).toBe(
      'completed',
    );
  } finally {
    await service.close();
    await pool.end();
  }
});
