import { BACKGROUND_CONTEXT as context } from "@earendil-works/chord/context";
import { createModels, Type } from "@earendil-works/pi-ai";
import {
  Harness,
  configure,
  createRegistry,
  defineExtension,
  defineTool,
} from "@earendil-works/pi-durable";
import { afterAll, expect, it } from "vitest";
import { createConversationGateway } from "../src/model-gateway.js";
import { openPostgresStorage } from "../src/postgres-storage.js";
import {
  ControlledGateway,
  answer,
  identity,
  model,
  pool,
  userText,
} from "./fixtures/gateway.js";
const sessions: string[] = [];
afterAll(async () => {
  await pool.query(
    "DELETE FROM agent.runtime_session WHERE id=ANY($1::uuid[])",
    [sessions],
  );
  await pool.end();
});

// SDK integration characterization only: these fixture tools are never installed in the product Host.
async function openFixture(holdChildren = false) {
  const gateway = await new ControlledGateway().start();
  const scope = identity();
  sessions.push(scope.runtime_session_id);
  const storage = await openPostgresStorage(pool, scope.runtime_session_id);
  const models = createModels({
    authContext: { env: async () => undefined, fileExists: async () => false },
  });
  const registry = createRegistry();
  const read = defineTool({
    name: "fixture_read",
    description: "Read an isolated fixture",
    parameters: Type.Object({}),
    replay: "safe",
    execute: async () => ({
      output: [{ type: "text", text: "source fixture" }],
    }),
  });
  const delegate = defineTool({
    name: "fixture_delegate",
    description: "SDK ownership fixture",
    parameters: Type.Object({}),
    replay: "safe",
    execute: async (_args, api, ctx) => {
      const ids = await api.commit(async (tx) => {
        const existing = await tx.scanConversations(
          { ownerTaskId: api.taskId },
          2,
        );
        if (existing.items.length)
          return existing.items.map((record) => record.id);
        const first = await tx.createConversation({
          ownership: { kind: "task", taskId: api.taskId },
        });
        const second = await tx.createConversation({
          ownership: { kind: "task", taskId: api.taskId },
        });
        return [first.id, second.id];
      }, ctx);
      const results = await Promise.all(
        ids.map(async (id, index) => {
          const child = await api.conversation(id, ctx);
          if (!child) throw new Error("missing child");
          return (
            await (
              await child.submit(
                {
                  type: "input",
                  content: index === 0 ? "child-A" : "child-B",
                  requestId: `${api.taskId}:${index}`,
                },
                ctx,
              )
            ).wait(ctx)
          ).status;
        }),
      );
      return { output: [{ type: "text", text: JSON.stringify(results) }] };
    },
  });
  const tools = defineExtension({
    name: "glowloom.fixture",
    tools: [read, delegate],
  });
  registry.install(tools);
  const harness = await Harness.open(
    storage,
    {
      models,
      registry,
      settings: {
        retry: { enabled: false, maxRetries: 0 },
        stream: { maxRetries: 0 },
        compaction: { enabled: false },
      },
      conversationCreated: async (tx, record) => {
        const bridge = createConversationGateway(
          scope,
          model,
          record.id,
          gateway.client,
          storage.signal,
          async () => {},
        );
        models.setProvider(bridge.provider);
        registry.install(bridge.extension);
        await configure(tx, record.id, {
          model: { provider: bridge.provider.id, modelId: model.release_id },
          extensions: [bridge.extension, tools],
          tools: record.owner ? [read] : [delegate],
        });
      },
    },
    context,
  );
  gateway.respond = (intent) => {
    const child = intent.conversation_id !== 1;
    if (child && holdChildren) gateway.hold = true;
    const hasResult = intent.messages.some(
      (message) => message.role === "toolResult",
    );
    return hasResult
      ? answer(child ? `result:${userText(intent.messages)}` : "children done")
      : {
          ...answer(),
          stopReason: "toolUse",
          content: [
            {
              type: "toolCall",
              id: `call-${intent.conversation_id}`,
              name: child ? "fixture_read" : "fixture_delegate",
              arguments: {},
            },
          ],
        };
  };
  const root = await harness.root(context);
  return {
    harness,
    root,
    gateway,
    close: async () => {
      await harness.close(context);
      await gateway.close();
    },
  };
}

it("isolates two owned SDK conversations, their tool contexts and durable exchange identities", async () => {
  const fixture = await openFixture();
  try {
    expect(
      (
        await (
          await fixture.root.submit(
            { type: "input", content: "coordinator" },
            context,
          )
        ).wait(context)
      ).status,
    ).toBe("done");
    const children = await fixture.harness.commit(
      (tx) =>
        tx.scanConversations({ ownerConversationId: fixture.root.id }, 10),
      context,
    );
    expect(children.items).toHaveLength(2);
    const childIds = children.items.map((child) => child.id);
    expect(childIds[0]).not.toBe(childIds[1]);
    for (const child of children.items)
      expect(child.owner?.conversationId).toBe(fixture.root.id);
    const calls = fixture.gateway.calls.filter((call) =>
      childIds.includes(call.conversation_id as (typeof childIds)[number]),
    );
    expect(calls).toHaveLength(4);
    const labels = childIds.map(
      (id) =>
        new Set(
          calls
            .filter((call) => call.conversation_id === id)
            .map((call) => userText(call.messages)),
        ),
    );
    expect([...labels[0]!]).toEqual(["child-A"]);
    expect([...labels[1]!]).toEqual(["child-B"]);
    expect(
      new Set(fixture.gateway.calls.map((call) => call.exchange_id)).size,
    ).toBe(6);
    expect(
      new Set(fixture.gateway.calls.map((call) => call.task_id)).size,
    ).toBe(6);
  } finally {
    await fixture.close();
  }
});

it("aborts both owned child conversations with the parent instead of leaving background work", async () => {
  const fixture = await openFixture(true);
  try {
    const submission = await fixture.root.submit(
      { type: "input", content: "coordinator" },
      context,
    );
    await expect
      .poll(
        () =>
          fixture.gateway.calls.filter((call) => call.conversation_id !== 1)
            .length,
      )
      .toBe(2);
    await fixture.root.abort(context, { background: true });
    expect((await submission.wait(context)).status).toBe("unanswered");
    expect((await fixture.harness.inspect(context)).tasks).toHaveLength(0);
    expect(
      fixture.gateway.calls.filter((call) => call.conversation_id === 1),
    ).toHaveLength(1);
  } finally {
    await fixture.close();
  }
});
