import { createHash, createHmac, randomUUID } from "node:crypto";
import { once } from "node:events";
import { afterAll, expect, it } from "vitest";
import { createRuntimeServer } from "../src/host.js";
import { hashJSON } from "../src/canonical.js";
import { PROTOCOL } from "../src/protocol.js";
import {
  ControlledGateway,
  identity,
  model,
  pool,
} from "./fixtures/gateway.js";
const key = "host-contract-key".repeat(3);
const sessions: string[] = [];
afterAll(async () => {
  await pool.query(
    "DELETE FROM agent.runtime_session WHERE id=ANY($1::uuid[])",
    [sessions],
  );
  await pool.end();
});
function headers(
  path: string,
  body: string,
  expires = String(Math.floor(Date.now() / 1000) + 60),
) {
  const hash = createHash("sha256").update(body).digest("hex");
  return {
    "content-type": "application/json",
    "x-glowloom-runtime-expires": expires,
    "x-glowloom-runtime-signature": createHmac("sha256", key)
      .update(`POST\n${path}\n${expires}\n${hash}`)
      .digest("hex"),
  };
}
it("requires a fresh path/body-bound signature and durably replays concurrent command delivery", async () => {
  const gateway = await new ControlledGateway().start();
  const host = createRuntimeServer({
    pool,
    gateway: gateway.client,
    signingKey: key,
    maxSessions: 2,
  });
  host.server.listen(0, "127.0.0.1");
  await once(host.server, "listening");
  const address = host.server.address();
  if (!address || typeof address === "string")
    throw new Error("missing address");
  const origin = `http://127.0.0.1:${address.port}`;
  const scope = identity();
  sessions.push(scope.runtime_session_id);
  const content = {
    identity: scope,
    model,
    command: { request_id: randomUUID(), message: "规划一集雨夜短剧" },
  };
  const body = JSON.stringify({
    protocol: PROTOCOL,
    ...content,
    payload_hash: hashJSON(content),
  });
  const path = "/internal/v1/commands";
  try {
    expect((await fetch(origin + path, { method: "POST", body })).status).toBe(
      401,
    );
    expect(
      (
        await fetch(origin + path, {
          method: "POST",
          body,
          headers: headers(path, body, "1"),
        })
      ).status,
    ).toBe(401);
    expect(
      (
        await fetch(origin + path, {
          method: "POST",
          body: body.replace("雨夜", "晴天"),
          headers: headers(path, body),
        })
      ).status,
    ).toBe(401);
    const results = await Promise.all(
      [1, 2, 3].map(() =>
        fetch(origin + path, {
          method: "POST",
          body,
          headers: headers(path, body),
        }),
      ),
    );
    expect(results.map((r) => r.status)).toEqual([202, 202, 202]);
    const receipts = await Promise.all(results.map((r) => r.json()));
    expect(receipts[1]).toEqual(receipts[0]);
    expect(receipts[2]).toEqual(receipts[0]);
    await expect.poll(() => gateway.calls.length).toBe(1);
    const forged = {
      ...content,
      identity: { ...scope, actor_id: randomUUID() },
    };
    const forgedBody = JSON.stringify({
      protocol: PROTOCOL,
      ...forged,
      payload_hash: hashJSON(forged),
    });
    expect(
      (
        await fetch(origin + path, {
          method: "POST",
          body: forgedBody,
          headers: headers(path, forgedBody),
        })
      ).status,
    ).toBe(409);
    gateway.authorized = false;
    expect(
      (
        await fetch(origin + path, {
          method: "POST",
          body,
          headers: headers(path, body),
        })
      ).status,
    ).toBe(403);
  } finally {
    await host.close();
    await gateway.close();
  }
});

