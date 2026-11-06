import { randomBytes, randomUUID } from 'node:crypto';
import { BACKGROUND_CONTEXT as context } from '@earendil-works/chord/context';
import {
  ROOT_CONVERSATION_ID,
  type StorageWrite,
  type SubmissionId,
  type DocumentId,
} from '@earendil-works/pi-durable';
import { afterAll, expect, it } from 'vitest';
import {
  claimSessionOwner,
  renewSessionOwner,
  releaseSessionOwner,
} from '../../src/pi-durable/storage/ownership.js';
import {
  openPostgresStorage,
  openReadonlyPostgresStorage,
} from '../../src/pi-durable/storage/postgres.js';
import { testPool } from './database.js';
import { interruptCommand } from './network-fault.js';

const pool = testPool();
afterAll(() => pool.end());
const rootWrite: StorageWrite = {
  type: 'conversation',
  value: { id: ROOT_CONVERSATION_ID },
};

it('fences competing claims and rejects a stale owner even after reopening', async () => {
  const session = randomUUID();
  const claims = await Promise.allSettled(
    [0, 1].map(() => claimSessionOwner(pool, session, randomUUID(), 60_000)),
  );
  const accepted = claims.filter((claim) => claim.status === 'fulfilled');
  expect(accepted).toHaveLength(1);
  const owner = accepted[0]!.value;
  const storage = await openPostgresStorage(pool, session, owner);
  await storage.mintId();
  await renewSessionOwner(pool, session, owner, 60_000);
  await releaseSessionOwner(pool, session, owner);
  const next = await claimSessionOwner(pool, session, randomUUID(), 60_000);
  expect(BigInt(next.epoch)).toBe(BigInt(owner.epoch) + 1n);
  await expect(
    storage.commit(
      [[rootWrite]],
      () => {
        throw Error('stale receipt');
      },
      context,
    ),
  ).rejects.toThrow('ownership');
  await expect(openPostgresStorage(pool, session, owner)).rejects.toThrow(
    'ownership',
  );
  await expect(renewSessionOwner(pool, session, owner, 60_000)).rejects.toThrow(
    'ownership',
  );
  const reopened = await openPostgresStorage(pool, session, next);
  expect(await reopened.mintId()).toBe(3);
  await reopened.close(context);
  await storage.close(context);
});

it('rollback on projection failure cannot publish state, cursor or receipt', async () => {
  const session = randomUUID();
  const owner = await claimSessionOwner(pool, session, randomUUID(), 60_000);
  const storage = await openPostgresStorage(pool, session, owner, {
    projectEvents() {
      throw Error('projection failed');
    },
  });
  const receipts: number[] = [];
  await expect(
    storage.commit([[rootWrite]], (_, seq) => receipts.push(seq), context),
  ).rejects.toThrow('projection failed');
  expect(receipts).toEqual([]);
  expect(
    await storage.conversation(ROOT_CONVERSATION_ID, context),
  ).toBeUndefined();
  expect(
    (
      await pool.query(
        'select 1 from agent.runtime_events where session_id=$1',
        [session],
      )
    ).rows,
  ).toEqual([]);
});

it.each([
  { command: 'COMMIT', durable: true },
  { command: 'INSERT 0 1', durable: false },
])(
  'disconnect after backend $command gives no receive; reopen sees durable=$durable',
  async ({ command, durable }) => {
    const session = randomUUID();
    const owner = await claimSessionOwner(pool, session, randomUUID(), 60_000);
    const proxy = await interruptCommand(
      process.env.GLOWLOOM_AGENT_TEST_DB_DSN!,
      command,
    );
    const storage = await openPostgresStorage(proxy.pool, session, owner);
    const receipts: number[] = [];
    try {
      const write = storage.commit(
        [[rootWrite]],
        (_, seq) => receipts.push(seq),
        context,
      );
      if (durable)
        await expect(write).rejects.toThrow('COMMIT outcome is unknown');
      else await expect(write).rejects.toThrow();
      expect(proxy.interrupted()).toBe(true);
      expect(receipts).toEqual([]);
    } finally {
      await storage.close(context);
      await proxy.close();
    }
    const reopened = await openReadonlyPostgresStorage(pool, session);
    expect(await reopened.conversation(ROOT_CONVERSATION_ID, context)).toEqual(
      durable ? { id: ROOT_CONVERSATION_ID } : undefined,
    );
    const events = await pool.query(
      'SELECT commit_seq,event_ordinal FROM agent.runtime_events WHERE session_id=$1',
      [session],
    );
    expect(events.rows).toEqual(
      durable ? [{ commit_seq: '1', event_ordinal: 1 }] : [],
    );
    await expect(reopened.mintId()).rejects.toThrow('read-only');
    await reopened.close(context);
  },
);

it('rejects writes and renewals after lease expiry even without a competing owner', async () => {
  const session = randomUUID();
  const owner = await claimSessionOwner(pool, session, randomUUID(), 500);
  const storage = await openPostgresStorage(pool, session, owner);
  await pool.query('SELECT pg_sleep(0.55)');
  await expect(
    storage.commit(
      [[rootWrite]],
      () => {
        throw Error('unexpected receipt');
      },
      context,
    ),
  ).rejects.toThrow('ownership');
  await expect(renewSessionOwner(pool, session, owner, 10_000)).rejects.toThrow(
    'ownership',
  );
  const next = await claimSessionOwner(pool, session, owner.id, 10_000);
  expect(BigInt(next.epoch)).toBeGreaterThan(BigInt(owner.epoch));
  await storage.close(context);
});

it('uses only the provisioned runtime role and has no schema DDL privileges', async () => {
  const result = await pool.query(
    "SELECT current_user, has_schema_privilege(current_user, 'agent', 'CREATE') AS ddl",
  );
  expect(result.rows).toEqual([{ current_user: 'glowloom_agent', ddl: false }]);
});

it('supports long indexed Pi request and document keys without truncating their identity', async () => {
  const session = randomUUID();
  const owner = await claimSessionOwner(pool, session, randomUUID(), 60_000);
  const storage = await openPostgresStorage(pool, session, owner);
  const key = randomBytes(8192).toString('hex');
  const request = {
    id: await storage.mintId<SubmissionId>(),
    conversationId: ROOT_CONVERSATION_ID,
    requestId: key,
    type: 'continue',
    status: 'placed',
  } as const;
  const doc = {
    id: await storage.mintId<DocumentId>(),
    kind: 'test.notes',
    scope: { kind: 'session' },
    key,
  } as const;
  await expect(
    storage.commit(
      [
        [
          rootWrite,
          { type: 'submission', value: request },
          {
            type: 'document.create',
            record: doc,
            content: { version: 1, kind: 'base', value: {} },
          },
        ],
      ],
      () => {},
      context,
    ),
  ).resolves.toEqual({ stored: 1 });
  expect(
    await storage.submissionByRequest(ROOT_CONVERSATION_ID, key, context),
  ).toEqual(request);
  expect((await storage.findDocument(doc, 'current', context))?.id).toBe(
    doc.id,
  );
  await storage.close(context);
});
