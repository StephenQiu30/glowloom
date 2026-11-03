import { afterAll, expect, it } from "vitest";
import { randomUUID } from "node:crypto";
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

it("does not call a model when opening; rejects a competing Host and mismatched command replay", async () => {
  const gateway = await new ControlledGateway().start();
  const scope = identity();
  sessions.push(scope.runtime_session_id);
  const session = await openOwnedSession({
    pool,
    identity: scope,
    model,
    gateway: gateway.client,
  });
  try {
    expect(gateway.calls).toHaveLength(0);
    await expect(
      openOwnedSession({
        pool,
        identity: scope,
        model,
        gateway: gateway.client,
      }),
    ).rejects.toThrow("runtime_session_owned");
    const command = { request_id: randomUUID(), message: "请先了解雨夜故事" };
    const receipt = await session.acceptCommand(command);
    expect((await session.waitSubmission(receipt.submission_id)).status).toBe(
      "done",
    );
    expect(await session.acceptCommand(command)).toEqual(receipt);
    await expect(
      session.acceptCommand({ ...command, message: "不同的目标" }),
    ).rejects.toThrow("runtime_command_conflict");
    expect(gateway.calls).toHaveLength(1);
    gateway.authorized = false;
    await expect(session.acceptCommand(command)).rejects.toThrow(
      "runtime_authorization_denied",
    );
  } finally {
    await session.close();
    await gateway.close();
  }
});

it("keeps original submission and immutable session binding after reopening", async () => {
  const gateway = await new ControlledGateway().start();
  const scope = identity();
  sessions.push(scope.runtime_session_id);
  let session = await openOwnedSession({
    pool,
    identity: scope,
    model,
    gateway: gateway.client,
  });
  try {
    const command = { request_id: randomUUID(), message: "人物：林雨" };
    const receipt = await session.acceptCommand(command);
    await session.waitSubmission(receipt.submission_id);
    await session.close();
    await expect(
      openOwnedSession({
        pool,
        identity: { ...scope, actor_id: randomUUID() },
        model,
        gateway: gateway.client,
      }),
    ).rejects.toThrow("runtime_identity_conflict");
    session = await openOwnedSession({
      pool,
      identity: { ...scope, owner_epoch: 2 },
      model,
      gateway: gateway.client,
    });
    expect(await session.acceptCommand(command)).toEqual(receipt);
    expect(
      (await session.snapshot()).entries.some((e) =>
        JSON.stringify(e).includes("人物：林雨"),
      ),
    ).toBe(true);
    expect(gateway.calls).toHaveLength(1);
  } finally {
    await session.close();
    await gateway.close();
  }
});

it("fails closed when a generation hook is rejected; SDK hook error recovery cannot reach the gateway", async () => {
  const gateway = await new ControlledGateway().start();
  gateway.deniedActions.add("model.prepare");
  const scope = identity();
  sessions.push(scope.runtime_session_id);
  const session = await openOwnedSession({
    pool,
    identity: scope,
    model,
    gateway: gateway.client,
  });
  try {
    const receipt = await session.acceptCommand({
      request_id: randomUUID(),
      message: "未经授权的生成",
    });
    expect((await session.waitSubmission(receipt.submission_id)).status).toBe(
      "unanswered",
    );
    expect(gateway.calls).toHaveLength(0);
  } finally {
    await session.close();
    await gateway.close();
  }
});