it("rejects a stale owner drain after the same session is opened by a newer owner", async () => {
  const gateway = await new ControlledGateway().start();
  const host = createRuntimeServer({
    pool,
    gateway: gateway.client,
    signingKey: key,
  });
  host.server.listen(0, "127.0.0.1");
  await once(host.server, "listening");
  const address = host.server.address();
  if (!address || typeof address === "string")
    throw new Error("missing address");
  const origin = `http://127.0.0.1:${address.port}`;
  const scope = identity();
  sessions.push(scope.runtime_session_id);
  const content = {
    identity: scope,
    model,
    command: { request_id: randomUUID(), message: "保持同一运行" },
  };
  const submit = async (identityScope: typeof scope) => {
    const data = { ...content, identity: identityScope };
    const body = JSON.stringify({
      protocol: PROTOCOL,
      ...data,
      payload_hash: hashJSON(data),
    });
    return fetch(origin + "/internal/v1/commands", {
      method: "POST",
      body,
      headers: headers("/internal/v1/commands", body),
    });
  };
  const path = `/internal/v1/owners/${scope.runtime_session_id}/drain`;
  const drainBody = JSON.stringify({
    protocol: PROTOCOL,
    identity: scope,
    payload_hash: hashJSON({ identity: scope }),
  });
  try {
    expect((await submit(scope)).status).toBe(202);
    expect(
      (
        await fetch(origin + path, {
          method: "POST",
          body: drainBody,
          headers: headers(path, drainBody),
        })
      ).status,
    ).toBe(200);
    expect((await submit({ ...scope, owner_epoch: 2 })).status).toBe(202);
    expect(
      (
        await fetch(origin + path, {
          method: "POST",
          body: drainBody,
          headers: headers(path, drainBody),
        })
      ).status,
    ).toBe(409);
    gateway.deniedActions.add("owner.drain");
    const nextScope = { ...scope, owner_epoch: 2 };
    const nextBody = JSON.stringify({
      protocol: PROTOCOL,
      identity: nextScope,
      payload_hash: hashJSON({ identity: nextScope }),
    });
    expect(
      (
        await fetch(origin + path, {
          method: "POST",
          body: nextBody,
          headers: headers(path, nextBody),
        })
      ).status,
    ).toBe(403);
  } finally {
    await host.close();
    await gateway.close();
  }
});

it("drains older control revisions and hands the durable owner to the approved next phase", async () => {
  const gateway = await new ControlledGateway().start();
  const host = createRuntimeServer({ pool, gateway: gateway.client, signingKey: key });
  host.server.listen(0, "127.0.0.1");
  await once(host.server, "listening");
  const address = host.server.address();
  if (!address || typeof address === "string") throw new Error("missing address");
  const origin = `http://127.0.0.1:${address.port}`;
  const planning = identity();
  sessions.push(planning.runtime_session_id);
  const control = { ...planning, control_revision: 2 };
  const execution = {
    ...planning,
    control_revision: 3,
    approval_context: {
      purpose: "execution" as const,
      candidate_id: randomUUID(),
      manifest_hash: "b".repeat(64),
      step_id: "parse",
    },
  };
  const send = (scope: typeof planning) => {
    const content = {
      identity: scope,
      model,
      command: { request_id: randomUUID(), message: "继续当前创作阶段" },
    };
    const path = "/internal/v1/commands";
    const body = JSON.stringify({ protocol: PROTOCOL, ...content, payload_hash: hashJSON(content) });
    return fetch(origin + path, { method: "POST", body, headers: headers(path, body) });
  };
  const drain = (scope: typeof planning | typeof execution) => {
    const path = `/internal/v1/owners/${scope.runtime_session_id}/drain`;
    const body = JSON.stringify({ protocol: PROTOCOL, identity: scope, payload_hash: hashJSON({ identity: scope }) });
    return fetch(origin + path, { method: "POST", body, headers: headers(path, body) });
  };
  try {
    expect((await send(planning)).status).toBe(202);
    expect((await drain(control)).status).toBe(200);
    expect((await send(control)).status).toBe(202);
    expect((await drain(planning)).status).toBe(409);
    expect((await drain(execution)).status).toBe(200);
    expect((await send(execution)).status).toBe(202);
    expect((await drain(control)).status).toBe(409);
  } finally {
    await host.close();
    await gateway.close();
  }
});

