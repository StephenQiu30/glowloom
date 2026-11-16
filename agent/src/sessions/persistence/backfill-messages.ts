import type { Pool } from 'pg';
import { sql } from 'kysely';
import {
  database,
  queries,
  withTransaction,
} from '../../database/connection.js';
import { publicMessageParts } from '../../pi-durable/messages.js';
/** Offline, bounded, restart-safe projection of retained public events. No Harness or business calls. */
export async function backfillConversationMessages(
  pool: Pool,
  batchSize = 100,
) {
  if (!Number.isInteger(batchSize) || batchSize < 1 || batchSize > 500)
    throw new Error('Invalid backfill batch size');
  let cursor: [string, bigint, number] = ['', 0n, 0],
    scanned = 0,
    inserted = 0;
  for (;;) {
    const batch = await withTransaction(pool, async (connection) => {
      const rows = await queries(connection)
        .selectFrom('runtime_events')
        .select([
          'session_id',
          'commit_seq',
          'event_ordinal',
          'event_type',
          'payload',
        ])
        .where(
          sql<boolean>`(session_id, commit_seq, event_ordinal) > (${cursor[0]}, ${cursor[1]}::bigint, ${cursor[2]}::int)`,
        )
        .where('event_type', 'in', ['text', 'operation'])
        .orderBy('session_id', 'asc')
        .orderBy('commit_seq', 'asc')
        .orderBy('event_ordinal', 'asc')
        .limit(batchSize)
        .execute();
      let count = 0;
      for (const row of rows) {
        const parts = publicMessageParts(
          { type: row.event_type, payload: row.payload },
          Number(row.commit_seq),
          row.event_ordinal,
        );
        if (!parts.length) continue;
        const result = await database(connection).publicMessage.createMany({
          data: parts.map((part) => ({
            session_id: row.session_id,
            commit_seq: row.commit_seq,
            event_ordinal: row.event_ordinal,
            ...part,
            conversation_id: BigInt(part.conversation_id),
          })),
          skipDuplicates: true,
        });
        count += result.count;
      }
      return { rows, count };
    });
    inserted += batch.count;
    scanned += batch.rows.length;
    const last = batch.rows.at(-1);
    if (!last) break;
    cursor = [last.session_id, last.commit_seq, last.event_ordinal];
    if (batch.rows.length < batchSize) break;
  }
  return { scanned, inserted };
}
