import {
  StorageRequestError,
  type EntryRecord,
  type EntryQuery,
  type Seq,
} from '@earendil-works/pi-durable';
import {
  and,
  asc,
  desc,
  eq,
  gt,
  gte,
  isNotNull,
  lt,
  lte,
  or,
  type SQL,
} from 'drizzle-orm';
import { database } from '../../database/connection.js';
import { runtimeRecords } from '../../database/schema.js';
import type { Database, ScanStart } from './records.js';

// The caller supplies a repeatable-read transaction so ancestry and entries share
// the single snapshot previously supplied by the recursive statement.
export async function visibleEntries(
  connection: Database,
  session: string,
  query: EntryQuery,
  limit: number,
  start: ScanStart,
  headsOnly = false,
) {
  const db = database(connection);
  const ancestry = [];
  let id: number | null = query.conversationId;
  let upper = query.maxEntryId ?? Number.MAX_SAFE_INTEGER;
  const minimum = query.minEntryId ?? 0;
  while (id !== null) {
    const row:
      | Pick<
          typeof runtimeRecords.$inferSelect,
          'id' | 'parent_id' | 'parent_at'
        >
      | undefined = (
      await db
        .select({
          id: runtimeRecords.id,
          parent_id: runtimeRecords.parent_id,
          parent_at: runtimeRecords.parent_at,
        })
        .from(runtimeRecords)
        .where(
          and(
            eq(runtimeRecords.session_id, session),
            eq(runtimeRecords.id, id),
            ancestry.length === 0
              ? eq(runtimeRecords.record_type, 'conversation')
              : undefined,
          ),
        )
    )[0];
    if (!row) {
      if (id === query.conversationId)
        throw new StorageRequestError(
          `Unknown conversation: ${query.conversationId}`,
        );
      break;
    }
    ancestry.push(
      and(
        eq(runtimeRecords.conversation_id, row.id),
        lte(runtimeRecords.id, upper),
      ),
    );
    if (row.parent_at === null || row.parent_at < minimum) break;
    upper = Math.min(upper, row.parent_at);
    id = row.parent_id;
  }
  const where: (SQL | undefined)[] = [
    eq(runtimeRecords.session_id, session),
    eq(runtimeRecords.record_type, 'entry'),
    gte(runtimeRecords.id, minimum),
    or(...ancestry),
  ];
  if (start.after !== undefined)
    where.push(
      start.order === 'ascending'
        ? gt(runtimeRecords.id, start.after)
        : lt(runtimeRecords.id, start.after),
    );
  if (headsOnly) where.push(isNotNull(runtimeRecords.head_id));
  const rows = await db
    .select({
      body: runtimeRecords.body,
      commit_seq: runtimeRecords.commit_seq,
    })
    .from(runtimeRecords)
    .where(and(...where))
    .orderBy(
      start.order === 'ascending'
        ? asc(runtimeRecords.id)
        : desc(runtimeRecords.id),
    )
    .limit(limit);
  return rows.map((row) => ({
    entry: JSON.parse(row.body) as EntryRecord,
    commitSeq: row.commit_seq as Seq,
  }));
}
