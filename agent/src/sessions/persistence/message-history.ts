import { sql } from 'kysely';
import {
  database,
  queries,
  type DatabaseConnection,
} from '../../database/connection.js';
import type { MessageInput } from '../types.js';
import { AgentError } from '../errors.js';
import { eventPosition, type ScopedCursors } from './pagination.js';
export async function readMessagePage(
  connection: DatabaseConnection,
  cursors: ScopedCursors,
  run: { id: string; session_id: string },
  query: MessageInput,
) {
  const [through, ordinal] = eventPosition(query.through);
  const watermark = await database(connection).runtimeEvent.findFirst({
    select: { commit_seq: true, event_ordinal: true },
    where: { session_id: run.session_id },
    orderBy: [{ commit_seq: 'desc' }, { event_ordinal: 'desc' }],
  });
  const currentSeq = watermark?.commit_seq ?? 0n,
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
  const rows = await queries(connection)
    .with('candidate', (qb) => {
      let candidates = qb
        .selectFrom('public_messages')
        .selectAll()
        .where('session_id', '=', run.session_id)
        .where('run_id', '=', run.id)
        .where(
          sql<boolean>`(commit_seq, event_ordinal) <= (${through}::bigint, ${ordinal}::int)`,
        );
      if (before)
        candidates = candidates.where(
          sql<boolean>`(commit_seq, event_ordinal, part_index) < (${before[0]}::bigint, ${before[1]}::int, ${before[2]}::int)`,
        );
      return candidates
        .orderBy('commit_seq', 'desc')
        .orderBy('event_ordinal', 'desc')
        .orderBy('part_index', 'desc')
        .limit((query.limit ?? 50) + 1);
    })
    .with('bounded', (qb) =>
      qb
        .selectFrom('candidate')
        .select([
          'message_id',
          'run_id',
          'conversation_id',
          'sequence',
          'part_index',
          'kind',
          'content',
          'commit_seq',
          'event_ordinal',
        ])
        .select([
          sql<bigint>`count(*) over()`.as('candidate_count'),
          sql<bigint>`sum(octet_length(content)+1024) over(order by commit_seq desc, event_ordinal desc, part_index desc)`.as(
            'bytes',
          ),
        ]),
    )
    .selectFrom('bounded')
    .selectAll()
    .where('bytes', '<=', 1048576n)
    .orderBy('commit_seq', 'desc')
    .orderBy('event_ordinal', 'desc')
    .orderBy('part_index', 'desc')
    .execute();
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
