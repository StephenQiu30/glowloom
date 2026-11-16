import type { Pool } from 'pg';
import { PrismaClient, type Prisma } from './generated/client.js';
import { AgentPrismaPg } from './pg-adapter.js';
import { createQueries } from './query-driver.js';

export type AgentDatabase = PrismaClient;
export type AgentTransaction = Prisma.TransactionClient;
export type DatabaseConnection = Pool | AgentDatabase | AgentTransaction;
const clients = new WeakMap<Pool, AgentDatabase>();

export function database(connection: DatabaseConnection): AgentTransaction {
  if ('$transaction' in connection) return connection;
  let client = clients.get(connection);
  if (!client) {
    client = new PrismaClient({
      adapter: new AgentPrismaPg(connection),
      transactionOptions: { maxWait: 5000, timeout: 30000 },
    });
    clients.set(connection, client);
  }
  return client;
}

/** The compiler executes through this exact Prisma client or savepoint client. */
export function queries(connection: DatabaseConnection) {
  return createQueries(database(connection));
}

export async function disconnectDatabase(pool: Pool) {
  const client = clients.get(pool);
  if (client) {
    await client.$disconnect();
    clients.delete(pool);
  }
}

export async function withSavepoint<T>(
  transaction: AgentTransaction,
  work: (transaction: AgentTransaction) => Promise<T>,
): Promise<T> {
  // Prisma nested interactive transactions own savepoint creation and rollback.
  // Builder queries bind to the nested Prisma transaction, never the pool.
  return transaction.$transaction(work);
}

/** Retain original application errors and never acknowledge an uncertain COMMIT. */
export async function withTransaction<T>(
  pool: Pool,
  work: (transaction: AgentTransaction) => Promise<T>,
  readOnly = false,
): Promise<T> {
  let callbackFinished = false;
  let workFailed = false;
  let workError: unknown;
  try {
    return await database(pool).$transaction(
      async (transaction) => {
        try {
          if (readOnly) {
            await queries(transaction)
              .selectNoFrom((eb) =>
                eb
                  .fn<string>('set_config', [
                    eb.val('transaction_read_only'),
                    eb.val('on'),
                    eb.val(true),
                  ])
                  .as('read_only'),
              )
              .execute();
          }
          const result = await work(transaction);
          callbackFinished = true;
          return result;
        } catch (error) {
          workFailed = true;
          workError = error;
          throw error;
        }
      },
      readOnly ? { isolationLevel: 'RepeatableRead' } : undefined,
    );
  } catch (error) {
    if (callbackFinished) {
      throw new Error(
        'PostgreSQL COMMIT outcome is unknown; reopen and read the durable records',
        { cause: error },
      );
    }
    if (workFailed) throw workError;
    throw error;
  }
}
