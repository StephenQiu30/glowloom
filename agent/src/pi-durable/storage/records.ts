import {
  StorageRequestError,
  type Cursor,
  type Id,
  type Page,
  type ScanOrder,
  type Seq,
  type StorageWrite,
} from '@earendil-works/pi-durable';
import {
  and,
  asc,
  desc,
  eq,
  gt,
  isNull,
  lt,
  lte,
  or,
  type SQL,
} from 'drizzle-orm';
import {
  database,
  type DatabaseConnection,
  type AgentTransaction,
} from '../../database/connection.js';
import { runtimeRecords } from '../../database/schema.js';

export type Database = DatabaseConnection;
export type RecordWrite = Extract<
  StorageWrite,
  { type: 'conversation' | 'entry' | 'task' | 'submission' }
>;
export type TaskRecord = Extract<RecordWrite, { type: 'task' }>['value'];
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
    throw new Error('Commit sequence space exhausted');
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
      await database(db)
        .select({ body: runtimeRecords.body })
        .from(runtimeRecords)
        .where(
          and(
            eq(runtimeRecords.session_id, session),
            eq(runtimeRecords.record_type, type),
            eq(runtimeRecords.id, id),
          ),
        )
    )[0],
  );
}

export async function writeRecord(
  client: AgentTransaction,
  session: string,
  seq: Seq,
  write: RecordWrite,
) {
  const { type, value } = write;
  const conversation = write.type === 'conversation' ? write.value : undefined;
  const task = write.type === 'task' ? write.value : undefined;
  const submission = write.type === 'submission' ? write.value : undefined;
  const existing = await client
    .select({ record_type: runtimeRecords.record_type })
    .from(runtimeRecords)
    .where(
      and(
        eq(runtimeRecords.session_id, session),
        eq(runtimeRecords.id, value.id),
      ),
    );
  const prior = existing[0]?.record_type;
  if (prior && (prior !== type || type === 'conversation' || type === 'entry'))
    throw new Refused(`ID ${value.id} already belongs to ${prior}`);
  const mutable = {
    body: JSON.stringify(value),
    commit_seq: seq,
    conversation_id: 'conversationId' in value ? value.conversationId : null,
    kind_key: indexKey(task?.kind),
    status: task?.state.status ?? submission?.status ?? null,
    background: task?.background ?? null,
    abort_requested: task?.abortRequested ?? null,
    request_key: indexKey(submission?.requestId),
  };
  await client
    .insert(runtimeRecords)
    .values({
      session_id: session,
      id: value.id,
      record_type: type,
      ...mutable,
      owner_conversation_id: conversation?.owner?.conversationId ?? null,
      owner_task_id: conversation?.owner?.taskId ?? null,
      parent_id: conversation?.parent?.conversationId ?? null,
      parent_at: conversation?.parent?.at ?? null,
      head_id: write.type === 'entry' ? (write.value.head ?? null) : null,
    })
    .onConflictDoUpdate({
      target: [runtimeRecords.session_id, runtimeRecords.id],
      set: mutable,
    });
}

export interface ScanStart {
  order: ScanOrder;
  after: number | undefined;
}
export function scanStart(
  order: ScanOrder | undefined,
  cursor: Cursor | undefined,
  fallback: ScanOrder = 'ascending',
): ScanStart {
  if (order && order !== 'ascending' && order !== 'descending')
    throw new StorageRequestError('Invalid scan order');
  if (!cursor) return { order: order ?? fallback, after: undefined };
  const { after, order: prior } = cursor;
  if (
    typeof after !== 'number' ||
    !Number.isSafeInteger(after) ||
    (prior !== undefined && prior !== 'ascending' && prior !== 'descending')
  )
    throw new StorageRequestError('Invalid storage cursor');
  const direction = prior ?? fallback;
  if (order && order !== direction)
    throw new StorageRequestError(
      'The cursor continues a different scan order',
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
  if (!last) throw new StorageRequestError('Scan limit must be positive');
  return { items, next: { after: last.id, order } };
}

type FilterColumns = Pick<
  typeof runtimeRecords.$inferSelect,
  | 'owner_task_id'
  | 'owner_conversation_id'
  | 'conversation_id'
  | 'kind_key'
  | 'status'
  | 'abort_requested'
  | 'background'
  | 'scope_key'
>;
type ScanFilters = {
  [K in keyof FilterColumns]?: FilterColumns[K] | undefined;
};

export async function scan<T extends { id: Id<string> }>(
  db: Database,
  session: string,
  type: string,
  filters: ScanFilters,
  limit: number,
  start: ScanStart,
  at?: Seq | 'current',
) {
  const where: (SQL | undefined)[] = [
    eq(runtimeRecords.session_id, session),
    eq(runtimeRecords.record_type, type),
  ];
  for (const col of Object.keys(filters) as (keyof ScanFilters)[]) {
    const value = filters[col];
    if (value !== undefined)
      where.push(
        value === null
          ? isNull(runtimeRecords[col])
          : eq(runtimeRecords[col], value),
      );
  }
  if (at === 'current') where.push(isNull(runtimeRecords.retired_at_seq));
  else if (at !== undefined) {
    where.push(
      lte(runtimeRecords.created_at_seq, at),
      or(
        isNull(runtimeRecords.retired_at_seq),
        gt(runtimeRecords.retired_at_seq, at),
      ),
    );
  }
  if (start.after !== undefined)
    where.push(
      start.order === 'ascending'
        ? gt(runtimeRecords.id, start.after)
        : lt(runtimeRecords.id, start.after),
    );
  const rows = await database(db)
    .select({ body: runtimeRecords.body })
    .from(runtimeRecords)
    .where(and(...where))
    .orderBy(
      start.order === 'ascending'
        ? asc(runtimeRecords.id)
        : desc(runtimeRecords.id),
    )
    .limit(limit + 1);
  return page(
    rows.map((row) => JSON.parse(row.body) as T),
    limit,
    start.order,
  );
}
