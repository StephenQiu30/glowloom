import type { Context } from '@earendil-works/chord';
import type {
  CommitResult,
  ConversationId,
  ConversationQuery,
  ConversationRecord,
  Cursor,
  DocumentAddress,
  DocumentId,
  DocumentPoint,
  DocumentQuery,
  DocumentRecord,
  EntryId,
  EntryQuery,
  EntryRecord,
  Id,
  JsonObject,
  Seq,
  Storage,
  StorageWrite,
  SubmissionId,
  SubmissionQuery,
  SubmissionRecord,
  TaskId,
  TaskQuery,
} from '@earendil-works/pi-durable';
import type { Pool } from 'pg';
import {
  database,
  withSavepoint,
  withTransaction,
  type AgentTransaction,
} from '../../database/connection.js';
import { publicMessageParts } from '../messages.js';
import {
  document,
  findDocument,
  scopeKey,
  writeDocuments,
} from './documents.js';
import { visibleEntries } from './entries.js';
import { assertOwnership, type SessionOwner } from './ownership.js';
import {
  decode,
  indexKey,
  page,
  record,
  Refused,
  safeSeq,
  scan,
  scanStart,
  writeRecord,
  type TaskRecord,
} from './records.js';

export interface RuntimeEvent {
  readonly type: string;
  readonly payload: JsonObject;
}
export interface PostgresStorageOptions {
  readonly projectEvents?: (
    writes: readonly StorageWrite[],
    seq: Seq,
  ) => readonly RuntimeEvent[];
}

class PostgresStorage implements Storage {
  private closed = false;
  private commitLine: Promise<unknown> = Promise.resolve();
  constructor(
    private pool: Pool,
    private session: string,
    private owner: SessionOwner | undefined,
    private options: PostgresStorageOptions,
  ) {}

