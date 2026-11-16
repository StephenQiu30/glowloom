import { EventEmitter } from 'node:events';
import { PrismaPg } from '@prisma/adapter-pg';
import { Pool } from 'pg';
import { expect, it } from 'vitest';
import { AgentPrismaPg } from '../src/database/pg-adapter.js';
import {
  disconnectDatabase,
  withTransaction,
} from '../src/database/connection.js';

type Adapter = Awaited<ReturnType<PrismaPg['connect']>>;
type Transaction = Awaited<ReturnType<Adapter['startTransaction']>>;
type DriverQuery = Parameters<Transaction['executeRaw']>[0];
const query = (sql: string): DriverQuery => ({ sql, args: [], argTypes: [] });

function faultPool(failedStatement?: string) {
  const releases: (Error | boolean | undefined)[] = [];
  const clients: EventEmitter[] = [];
  const statements: string[] = [];
  const failure = new Error('injected connection failure');
  let poisoned = false;
  let active: EventEmitter | undefined;
  // Keep the real Pool prototype so PrismaPg uses its external-pool path. No
  // database connection is opened: only this fixture replaces connect().
  const pool = new Pool();
  const connect = async () => {
    const client = new EventEmitter();
    clients.push(client);
    active = client;
    return Object.assign(client, {
      query: async (config: { text: string }) => {
        statements.push(config.text);
        if (poisoned) throw new Error('poisoned connection reused');
        if (config.text === failedStatement) {
          poisoned = true;
          throw failure;
        }
        return { fields: [], rows: [], rowCount: 0 };
      },
      release: (error?: Error | boolean) => {
        releases.push(error);
        if (error) poisoned = false;
      },
    });
  };
  // The fixture supplies every member used by the official promise-connect
  // adapter; the unused pg callback/query overloads are intentionally omitted.
  pool.connect = connect as unknown as Pool['connect'];
  return { pool, releases, clients, statements, failure, active: () => active };
}

it.each(['COMMIT', 'ROLLBACK'])(
  'preserves the wrapper error and discards %s failures through the real Prisma runtime',
  async (statement) => {
    const fixture = faultPool(statement);
    const workError = new Error('original application failure');
    let calls = 0;
    try {
      const result = withTransaction(fixture.pool, async () => {
        calls++;
        if (statement === 'ROLLBACK') throw workError;
        return 'accepted';
      });
      if (statement === 'COMMIT') {
        await expect(result).rejects.toThrow('COMMIT outcome is unknown');
      } else {
        await expect(result).rejects.toBe(workError);
      }
      expect(calls).toBe(1);
      expect(fixture.releases).toEqual([true]);
      expect(fixture.statements).toEqual(['BEGIN', statement]);
      expect(
        fixture.clients.every((client) => client.listenerCount('error') === 0),
      ).toBe(true);
    } finally {
      await disconnectDatabase(fixture.pool);
      await fixture.pool.end();
    }
  },
);

it.each(['COMMIT', 'ROLLBACK'])(
  'discards a failed %s connection even when Prisma releases without an error',
  async (statement) => {
    const fixture = faultPool(statement);
    const adapter = await new AgentPrismaPg(fixture.pool).connect();
    try {
      const tx = await adapter.startTransaction();
      await expect(tx.executeRaw(query(statement))).rejects.toThrow();
      // Prisma issues the statement through executeRaw and then calls the
      // release-only lifecycle method, including after swallowing a rollback error.
      await (statement === 'COMMIT' ? tx.commit() : tx.rollback());
      expect(fixture.releases).toEqual([true]);
      expect(fixture.clients[0]?.listenerCount('error')).toBe(0);
      const next = await adapter.startTransaction();
      await next.queryRaw(query('SELECT 1'));
      await next.commit();
      expect(fixture.releases).toEqual([true, false]);
      expect(fixture.statements).toEqual([
        'BEGIN',
        statement,
        'BEGIN',
        'SELECT 1',
      ]);
    } finally {
      await adapter.dispose();
      await fixture.pool.end();
    }
  },
);

it('discards clients after query or connection-event failure', async () => {
  const fixture = faultPool('SELECT broken');
  const adapter = await new AgentPrismaPg(fixture.pool).connect();
  try {
    const queryTx = await adapter.startTransaction();
    await expect(queryTx.queryRaw(query('SELECT broken'))).rejects.toThrow();
    await queryTx.rollback();
    expect(fixture.releases).toEqual([true]);

    const eventTx = await adapter.startTransaction();
    fixture.active()?.emit('error', fixture.failure);
    await eventTx.rollback();
    expect(fixture.releases).toEqual([true, true]);
    expect(
      fixture.clients.every((client) => client.listenerCount('error') === 0),
    ).toBe(true);
  } finally {
    await adapter.dispose();
    await fixture.pool.end();
  }
});

it.each([
  ['createSavepoint', 'SAVEPOINT test_point'],
  ['rollbackToSavepoint', 'ROLLBACK TO SAVEPOINT test_point'],
  ['releaseSavepoint', 'RELEASE SAVEPOINT test_point'],
] as const)('discards connections when %s fails', async (method, statement) => {
  const fixture = faultPool(statement);
  const adapter = await new AgentPrismaPg(fixture.pool).connect();
  try {
    const tx = await adapter.startTransaction();
    const savepoint = tx[method];
    if (!savepoint)
      throw new Error('Official pg adapter is missing savepoints');
    await expect(savepoint.call(tx, 'test_point')).rejects.toThrow();
    await tx.rollback();
    expect(fixture.releases).toEqual([true]);
    expect(fixture.clients[0]?.listenerCount('error')).toBe(0);
  } finally {
    await adapter.dispose();
    await fixture.pool.end();
  }
});

it('preserves isolation setup failure discard and never ends the external pool', async () => {
  const fixture = faultPool('SET TRANSACTION ISOLATION LEVEL SERIALIZABLE');
  const adapter = await new AgentPrismaPg(fixture.pool).connect();
  await expect(adapter.startTransaction('SERIALIZABLE')).rejects.toThrow();
  expect(fixture.releases).toHaveLength(1);
  expect(fixture.releases[0]).toBeTruthy();
  expect(fixture.clients[0]?.listenerCount('error')).toBe(0);
  await adapter.dispose();
  // An externally owned pool remains usable after adapter disposal.
  const client = await fixture.pool.connect();
  client.release();
  await fixture.pool.end();
});
