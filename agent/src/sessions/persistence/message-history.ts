import { and, desc, eq, getTableColumns, lt, lte, sql } from 'drizzle-orm';
import {
  database,
  type DatabaseConnection,
} from '../../database/connection.js';
import { publicMessages, runtimeEvents } from '../../database/schema.js';
import type { MessageInput } from '../types.js';
import { AgentError } from '../errors.js';
import { eventPosition, type ScopedCursors } from './pagination.js';
export async function readMessagePage(
  connection: DatabaseConnection,
  cursors: ScopedCursors,
  run: { id: string; session_id: string },
  query: MessageInput,
) {
  const db = database(connection);
  const [through, ordinal] = eventPosition(query.through);
  const [watermark] = await db
    .select({
      commit_seq: runtimeEvents.commit_seq,
      event_ordinal: runtimeEvents.event_ordinal,
    })
    .from(runtimeEvents)
    .where(eq(runtimeEvents.session_id, run.session_id))
    .orderBy(desc(runtimeEvents.commit_seq), desc(runtimeEvents.event_ordinal))
    .limit(1);
  const currentSeq = BigInt(watermark?.commit_seq ?? 0),
    currentOrdinal = watermark?.event_ordinal ?? 0;
  if (
    BigInt(through) > currentSeq ||
    (BigInt(through) === currentSeq && ordinal > currentOrdinal)
  )
    throw new AgentError('invalid_snapshot_watermark');

  const scope = `messages:${query.actor_id}:${query.project_id}:${run.id}`;
  const cursor = cursors.decode(query.before, scope);
  if (cursor && cursor.upper !== query.through)
    throw new AgentError('cursor_snapshot_mismatch');
  const before = cursor
    ? (JSON.parse(cursor.before) as [string, number, number])
    : undefined;
  const candidate = db.$with('candidate').as(
    db
      .select(getTableColumns(publicMessages))
      .from(publicMessages)
      .where(
        and(
          eq(publicMessages.session_id, run.session_id),
          eq(publicMessages.run_id, run.id),
          lte(
            sql`(${publicMessages.commit_seq}, ${publicMessages.event_ordinal})`,
            sql`(${through}::bigint, ${ordinal}::int)`,
          ),
          before
            ? lt(
                sql`(${publicMessages.commit_seq}, ${publicMessages.event_ordinal}, ${publicMessages.part_index})`,
                sql`(${before[0]}::bigint, ${before[1]}::int, ${before[2]}::int)`,
              )
            : undefined,
        ),
      )
      .orderBy(
        desc(publicMessages.commit_seq),
        desc(publicMessages.event_ordinal),
        desc(publicMessages.part_index),
      )
      .limit((query.limit ?? 50) + 1),
  );
  const bounded = db.$with('bounded').as(
    db
      .select({
        message_id: candidate.message_id,
        run_id: candidate.run_id,
        conversation_id: candidate.conversation_id,
        sequence: candidate.sequence,
        part_index: candidate.part_index,
        kind: candidate.kind,
        content: candidate.content,
        commit_seq: candidate.commit_seq,
        event_ordinal: candidate.event_ordinal,
        candidate_count: sql<string>`count(*) over()`.as('candidate_count'),
        bytes:
          sql<number>`sum(octet_length(${candidate.content})+1024) over(order by ${candidate.commit_seq} desc, ${candidate.event_ordinal} desc, ${candidate.part_index} desc)`.as(
            'bytes',
          ),
      })
      .from(candidate),
  );
  const rows = await db
    .with(candidate, bounded)
    .select()
    .from(bounded)
    .where(lte(bounded.bytes, 1048576))
    .orderBy(
      desc(bounded.commit_seq),
      desc(bounded.event_ordinal),
      desc(bounded.part_index),
    );
  const page = rows.slice(0, query.limit ?? 50),
    last = page.at(-1);
  const more = last && Number(last.candidate_count) > page.length;
  return {
    items: page
      .toReversed()
      .map(
        ({
          message_id,
          run_id,
          conversation_id,
          sequence,
          part_index,
          kind,
          content,
        }) => ({
          message_id,
          run_id,
          conversation_id: Number(conversation_id),
          sequence,
          part_index,
          kind,
          content,
        }),
      ),
    through: query.through,
    ...(more && last
      ? {
          next_cursor: cursors.encode({
            scope,
            upper: query.through,
            before: JSON.stringify([
              String(last.commit_seq),
              last.event_ordinal,
              last.part_index,
            ]),
          }),
        }
      : {}),
  };
}
