import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { CanvasLive } from "@/app/canvas/live";
import { setCSRFToken, setCreationScope } from "@/lib/request";

vi.mock("@/api/identity", () => ({
  getSession: vi.fn().mockResolvedValue({}),
}));

vi.mock("@/api/canvases", () => ({ getCanvas: vi.fn().mockResolvedValue({}) }));

class Socket {
  static all: Socket[] = [];
  readyState = 0;
  sent: Record<string, unknown>[] = [];
  onopen: (() => void) | null = null;
  onmessage: ((e: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  constructor(public url: string) {
    Socket.all.push(this);
  }
  send(value: string) {
    this.sent.push(JSON.parse(value));
  }
  close() {
    this.readyState = 3;
    this.onclose?.();
  }
  open() {
    this.readyState = 1;
    this.onopen?.();
  }
  receive(value: unknown) {
    this.onmessage?.({ data: JSON.stringify(value) });
  }
}
const id = "11111111-1111-4111-8111-111111111111";
const body: API.CommandsInput = {
  expected_revision: 1,
  commands: [{ type: "AddNodes", nodes: [] }],
};
let live: CanvasLive;
beforeEach(() => {
  vi.useFakeTimers();
  Socket.all = [];
  vi.stubGlobal("WebSocket", Socket);
  setCSRFToken("csrf");
  setCreationScope(id);
});
afterEach(() => {
  live?.stop();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  setCSRFToken(null);
});
function start() {
  const snapshot = vi.fn(),
    status = vi.fn(),
    revoked = vi.fn();
  live = new CanvasLive(id, { snapshot, status, revoked });
  live.start();
  const socket = Socket.all[0];
  socket.open();
  socket.receive({ type: "ready", revision: 1, writable: true });
  return { socket, snapshot, status, revoked };
}
it("sends real submit; only matching committed ack completes save", async () => {
  const { socket } = start();
  const saved = vi.fn();
  const pending = live.submit(id, body).then(saved);
  expect(socket.url).toContain(`/api/canvases/${id}/live`);
  expect(socket.sent[0]).toEqual({
    type: "join",
    protocol_version: 1,
    scope_id: id,
    csrf_token: "csrf",
  });
  expect(socket.sent[1]).toEqual({ type: "submit", op_id: id, body });
  socket.receive({ type: "heartbeat", revision: 2, writable: true });
  await Promise.resolve();
  expect(saved).not.toHaveBeenCalled();
  const result = { id, revision: 2 };
  socket.receive({ type: "ack", op_id: id, revision: 2, result });
  await pending;
  expect(saved).toHaveBeenCalledWith(result);
});
it("reconnect resolves original frozen envelope before resending; no new key or revision", async () => {
  const { socket } = start();
  const pending = live.submit(id, body);
  body.expected_revision = 99;
  socket.close();
  await vi.advanceTimersByTimeAsync(2000);
  const reconnected = Socket.all.at(-1)!;
  expect(reconnected).not.toBe(socket);
  reconnected.open();
  reconnected.receive({ type: "ready", revision: 3, writable: true });
  expect(reconnected.sent.at(-1)).toEqual({
    type: "resolve",
    op_id: id,
    body: { ...body, expected_revision: 1 },
  });
  reconnected.receive({ type: "not_found", op_id: id });
  expect(reconnected.sent.at(-1)?.type).toBe("submit");
  expect(
    (reconnected.sent.at(-1)?.body as API.CommandsInput).expected_revision,
  ).toBe(1);
  reconnected.receive({
    type: "ack",
    op_id: id,
    result: { id, revision: 2 },
    revision: 2,
  });
  await expect(pending).resolves.toMatchObject({ revision: 2 });
  body.expected_revision = 1;
});
it("definite revision conflicts stop the original request", async () => {
  const { socket } = start();
  const pending = live.submit(id, body);
  const rejected = expect(pending).rejects.toMatchObject({
    code: "revision_conflict",
    status: 409,
  });
  socket.receive({
    type: "error",
    op_id: id,
    status: 409,
    code: "revision_conflict",
  });
  await rejected;
});
it("detects stalled heartbeat and closes permanently on authorization loss", async () => {
  const { socket, revoked } = start();
  await vi.advanceTimersByTimeAsync(21000);
  expect(socket.readyState).toBe(3);
  const current = Socket.all.at(-1)!;
  current.open();
  current.receive({ type: "error", status: 403, code: "forbidden" });
  expect(revoked).toHaveBeenCalledOnce();
  const count = Socket.all.length;
  await vi.advanceTimersByTimeAsync(60000);
  expect(Socket.all).toHaveLength(count);
});

it("background recovery probes the canvas without recording session activity", async () => {
  const { getCanvas } = await import("@/api/canvases");
  const { getSession } = await import("@/api/identity");
  vi.mocked(getSession).mockClear();
  vi.mocked(getCanvas).mockClear();
  const { socket } = start();
  socket.close();
  await vi.advanceTimersByTimeAsync(2000);
  expect(getCanvas).toHaveBeenCalledWith({ id });
  expect(getSession).not.toHaveBeenCalled();
});
it("an obsolete delayed probe cannot replace the resumed live connection", async () => {
  const { getSession } = await import("@/api/identity");
  const { getCanvas } = await import("@/api/canvases");
  let finish!: () => void;
  const probe = new Promise<void>((resolve) => {
    finish = resolve;
  });
  vi.mocked(getSession).mockReturnValueOnce(probe as never);
  vi.mocked(getCanvas).mockReturnValueOnce(probe as never);
  const { socket } = start();
  socket.close();
  await vi.advanceTimersByTimeAsync(2000);
  window.dispatchEvent(new Event("online"));
  const resumed = Socket.all.at(-1)!;
  resumed.open();
  resumed.receive({ type: "ready", revision: 1, writable: true });
  const count = Socket.all.length;
  finish();
  await Promise.resolve();
  await Promise.resolve();
  expect(Socket.all).toHaveLength(count);
  live.stop();
  expect(Socket.all.filter((s) => s.readyState === 1)).toHaveLength(0);
  vi.mocked(getSession)
    .mockReset()
    .mockResolvedValue({} as never);
  vi.mocked(getCanvas)
    .mockReset()
    .mockResolvedValue({} as never);
});

it("rejects an oversized UTF-8 envelope before sending and leaves no unknown pending save", async () => {
  const { socket } = start();
  const large: API.CommandsInput = {
    expected_revision: 1,
    commands: [
      {
        type: "AddNodes",
        nodes: [
          {
            id,
            title: "文本",
            node_type: "text",
            node_action: "resource",
            config: { text: "画".repeat(400000) },
          },
        ],
      },
    ],
  };
  const pending = live.submit(id, large);
  void pending.catch(() => {});
  expect(socket.sent.filter((p) => p.type === "submit")).toHaveLength(0);
  await expect(pending).rejects.toMatchObject({
    status: 413,
    code: "canvas_request_too_large",
  });
  const small = live.submit(id, body);
  socket.receive({
    type: "ack",
    op_id: id,
    revision: 2,
    result: { id, revision: 2 },
  });
  await small;
});

it("protocol 2 resumes changes then resolves the exact coordinator envelope", async () => {
  const envelope = {
    key: id,
    body: {
      operation_version: 1 as const,
      base_revision: 2,
      operations: [{ type: "set", node_id: id, path: ["title"], value: "OT" }],
    },
  };
  const changes = vi.fn(),
    receipt = vi.fn(),
    resync = vi.fn(),
    actor = vi.fn();
  live = new CanvasLive(id, {
    snapshot: vi.fn(),
    status: vi.fn(),
    revoked: vi.fn(),
    collaboration: {
      revision: () => 2,
      pending: () => envelope,
      changes,
      receipt,
      resync,
      actor,
    },
  });
  live.start();
  const socket = Socket.all[0];
  socket.open();
  expect(socket.sent[0]).toMatchObject({
    protocol_version: 2,
    last_revision: 2,
  });
  socket.receive({ type: "changes", revision: 3, changes: [{ revision: 3 }] });
  expect(changes).toHaveBeenCalledWith([{ revision: 3 }]);
  socket.receive({
    type: "ready",
    actor_id: "actor",
    revision: 3,
    writable: true,
  });
  expect(actor).toHaveBeenCalledWith("actor");
  expect(socket.sent.at(-1)).toEqual({
    type: "resolve",
    op_id: id,
    body: envelope.body,
  });
  socket.receive({ type: "not_found", op_id: id });
  expect(socket.sent.at(-1)).toEqual({
    type: "submit",
    op_id: id,
    body: envelope.body,
  });
  socket.receive({
    type: "ack",
    op_id: id,
    receipt: { status: "rejected", op_id: id, code: "field_conflict" },
  });
  expect(receipt).toHaveBeenCalledWith({
    status: "rejected",
    op_id: id,
    code: "field_conflict",
  });
  socket.receive({ type: "resync_required", code: "history_unavailable" });
  expect(resync).toHaveBeenCalled();
});

it("protocol 2 waits for write access before replaying a proven missing request", () => {
  const envelope = {
    key: id,
    body: {
      operation_version: 1 as const,
      base_revision: 2,
      operations: [
        { type: "set", node_id: id, path: ["title"], value: "mine" },
      ],
    },
  };
  live = new CanvasLive(id, {
    snapshot: vi.fn(),
    status: vi.fn(),
    revoked: vi.fn(),
    collaboration: {
      revision: () => 2,
      pending: () => envelope,
      changes: vi.fn(),
      receipt: vi.fn(),
      resync: vi.fn(),
      actor: vi.fn(),
    },
  });
  live.start();
  const socket = Socket.all[0];
  socket.open();
  socket.receive({
    type: "ready",
    actor_id: "me",
    revision: 2,
    writable: false,
  });
  socket.receive({ type: "not_found", op_id: id });
  expect(socket.sent.at(-1)?.type).toBe("resolve");
  socket.receive({ type: "heartbeat", revision: 2, writable: true });
  expect(socket.sent.at(-1)).toEqual({
    type: "resolve",
    op_id: id,
    body: envelope.body,
  });
  socket.receive({ type: "not_found", op_id: id });
  expect(socket.sent.at(-1)?.type).toBe("submit");
});
