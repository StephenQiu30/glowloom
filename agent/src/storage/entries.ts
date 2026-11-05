import {
  StorageRequestError,
  type EntryRecord,
  type EntryQuery,
  type Seq,
} from "@earendil-works/pi-durable";
import type { Database, ScanStart } from "./records.js";

export async function visibleEntries(
  db: Database,
  session: string,
  query: EntryQuery,
  limit: number,
  start: ScanStart,
  headsOnly = false,
) {
  const known = await db.query(
    "SELECT 1 FROM agent.runtime_records WHERE session_id=$1 AND id=$2 AND record_type='conversation'",
    [session, query.conversationId],
  );
  if (!known.rowCount)
    throw new StorageRequestError(
      `Unknown conversation: ${query.conversationId}`,
    );
  const args: unknown[] = [
    session,
    query.conversationId,
    query.maxEntryId ?? Number.MAX_SAFE_INTEGER,
    query.minEntryId ?? 0,
    limit,
  ];
  const after =
    start.after === undefined
      ? ""
      : `AND e.id ${start.order === "ascending" ? ">" : "<"} $${args.push(start.after)}`;
  const result = await db.query<{ body: string; commit_seq: string }>(
    `WITH RECURSIVE ancestry(id,parent_id,parent_at,upper_id) AS (
      SELECT id,parent_id,parent_at,$3::bigint FROM agent.runtime_records WHERE session_id=$1 AND id=$2
      UNION ALL SELECT p.id,p.parent_id,p.parent_at,least(a.upper_id,a.parent_at) FROM ancestry a
        JOIN agent.runtime_records p ON p.session_id=$1 AND p.id=a.parent_id WHERE a.parent_at >= $4
    )
    SELECT e.body,e.commit_seq FROM ancestry a JOIN agent.runtime_records e ON e.session_id=$1 AND e.record_type='entry' AND e.conversation_id=a.id
      WHERE e.id BETWEEN $4 AND a.upper_id ${after} ${headsOnly ? "AND e.head_id IS NOT NULL" : ""}
    ORDER BY e.id ${start.order === "ascending" ? "ASC" : "DESC"} LIMIT $5`,
    args,
  );
  return result.rows.map((row) => ({
    entry: JSON.parse(row.body) as EntryRecord,
    commitSeq: Number(row.commit_seq) as Seq,
  }));
}
