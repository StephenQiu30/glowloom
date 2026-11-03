import { BACKGROUND_CONTEXT } from "@earendil-works/chord/context";
import type { Context } from "@earendil-works/chord";
import {
  MemoryStorage,
  type Id,
  type Seq,
  type Storage,
  type StorageWrite,
} from "@earendil-works/pi-durable";
import type { Pool, PoolClient } from "pg";
import { hashJSON } from "./canonical.js";

const STORAGE_ABI = 1;
const SDK_VERSION = "1.3.0";
const LOCK_NAMESPACE = 716438;

export interface OwnedStorage extends Storage {
  readonly ownerEpoch: number;
  readonly signal: AbortSignal;
}

type SessionRow = {
  lock_key: number;
  storage_abi: number;
  sdk_version: string;
  owner_epoch: string;
  last_seq: string;
  last_hash: string;
};

// MemoryStorage is the SDK's validated read index, never a durable backend here.
// The same prepare → durable write → apply contract is used by the upstream journal adapter.
class PostgresStorage extends MemoryStorage implements OwnedStorage {
  private dead = false;
  private readonly lifecycle = new AbortController();
  readonly signal = this.lifecycle.signal;
  private databaseLine: Promise<unknown> = Promise.resolve();
  private lastSeq = 0;
  private lastHash = "";
  readonly ownerEpoch: number;

  constructor(
    private readonly connection: PoolClient,
    private readonly sessionId: string,
    epoch: number,
  ) {
    super();
    this.ownerEpoch = epoch;
    connection.on("error", () => this.poison());
    connection.on("end", () => this.poison());
  }

  async restore(row: SessionRow): Promise<void> {
    while (this.lastSeq < Number(row.last_seq)) {
      const batch = await this.connection.query<{
        seq: string;
        writes: Buffer;
        previous_hash: string;
        content_hash: string;
      }>(
        "SELECT seq,writes,previous_hash,content_hash FROM agent.runtime_commit WHERE session_id=$1 AND seq>$2 ORDER BY seq LIMIT 256",
        [this.sessionId, this.lastSeq],
      );
      if (batch.rowCount === 0) throw new Error("runtime_storage_corrupt");
      for (const record of batch.rows) {
        const seq = Number(record.seq);
        const writes = JSON.parse(
          record.writes.toString("utf8"),
        ) as StorageWrite[];
        if (
          seq !== this.lastSeq + 1 ||
          seq > Number(row.last_seq) ||
          record.previous_hash !== this.lastHash ||
          hashJSON({ seq, previous_hash: this.lastHash, writes }) !==
            record.content_hash
        )
          throw new Error("runtime_storage_corrupt");
        super.prepareCommit(writes, seq as Seq).apply();
        this.lastSeq = seq;
        this.lastHash = record.content_hash;
      }
    }
    if (this.lastHash !== row.last_hash)
      throw new Error("runtime_storage_corrupt");
  }

  override commit(
    writes: readonly StorageWrite[],
    context: Context,
  ): Promise<Seq> {
    const detached = JSON.parse(JSON.stringify(writes)) as StorageWrite[];
    return this.serialize(() => this.persist(detached, context));
  }

  private async persist(
    writes: readonly StorageWrite[],
    context: Context,
  ): Promise<Seq> {
    context.abortSignal?.throwIfAborted();
    this.assertAlive();
    // Invalid writes must have no durable effects. Detach before the first await.
    const prepared = super.prepareCommit(writes);
    const seq = Number(prepared.seq);
    const hash = hashJSON({
      seq,
      previous_hash: this.lastHash,
      writes: JSON.parse(JSON.stringify(prepared.writes)),
    });
    try {
      await this.connection.query("BEGIN");
      const updated = await this.connection.query(
        "UPDATE agent.runtime_session SET last_seq=$3,last_hash=$4,updated_at=now() WHERE id=$1 AND owner_epoch=$2 AND last_seq=$5 AND last_hash=$6 RETURNING id",
        [
          this.sessionId,
          this.ownerEpoch,
          seq,
          hash,
          this.lastSeq,
          this.lastHash,
        ],
      );
      if (updated.rowCount !== 1) throw new Error("runtime_owner_fenced");
      await this.connection.query(
        "INSERT INTO agent.runtime_commit(session_id,seq,owner_epoch,writes,previous_hash,content_hash) VALUES($1,$2,$3,$4,$5,$6)",
        [
          this.sessionId,
          seq,
          this.ownerEpoch,
          Buffer.from(JSON.stringify(prepared.writes)),
          this.lastHash,
          hash,
        ],
      );
      await this.connection.query("COMMIT");
      // No retry on ambiguous commit: reopening replays PostgreSQL's actual result.
      this.assertAlive();
      prepared.apply();
      this.lastSeq = seq;
      this.lastHash = hash;
      return prepared.seq;
    } catch (error) {
      this.poison();
      throw error;
    }
  }

