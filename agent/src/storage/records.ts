import {
  StorageRequestError,
  type Cursor,
  type Id,
  type Page,
  type ScanOrder,
  type Seq,
  type StorageWrite,
} from "@earendil-works/pi-durable";
import type { Pool, PoolClient } from "pg";

export type Database = Pool | PoolClient;
export type RecordWrite = Extract<
  StorageWrite,
  { type: "conversation" | "entry" | "task" | "submission" }
>;
export type TaskRecord = Extract<RecordWrite, { type: "task" }>["value"];
export type BodyRow = { body: string };
export function decode<T>(row: BodyRow | undefined): T | undefined {
  return row && (JSON.parse(row.body) as T);
}
export function indexKey(value: string | undefined) {
  return value === undefined ? null : JSON.stringify(value);
}
export class Refused extends Error {}
export function safeSeq(n: number): Seq {
  if (!Number.isSafeInteger(n) || n < 1)
    throw new Error("Commit sequence space exhausted");
  return n as Seq;
}

export async function record<T>(
  db: Database,
  session: string,
  type: string,
  id: number,
) {
  return decode<T>(
    (
      await db.query<BodyRow>(
        "SELECT body FROM agent.runtime_records WHERE session_id=$1 AND record_type=$2 AND id=$3",
        [session, type, id],
      )
    ).rows[0],
  );
}

export async function writeRecord(
  client: PoolClient,
  session: string,
  seq: Seq,
  write: RecordWrite,
) {
  const { type, value } = write;
  const conversation = write.type === "conversation" ? write.value : undefined;
  const task = write.type === "task" ? write.value : undefined;
  const submission = write.type === "submission" ? write.value : undefined;
  const existing = await client.query<{ record_type: string }>(
    "SELECT record_type FROM agent.runtime_records WHERE session_id=$1 AND id=$2",
    [session, value.id],
  );
  const prior = existing.rows[0]?.record_type;
  if (prior && (prior !== type || type === "conversation" || type === "entry"))
    throw new Refused(`ID ${value.id} already belongs to ${prior}`);
  await client.query(
    `INSERT INTO agent.runtime_records
    (session_id,id,record_type,body,commit_seq,conversation_id,owner_conversation_id,owner_task_id,parent_id,parent_at,head_id,
     kind_key,status,background,abort_requested,request_key)
    VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
    ON CONFLICT(session_id,id) DO UPDATE SET body=excluded.body,commit_seq=excluded.commit_seq,conversation_id=excluded.conversation_id,
      kind_key=excluded.kind_key,status=excluded.status,background=excluded.background,abort_requested=excluded.abort_requested,request_key=excluded.request_key`,
    [
      session,
      value.id,
      type,
      JSON.stringify(value),
      seq,
      "conversationId" in value ? value.conversationId : null,
      conversation?.owner?.conversationId,
      conversation?.owner?.taskId,
      conversation?.parent?.conversationId,
      conversation?.parent?.at,
      write.type === "entry" ? write.value.head : null,
      indexKey(task?.kind),
      task?.state.status ?? submission?.status,
      task?.background,
      task?.abortRequested,
      indexKey(submission?.requestId),
    ],
  );
}

export interface ScanStart {
  order: ScanOrder;
  after: number | undefined;
}
export function scanStart(
  order: ScanOrder | undefined,
  cursor: Cursor | undefined,
  fallback: ScanOrder = "ascending",
): ScanStart {
  if (order && order !== "ascending" && order !== "descending")
    throw new StorageRequestError("Invalid scan order");
  if (!cursor) return { order: order ?? fallback, after: undefined };
  const { after, order: prior } = cursor;
  if (
    typeof after !== "number" ||
    !Number.isSafeInteger(after) ||
    (prior !== undefined && prior !== "ascending" && prior !== "descending")
  )
    throw new StorageRequestError("Invalid storage cursor");
  const direction = prior ?? fallback;
  if (order && order !== direction)
    throw new StorageRequestError(
      "The cursor continues a different scan order",
    );
  return { order: direction, after };
}

export function page<T extends { id: Id<string> }>(
  rows: T[],
  limit: number,
  order: ScanOrder,
): Page<T, Cursor> {
  if (rows.length <= limit) return { items: rows };
  const items = rows.slice(0, limit);
  const last = items.at(-1);
  if (!last) throw new StorageRequestError("Scan limit must be positive");
  return { items, next: { after: last.id, order } };
}

// Column names are specified solely by the storage methods. All query values use parameters.
export async function scan<T extends { id: Id<string> }>(
  db: Database,
  session: string,
  type: string,
  filters: Record<string, unknown>,
  limit: number,
  start: ScanStart,
  at?: Seq | "current",
) {
  const args: unknown[] = [session, type];
  const where = ["session_id=$1", "record_type=$2"];
  for (const [col, value] of Object.entries(filters)) {
    if (value === undefined) continue;
    where.push(`${col}=$${args.push(value)}`);
  }
  if (at === "current") where.push("retired_at_seq IS NULL");
  else if (at !== undefined) {
    const p = args.push(at);
    where.push(
      `created_at_seq <= $${p} AND (retired_at_seq IS NULL OR retired_at_seq > $${p})`,
    );
  }
  if (start.after !== undefined)
    where.push(
      `id ${start.order === "ascending" ? ">" : "<"} $${args.push(start.after)}`,
    );
  const result = await db.query<BodyRow>(
    `SELECT body FROM agent.runtime_records WHERE ${where.join(" AND ")}
    ORDER BY id ${start.order === "ascending" ? "ASC" : "DESC"} LIMIT $${args.push(limit + 1)}`,
    args,
  );
  return page(
    result.rows.map((row) => JSON.parse(row.body) as T),
    limit,
    start.order,
  );
}
