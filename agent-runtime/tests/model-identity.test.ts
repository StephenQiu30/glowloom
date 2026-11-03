import { once } from "node:events";
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

it("recovers the original committed taskId/memo and never submits a second physical exchange", async () => {
  const gateway = await new ControlledGateway().start();
  gateway.hold = true;
  const scope = identity();
  sessions.push(scope.runtime_session_id);
  let session = await openOwnedSession({
    pool,
    identity: scope,
    model,
    gateway: gateway.client,
  });
  try {
    const command = { request_id: randomUUID(), message: "雨夜的第一个镜头" };
    const receipt = await session.acceptCommand(command);
    await expect.poll(() => gateway.calls.length).toBe(1);
    const original = structuredClone(gateway.calls[0]);
    await session.close();
    gateway.hold = false;
    session = await openOwnedSession({
      pool,
      identity: { ...scope, owner_epoch: 2 },
      model,
      gateway: gateway.client,
    });
    expect(await session.acceptCommand(command)).toEqual(receipt);
    expect((await session.waitSubmission(receipt.submission_id)).status).toBe(
      "done",
    );
    expect(gateway.calls).toHaveLength(1);
    expect(gateway.requests).toHaveLength(2);
    expect(gateway.requests[1]).toEqual(original);
    await session
      .acceptCommand({ request_id: randomUUID(), message: "雨夜的第一个镜头" })
      .then((r) => session.waitSubmission(r.submission_id));
    expect(gateway.calls).toHaveLength(2);
    expect(gateway.calls[1]!.task_id).not.toBe(original!.task_id);
    expect(gateway.calls[1]!.exchange_id).not.toBe(original!.exchange_id);
  } finally {
    await session.close();
    await gateway.close();
  }
});

it("resumes after a real SIGKILL while the durable gateway already has a result", async () => {
  const { fork } = await import("node:child_process");
  const { dsn } = await import("./fixtures/gateway.js");
  const gateway = await new ControlledGateway().start();
  gateway.hold = true;
  const scope = identity();
  sessions.push(scope.runtime_session_id);
  const address = gateway.server.address();
  if (!address || typeof address === "string")
    throw new Error("missing address");
  const child = fork(
    new URL("./fixtures/runtime-worker.mjs", import.meta.url),
    {
      env: { GLOWLOOM_TEST_AGENT_DB_DSN: dsn! },
      stdio: ["ignore", "ignore", "pipe", "ipc"],
    },
  );
  const command = { request_id: randomUUID(), message: "重启后的同一目标" };
  try {
    const message = once(child, "message");
    child.send({
      identity: scope,
      model,
      origin: `http://127.0.0.1:${address.port}`,
      command,
    });
    const [result] = (await message) as [
      { receipt: { submission_id: number }; error?: string },
    ];
    expect(result.error).toBeUndefined();
    await expect.poll(() => gateway.calls.length).toBe(1);
    const exited = once(child, "exit");
    child.kill("SIGKILL");
    await exited;
    gateway.hold = false;
    const session = await openOwnedSession({
      pool,
      identity: { ...scope, owner_epoch: 2 },
      model,
      gateway: gateway.client,
    });
    try {
      const replay = await session.acceptCommand(command);
      expect(replay.submission_id).toBe(result.receipt.submission_id);
      expect((await session.waitSubmission(replay.submission_id)).status).toBe(
        "done",
      );
      expect(gateway.calls).toHaveLength(1);
      expect(gateway.requests[1]).toEqual(gateway.requests[0]);
    } finally {
      await session.close();
    }
  } finally {
    if (child.exitCode === null && child.signalCode === null)
      child.kill("SIGKILL");
    await gateway.close();
  }
});

it("never sends a physical exchange after its PostgreSQL writer connection fails during authorization", async () => {
  const gateway = await new ControlledGateway().start();
  gateway.heldActions.add("model.generation");
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
      message: "数据库断连屏障",
    });
    await expect.poll(() => gateway.heldResponses.length).toBe(1);
    await pool.query(
      "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE application_name=$1",
      [`agent-runtime:${scope.runtime_session_id}`],
    );
    for (const response of gateway.heldResponses)
      response.end('{"authorized":true}');
    await expect(
      session.waitSubmission(receipt.submission_id),
    ).rejects.toThrow();
    // The request handler must settle after the release, including any delayed outbound exchange.
    await new Promise<void>((resolve) => setImmediate(resolve));
    expect(gateway.calls).toHaveLength(0);
  } finally {
    await session.close();
    await gateway.close();
  }
});
