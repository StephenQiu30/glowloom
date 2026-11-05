import { authorizedText } from './helpers/conversation-input.js';
import { afterAll, expect, it } from 'vitest';
import { randomUUID } from 'node:crypto';
import { SessionService } from '../src/sessions/service.js';
import {
  BusinessClient,
  type Grant,
  type CallReceipt,
} from '../src/integrations/go-business-api/client.js';
import { testPool } from './storage/database.js';
const pool = testPool();
class BusinessFixture extends BusinessClient {
  calls = 0;
  lastInput: unknown;
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
  override async grant(input: {
    root_run_id: string;
    submission_id: string;
    owner_epoch: number;
    authorization_revision: number;
    lease_until: string;
  }): Promise<Grant> {
    return {
      grant_id: randomUUID(),
      ...input,
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
    this.calls++;
    this.lastInput = args[4];
    const [, , grant, call] = args;
    return {
      receipt_id: randomUUID(),
      root_run_id: grant.root_run_id,
      submission_id: grant.submission_id,
      call,
      status: 'completed',
      cost_micros: 1,
      result: {
        output: '已完成目标分析',
        usage: { input_tokens: 4, output_tokens: 5 },
        stop_reason: 'stop',
        model_key: 'test-model',
      },
    };
  }
}
const scope = {
  actor_id: randomUUID(),
  org_id: randomUUID(),
  scope_id: randomUUID(),
  project_id: randomUUID(),
};
const business = new BusinessFixture();
const service = new SessionService(pool, business, { leaseMs: 30000 });
afterAll(async () => {
  await service.close();
  await pool.end();
});
it('runs the official Harness against real PostgreSQL, durably deduplicates input and keeps private scope', async () => {
  const session = await service.createSession({
    ...scope,
    request_id: randomUUID(),
    content_hash: 'a'.repeat(64),
    title: '测试创作',
  });
  const input = {
    input: authorizedText('分析创作目标'),
    ...scope,
    request_id: randomUUID(),
    content_hash: 'b'.repeat(64),
    expected_execution_revision: 0,
    goal: '分析创作目标',
    manifest: { model_key: 'test-model' },
  };
  const run = await service.createRun(session.id, input);
  await service.waitForRun(run.root_run_id);
  expect(business.calls).toBe(1);
  expect(JSON.stringify(business.lastInput)).toContain('business_snapshot');
  const again = await service.createRun(session.id, input);
  expect(again.root_run_id).toBe(run.root_run_id);
  expect(business.calls).toBe(1);
  const snapshot = await service.readRun(run.root_run_id, scope);
  expect(snapshot?.state.status).toBe('completed');
  expect(snapshot?.snapshot_cursor).toMatch(/^\d+:\d+$/);
  expect(
    await service.readRun(run.root_run_id, {
      ...scope,
      actor_id: randomUUID(),
    }),
  ).toBeUndefined();
  const events = await service.events(run.root_run_id, scope, '0:0');
  expect(events.some((e) => e.type === 'text')).toBe(true);
});

it('reconfigures the root between consecutive runs with different models in one session', async () => {
  const session = await service.createSession({
    ...scope,
    request_id: randomUUID(),
    content_hash: 'd'.repeat(64),
    title: '切换模型',
  });
  const first = await service.createRun(session.id, {
    input: authorizedText('第一轮'),
    ...scope,
    request_id: randomUUID(),
    content_hash: 'e'.repeat(64),
    expected_execution_revision: 0,
    goal: '第一轮',
    manifest: { model_key: 'A' },
  });
  await service.waitForRun(first.root_run_id);
  const saved = await service.readSession(session.id, scope);
  const second = await service.createRun(session.id, {
    input: authorizedText('第二轮'),
    ...scope,
    request_id: randomUUID(),
    content_hash: 'f'.repeat(64),
    expected_execution_revision: saved!.execution_revision,
    goal: '第二轮',
    manifest: { model_key: 'B' },
  });
  await service.waitForRun(second.root_run_id);
  expect((await service.readRun(second.root_run_id, scope))?.state.status).toBe(
    'completed',
  );
});

it('uses the current Go remaining budget for a second paid model request in the same run', async () => {
  class BudgetBusiness extends BusinessFixture {
    caps: number[] = [];
    override async validate(grant: Grant) {
      return { ...grant, available_micros: 100 - this.calls };
    }
    override async execute(
      ...args: Parameters<BusinessClient['execute']>
    ): Promise<CallReceipt> {
      const cap = args[5];
      if (cap > 100 - this.calls)
        throw new Error('Budget reservation exceeds current Go availability');
      this.caps.push(cap);
      const receipt = await super.execute(...args);
      if (this.calls === 1)
        receipt.result = {
          output: 'Set plan',
          usage: { input_tokens: 1, output_tokens: 1 },
          model_key: 'test-model',
          stop_reason: 'tool_use',
          tool_calls: [
            { id: 'plan-once', name: 'plan_update', arguments: { items: [] } },
          ],
        };
      return receipt;
    }
  }
  const business = new BudgetBusiness();
  const service = new SessionService(pool, business, { leaseMs: 30000 });
  try {
    const session = await service.createSession({
      ...scope,
      request_id: randomUUID(),
      content_hash: 'a'.repeat(64),
      title: 'Shared paid budget',
    });
    const run = await service.createRun(session.id, {
      input: authorizedText('Plan then answer'),
      ...scope,
      request_id: randomUUID(),
      content_hash: 'b'.repeat(64),
      expected_execution_revision: 0,
      goal: 'Plan then answer',
      manifest: { model_key: 'test-model', max_micros: 100 },
    });
    await service.waitForRun(run.root_run_id);
    expect((await service.readRun(run.root_run_id, scope))?.state.status).toBe(
      'completed',
    );
    expect(business.caps).toEqual([100, 99]);
  } finally {
    await service.close();
  }
});

it('keeps historical input readable while refusing execution with unavailable pinned definitions', async () => {
  const session = await service.createSession({
    ...scope,
    request_id: randomUUID(),
    content_hash: 'a'.repeat(64),
    title: 'Missing definition',
  });
  const { freezeManifest } = await import('../src/sessions/manifest.js');
  const manifest = {
    ...freezeManifest({ model_key: 'test-model' }, []),
    definition_hash: '0'.repeat(64),
  };
  const run = await service.store.admitRun(
    session.id,
    {
      input: authorizedText('Old unavailable definition'),
      ...scope,
      request_id: randomUUID(),
      content_hash: 'b'.repeat(64),
      expected_execution_revision: 0,
      goal: 'Old unavailable definition',
      manifest: { model_key: 'test-model' },
    },
    manifest,
  );
  const snapshot = await service.readRun(run.id, scope);
  expect(snapshot?.goal).toBe('Old unavailable definition');
  expect(snapshot?.execution_available).toBe(false);
  expect(
    (await service.readSession(session.id, scope))?.run?.execution_available,
  ).toBe(false);
  const { SessionRuntime } = await import('../src/sessions/runtime.js');
  await expect(
    SessionRuntime.open(pool, new BusinessFixture(), run, 60000),
  ).rejects.toThrow(/Pinned Harness definitions/);
  expect(
    (
      await pool.query(
        'SELECT count(*) n FROM agent.runtime_records WHERE session_id=$1',
        [session.id],
      )
    ).rows[0]?.n,
  ).toBe('0');
});

it('closes a completed session runtime and releases its PostgreSQL lease', async () => {
  const s = await service.createSession({
    ...scope,
    request_id: randomUUID(),
    content_hash: 'a'.repeat(64),
    title: 'Release completed owner',
  });
  const run = await service.createRun(s.id, {
    input: authorizedText('Finish'),
    ...scope,
    request_id: randomUUID(),
    content_hash: 'b'.repeat(64),
    expected_execution_revision: 0,
    goal: 'Finish',
    manifest: { model_key: 'test-model' },
  });
  await service.waitForRun(run.root_run_id);
  expect((await service.readRun(run.root_run_id, scope))?.state.status).toBe(
    'completed',
  );
  expect(
    (
      await pool.query(
        'SELECT lease_until<=clock_timestamp() released FROM agent.runtime_sessions WHERE session_id=$1',
        [s.id],
      )
    ).rows[0]?.released,
  ).toBe(true);
});

it('returns a byte-bounded recent event window with the current snapshot cursor', async () => {
  const s = await service.createSession({
    ...scope,
    request_id: randomUUID(),
    content_hash: 'a'.repeat(64),
    title: 'Bounded history',
  });
  const run = await service.createRun(s.id, {
    input: authorizedText('Finish'),
    ...scope,
    request_id: randomUUID(),
    content_hash: 'b'.repeat(64),
    expected_execution_revision: 0,
    goal: 'Finish',
    manifest: { model_key: 'test-model' },
  });
  await service.waitForRun(run.root_run_id);
  // A complete projection batch: synthetic large text, real PG read/cursor behavior.
  await pool.query(
    "INSERT INTO agent.runtime_events(session_id,commit_seq,event_ordinal,event_type,payload) SELECT $1,(SELECT max(commit_seq) FROM agent.runtime_events WHERE session_id=$1),1000+n,'text',jsonb_build_object('root_run_id',$2::text,'text',repeat('x',100000)) FROM generate_series(1,100) n",
    [s.id, run.root_run_id],
  );
  const snapshot = await service.readRun(run.root_run_id, scope);
  expect(Buffer.byteLength(JSON.stringify(snapshot))).toBeLessThanOrEqual(
    4 * 1024 * 1024,
  );
  expect(snapshot?.events.length).toBeGreaterThan(0);
  expect(snapshot?.snapshot_cursor).toMatch(/:1100$/);
  expect(snapshot?.state.status).toBe('completed');
});
