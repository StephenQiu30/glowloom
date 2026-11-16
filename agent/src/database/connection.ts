import { drizzle, type NodePgDatabase } from 'drizzle-orm/node-postgres';
import type { NodePgTransaction } from 'drizzle-orm/node-postgres';
import type { ExtractTablesWithRelations } from 'drizzle-orm';
import type { Pool, PoolClient } from 'pg';
import * as schema from './schema.js';

export type AgentDatabase = NodePgDatabase<typeof schema>;
export type AgentTransaction = NodePgTransaction<
  typeof schema,
  ExtractTablesWithRelations<typeof schema>
>;
export type DatabaseConnection =
  Pool | PoolClient | AgentDatabase | AgentTransaction;

export function database(connection: DatabaseConnection): AgentDatabase {
  if ('select' in connection) return connection;
  return drizzle({ client: connection, schema });
}

/** Retain connection ownership and unknown-COMMIT semantics around ORM transactions. */
export async function withTransaction<T>(
  pool: Pool,
  work: (transaction: AgentTransaction) => Promise<T>,
  readOnly = false,
): Promise<T> {
  const client = await pool.connect();
  let broken = false;
  let callbackFinished = false;
  let workFailed = false;
  let workError: unknown;
  const disconnected = () => {
    broken = true;
  };
  client.on('error', disconnected);
  try {
    return await database(client).transaction(
      async (transaction) => {
        try {
          const value = await work(transaction);
          callbackFinished = true;
          return value;
        } catch (error) {
          workFailed = true;
          workError = error;
          throw error;
        }
      },
      readOnly
        ? { isolationLevel: 'repeatable read', accessMode: 'read only' }
        : undefined,
    );
  } catch (error) {
    if (callbackFinished) {
      broken = true;
      throw new Error(
        'PostgreSQL COMMIT outcome is unknown; reopen and read the durable records',
        { cause: error },
      );
    }
    // A rollback failure must not replace the original application failure, and
    // its connection must never return to the pool as healthy.
    if (workFailed) {
      if (error !== workError) broken = true;
      throw workError;
    }
    broken = true;
    throw error;
  } finally {
    client.removeListener('error', disconnected);
    client.release(broken);
  }
}
