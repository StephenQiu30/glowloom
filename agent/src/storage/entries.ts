import {
  StorageRequestError,
  type EntryRecord,
  type EntryQuery,
} from '@earendil-works/pi-durable';
import { database } from '../database/connection.js';
import type { Prisma, RuntimeRecord } from '../database/generated/client.js';
import { safeSeq, type Database, type ScanStart } from './records.js';

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
  const ancestry: Prisma.RuntimeRecordWhereInput[] = [];
  let id: bigint | null = BigInt(query.conversationId);
  let upper = BigInt(query.maxEntryId ?? Number.MAX_SAFE_INTEGER);
  const minimum = BigInt(query.minEntryId ?? 0);
  while (id !== null) {
    const row: Pick<RuntimeRecord, 'id' | 'parent_id' | 'parent_at'> | null =
      await db.runtimeRecord.findFirst({
        select: { id: true, parent_id: true, parent_at: true },
        where: {
          session_id: session,
          id,
          ...(ancestry.length === 0 ? { record_type: 'conversation' } : {}),
        },
      });
    if (!row) {
      if (id === BigInt(query.conversationId))
        throw new StorageRequestError(
          `Unknown conversation: ${query.conversationId}`,
        );
      break;
    }
    ancestry.push({ conversation_id: row.id, id: { lte: upper } });
    if (row.parent_at === null || row.parent_at < minimum) break;
    upper = upper < row.parent_at ? upper : row.parent_at;
    id = row.parent_id;
  }
  const where: Prisma.RuntimeRecordWhereInput = {
    session_id: session,
    record_type: 'entry',
    id: { gte: minimum },
    OR: ancestry,
  };
  if (start.after !== undefined)
    where.AND = [
      {
        id:
          start.order === 'ascending'
            ? { gt: BigInt(start.after) }
            : { lt: BigInt(start.after) },
      },
    ];
  if (headsOnly) where.head_id = { not: null };
  const rows = await db.runtimeRecord.findMany({
    select: { body: true, commit_seq: true },
    where,
    orderBy: { id: start.order === 'ascending' ? 'asc' : 'desc' },
    take: limit,
  });
  return rows.map((row) => ({
    entry: JSON.parse(row.body) as EntryRecord,
    commitSeq: safeSeq(row.commit_seq),
  }));
}
