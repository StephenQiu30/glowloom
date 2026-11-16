import { randomUUID } from 'node:crypto';
import { afterAll, expect, it } from 'vitest';
import {
  database,
  disconnectDatabase,
  queries,
  withSavepoint,
  withTransaction,
} from '../src/database/connection.js';
import { testPool } from './storage/database.js';

const pool = testPool();
afterAll(async () => {
  await disconnectDatabase(pool);
  await pool.end();
});

it('shares Prisma and builder writes inside nested savepoints, preserving the accepted prefix', async () => {
  const session = randomUUID();
  const record = (id: bigint) => ({
    session_id: session,
    id,
    record_type: 'conversation',
    body: '{}',
    commit_seq: 1n,
  });
  await withTransaction(pool, async (tx) => {
    await tx.runtimeSession.create({
      data: {
        session_id: session,
        owner_id: randomUUID(),
        owner_epoch: 1n,
        lease_until: new Date(),
      },
    });
    await tx.runtimeRecord.create({ data: record(1n) });
    await expect(
      withSavepoint(tx, async (nested) => {
        await nested.runtimeRecord.create({ data: record(2n) });
        expect(
          await queries(nested)
            .selectFrom('runtime_records')
            .select('id')
            .where('session_id', '=', session)
            .execute(),
        ).toHaveLength(2);
        await queries(nested)
          .updateTable('runtime_records')
          .set({ body: 'changed' })
          .where('session_id', '=', session)
          .execute();
        throw new Error('refused batch');
      }),
    ).rejects.toThrow('refused batch');
    expect(
      await queries(tx)
        .selectFrom('runtime_records')
        .select('id')
        .where('session_id', '=', session)
        .execute(),
    ).toEqual([{ id: 1n }]);
  });
  expect(
    await database(pool).runtimeRecord.findMany({
      where: { session_id: session },
      select: { id: true, body: true },
    }),
  ).toEqual([{ id: 1n, body: '{}' }]);
});

it('retains a repeatable read, read-only transaction for ancestry snapshots', async () => {
  await expect(
    withTransaction(
      pool,
      async (tx) => {
        const settings = await queries(tx)
          .selectNoFrom((eb) => [
            eb
              .fn<string>('current_setting', [eb.val('transaction_isolation')])
              .as('isolation'),
            eb
              .fn<string>('current_setting', [eb.val('transaction_read_only')])
              .as('read_only'),
          ])
          .executeTakeFirstOrThrow();
        expect(settings).toEqual({
          isolation: 'repeatable read',
          read_only: 'on',
        });
        await expect(
          tx.recoveryBackoff.create({
            data: {
              kind: 'test',
              subject_id: randomUUID(),
              root_run_id: randomUUID(),
            },
          }),
        ).rejects.toThrow();
        // PostgreSQL aborts a read-only violation; the outer wrapper must roll back.
        throw new Error('read-only verified');
      },
      true,
    ),
  ).rejects.toThrow('read-only verified');
});
