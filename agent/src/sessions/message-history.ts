import type { Pool, PoolClient } from "pg";
import type { MessageInput } from "../../contracts/agent.js";
import type { PublicMessagePart } from "../harness/messages.js";
import { HttpProblem } from "../http/app.js";
import { eventPosition, type ScopedCursors } from "./pagination.js";
export async function readMessagePage(
  db: Pool | PoolClient,
  cursors: ScopedCursors,
  run: { id: string; session_id: string },
  query: MessageInput,
) {
  const [through, ordinal] = eventPosition(query.through);
  const watermark = (
    await db.query<{ commit_seq: string; event_ordinal: number }>(
      "SELECT commit_seq,event_ordinal FROM agent.runtime_events WHERE session_id=$1 ORDER BY commit_seq DESC,event_ordinal DESC LIMIT 1",
      [run.session_id],
    )
  ).rows[0];
  const currentSeq = BigInt(watermark?.commit_seq ?? "0"),
    currentOrdinal = watermark?.event_ordinal ?? 0;
  if (
    BigInt(through) > currentSeq ||
    (BigInt(through) === currentSeq && ordinal > currentOrdinal)
  )
    throw new HttpProblem(400, "invalid_snapshot_watermark");

  const scope = `messages:${query.actor_id}:${query.project_id}:${run.id}`;
  const cursor = cursors.decode(query.before, scope);
  if (cursor && cursor.upper !== query.through)
    throw new HttpProblem(400, "cursor_snapshot_mismatch");
  const before = cursor
    ? (JSON.parse(cursor.before) as [string, number, number])
    : undefined;
  const rows = (
    await db.query<
      PublicMessagePart & {
        commit_seq: string;
        event_ordinal: number;
        candidate_count: string;
      }
    >(
      `
    WITH candidate AS (
      SELECT * FROM agent.public_messages WHERE session_id=$1 AND run_id=$2
      AND (commit_seq,event_ordinal)<=($3::bigint,$4::int)
      AND ($5::bigint IS NULL OR (commit_seq,event_ordinal,part_index)<($5::bigint,$6::int,$7::int))
      ORDER BY commit_seq DESC,event_ordinal DESC,part_index DESC LIMIT $8
    ), bounded AS (
      SELECT *,count(*) OVER() AS candidate_count,sum(octet_length(content)+1024) OVER(ORDER BY commit_seq DESC,event_ordinal DESC,part_index DESC) AS bytes FROM candidate
    ) SELECT * FROM bounded WHERE bytes<=1048576 ORDER BY commit_seq DESC,event_ordinal DESC,part_index DESC`,
      [
        run.session_id,
        run.id,
        through,
        ordinal,
        before?.[0] ?? null,
        before?.[1] ?? null,
        before?.[2] ?? null,
        (query.limit ?? 50) + 1,
      ],
    )
  ).rows;
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
              last.commit_seq,
              last.event_ordinal,
              last.part_index,
            ]),
          }),
        }
      : {}),
  };
}
