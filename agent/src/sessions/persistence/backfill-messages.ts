import type { Pool } from 'pg';
import type { JsonObject } from '@earendil-works/pi-durable';
import { publicMessageParts } from '../../pi-durable/messages.js';
/** Offline, bounded, restart-safe projection of retained public events. No Harness or business calls. */
export async function backfillConversationMessages(
  pool: Pool,
  batchSize = 100,
) {
  if (!Number.isInteger(batchSize) || batchSize < 1 || batchSize > 500)
    throw new Error('Invalid backfill batch size');
  let cursor: [string, string, number] = ['', '0', 0],
    scanned = 0,
    inserted = 0;
  for (;;) {
    const db = await pool.connect();
    try {
      await db.query('BEGIN');
      const rows = (
        await db.query<{
          session_id: string;
          commit_seq: string;
          event_ordinal: number;
          event_type: string;
          payload: JsonObject;
        }>(
          `SELECT session_id,commit_seq,event_ordinal,event_type,payload FROM agent.runtime_events WHERE (session_id,commit_seq,event_ordinal)>($1,$2::bigint,$3::int) AND event_type IN ('text','operation') ORDER BY session_id,commit_seq,event_ordinal LIMIT $4`,
          [...cursor, batchSize],
        )
      ).rows;
      for (const row of rows) {
        for (const p of publicMessageParts(
          { type: row.event_type, payload: row.payload },
          Number(row.commit_seq),
          row.event_ordinal,
        )) {
          const result = await db.query(
            `INSERT INTO agent.public_messages(session_id,run_id,commit_seq,event_ordinal,part_index,message_id,conversation_id,sequence,kind,content) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(session_id,commit_seq,event_ordinal,part_index) DO NOTHING`,
            [
              row.session_id,
              p.run_id,
              row.commit_seq,
              row.event_ordinal,
              p.part_index,
              p.message_id,
              p.conversation_id,
              p.sequence,
              p.kind,
              p.content,
            ],
          );
          inserted += result.rowCount ?? 0;
        }
      }
      await db.query('COMMIT');
      scanned += rows.length;
      const last = rows.at(-1);
      if (!last) break;
      cursor = [last.session_id, last.commit_seq, last.event_ordinal];
      if (rows.length < batchSize) break;
    } catch (error) {
      await db.query('ROLLBACK');
      throw error;
    } finally {
      db.release();
    }
  }
  return { scanned, inserted };
}
