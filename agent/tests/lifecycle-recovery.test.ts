import { authorizedText } from './helpers/conversation-input.js';
import { randomUUID } from 'node:crypto';
import { BACKGROUND_CONTEXT as context } from '@earendil-works/chord/context';
import { expect, it, vi } from 'vitest';
import {
  BusinessClient,
  BusinessError,
  type Grant,
  type CallReceipt,
  type BusinessDecision,
} from '../src/business/client.js';
import { SessionRuntime } from '../src/sessions/runtime.js';
import { SessionService } from '../src/sessions/service.js';
import { SessionStore } from '../src/sessions/persistence/store.js';
import { freezeManifest } from '../src/sessions/manifest.js';
import { StateDoc } from '../src/pi-durable/docs.js';
import { testPool } from './storage/database.js';
import { recoveryPumpPool } from './helpers/recovery-database.js';

const scope = () => ({
  actor_id: randomUUID(),
  org_id: randomUUID(),
  scope_id: randomUUID(),
  project_id: randomUUID(),
});
class Fixture extends BusinessClient {
  grants = 0;
  calls = 0;
  callsByRun = new Map<string, number>();
  constructor(readonly who: ReturnType<typeof scope>) {
    super('http://fixture.invalid', 'fixture');
  }
  override async tools() {
    return [];
  }
  override async decisions(): Promise<
    Awaited<ReturnType<BusinessClient['decisions']>>
  > {
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
    this.grants++;
    return {
      ...input,
      grant_id: randomUUID(),
      available_micros: 1000000,
      expires_at: input.lease_until,
      scope: this.who,
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
    const [, , grant, call] = args;
    this.callsByRun.set(
      grant.root_run_id,
      (this.callsByRun.get(grant.root_run_id) ?? 0) + 1,
    );
    return {
      receipt_id: randomUUID(),
      root_run_id: grant.root_run_id,
      submission_id: grant.submission_id,
      call,
      status: 'completed',
      cost_micros: 0,
      result: {
        output: 'Done',
        stop_reason: 'stop',
        usage: { input_tokens: 1, output_tokens: 1 },
        model_key: 'fixture',
      },
    };
  }
}
async function until(check: () => Promise<boolean>) {
  for (let i = 0; i < 250; i++) {
    if (await check()) return;
    await new Promise((r) => setTimeout(r, 10));
  }
  throw Error('Lifecycle state did not converge');
}

it('closes a real PG Harness immediately on StorageFailed and releases its owner without renewing grants', async () => {
  const pool = testPool(),
    who = scope(),
    business = new Fixture(who),
    store = new SessionStore(pool);
  const session = await store.createSession({
    ...who,
    request_id: randomUUID(),
    content_hash: 'a'.repeat(64),
    title: 'Storage failure',
  });
  const run = await store.admitRun(
    session.id,
    {
      input: authorizedText('Test'),
      ...who,
      request_id: randomUUID(),
      content_hash: 'b'.repeat(64),
      expected_execution_revision: 0,
      goal: 'Test',
      manifest: { model_key: 'fixture' },
    },
    freezeManifest({ model_key: 'fixture' }, []),
  );
  const runtime = await SessionRuntime.open(pool, business, run, 30000);
  const connect = pool.connect.bind(pool);
  let armed = false;
  const spy = vi.spyOn(pool, 'connect').mockImplementation(((
    ...args: unknown[]
  ) => {
    if (args.length) return Reflect.apply(connect, pool, args);
    return (async () => {
      const client = await connect();
      const query = client.query.bind(client);
      return new Proxy(client, {
        get(target, key) {
          if (key === 'query')
            return async (...args: unknown[]) => {
              if (
                armed &&
                String(args[0]).includes('INSERT INTO agent.runtime_events')
              ) {
                armed = false;
                throw Error('faultgate projection unavailable');
              }
              return Reflect.apply(query, target, args);
            };
          const value = Reflect.get(target, key);
          return typeof value === 'function' ? value.bind(target) : value;
        },
      });
    })();
  }) as typeof pool.connect);
  try {
    await runtime.authorize();
    armed = true;
    await expect(
      runtime.harness.commit(async (tx) => {
        const state = await tx.doc(StateDoc, runtime.root.id);
        state.execution_revision++;
      }, context),
    ).rejects.toThrow();
    expect(runtime.isClosed).toBe(true);
    expect(() => runtime.grant()).toThrow();
    await until(async () =>
      Boolean(
        (
          await pool.query(
            'SELECT lease_until<=clock_timestamp() released FROM agent.runtime_sessions WHERE session_id=$1',
            [session.id],
          )
        ).rows[0]?.released,
      ),
    );
    const grants = business.grants;
    await runtime.renew();
    expect(business.grants).toBe(grants);
    await expect(runtime.assertActive()).rejects.toThrow();
  } finally {
    spy.mockRestore();
    await runtime.close();
    await pool.end();
  }
});

it('waits for an in-flight real PG runtime opening during close and relinquishes its lease', async () => {
  const pool = testPool(),
    who = scope(),
    business = new Fixture(who),
    service = new SessionService(pool, business, { leaseMs: 30000 });
  let released!: () => void, opened!: () => void;
  const gate = new Promise<void>((r) => (released = r)),
    ready = new Promise<void>((r) => (opened = r));
  const open = SessionRuntime.open.bind(SessionRuntime);
  const spy = vi
    .spyOn(SessionRuntime, 'open')
    .mockImplementation(async (...args) => {
      const runtime = await open(...args);
      opened();
      await gate;
      return runtime;
    });
  try {
    const session = await service.createSession({
      ...who,
      request_id: randomUUID(),
      content_hash: 'a'.repeat(64),
      title: 'Opening close race',
    });
    await service.createRun(session.id, {
      input: authorizedText('Test'),
      ...who,
      request_id: randomUUID(),
      content_hash: 'b'.repeat(64),
      expected_execution_revision: 0,
      goal: 'Test',
      manifest: { model_key: 'fixture' },
    });
    await ready;
    let closed = false;
    const closing = service.close().then(() => {
      closed = true;
    });
    await new Promise((r) => setImmediate(r));
    expect(closed).toBe(false);
    released();
    await closing;
    expect(business.calls).toBe(0);
    expect(
      (
        await pool.query(
          'SELECT lease_until<=clock_timestamp() released FROM agent.runtime_sessions WHERE session_id=$1',
          [session.id],
        )
      ).rows[0]?.released,
    ).toBe(true);
  } finally {
    released();
    spy.mockRestore();
    await service.close();
    await pool.end();
  }
});

it('recovers an unresolved run beyond 33 earlier completed sessions using real PG projections', async () => {
  const pool = recoveryPumpPool(),
    who = scope(),
    business = new Fixture(who),
    service = new SessionService(pool, business, { leaseMs: 30000 });
  let recovery: SessionService | undefined;
  try {
    for (let i = 0; i < 33; i++) {
      const session = await service.createSession({
        ...who,
        request_id: randomUUID(),
        content_hash: 'a'.repeat(64),
        title: `Completed ${i}`,
      });
      const run = await service.createRun(session.id, {
        input: authorizedText('Test'),
        ...who,
        request_id: randomUUID(),
        content_hash: 'b'.repeat(64),
        expected_execution_revision: 0,
        goal: 'Test',
        manifest: { model_key: 'fixture' },
      });
      await service.waitForRun(run.root_run_id);
      expect((await service.readRun(run.root_run_id, who))?.state.status).toBe(
        'completed',
      );
    }
    await service.close();
    const session = await service.store.createSession({
      ...who,
      request_id: randomUUID(),
      content_hash: 'a'.repeat(64),
      title: 'Unresolved beyond batch',
    });
    const run = await service.store.admitRun(
      session.id,
      {
        input: authorizedText('Test'),
        ...who,
        request_id: randomUUID(),
        content_hash: 'b'.repeat(64),
        expected_execution_revision: 0,
        goal: 'Test',
        manifest: { model_key: 'fixture' },
      },
      freezeManifest({ model_key: 'fixture' }, []),
    );
    recovery = new SessionService(pool, business, { leaseMs: 30000 });
    recovery.startRecovery();
    await until(
      async () =>
        (await recovery!.readRun(run.id, who))?.state.status === 'completed',
    );
    expect(business.callsByRun.get(run.id)).toBe(1);
  } finally {
    await recovery?.close();
    await service.close();
    await pool.end();
  }
}, 30000);

it('persists a real Harness delivery wait across restarts without another model call, and completes only from the original authoritative delivery', async () => {
  const pool = testPool();
  const who = scope();
  const id = randomUUID();
  class DeliveryFixture extends Fixture {
    deliveryStatus = 'pending';
    override async snapshot() {
      return {
        revision: 1,
        shot_deliveries: [{ delivery_id: id, status: this.deliveryStatus }],
      };
    }
  }
  const business = new DeliveryFixture(who);
  const store = new SessionStore(pool);
  const session = await store.createSession({
    ...who,
    request_id: randomUUID(),
    content_hash: 'a'.repeat(64),
    title: 'Original delivery',
  });
  const run = await store.admitRun(
    session.id,
    {
      ...who,
      request_id: randomUUID(),
      content_hash: 'b'.repeat(64),
      expected_execution_revision: 0,
      goal: 'Deliver the original shot',
      input: authorizedText('Deliver the original shot'),
      manifest: { model_key: 'fixture' },
    },
    freezeManifest({ model_key: 'fixture' }, []),
  );
  let runtime = await SessionRuntime.open(pool, business, run, 30000);
  try {
    await runtime.start();
    let state = await runtime.state();
    expect(state?.status).toBe('waiting_delivery');
    expect(state?.submission_status).toBe('completed');
    const waiting = state?.delivery_wait as Record<string, unknown>;
    expect(waiting.deadline_at).toEqual(expect.any(String));
    expect(waiting.next_wakeup_at).toEqual(expect.any(String));
    const calls = business.calls;
    expect(calls).toBe(1);
    await runtime.close();
    runtime = await SessionRuntime.open(pool, business, run, 30000);
    business.deliveryStatus = 'awaiting_review';
    await runtime.start();
    state = await runtime.state();
    expect(state?.status).toBe('waiting_delivery');
    expect((state?.delivery_wait as Record<string, unknown>).deadline_at).toBe(
      waiting.deadline_at,
    );
    expect(business.calls).toBe(calls);
    await runtime.close();
    runtime = await SessionRuntime.open(pool, business, run, 30000);
    business.deliveryStatus = 'completed';
    await runtime.start();
    expect((await runtime.state())?.status).toBe('completed');
    expect(business.calls).toBe(calls);
  } finally {
    await runtime.close();
    await pool.end();
  }
}, 20000);

it('pauses and resumes a persisted delivery wait without another model submission', async () => {
  const pool = testPool(),
    who = scope(),
    id = randomUUID();
  class DeliveryControlFixture extends Fixture {
    revision = 0;
    barrier = false;
    done = false;
    current: BusinessDecision | null = null;
    override async decisions() {
      return {
        current: this.current,
        authorization_revision: this.revision,
        barrier: this.barrier,
        terminal: false,
      };
    }
    override async grant(input: Parameters<BusinessClient['grant']>[0]) {
      if (this.barrier)
        throw new BusinessError(409, 'agent_authorization_blocked');
      return super.grant(input);
    }
    override async snapshot() {
      return {
        revision: 1,
        shot_deliveries: [
          { delivery_id: id, status: this.done ? 'completed' : 'pending' },
        ],
      };
    }
    override async receipt() {
      return { accepted: true };
    }
    override async quiesced(
      run: string,
      input: Parameters<BusinessClient['quiesced']>[1],
    ) {
      this.barrier = false;
      return this.grant({
        root_run_id: run,
        submission_id: input.continuation_id,
        owner_epoch: input.owner_epoch,
        authorization_revision: input.authorization_revision,
        lease_until: input.lease_until,
      });
    }
  }
  const business = new DeliveryControlFixture(who),
    service = new SessionService(pool, business, { leaseMs: 30000 });
  try {
    const session = await service.createSession({
      ...who,
      request_id: randomUUID(),
      content_hash: 'a'.repeat(64),
      title: 'Paused delivery',
    });
    const run = await service.createRun(session.id, {
      ...who,
      input: authorizedText('Make a shot'),
      request_id: randomUUID(),
      content_hash: 'b'.repeat(64),
      expected_execution_revision: 0,
      goal: 'Make a shot',
      manifest: { model_key: 'fixture' },
    });
    await service.waitForRun(run.root_run_id);
    const deadline = (await service.readRun(run.root_run_id, who))?.state
      .delivery_wait;
    for (const action of ['pause', 'resume'] as const) {
      business.revision++;
      business.barrier = true;
      business.current = {
        ...who,
        decision_id: randomUUID(),
        root_run_id: run.root_run_id,
        session_id: session.id,
        authorization_revision: business.revision,
        action,
        content_hash: randomUUID(),
      };
      if (action === 'resume') business.done = true;
      await service.command(run.root_run_id, business.current);
      await service.waitForRun(run.root_run_id);
      if (action === 'pause')
        expect(
          (await service.readRun(run.root_run_id, who))?.state.delivery_wait,
        ).toEqual(deadline);
    }
    expect((await service.readRun(run.root_run_id, who))?.state.status).toBe(
      'completed',
    );
    expect(business.calls).toBe(1);
  } finally {
    await service.close();
    await pool.end();
  }
}, 20000);

it('recovers a due delivery wait through the service pump without submitting another model turn', async () => {
  const pool = recoveryPumpPool(),
    who = scope(),
    id = randomUUID();
  class DeliveryFixture extends Fixture {
    done = false;
    override async snapshot() {
      return {
        revision: 1,
        shot_deliveries: [
          { delivery_id: id, status: this.done ? 'completed' : 'pending' },
        ],
      };
    }
  }
  const business = new DeliveryFixture(who);
  const service = new SessionService(pool, business, {
    leaseMs: 30000,
    deliveryPollMs: 100,
  });
  let recovery: SessionService | undefined;
  try {
    const session = await service.createSession({
      ...who,
      request_id: randomUUID(),
      content_hash: 'a'.repeat(64),
      title: 'Delivery recovery',
    });
    const accepted = await service.createRun(session.id, {
      ...who,
      input: authorizedText('Make a shot'),
      request_id: randomUUID(),
      content_hash: 'b'.repeat(64),
      expected_execution_revision: 0,
      goal: 'Make a shot',
      manifest: { model_key: 'fixture' },
    });
    await service.waitForRun(accepted.root_run_id);
    expect(
      (await service.readRun(accepted.root_run_id, who))?.state.status,
    ).toBe('waiting_delivery');
    await service.close();
    business.done = true;
    recovery = new SessionService(pool, business, { leaseMs: 30000 });
    recovery.startRecovery();
    await until(
      async () =>
        (await recovery!.readRun(accepted.root_run_id, who))?.state.status ===
        'completed',
    );
    expect(business.callsByRun.get(accepted.root_run_id)).toBe(1);
  } finally {
    await recovery?.close();
    await service.close();
    await pool.end();
  }
}, 20000);

it.each([
  'missing',
  'unavailable',
  'incomplete',
  'expired',
  'authority',
  'expired_authority',
])(
  'keeps original delivery identity and fails closed on %s facts without another model call',
  async (failure) => {
    const pool = testPool(),
      who = scope(),
      id = randomUUID();
    class DeliveryFixture extends Fixture {
      failure: string | undefined;
      override async decisions() {
        if (
          this.failure === 'authority' ||
          this.failure === 'expired_authority'
        )
          throw new Error('authority unavailable');
        return super.decisions();
      }
      override async snapshot() {
        if (this.failure === 'unavailable') throw new Error('read unavailable');
        return {
          revision: 1,
          shot_deliveries:
            this.failure === 'missing'
              ? []
              : [
                  {
                    delivery_id: id,
                    status:
                      this.failure === 'incomplete' ? 'incomplete' : 'pending',
                  },
                ],
        };
      }
    }
    const business = new DeliveryFixture(who),
      store = new SessionStore(pool);
    const session = await store.createSession({
      ...who,
      request_id: randomUUID(),
      content_hash: 'a'.repeat(64),
      title: 'Delivery failure',
    });
    const run = await store.admitRun(
      session.id,
      {
        ...who,
        input: authorizedText('Make a shot'),
        request_id: randomUUID(),
        content_hash: 'b'.repeat(64),
        expected_execution_revision: 0,
        goal: 'Make a shot',
        manifest: { model_key: 'fixture' },
      },
      freezeManifest({ model_key: 'fixture' }, []),
    );
    let runtime = await SessionRuntime.open(pool, business, run, 30000);
    try {
      await runtime.start();
      const deadline = (await runtime.state())?.delivery_wait?.deadline_at;
      if (failure.startsWith('expired'))
        await runtime.harness.commit(async (tx) => {
          const state = await tx.doc(StateDoc, runtime.root.id);
          state.delivery_wait!.deadline_at = new Date(0).toISOString();
        }, context);
      await runtime.close();
      business.failure = failure;
      runtime = await SessionRuntime.open(pool, business, run, 30000);
      await runtime.start();
      const state = await runtime.state();
      expect(state?.status).toBe(
        ['incomplete', 'expired', 'expired_authority'].includes(failure)
          ? 'needs_reconciliation'
          : 'waiting_delivery',
      );
      expect(state?.delivery_wait?.delivery_ids).toEqual([id]);
      if (!failure.startsWith('expired'))
        expect(state?.delivery_wait?.deadline_at).toBe(deadline);
      expect(business.calls).toBe(1);
    } finally {
      await runtime.close();
      await pool.end();
    }
  },
  20000,
);
