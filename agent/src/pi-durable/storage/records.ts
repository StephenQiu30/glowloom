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
  database,
  type DatabaseConnection,
  type AgentTransaction,
} from '../../database/connection.js';
import type { Prisma } from '../../database/generated/client.js';

export type Database = DatabaseConnection;
export type RecordWrite = Extract<
  StorageWrite,
  { type: 'conversation' | 'entry' | 'task' | 'submission' }
>;
export type TaskRecord = Extract<RecordWrite, { type: 'task' }>['value'];
export type BodyRow = { body: string };
export function decode<T>(row: BodyRow | null | undefined): T | undefined {
  return row ? (JSON.parse(row.body) as T) : undefined;
}
export function indexKey(value: string | undefined) {
  return value === undefined ? null : JSON.stringify(value);
}
export class Refused extends Error {}
export function safeSeq(value: number | bigint): Seq {
  const n = Number(value);
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
    await database(db).runtimeRecord.findFirst({
      select: { body: true },
      where: { session_id: session, record_type: type, id: BigInt(id) },
    }),
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
  const db = database(client);
  const existing = await db.runtimeRecord.findUnique({
    select: { record_type: true },
    where: { session_id_id: { session_id: session, id: BigInt(value.id) } },
  });
  const prior = existing?.record_type;
  if (prior && (prior !== type || type === 'conversation' || type === 'entry'))
    throw new Refused(`ID ${value.id} already belongs to ${prior}`);
  const mutable = {
    body: JSON.stringify(value),
    commit_seq: BigInt(seq),
    conversation_id:
      'conversationId' in value ? BigInt(value.conversationId) : null,
    kind_key: indexKey(task?.kind),
    status: task?.state.status ?? submission?.status ?? null,
    background: task?.background ?? null,
    abort_requested: task?.abortRequested ?? null,
    request_key: indexKey(submission?.requestId),
  };
  await db.runtimeRecord.upsert({
    where: { session_id_id: { session_id: session, id: BigInt(value.id) } },
    create: {
      session_id: session,
      id: BigInt(value.id),
      record_type: type,
      ...mutable,
      owner_conversation_id: conversation?.owner
        ? BigInt(conversation.owner.conversationId)
        : null,
      owner_task_id: conversation?.owner
        ? BigInt(conversation.owner.taskId)
        : null,
      parent_id: conversation?.parent
        ? BigInt(conversation.parent.conversationId)
        : null,
      parent_at: conversation?.parent ? BigInt(conversation.parent.at) : null,
      head_id:
        write.type === 'entry' && write.value.head !== undefined
          ? BigInt(write.value.head)
          : null,
    },
    update: mutable,
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

type ScanFilters = {
  owner_task_id?: number | undefined;
  owner_conversation_id?: number | undefined;
  conversation_id?: number | undefined;
  kind_key?: string | null | undefined;
  status?: string | undefined;
  abort_requested?: boolean | undefined;
  background?: boolean | undefined;
  scope_key?: string | undefined;
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
  const where: Prisma.RuntimeRecordWhereInput = {
    session_id: session,
    record_type: type,
    ...(filters.owner_task_id === undefined
      ? {}
      : { owner_task_id: BigInt(filters.owner_task_id) }),
    ...(filters.owner_conversation_id === undefined
      ? {}
      : { owner_conversation_id: BigInt(filters.owner_conversation_id) }),
    ...(filters.conversation_id === undefined
      ? {}
      : { conversation_id: BigInt(filters.conversation_id) }),
    ...(filters.kind_key === undefined ? {} : { kind_key: filters.kind_key }),
    ...(filters.status === undefined ? {} : { status: filters.status }),
    ...(filters.abort_requested === undefined
      ? {}
      : { abort_requested: filters.abort_requested }),
    ...(filters.background === undefined
      ? {}
      : { background: filters.background }),
    ...(filters.scope_key === undefined
      ? {}
      : { scope_key: filters.scope_key }),
  };
  if (at === 'current') where.retired_at_seq = null;
  else if (at !== undefined) {
    where.created_at_seq = { lte: BigInt(at) };
    where.OR = [
      { retired_at_seq: null },
      { retired_at_seq: { gt: BigInt(at) } },
    ];
  }
  if (start.after !== undefined)
    where.id =
      start.order === 'ascending'
        ? { gt: BigInt(start.after) }
        : { lt: BigInt(start.after) };
  const rows = await database(db).runtimeRecord.findMany({
    select: { body: true },
    where,
    orderBy: { id: start.order === 'ascending' ? 'asc' : 'desc' },
    take: limit + 1,
  });
  return page(
    rows.map((row) => JSON.parse(row.body) as T),
    limit,
    start.order,
  );
}