  private check(ctx?: Context) {
    if (this.closed) throw new Error('PostgreSQL storage is closed');
    ctx?.abortSignal?.throwIfAborted();
  }
  private writer() {
    if (!this.owner) throw new Error('Storage is read-only');
    return this.owner;
  }
  async close(context?: Context) {
    // A cancelled caller must still release storage.
    void context;
    this.closed = true;
    await this.commitLine;
  }
  async mintId<I extends Id<string>>(): Promise<I> {
    this.check();
    const owner = this.writer();
    return withTransaction(this.pool, async (client) => {
      await assertOwnership(client, this.session, owner);
      const row = await database(client).runtimeSession.updateManyAndReturn({
        where: {
          session_id: this.session,
          last_id: { lt: BigInt(Number.MAX_SAFE_INTEGER) },
        },
        data: { last_id: { increment: 1n } },
        select: { last_id: true },
      });
      if (!row[0]) throw new Error('ID space is exhausted');
      await assertOwnership(client, this.session, owner);
      return Number(row[0].last_id) as I;
    });
  }
  async commit(
    batches: readonly (readonly StorageWrite[])[],
    receive: (index: number, seq: Seq) => void,
    context: Context,
  ): Promise<CommitResult> {
    this.check(context);
    const owner = this.writer();
    const copy = structuredClone(batches);
    const operation = this.commitLine.then(async () => {
      this.check(context);
      const { seqs, refused } = await withTransaction(
        this.pool,
        async (client) => {
          let lastSeq = await assertOwnership(client, this.session, owner);
          const seqs: Seq[] = [];
          let refused: Error | undefined;
          for (const batch of copy) {
            const seq = safeSeq(lastSeq + 1n);
            try {
              await withSavepoint(client, (batchTransaction) =>
                this.storeBatch(batchTransaction, batch, seq),
              );
            } catch (e) {
              if (!(e instanceof Refused)) throw e;
              refused = new Error(e.message);
              break;
            }
            lastSeq = BigInt(seq);
            seqs.push(seq);
          }
          await assertOwnership(client, this.session, owner);
          return { seqs, refused };
        },
      );
      seqs.forEach((seq, index) => receive(index, seq));
      return refused
        ? { stored: seqs.length, refused }
        : { stored: seqs.length };
    });
    this.commitLine = operation.catch(() => {});
    return operation;
  }
  private async storeBatch(
    client: AgentTransaction,
    batch: readonly StorageWrite[],
    seq: Seq,
  ) {
    const db = database(client);
    let maxId = 1;
    for (const write of batch) {
      if ('value' in write) {
        await writeRecord(client, this.session, seq, write);
        maxId = Math.max(maxId, write.value.id);
      } else if ('record' in write) maxId = Math.max(maxId, write.record.id);
    }
    await writeDocuments(client, this.session, seq, batch);
    const events = this.options.projectEvents?.(batch, seq) ?? [
      { type: 'runtime.committed', payload: {} },
    ];
    for (const [idx, event] of events.entries()) {
      const parts = publicMessageParts(event, seq, idx + 1);
      const payload: JsonObject = parts.length
        ? {
            ...event.payload,
            message_parts: parts.map((part) => ({ ...part })),
          }
        : event.payload;
      await db.runtimeEvent.create({
        data: {
          session_id: this.session,
          commit_seq: BigInt(seq),
          event_ordinal: idx + 1,
          event_type: event.type,
          payload,
        },
      });
      for (const part of parts) {
        await db.publicMessage.create({
          data: {
            session_id: this.session,
            run_id: part.run_id,
            commit_seq: BigInt(seq),
            event_ordinal: idx + 1,
            part_index: part.part_index,
            message_id: part.message_id,
            conversation_id: BigInt(part.conversation_id),
            sequence: part.sequence,
            kind: part.kind,
            content: part.content,
          },
        });
      }
    }
    // Ownership holds the session row lock for the whole commit transaction.
    const current = await db.runtimeSession.findUniqueOrThrow({
      select: { last_id: true },
      where: { session_id: this.session },
    });
    await db.runtimeSession.update({
      where: { session_id: this.session },
      data: {
        last_id:
          current.last_id > BigInt(maxId) ? current.last_id : BigInt(maxId),
        commit_seq: BigInt(seq),
      },
    });
  }
  async conversation(id: ConversationId, ctx: Context) {
    this.check(ctx);
    return record<ConversationRecord>(
      this.pool,
      this.session,
      'conversation',
      id,
    );
  }
  async scanConversations(
    q: ConversationQuery,
    limit: number,
    cursor: Cursor | undefined,
    ctx: Context,
  ) {
    this.check(ctx);
    return scan<ConversationRecord>(
      this.pool,
      this.session,
      'conversation',
      {
        owner_task_id: q.ownerTaskId,
        owner_conversation_id: q.ownerConversationId,
      },
      limit,
      scanStart(q.order, cursor),
    );
  }
  async entry(
    id: EntryId,
    ctx: Context,
  ): Promise<{ entry: EntryRecord; commitSeq: Seq } | undefined>;
  async entry(
    conversationId: ConversationId,
    id: EntryId,
    ctx: Context,
  ): Promise<{ entry: EntryRecord; commitSeq: Seq } | undefined>;
  async entry(
    first: ConversationId | EntryId,
    second: EntryId | Context,
    ctx?: Context,
  ) {
    if (typeof second === 'number') {
      this.check(ctx);
      return (
        await withTransaction(
          this.pool,
          (client) =>
            visibleEntries(
              client,
              this.session,
              {
                conversationId: first as ConversationId,
                minEntryId: second,
                maxEntryId: second,
              },
              1,
              scanStart(undefined, undefined),
            ),
          true,
        )
      )[0];
    }
    this.check(second);
    const row = await database(this.pool).runtimeRecord.findFirst({
      select: { body: true, commit_seq: true },
      where: {
        session_id: this.session,
        record_type: 'entry',
        id: BigInt(first),
      },
    });
    return row
      ? {
          entry: JSON.parse(row.body) as EntryRecord,
          commitSeq: safeSeq(row.commit_seq),
        }
      : undefined;
  }
  async findLatestHeadMarker(
    conversationId: ConversationId,
    atOrBeforeEntryId: EntryId | undefined,
    ctx: Context,
  ) {
    this.check(ctx);
    const query =
      atOrBeforeEntryId === undefined
        ? { conversationId }
        : { conversationId, maxEntryId: atOrBeforeEntryId };
    const value = (
      await withTransaction(
        this.pool,
        (client) =>
          visibleEntries(
            client,
            this.session,
            query,
            1,
            scanStart('descending', undefined),
            true,
          ),
        true,
      )
    )[0];
    return value?.entry as (EntryRecord & { head: EntryId }) | undefined;
  }
  async scanEntries(
    q: EntryQuery,
    limit: number,
    cursor: Cursor | undefined,
    ctx: Context,
  ) {
    this.check(ctx);
    const start = scanStart(q.order, cursor, 'descending');
    const rows = await withTransaction(
      this.pool,
      (client) => visibleEntries(client, this.session, q, limit + 1, start),
      true,
    );
    return page(
      rows.map((row) => row.entry),
      limit,
      start.order,
    );
  }
  async task(id: TaskId, ctx: Context) {
    this.check(ctx);
    return record<TaskRecord>(this.pool, this.session, 'task', id);
  }
  async scanTasks(
    q: TaskQuery,
    limit: number,
    cursor: Cursor | undefined,
    ctx: Context,
  ) {
    this.check(ctx);
    return scan<TaskRecord>(
      this.pool,
      this.session,
      'task',
      {
        conversation_id: q.conversationId,
        kind_key: q.kind === undefined ? undefined : indexKey(q.kind),
        status: q.status,
        abort_requested: q.abortRequested,
        background: q.background,
      },
      limit,
      scanStart(q.order, cursor),
    );
  }
  async submission(id: SubmissionId, ctx: Context) {
    this.check(ctx);
    return record<SubmissionRecord>(this.pool, this.session, 'submission', id);
  }
  async scanSubmissions(
    q: SubmissionQuery,
    limit: number,
    cursor: Cursor | undefined,
    ctx: Context,
  ) {
    this.check(ctx);
    return scan<SubmissionRecord>(
      this.pool,
      this.session,
      'submission',
      { conversation_id: q.conversationId, status: q.status },
      limit,
      scanStart(q.order, cursor),
    );
  }
  async submissionByRequest(
    conversationId: ConversationId,
    requestId: string,
    ctx: Context,
  ) {
    this.check(ctx);
    const row = await database(this.pool).runtimeRecord.findFirst({
      select: { body: true },
      where: {
        session_id: this.session,
        conversation_id: BigInt(conversationId),
        request_key: JSON.stringify(requestId),
      },
    });
    return decode<SubmissionRecord>(row);
  }
  async findDocument(
    address: DocumentAddress,
    at: DocumentPoint,
    ctx: Context,
  ) {
    this.check(ctx);
    return findDocument(this.pool, this.session, address, at);
  }
  async document(id: DocumentId, at: DocumentPoint, ctx: Context) {
    this.check(ctx);
    return document(this.pool, this.session, id, at);
  }
  async scanDocuments(
    q: DocumentQuery,
    limit: number,
    cursor: Cursor | undefined,
    ctx: Context,
  ) {
    this.check(ctx);
    return scan<DocumentRecord>(
      this.pool,
      this.session,
      'document',
      {
        scope_key: scopeKey(q.scope),
        kind_key: q.kind === undefined ? undefined : indexKey(q.kind),
      },
      limit,
      scanStart('ascending', cursor),
      q.at,
    );
  }
}

export async function openPostgresStorage(
  pool: Pool,
  sessionId: string,
  owner: SessionOwner,
  options: PostgresStorageOptions = {},
) {
  await assertOwnership(pool, sessionId, owner);
  return new PostgresStorage(pool, sessionId, owner, options);
}
export async function openReadonlyPostgresStorage(
  pool: Pool,
  sessionId: string,
) {
  const found = await database(pool).runtimeSession.findUnique({
    select: { session_id: true },
    where: { session_id: sessionId },
  });
  if (!found) throw new Error('Unknown storage session');
  return new PostgresStorage(pool, sessionId, undefined, {});
}