it("seals sessions against a request whose body finishes during shutdown", async () => {
  const { request } = await import("node:http");
  const gateway = await new ControlledGateway().start();
  gateway.heldActions.add("command.submit");
  const host = createRuntimeServer({
    pool,
    gateway: gateway.client,
    signingKey: key,
  });
  host.server.listen(0, "127.0.0.1");
  await once(host.server, "listening");
  const address = host.server.address();
  if (!address || typeof address === "string")
    throw new Error("missing address");
  const origin = `http://127.0.0.1:${address.port}`;
  const path = "/internal/v1/commands";
  const first = identity(),
    second = identity();
  sessions.push(first.runtime_session_id, second.runtime_session_id);
  const bodyFor = (scope: typeof first) => {
    const content = {
      identity: scope,
      model,
      command: { request_id: randomUUID(), message: "关闭期间的提交" },
    };
    return JSON.stringify({
      protocol: PROTOCOL,
      ...content,
      payload_hash: hashJSON(content),
    });
  };
  const firstBody = bodyFor(first);
  const firstResponse = fetch(origin + path, {
    method: "POST",
    body: firstBody,
    headers: headers(path, firstBody),
  }).catch(() => null);
  await expect.poll(() => gateway.heldResponses.length).toBe(1);
  gateway.heldActions.delete("command.submit");
  const secondBody = bodyFor(second);
  const entered = once(host.server, "request");
  const slow = request(origin + path, {
    method: "POST",
    headers: {
      ...headers(path, secondBody),
      "content-length": Buffer.byteLength(secondBody),
    },
  });
  const result = new Promise<number>((resolve, reject) => {
    slow.on("response", (response) => {
      response.resume();
      resolve(response.statusCode ?? 0);
    });
    slow.on("error", reject);
  });
  slow.write(secondBody.slice(0, 10));
  await entered;
  const closed = host.close();
  slow.end(secondBody.slice(10));
  try {
    expect(await result).toBe(503);
  } finally {
    for (const response of gateway.heldResponses)
      response.end('{"authorized":true}');
    await firstResponse;
    await closed;
    await pool.query(
      "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE application_name=$1",
      [`agent-runtime:${second.runtime_session_id}`],
    );
    await gateway.close();
  }
});

it("removes a failed cached Host session and reopens the same durable submission on retry", async () => {
  const gateway = await new ControlledGateway().start();
  const host = createRuntimeServer({
    pool,
    gateway: gateway.client,
    signingKey: key,
  });
  host.server.listen(0, "127.0.0.1");
  await once(host.server, "listening");
  const address = host.server.address();
  if (!address || typeof address === "string")
    throw new Error("missing address");
  const origin = `http://127.0.0.1:${address.port}`;
  const path = "/internal/v1/commands";
  const scope = identity();
  sessions.push(scope.runtime_session_id);
  const command = { request_id: randomUUID(), message: "恢复同一提交" };
  const submit = async (epoch: number) => {
    const content = {
      identity: { ...scope, owner_epoch: epoch },
      model,
      command,
    };
    const body = JSON.stringify({
      protocol: PROTOCOL,
      ...content,
      payload_hash: hashJSON(content),
    });
    return fetch(origin + path, {
      method: "POST",
      body,
      headers: headers(path, body),
    });
  };
  try {
    const original = await (await submit(1)).json();
    await expect.poll(() => gateway.calls.length).toBe(1);
    await pool.query(
      "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE application_name=$1",
      [`agent-runtime:${scope.runtime_session_id}`],
    );
    let recovered: unknown;
    await expect
      .poll(async () => {
        const response = await submit(2);
        if (response.status === 202) recovered = await response.json();
        return response.status;
      })
      .toBe(202);
    expect(recovered).toEqual(original);
    expect(gateway.calls).toHaveLength(1);
  } finally {
    await host.close();
    await gateway.close();
  }
});
