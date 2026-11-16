import type { Pool } from 'pg';
import { and, asc, gt, inArray, sql } from 'drizzle-orm';
import { withTransaction } from '../../database/connection.js';
import { publicMessages, runtimeEvents } from '../../database/schema.js';
import { publicMessageParts } from '../../pi-durable/messages.js';
/** Offline, bounded, restart-safe projection of retained public events. No Harness or business calls. */
export async function backfillConversationMessages(
  pool: Pool,
  batchSize = 100,
) {
  if (!Number.isInteger(batchSize) || batchSize < 1 || batchSize > 500)
    throw new Error('Invalid backfill batch size');
  let cursor: [string, number, number] = ['', 0, 0],
    scanned = 0,
    inserted = 0;
  for (;;) {
    const batch = await withTransaction(pool, async (db) => {
      const rows = await db
        .select({
          session_id: runtimeEvents.session_id,
          commit_seq: runtimeEvents.commit_seq,
          event_ordinal: runtimeEvents.event_ordinal,
          event_type: runtimeEvents.event_type,
          payload: runtimeEvents.payload,
        })
        .from(runtimeEvents)
        .where(
          and(
            gt(
              sql`(${runtimeEvents.session_id}, ${runtimeEvents.commit_seq}, ${runtimeEvents.event_ordinal})`,
              sql`(${cursor[0]}, ${cursor[1]}::bigint, ${cursor[2]}::int)`,
            ),
            inArray(runtimeEvents.event_type, ['text', 'operation']),
          ),
        )
        .orderBy(
          asc(runtimeEvents.session_id),
          asc(runtimeEvents.commit_seq),
          asc(runtimeEvents.event_ordinal),
        )
        .limit(batchSize);
      let count = 0;
      for (const row of rows) {
        for (const part of publicMessageParts(
          { type: row.event_type, payload: row.payload },
          row.commit_seq,
          row.event_ordinal,
        )) {
          const result = await db
            .insert(publicMessages)
            .values({
              session_id: row.session_id,
              commit_seq: row.commit_seq,
              event_ordinal: row.event_ordinal,
              ...part,
            })
            .onConflictDoNothing({
              target: [
                publicMessages.session_id,
                publicMessages.commit_seq,
                publicMessages.event_ordinal,
                publicMessages.part_index,
              ],
            })
            .returning({ message_id: publicMessages.message_id });
          count += result.length;
        }
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
