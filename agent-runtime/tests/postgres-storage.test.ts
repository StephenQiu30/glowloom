import { randomUUID } from "node:crypto";
import { BACKGROUND_CONTEXT as context } from "@earendil-works/chord/context";
import type { ConversationId, EntryId } from "@earendil-works/pi-durable";
import { registerStorageConformance } from "@earendil-works/pi-durable/testing";
import { Pool } from "pg";
import { afterAll, describe, expect, it } from "vitest";
import { openPostgresStorage } from "../src/postgres-storage.js";

const connectionString = process.env.GLOWLOOM_TEST_AGENT_DB_DSN;
if (!connectionString)
  throw new Error(
    "GLOWLOOM_TEST_AGENT_DB_DSN is required; use an isolated test database",
  );
const pool = new Pool({ connectionString, max: 6 });
const runtimePool = new Pool({
  connectionString,
  max: 6,
  options: "-c role=glowloom_agent_runtime",
});
const sessions: string[] = [];
const sessionID = () => {
  const id = randomUUID();
  sessions.push(id);
  return id;
};
afterAll(async () => {
  await pool.query(
    "DELETE FROM agent.runtime_session WHERE id = ANY($1::uuid[])",
    [sessions],
  );
  await runtimePool.end();
  await pool.end();
});

registerStorageConformance(
  { describe, expect, it },
  "PostgreSQL durable SDK storage",
  async (use) => {
    const storage = await openPostgresStorage(runtimePool, sessionID());
    try {
      await use(storage);
    } finally {
      await storage.close(context);
    }
  },
);

it("rejects a second writer until the database lock is released", async () => {
  const id = sessionID();
  const first = await openPostgresStorage(runtimePool, id);
  try {
    await expect(openPostgresStorage(runtimePool, id)).rejects.toThrow(
      "runtime_session_owned",
    );
  } finally {
    await first.close(context);
  }
  const second = await openPostgresStorage(runtimePool, id);
  expect(second.ownerEpoch).toBe(2);
  await second.close(context);
});

it("never reuses an ID that was allocated before a crash without a checkpoint", async () => {
  const id = sessionID();
  const first = await openPostgresStorage(runtimePool, id);
  const allocated = await first.mintId();
  await first.close(context);
  const reopened = await openPostgresStorage(runtimePool, id);
  expect(await reopened.mintId()).toBeGreaterThan(allocated);
  await reopened.close(context);
});

it("restores the exact original transcript after reopening PostgreSQL storage", async () => {
  const id = sessionID();
  const first = await openPostgresStorage(runtimePool, id);
  const conversationId = await first.mintId<ConversationId>();
  const entryId = await first.mintId<EntryId>();
  await first.commit(
    [
      { type: "conversation", value: { id: conversationId } },
      {
        type: "entry",
        value: {
          id: entryId,
          conversationId,
          kind: "story.input",
          data: { text: "序幕：雨夜" },
        },
      },
    ],
    context,
  );
  await first.close(context);
  const reopened = await openPostgresStorage(runtimePool, id);
  expect((await reopened.entry(entryId, context))?.entry.data).toEqual({
    text: "序幕：雨夜",
  });
  await reopened.close(context);
});

it("fails closed on connection loss and refuses reads from a stale in-memory index", async () => {
  const id = sessionID();
  const first = await openPostgresStorage(runtimePool, id);
  await pool.query(
    "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE application_name=$1",
    [`agent-runtime:${id}`],
  );
  await expect
    .poll(async () => {
      try {
        await first.scanConversations({}, 1, undefined, context);
        return false;
      } catch {
        return true;
      }
    })
    .toBe(true);
  await expect(first.mintId()).rejects.toThrow();
  await first.close(context);
  const recovered = await openPostgresStorage(runtimePool, id);
  expect(recovered.ownerEpoch).toBe(2);
  await recovered.close(context);
});

it("refuses incompatible storage versions and damaged journal hashes", async () => {
  const id = sessionID();
  const first = await openPostgresStorage(runtimePool, id);
  await first.commit([], context);
  await first.close(context);
  await pool.query(
    "UPDATE agent.runtime_session SET storage_abi=999 WHERE id=$1",
    [id],
  );
  await expect(openPostgresStorage(runtimePool, id)).rejects.toThrow(
    "runtime_storage_incompatible",
  );
  await pool.query(
    "UPDATE agent.runtime_session SET storage_abi=1 WHERE id=$1",
    [id],
  );
  await pool.query(
    "UPDATE agent.runtime_commit SET writes=convert_to('[{}]','UTF8') WHERE session_id=$1",
    [id],
  );
  await expect(openPostgresStorage(runtimePool, id)).rejects.toThrow(
    "runtime_storage_corrupt",
  );
});

it("serializes ID allocation with durable commits so an acknowledged ID cannot join a rollback", async () => {
  const id = sessionID();
  const storage = await openPostgresStorage(runtimePool, id);
  try {
    const [_, next] = await Promise.all([
      storage.commit(
        [{ type: "conversation", value: { id: 100 as ConversationId } }],
        context,
      ),
      storage.mintId(),
    ]);
    expect(next).toBeGreaterThan(100);
  } finally {
    await storage.close(context);
  }
});

it("poisons the writer after a lost COMMIT acknowledgement and recovers the actual database result", async () => {
  const id = sessionID();
  const client = await runtimePool.connect();
  const original = client.query.bind(client);
  const storage = await openPostgresStorage(
    { connect: async () => client } as unknown as Pool,
    id,
  );
  // Inject only the network acknowledgement failure; the real PostgreSQL COMMIT still executes.
  client.query = (async (...args: unknown[]) => {
    const result = await (original as (...args: unknown[]) => Promise<unknown>)(
      ...args,
    );
    if (args[0] === "COMMIT") throw new Error("commit_ack_lost");
    return result;
  }) as typeof client.query;
  await expect(
    storage.commit(
      [{ type: "conversation", value: { id: 100 as ConversationId } }],
      context,
    ),
  ).rejects.toThrow("commit_ack_lost");
  await expect(
    storage.conversation(100 as ConversationId, context),
  ).rejects.toThrow();
  await storage.close(context);
  const recovered = await openPostgresStorage(runtimePool, id);
  try {
    expect(
      (await recovered.conversation(100 as ConversationId, context))?.id,
    ).toBe(100);
    expect(await recovered.mintId()).toBeGreaterThan(100);
    expect(
      (
        await pool.query(
          "SELECT count(*) AS count FROM agent.runtime_commit WHERE session_id=$1",
          [id],
        )
      ).rows[0].count,
    ).toBe("1");
  } finally {
    await recovered.close(context);
  }
});