  override mintId<I extends Id<string>>(): Promise<I> {
    return this.serialize(() => this.allocate<I>());
  }

  private async allocate<I extends Id<string>>(): Promise<I> {
    this.assertAlive();
    const minimum = await super.mintId<I>();
    try {
      const result = await this.connection.query<{ next_id: string }>(
        "UPDATE agent.runtime_session SET next_id=GREATEST(next_id+1,$3) WHERE id=$1 AND owner_epoch=$2 AND next_id<9007199254740991 RETURNING next_id",
        [this.sessionId, this.ownerEpoch, minimum],
      );
      if (result.rowCount !== 1) throw new Error("runtime_owner_fenced");
      this.assertAlive();
      return Number(result.rows[0]!.next_id) as I;
    } catch (error) {
      this.poison();
      throw error;
    }
  }

  override async close(context: Context): Promise<void> {
    if (this.dead) return;
    this.dead = true;
    await super.close(context);
    this.lifecycle.abort(new Error("runtime_storage_closed"));
    // A session lock must never be returned to the pool on a reusable connection.
    this.connection.release(true);
  }

  private serialize<T>(operation: () => Promise<T>): Promise<T> {
    const next = this.databaseLine.then(operation);
    this.databaseLine = next.catch(() => {});
    return next;
  }

  private assertAlive(): void {
    if (this.dead) throw new Error("runtime_storage_closed");
  }

  private poison(): void {
    if (this.dead) return;
    this.dead = true;
    void super.close(BACKGROUND_CONTEXT);
    this.lifecycle.abort(new Error("runtime_storage_failed"));
    this.connection.release(true);
  }
}

export async function openPostgresStorage(
  pool: Pool,
  sessionId: string,
): Promise<OwnedStorage> {
  if (
    !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
      sessionId,
    )
  )
    throw new Error("invalid_runtime_session");
  const connection = await pool.connect();
  let store: PostgresStorage | undefined;
  try {
    await connection.query("SELECT set_config('application_name',$1,false)", [
      `agent-runtime:${sessionId}`,
    ]);
    await connection.query(
      "INSERT INTO agent.runtime_session(id) VALUES($1) ON CONFLICT(id) DO NOTHING",
      [sessionId],
    );
    const found = await connection.query<SessionRow>(
      "SELECT lock_key,storage_abi,sdk_version,owner_epoch,last_seq,last_hash FROM agent.runtime_session WHERE id=$1",
      [sessionId],
    );
    const row = found.rows[0]!;
    const lock = await connection.query<{ owned: boolean }>(
      "SELECT pg_try_advisory_lock($1::integer,$2::integer) AS owned",
      [LOCK_NAMESPACE, row.lock_key],
    );
    if (!lock.rows[0]?.owned) throw new Error("runtime_session_owned");
    if (row.storage_abi !== STORAGE_ABI || row.sdk_version !== SDK_VERSION)
      throw new Error("runtime_storage_incompatible");
    const claimed = await connection.query<SessionRow>(
      "UPDATE agent.runtime_session SET owner_epoch=owner_epoch+1,updated_at=now() WHERE id=$1 RETURNING lock_key,storage_abi,sdk_version,owner_epoch,last_seq,last_hash",
      [sessionId],
    );
    const current = claimed.rows[0]!;
    store = new PostgresStorage(
      connection,
      sessionId,
      Number(current.owner_epoch),
    );
    await store.restore(current);
    return store;
  } catch (error) {
    if (store) await store.close(BACKGROUND_CONTEXT);
    else connection.release(true);
    throw error;
  }
}
