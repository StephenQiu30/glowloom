/**
 * Production transport smoke test. Prerequisite: build this frontend with
 * `pnpm exec next build` (output: standalone), then run from frontend:
 * `node --test tests/production/standalone-live-proxy.mjs`.
 * Uses the actual built proxy middleware in a temporary standalone copy. Only
 * its upstream rewrite and process origin/ports change. No real credentials,
 * Go service, browser, or application data are used; this is not acceptance.
 */
import assert from "node:assert/strict";
import { createHash, randomBytes } from "node:crypto";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { cp, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import http from "node:http";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const frontend = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../..",
);
const route = "/api/canvases/10000000-0000-4000-8000-000000000001/live";
const cookie = "synthetic_session=standalone_transport_test";
const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
async function listen(server) {
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  return server.address().port;
}
function textFrame(text, masked = false) {
  const body = Buffer.from(text);
  assert.ok(body.length < 126);
  if (!masked) return Buffer.concat([Buffer.from([0x81, body.length]), body]);
  const mask = randomBytes(4);
  return Buffer.concat([
    Buffer.from([0x81, body.length | 0x80]),
    mask,
    Buffer.from(body.map((value, index) => value ^ mask[index % 4])),
  ]);
}
function readFrames(socket, head, onText) {
  let buffered = Buffer.alloc(0);
  const receive = (chunk) => {
    buffered = Buffer.concat([buffered, chunk]);
    while (buffered.length >= 2) {
      const length = buffered[1] & 127;
      assert.ok(length < 126, "fixture exchanges only short text frames");
      const masked = !!(buffered[1] & 128),
        offset = masked ? 6 : 2;
      if (buffered.length < offset + length) return;
      const payload = Buffer.from(buffered.subarray(offset, offset + length));
      if (masked)
        for (let i = 0; i < length; i++) payload[i] ^= buffered[2 + (i % 4)];
      const opcode = buffered[0] & 15;
      buffered = buffered.subarray(offset + length);
      if (opcode === 1) onText(payload.toString());
    }
  };
  socket.on("data", receive);
  if (head.length) receive(head);
}
function probe(port, origin, options = {}) {
  return new Promise((resolve) => {
    const started = Date.now();
    let socket,
      finished = false,
      lateTimer;
    const messages = [];
    const finish = (result) => {
      if (finished) return;
      finished = true;
      clearTimeout(timer);
      clearTimeout(lateTimer);
      socket?.destroy();
      request.destroy();
      resolve({ ...result, messages, elapsedMs: Date.now() - started });
    };
    const headers = {
      Host: options.host ?? new URL(origin).host,
      Origin: options.origin ?? origin,
      Connection: "Upgrade",
      Upgrade: "websocket",
      "Sec-WebSocket-Version": "13",
      "Sec-WebSocket-Key": randomBytes(16).toString("base64"),
      "Sec-WebSocket-Extensions": "permessage-deflate; client_max_window_bits",
      "Cache-Control": "no-cache",
      Pragma: "no-cache",
    };
    if (options.cookie) headers.Cookie = options.cookie;
    const request = http.request({
      hostname: "127.0.0.1",
      port,
      path: route,
      headers,
    });
    const timer = setTimeout(
      () => finish({ error: "timeout" }),
      options.timeout ?? 6000,
    );
    request.on("response", (response) => {
      response.resume();
      finish({ status: response.statusCode });
    });
    request.on("error", (error) => finish({ error: error.code }));
    request.on("upgrade", (response, upgraded, head) => {
      socket = upgraded;
      socket.on("error", (error) => finish({ error: error.code }));
      socket.on("close", () => finish({ error: "closed" }));
      readFrames(socket, head, (message) => {
        messages.push(message);
        if (message === "ack:join") {
          if (options.holdMs)
            lateTimer = setTimeout(
              () => socket.write(textFrame("late", true)),
              options.holdMs,
            );
          else finish({ status: response.statusCode });
        }
        if (message === "ack:late") finish({ status: response.statusCode });
      });
      socket.write(textFrame("join", true));
    });
    request.end();
  });
}

