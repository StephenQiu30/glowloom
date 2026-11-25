import { Kysely, PostgresDialect } from 'kysely';
import { Pool } from 'pg';
import { PostgresStore } from '@mastra/pg';
import type { AgentDatabase } from './schema.js';

export interface DatabaseBundle {
  pool: Pool;
  db: Kysely<AgentDatabase>;
  mastraStore: PostgresStore;
  close(): Promise<void>;
}

export function createDatabaseBundle(
  connectionString: string,
  maxConnections: number,
): DatabaseBundle {
  // Enforce Glowloom's strict no-runtime-DDL policy for @mastra/pg.
  process.env.MASTRA_DISABLE_STORAGE_INIT = 'true';

  const pool = new Pool({
    connectionString,
    max: maxConnections,
    connectionTimeoutMillis: 5000,
    idleTimeoutMillis: 30000,
    statement_timeout: 10000,
  });

  const db = new Kysely<AgentDatabase>({
    dialect: new PostgresDialect({ pool }),
  });

  const mastraStore = new PostgresStore({
    id: 'glowloom-agent-pg',
    connectionString,
    schemaName: 'agent',
    disableInit: true,
    max: Math.max(2, Math.floor(maxConnections / 2)),
  });

  return {
    pool,
    db,
    mastraStore,
    async close(): Promise<void> {
      await db.destroy();
    },
  };
}
