import { BACKGROUND_CONTEXT as context } from "@earendil-works/chord/context";
import { ROOT_CONVERSATION_ID } from "@earendil-works/pi-durable";
import { openPostgresStorage } from "../src/postgres-storage.js";
import { randomUUID } from "node:crypto";
import { afterAll, expect, it } from "vitest";
import { openOwnedSession } from "../src/session-owner.js";
import {
  ControlledGateway,
  identity,
  model,
  pool,
} from "./fixtures/gateway.js";
const sessions: string[] = [];
afterAll(async () => {
  await pool.query(
    "DELETE FROM agent.runtime_session WHERE id=ANY($1::uuid[])",
    [sessions],
  );
  await pool.end();
});

it("routes compaction through its own durable task identity and refuses SDK fallback requests", async () => {
  const gateway = await new ControlledGateway().start();
  const scope = identity();
  sessions.push(scope.runtime_session_id);
  const session = await openOwnedSession({
    pool,
    identity: scope,
    model: { ...model, context_window: 65536 },
    gateway: gateway.client,
  });
  try {
    for (let n = 0; n < 3; n++) {
      const receipt = await session.acceptCommand({
        request_id: randomUUID(),
        message: `第${n}幕：${"雨夜人物对白。".repeat(150)}`,
      });
      await session.waitSubmission(receipt.submission_id);
    }
    await session.compact("保留人物与场景约束");
    const exchanges = gateway.calls.filter((c) => c.kind === "compaction");
    expect(exchanges).toHaveLength(1);
    expect(exchanges[0]!.task_id).not.toBe(gateway.calls[0]!.task_id);
    expect(
      (await session.snapshot()).entries.some(
        (e) => e.kind === "pi.compaction",
      ),
    ).toBe(true);
    await session.close();
    const restored = await openPostgresStorage(pool, scope.runtime_session_id);
    try {
      const docs = await restored.scanDocuments(
        {
          scope: { kind: "conversation", conversationId: ROOT_CONVERSATION_ID },
          at: "current",
          kind: "glowloom.compaction-receipt",
        },
        10,
        undefined,
        context,
      );
      expect(docs.items).toHaveLength(1);
      const persisted = await restored.document(
        docs.items[0]!.id,
        "current",
        context,
      );
      expect(persisted?.value.receipt).toMatchObject({
        exchange_id: exchanges[0]!.exchange_id,
        usage: { input: 3, output: 2, totalTokens: 5 },
      });
    } finally {
      await restored.close(context);
    }
  } finally {
    await session.close();
    await gateway.close();
  }
});

it.each(["length", "toolUse", "whitespace"] as const)(
  "preserves original history when a compaction response ends with %s",
  async (stopReason) => {
    const { answer } = await import("./fixtures/gateway.js");
    const gateway = await new ControlledGateway().start();
    gateway.respond = (intent) =>
      intent.kind === "compaction"
        ? {
            ...answer("不完整摘要"),
            stopReason: stopReason === "whitespace" ? "stop" : stopReason,
            content:
              stopReason === "toolUse"
                ? [
                    { type: "text", text: "不完整摘要" },
                    {
                      type: "toolCall",
                      id: "unexpected",
                      name: "unknown",
                      arguments: {},
                    },
                  ]
                : [
                    {
                      type: "text",
                      text: stopReason === "whitespace" ? "   " : "不完整摘要",
                    },
                  ],
          }
        : answer();
    const scope = identity();
    sessions.push(scope.runtime_session_id);
    const session = await openOwnedSession({
      pool,
      identity: scope,
      model: { ...model, context_window: 65536 },
      gateway: gateway.client,
    });
    try {
      for (let n = 0; n < 3; n++) {
        const receipt = await session.acceptCommand({
          request_id: randomUUID(),
          message: `原始第${n}幕：${"人物对白。".repeat(150)}`,
        });
        await session.waitSubmission(receipt.submission_id);
      }
      await expect(session.compact("保留全部已采用事实")).rejects.toThrow(
        "runtime_compaction_failed",
      );
      const snapshot = await session.snapshot();
      expect(
        snapshot.entries.some((entry) => entry.kind === "pi.compaction"),
      ).toBe(false);
      expect(JSON.stringify(snapshot.entries)).toContain("原始第0幕");
    } finally {
      await session.close();
      await gateway.close();
    }
  },
);

it("recovers the original compaction task and exchange memo after an interrupted summary acknowledgement", async () => {
  const gateway = await new ControlledGateway().start();
  const scope = identity();
  sessions.push(scope.runtime_session_id);
  const pin = { ...model, context_window: 65536 };
  let session = await openOwnedSession({
    pool,
    identity: scope,
    model: pin,
    gateway: gateway.client,
  });
  try {
    for (let n = 0; n < 3; n++) {
      const receipt = await session.acceptCommand({
        request_id: randomUUID(),
        message: `分片${n}：${"创作方法与来源。".repeat(150)}`,
      });
      await session.waitSubmission(receipt.submission_id);
    }
    gateway.hold = true;
    const pending = session.compact("保留来源").catch((error) => error);
    await expect
      .poll(
        () => gateway.calls.filter((call) => call.kind === "compaction").length,
      )
      .toBe(1);
    const original = structuredClone(
      gateway.calls.find((call) => call.kind === "compaction"),
    );
    await session.close();
    await pending;
    gateway.hold = false;
    session = await openOwnedSession({
      pool,
      identity: { ...scope, owner_epoch: 2 },
      model: pin,
      gateway: gateway.client,
    });
    await expect
      .poll(async () =>
        (await session.snapshot()).entries.some(
          (entry) => entry.kind === "pi.compaction",
        ),
      )
      .toBe(true);
    expect(
      gateway.calls.filter((call) => call.kind === "compaction"),
    ).toHaveLength(1);
    const requests = gateway.requests.filter(
      (call) => call.kind === "compaction",
    );
    expect(requests).toHaveLength(2);
    expect(requests[1]).toEqual(original);
  } finally {
    await session.close();
    await gateway.close();
  }
});