test(
  "built standalone forwards authenticated synthetic upgrades and retains middleware guards",
  { timeout: 90000 },
  async (t) => {
    const built = path.join(frontend, ".next/standalone");
    // Fail with the prerequisite, rather than silently skipping the production check.
    await readFile(path.join(built, "server.js")).catch(() => {
      throw new Error(
        "Missing standalone build: run pnpm exec next build first",
      );
    });
    const root = await mkdtemp(
      path.join(tmpdir(), "glowloom-standalone-smoke-"),
    );
    const sockets = new Set(),
      observed = [];
    let child,
      origin,
      serverLog = "";
    const upstream = http.createServer((_request, response) => {
      response.writeHead(401);
      response.end();
    });
    upstream.on("upgrade", (request, socket, head) => {
      sockets.add(socket);
      socket.once("close", () => sockets.delete(socket));
      socket.on("error", () => {});
      observed.push({
        cookie: request.headers.cookie,
        origin: request.headers.origin,
        connection: request.headers.connection,
      });
      // Synthetic auth and Origin contract only. Actual Go policy is not exercised.
      const status =
        request.headers.cookie !== cookie
          ? 401
          : request.headers.origin !== origin
            ? 403
            : 101;
      if (status !== 101) {
        socket.end(
          `HTTP/1.1 ${status} Rejected\r\nConnection: close\r\nContent-Length: 0\r\n\r\n`,
        );
        return;
      }
      const accept = createHash("sha1")
        .update(
          request.headers["sec-websocket-key"] +
            "258EAFA5-E914-47DA-95CA-C5AB0DC85B11",
        )
        .digest("base64");
      socket.write(
        `HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: ${accept}\r\n\r\n`,
      );
      socket.write(textFrame("welcome"));
      readFrames(socket, head, (message) =>
        socket.write(textFrame(`ack:${message}`)),
      );
    });
    try {
      const upstreamPort = await listen(upstream),
        reservation = http.createServer();
      const port = await listen(reservation);
      await new Promise((resolve) => reservation.close(resolve));
      origin = `http://127.0.0.1:${port}`;
      await cp(built, root, { recursive: true, verbatimSymlinks: true });
      const manifestPath = path.join(root, ".next/routes-manifest.json"),
        manifest = JSON.parse(await readFile(manifestPath, "utf8"));
      const rewrite = manifest.rewrites.beforeFiles.find(
        (entry) => entry.source === "/api/canvases/:id/live",
      );
      assert.ok(rewrite, "built live rewrite must exist");
      const previousTarget = rewrite.destination;
      rewrite.destination = `http://127.0.0.1:${upstreamPort}/api/canvases/:id/live`;
      await writeFile(manifestPath, JSON.stringify(manifest));
      const entry = path.join(root, "server.js"),
        source = await readFile(entry, "utf8");
      assert.ok(
        source.includes(previousTarget),
        "standalone entry and route manifest must agree",
      );
      await writeFile(
        entry,
        source.replaceAll(previousTarget, rewrite.destination),
      );
      // Deliberately do not copy the caller's environment (tokens, cookies, proxy settings).
      child = spawn(process.execPath, [entry], {
        cwd: root,
        env: {
          NODE_ENV: "production",
          PORT: String(port),
          HOSTNAME: "127.0.0.1",
          GLOWLOOM_PUBLIC_ORIGIN: origin,
          GLOWLOOM_BACKEND_ORIGIN: `http://127.0.0.1:${upstreamPort}`,
        },
        stdio: ["ignore", "pipe", "pipe"],
      });
      child.stdout.on("data", (data) => {
        serverLog += data;
      });
      child.stderr.on("data", (data) => {
        serverLog += data;
      });
      for (let i = 0; i < 150 && !serverLog.includes("Ready"); i++) {
        assert.equal(child.exitCode, null, serverLog);
        await delay(100);
      }
      assert.ok(serverLog.includes("Ready"), serverLog);
      for (const target of [upstreamPort, port]) {
        assert.equal((await probe(target, origin)).status, 401);
        const accepted = await probe(target, origin, { cookie });
        assert.equal(accepted.status, 101, JSON.stringify(accepted));
        assert.deepEqual(accepted.messages, ["welcome", "ack:join"]);
      }
      assert.equal(
        (await probe(port, origin, { cookie: "synthetic_session=invalid" }))
          .status,
        401,
      );
      assert.equal(
        (
          await probe(port, origin, {
            cookie,
            origin: "https://invalid.example",
          })
        ).status,
        403,
      );
      assert.equal(
        observed.at(-1).origin,
        "https://invalid.example",
        "Next preserves Origin for upstream policy",
      );
      // Native fetch can normalize Host; use Node HTTP to test the wire Host.
      const rejectedHTTP = await new Promise((resolve, reject) => {
        const request = http.get(
          {
            hostname: "127.0.0.1",
            port,
            path: route,
            headers: { Host: "invalid.example" },
          },
          (response) => {
            let body = "";
            response.on("data", (chunk) => {
              body += chunk;
            });
            response.on("end", () =>
              resolve({ status: response.statusCode, body }),
            );
            response.on("error", reject);
          },
        );
        request.on("error", reject);
        request.setTimeout(5000, () =>
          request.destroy(new Error("HTTP guard timeout")),
        );
      });
      assert.equal(rejectedHTTP.status, 400);
      assert.equal(JSON.parse(rejectedHTTP.body).code, "invalid_host");
      const before = observed.length;
      const rejectedWS = await probe(port, origin, {
        cookie,
        host: "invalid.example",
        timeout: 1500,
      });
      assert.notEqual(rejectedWS.status, 101);
      assert.equal(
        observed.length,
        before,
        "actual built middleware rejects Host before upstream",
      );
      // Next 16.3.6 currently leaves this rejected Upgrade pending; do not claim a 400 WS response.
      t.diagnostic(
        `invalid Host upgrade outcome: ${JSON.stringify(rejectedWS)}`,
      );
      const held = await probe(port, origin, {
        cookie,
        holdMs: 21000,
        timeout: 28000,
      });
      assert.equal(held.status, 101, JSON.stringify(held));
      assert.ok(held.elapsedMs >= 21000);
      assert.deepEqual(held.messages, ["welcome", "ack:join", "ack:late"]);
      assert.ok(
        observed
          .filter((entry) => entry.cookie === cookie)
          .every((entry) => entry.connection.toLowerCase().includes("upgrade")),
      );
      t.diagnostic(
        `synthetic 101, bidirectional late ACK after ${held.elapsedMs} ms; no real authentication or browser acceptance`,
      );
    } finally {
      for (const socket of sockets) socket.destroy();
      upstream.closeAllConnections();
      if (upstream.listening)
        await new Promise((resolve) => upstream.close(resolve));
      if (child && child.exitCode === null) {
        const exited = once(child, "exit");
        child.kill("SIGTERM");
        const force = setTimeout(() => child.kill("SIGKILL"), 3000);
        await exited;
        clearTimeout(force);
      }
      await rm(root, { recursive: true, force: true });
    }
  },
);
