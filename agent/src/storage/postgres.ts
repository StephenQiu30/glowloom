import type { Context } from "@earendil-works/chord";
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
} from "@earendil-works/pi-durable";
import type { Pool, PoolClient } from "pg";
import { publicMessageParts } from "../harness/messages.js";
import {
  document,
  findDocument,
  scopeKey,
  writeDocuments,
} from "./documents.js";
import { visibleEntries } from "./entries.js";
import { assertOwnership, type SessionOwner } from "./ownership.js";
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
  type BodyRow,
  type TaskRecord,
} from "./records.js";

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

async function transaction<T>(
  pool: Pool,
  work: (client: PoolClient) => Promise<T>,
) {
  const client = await pool.connect();
  let broken = false;
  let committing = false;
  // pg rejects the in-flight query and emits an error on the checked-out client.
  const disconnected = () => {
    broken = true;
  };
  client.on("error", disconnected);
  try {
    await client.query("BEGIN");
    const value = await work(client);
    committing = true;
    await client.query("COMMIT");
    return value;
  } catch (error) {
    // COMMIT may already have been made durable when the connection failed. Never replay it.
    if (committing) {
      broken = true;
      throw new Error(
        "PostgreSQL COMMIT outcome is unknown; reopen and read the durable records",
        { cause: error },
      );
    }
    try {
      await client.query("ROLLBACK");
    } catch {
      broken = true;
    }
    throw error;
  } finally {
    client.removeListener("error", disconnected);
    client.release(broken);
  }
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
    if (this.closed) throw new Error("PostgreSQL storage is closed");
    ctx?.abortSignal?.throwIfAborted();
  }
  private writer() {
    if (!this.owner) throw new Error("Storage is read-only");
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
    return transaction(this.pool, async (client) => {
      await assertOwnership(client, this.session, owner);
      const row = (
        await client.query<{ last_id: string }>(
          `UPDATE agent.runtime_sessions SET last_id=last_id+1
        WHERE session_id=$1 AND last_id<9007199254740991 RETURNING last_id`,
          [this.session],
        )
      ).rows[0];
      if (!row) throw new Error("ID space is exhausted");
      await assertOwnership(client, this.session, owner);
      return Number(row.last_id) as I;
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
      const { seqs, refused } = await transaction(this.pool, async (client) => {
        let lastSeq = await assertOwnership(client, this.session, owner);
        const seqs: Seq[] = [];
        let refused: Error | undefined;
        for (const batch of copy) {
          const seq = safeSeq(lastSeq + 1);
          await client.query("SAVEPOINT pi_batch");
          try {
            await this.storeBatch(client, batch, seq);
          } catch (e) {
            if (!(e instanceof Refused)) throw e;
            await client.query("ROLLBACK TO SAVEPOINT pi_batch");
            refused = new Error(e.message);
            break;
          }
          await client.query("RELEASE SAVEPOINT pi_batch");
          lastSeq = seq;
          seqs.push(seq);
        }
        await assertOwnership(client, this.session, owner);
        return { seqs, refused };
      });
      seqs.forEach((seq, index) => receive(index, seq));
      return refused
        ? { stored: seqs.length, refused }
        : { stored: seqs.length };
    });
    this.commitLine = operation.catch(() => {});
    return operation;
  }
  private async storeBatch(
    client: PoolClient,
    batch: readonly StorageWrite[],
    seq: Seq,
  ) {
    let maxId = 1;
    for (const write of batch) {
      if ("value" in write) {
        await writeRecord(client, this.session, seq, write);
        maxId = Math.max(maxId, write.value.id);
      } else if ("record" in write) maxId = Math.max(maxId, write.record.id);
    }
    await writeDocuments(client, this.session, seq, batch);
    const events = this.options.projectEvents?.(batch, seq) ?? [
      { type: "runtime.committed", payload: {} },
    ];
    for (const [idx, event] of events.entries()) {
      const parts = publicMessageParts(event, seq, idx + 1);
      const payload = parts.length
        ? { ...event.payload, message_parts: parts }
        : event.payload;
      await client.query(
        `INSERT INTO agent.runtime_events(session_id,commit_seq,event_ordinal,event_type,payload)
        VALUES($1,$2,$3,$4,$5::jsonb)`,
        [this.session, seq, idx + 1, event.type, JSON.stringify(payload)],
      );
      for (const part of parts) {
        await client.query(
          `INSERT INTO agent.public_messages(session_id,run_id,commit_seq,event_ordinal,part_index,message_id,conversation_id,sequence,kind,content) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
          [
            this.session,
            part.run_id,
            seq,
            idx + 1,
            part.part_index,
            part.message_id,
            part.conversation_id,
            part.sequence,
            part.kind,
            part.content,
          ],
        );
      }
    }
    await client.query(
      "UPDATE agent.runtime_sessions SET last_id=greatest(last_id,$2),commit_seq=$3 WHERE session_id=$1",
      [this.session, maxId, seq],
    );
  }
  async conversation(id: ConversationId, ctx: Context) {
    this.check(ctx);
    return record<ConversationRecord>(
      this.pool,
      this.session,
      "conversation",
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
      "conversation",
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
    if (typeof second === "number") {
      this.check(ctx);
      return (
        await visibleEntries(
          this.pool,
          this.session,
          {
            conversationId: first as ConversationId,
            minEntryId: second,
            maxEntryId: second,
          },
          1,
          scanStart(undefined, undefined),
        )
      )[0];
    }
    this.check(second);
    const row = (
      await this.pool.query<BodyRow & { commit_seq: string }>(
        "SELECT body,commit_seq FROM agent.runtime_records WHERE session_id=$1 AND record_type='entry' AND id=$2",
        [this.session, first],
      )
    ).rows[0];
    return (
      row && {
        entry: JSON.parse(row.body) as EntryRecord,
        commitSeq: safeSeq(Number(row.commit_seq)),
      }
    );
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
      await visibleEntries(
        this.pool,
        this.session,
        query,
        1,
        scanStart("descending", undefined),
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
    const start = scanStart(q.order, cursor, "descending");
    const rows = await visibleEntries(
      this.pool,
      this.session,
      q,
      limit + 1,
      start,
    );
    return page(
      rows.map((row) => row.entry),
      limit,
      start.order,
    );
  }
  async task(id: TaskId, ctx: Context) {
    this.check(ctx);
    return record<TaskRecord>(this.pool, this.session, "task", id);
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
      "task",
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
    return record<SubmissionRecord>(this.pool, this.session, "submission", id);
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
      "submission",
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
    const result = await this.pool.query<BodyRow>(
      "SELECT body FROM agent.runtime_records WHERE session_id=$1 AND conversation_id=$2 AND request_key=$3",
      [this.session, conversationId, indexKey(requestId)],
    );
    return decode<SubmissionRecord>(result.rows[0]);
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
      "document",
      {
        scope_key: scopeKey(q.scope),
        kind_key: q.kind === undefined ? undefined : indexKey(q.kind),
      },
      limit,
      scanStart("ascending", cursor),
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
  const found = await pool.query(
    "SELECT 1 FROM agent.runtime_sessions WHERE session_id=$1",
    [sessionId],
  );
  if (!found.rowCount) throw new Error("Unknown storage session");
  return new PostgresStorage(pool, sessionId, undefined, {});
}
