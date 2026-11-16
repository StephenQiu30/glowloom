import {
  DeleteQueryNode,
  InsertQueryNode,
  UpdateQueryNode,
  Kysely,
  PostgresAdapter,
  PostgresIntrospector,
  PostgresQueryCompiler,
  type CompiledQuery,
  type DatabaseConnection,
  type Driver,
  type QueryResult,
} from 'kysely';
import type { Prisma } from './generated/client.js';
import type { Database } from './schema.js';

/** Only Kysely's compiled statement and bound parameters cross this private bridge.
 * Application modules cannot submit raw strings; Prisma owns every connection,
 * transaction, savepoint and the actual execution. Do not add a raw SQL API here.
 */
class PrismaQueryDriver implements Driver {
  constructor(private readonly client: Prisma.TransactionClient) {}
  async init() {}
  async acquireConnection(): Promise<DatabaseConnection> {
    const client = this.client;
    return {
      async executeQuery<R>(compiled: CompiledQuery): Promise<QueryResult<R>> {
        const node = compiled.query;
        if (
          (InsertQueryNode.is(node) ||
            UpdateQueryNode.is(node) ||
            DeleteQueryNode.is(node)) &&
          !node.returning
        ) {
          const affected = await client.$executeRawUnsafe(
            compiled.sql,
            ...compiled.parameters,
          );
          return { rows: [], numAffectedRows: BigInt(affected) };
        }
        return {
          rows: await client.$queryRawUnsafe<R[]>(
            compiled.sql,
            ...compiled.parameters,
          ),
        };
      },
      // Streaming would escape the bounded result and transaction lifetime.
      async *streamQuery<R>(): AsyncIterableIterator<QueryResult<R>> {
        throw new Error('Prisma builder streaming is not supported');
      },
    };
  }
  async beginTransaction(): Promise<never> {
    throw new Error('Use Prisma transactions');
  }
  async commitTransaction(): Promise<never> {
    throw new Error('Use Prisma transactions');
  }
  async rollbackTransaction(): Promise<never> {
    throw new Error('Use Prisma transactions');
  }
  async releaseConnection() {}
  async destroy() {}
}
const builders = new WeakMap<Prisma.TransactionClient, Kysely<Database>>();
export function createQueries(
  client: Prisma.TransactionClient,
): Kysely<Database> {
  let builder = builders.get(client);
  if (!builder) {
    builder = new Kysely<Database>({
      dialect: {
        createDriver: () => new PrismaQueryDriver(client),
        createAdapter: () => new PostgresAdapter(),
        createIntrospector: (db) => new PostgresIntrospector(db),
        createQueryCompiler: () => new PostgresQueryCompiler(),
      },
    }).withSchema('agent');
    builders.set(client, builder);
  }
  return builder;
}
