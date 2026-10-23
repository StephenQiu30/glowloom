import { afterEach, describe, expect, it, vi } from "vitest";
import { CanvasLiveSession } from "@/components/canvas/live-session";
class Socket {
  readyState = 1;
  sent: Record<string, unknown>[] = [];
  onopen?: () => void;
  onmessage?: (event: { data: string }) => void;
  onclose?: () => void;
  send(value: string) {
    this.sent.push(JSON.parse(value));
  }
  close() {
    this.readyState = 3;
    this.onclose?.();
  }
  receive(value: unknown) {
    this.onmessage?.({ data: JSON.stringify(value) });
  }
}
afterEach(() => vi.useRealTimers());
function setup(
  renewSession?: (signal: AbortSignal) => Promise<void>,
  csrf = () => "csrf",
  revoked = vi.fn(),
) {
  vi.useFakeTimers();
  const sockets: Socket[] = [],
    snapshot = vi.fn(),
    status = vi.fn();
  const session = new CanvasLiveSession({
    canvasID: "canvas",
    actorID: "actor",
    scopeID: "space",
    csrf,
    renewSession,
    snapshot,
    status,
    revoked,
    socket: () => {
      const socket = new Socket();
      sockets.push(socket);
      return socket as unknown as WebSocket;
    },
  });
  const join = (socket: Socket, rev = "1") => {
    socket.onopen?.();
    socket.receive({
      type: "snapshot",
      document: {
        id: "canvas",
        revision: rev,
        nodes: [],
        edges: [],
        permissions: { read: true, write: true },
      },
    });
    socket.receive({
      type: "ready",
      actor_id: "actor",
      revision: rev,
      permissions: { read: true, write: true },
    });
  };
  session.start();
  join(sockets[0]);
  return { sockets, snapshot, status, session, join };
}
describe("durable canvas session", () => {
  it("keeps the reviewed revision fence in the original body when resolving a lost receipt", () => {
    const { session, sockets } = setup();
    void session
      .submit([{ type: "rename", name: "reviewed" }], undefined, "1", true)
      .catch(() => {});
    const submit = sockets[0].sent.find((packet) => packet.type === "submit")!;
    expect(submit.body).toMatchObject({
      strict_base: true,
      base_revision: "1",
    });
    sockets[0].receive({
      type: "heartbeat",
      permissions: { read: true, write: true },
    });
    const resolve = sockets[0].sent.find(
      (packet) => packet.type === "resolve",
    )!;
    expect(resolve.op_id).toBe(submit.op_id);
    expect(resolve.body).toEqual(submit.body);
    session.stop();
  });
  it("restarts exhausted reconnects on the browser online event and removes its listener on stop", () => {
    const { session, sockets, join } = setup();
    sockets[0].close();
    vi.advanceTimersByTime(2000);
    sockets[1].close();
    vi.advanceTimersByTime(2000);
    sockets[2].close();
    window.dispatchEvent(new Event("online"));
    expect(sockets).toHaveLength(4);
    join(sockets[3]);
    expect(session.canSubmit).toBe(true);
    session.stop();
    window.dispatchEvent(new Event("online"));
    expect(sockets).toHaveLength(4);
  });
  it("updates revoked write access while an original submission is still unresolved", () => {
    const { session, sockets, status } = setup();
    void session.submit([{ type: "set", node_id: "n" }]).catch(() => {});
    status.mockClear();
    sockets[0].receive({
      type: "heartbeat",
      permissions: { read: true, write: false },
    });
    expect(session.canEdit).toBe(false);
    expect(status).toHaveBeenCalled();
    expect(session.unresolved).toBe(true);
    session.stop();
  });
  it("releases private content after loss of read access and never retries its pending write", async () => {
    const revoked = vi.fn();
    const { session, sockets } = setup(undefined, () => "csrf", revoked);
    const result = session.submit([{ type: "set", node_id: "n" }]);
    const rejected = expect(result).rejects.toThrow("访问权限");
    sockets[0].receive({ type: "error", status: 403, code: "forbidden" });
    await rejected;
    expect(revoked).toHaveBeenCalledOnce();
    expect(session.unresolved).toBe(false);
    window.dispatchEvent(new Event("online"));
    expect(sockets).toHaveLength(1);
    session.stop();
  });
  it("uses server heartbeats to resolve a lost ACK without a new submit", async () => {
    const { session, sockets } = setup();
    const promise = session.submit([{ type: "set", node_id: "n" }]);
    const original = sockets[0].sent.at(-1)!;
    sockets[0].receive({
      type: "heartbeat",
      permissions: { read: true, write: true },
    });
    expect(sockets[0].sent.at(-1)).toEqual({ ...original, type: "resolve" });
    expect(session.unresolved).toBe(true);
    sockets[0].receive({
      type: "changes",
      from_revision: "1",
      to_revision: "2",
      changes: [{ revision: "2", effects: [] }],
    });
    sockets[0].receive({
      type: "ack",
      receipt: { op_id: original.op_id, status: "accepted", revision: "2" },
    });
    await expect(promise).resolves.toBe(original.op_id);
    session.stop();
  });

  it("keeps edit permission across a temporary disconnection but cannot send", () => {
    const { session, sockets } = setup();
    expect(session.canEdit).toBe(true);
    sockets[0].close();
    expect(session.canEdit).toBe(true);
    expect(session.canSubmit).toBe(false);
    session.stop();
  });

  it("renews expired authorization then resolves the original pending save", async () => {
    let proof = "original-proof";
    const renew = vi.fn(async () => {
      proof = "renewed-proof";
    });
    const { session, sockets, join, status, snapshot } = setup(
      renew,
      () => proof,
    );
    const saved = session.submit([{ type: "set", node_id: "n" }]);
    const original = sockets[0].sent.at(-1)!;
    const reads = snapshot.mock.calls.length;
    sockets[0].receive({
      type: "error",
      status: 401,
      code: "access_expired",
      retryable: false,
    });
    await Promise.resolve();
    expect(renew).toHaveBeenCalledOnce();
    expect(sockets).toHaveLength(2);
    join(sockets[1], "2");
    expect(sockets[1].sent[0]).toMatchObject({
      type: "join",
      csrf_token: "renewed-proof",
      scope_id: "space",
      client_id: session.clientID,
    });
    expect(snapshot).toHaveBeenCalledTimes(reads);
    expect(status.mock.calls.at(-1)?.[0]).not.toBe("ready");
    expect(sockets[1].sent.at(-1)).toEqual({ ...original, type: "resolve" });
    sockets[1].receive({
      type: "ack",
      receipt: { op_id: original.op_id, status: "accepted", revision: "2" },
    });
    await expect(saved).resolves.toBe(original.op_id);
    session.stop();
  });
  it("bounds unsuccessful renewal handshakes and permits explicit recovery", async () => {
    const renew = vi.fn().mockResolvedValue(undefined);
    const { session, sockets, status } = setup(renew);
    const expired = { type: "error", status: 401, code: "access_expired" };
    sockets[0].receive(expired);
    await Promise.resolve();
    expect(sockets).toHaveLength(2);
    sockets[1].receive(expired);
    await Promise.resolve();
    expect(renew).toHaveBeenCalledOnce();
    expect(status.mock.calls.at(-1)?.[0]).toBe("error");
    session.retry();
    await Promise.resolve();
    expect(renew).toHaveBeenCalledTimes(2);
    expect(sockets).toHaveLength(3);
    session.stop();
  });
  it("does not reconnect after closing while renewal is pending", async () => {
    let finish!: () => void;
    const renew = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve;
        }),
    );
    const { session, sockets } = setup(renew);
    sockets[0].receive({ type: "error", status: 401, code: "access_expired" });
    expect(renew).toHaveBeenCalledOnce();
    session.stop();
    finish();
    await Promise.resolve();
    expect(sockets).toHaveLength(1);
  });
  it("retains failed renewal for manual retry without treating revoked access as expiry", async () => {
    const renew = vi
      .fn()
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValue(undefined);
    const { session, sockets, status, join } = setup(renew);
    sockets[0].receive({ type: "error", status: 401, code: "access_expired" });
    await Promise.resolve();
    expect(renew).toHaveBeenCalledOnce();
    expect(status.mock.calls.at(-1)?.[0]).toBe("error");
    session.retry();
    await Promise.resolve();
    expect(sockets).toHaveLength(2);
    join(sockets[1]);
    sockets[1].receive({ type: "error", status: 403, code: "forbidden" });
    expect(renew).toHaveBeenCalledTimes(2);
    expect(status.mock.calls.at(-1)?.[0]).toBe("error");
    session.stop();
  });

  it("manual recovery preserves the original request after automatic retries stop", async () => {
    const { session, sockets, join, status } = setup();
    const promise = session.submit([{ type: "set", node_id: "n" }]);
    const original = sockets[0].sent.at(-1)!;
    for (let i = 0; i < 3; i++) {
      sockets[i].close();
      vi.advanceTimersByTime(2000);
    }
    expect(status).toHaveBeenLastCalledWith("unconfirmed", expect.any(String));
    session.retry();
    join(sockets[3], "2");
    expect(sockets[3].sent.at(-1)).toEqual({ ...original, type: "resolve" });
    sockets[3].receive({
      type: "ack",
      receipt: { op_id: original.op_id, status: "accepted", revision: "2" },
    });
    await expect(promise).resolves.toBe(original.op_id);
    session.stop();
    session.retry();
    expect(sockets).toHaveLength(4);
  });
  it("a reconnect snapshot cannot announce saved before the new connection is authorized", async () => {
    const { session, sockets, status } = setup();
    const promise = session.submit([{ type: "set" }]);
    const id = sockets[0].sent.at(-1)!.op_id;
    sockets[0].receive({
      type: "ack",
      receipt: { op_id: id, status: "accepted", revision: "2" },
    });
    sockets[0].close();
    vi.advanceTimersByTime(2000);
    sockets[1].receive({
      type: "snapshot",
      document: {
        id: "canvas",
        revision: "2",
        nodes: [],
        edges: [],
        permissions: { read: true, write: true },
      },
    });
    expect(session.unresolved).toBe(true);
    expect(status.mock.calls.at(-1)?.[0]).toBe("unconfirmed");
    sockets[1].receive({
      type: "ready",
      actor_id: "actor",
      revision: "2",
      permissions: { read: true, write: false },
    });
    await expect(promise).resolves.toBe(id);
    expect(status).toHaveBeenLastCalledWith("readonly");
    session.stop();
  });
  it("heartbeat permissions can restore editing and absent permissions do not revoke it", () => {
    const { session, sockets, status } = setup();
    sockets[0].receive({
      type: "heartbeat",
      permissions: { read: true, write: false },
    });
    expect(status).toHaveBeenLastCalledWith("readonly");
    sockets[0].receive({
      type: "heartbeat",
      permissions: { read: true, write: true },
    });
    expect(status).toHaveBeenLastCalledWith("ready");
    sockets[0].receive({ type: "heartbeat" });
    expect(status).toHaveBeenLastCalledWith("ready");
    session.stop();
  });
  it("accepts the service's omitted zero revision and completes receipt-before-snapshot", async () => {
    const { session, sockets, status } = setup();
    sockets[0].receive({
      type: "snapshot",
      document: {
        id: "canvas",
        revision: "0",
        nodes: [],
        edges: [],
        permissions: { read: true, write: true },
      },
    });
    sockets[0].receive({
      type: "ready",
      actor_id: "actor",
      permissions: { read: true, write: true },
    });
    expect(status).toHaveBeenLastCalledWith("ready");
    const promise = session.submit([{ type: "set" }]);
    const id = sockets[0].sent.at(-1)!.op_id;
    sockets[0].receive({
      type: "ack",
      receipt: { op_id: id, status: "accepted", revision: "1" },
    });
    sockets[0].receive({
      type: "snapshot",
      document: {
        id: "canvas",
        revision: "1",
        nodes: [],
        edges: [],
        permissions: { read: true, write: true },
      },
    });
    await expect(promise).resolves.toBe(id);
    session.stop();
  });
  it("stops repeated failed connections instead of leaving an endless spinner", () => {
    const { session, sockets, status } = setup();
    for (let i = 0; i < 3; i++) {
      sockets[i].close();
      vi.advanceTimersByTime(2000);
    }
    expect(status).toHaveBeenLastCalledWith(
      "error",
      expect.stringContaining("连接失败"),
    );
    expect(sockets).toHaveLength(3);
    session.stop();
  });
  it("waits for both receipt and authoritative changes, regardless of order", async () => {
    const { session, sockets } = setup();
    const done = vi.fn();
    const promise = session.submit([{ type: "set" }]).then(done);
    const id = sockets[0].sent.at(-1)!.op_id;
    sockets[0].receive({
      type: "ack",
      receipt: { op_id: id, status: "accepted", revision: "2" },
    });
    await Promise.resolve();
    expect(done).not.toHaveBeenCalled();
    sockets[0].receive({
      type: "changes",
      from_revision: "1",
      to_revision: "2",
      changes: [{ revision: "2", effects: [] }],
    });
    await promise;
    expect(done).toHaveBeenCalledOnce();
    expect(session.unresolved).toBe(false);
    session.stop();
  });
  it("reconnects with resolve and retries exactly the original request only after receipt_not_found", async () => {
    const { session, sockets, join } = setup();
    const promise = session.submit([{ type: "set", node_id: "n" }]);
    const original = sockets[0].sent.at(-1)!;
    sockets[0].close();
    vi.advanceTimersByTime(2000);
    join(sockets[1]);
    expect(sockets[1].sent.at(-1)).toEqual({ ...original, type: "resolve" });
    sockets[1].receive({ type: "receipt_not_found", op_id: original.op_id });
    expect(sockets[1].sent.at(-1)).toEqual(original);
    sockets[1].receive({
      type: "changes",
      from_revision: "1",
      to_revision: "2",
      changes: [{ revision: "2", effects: [] }],
    });
    sockets[1].receive({
      type: "ack",
      receipt: { op_id: original.op_id, status: "accepted", revision: "2" },
    });
    await expect(promise).resolves.toBe(original.op_id);
    session.stop();
  });
  it("does not treat heartbeats as a save receipt", async () => {
    const { session, sockets } = setup();
    const promise = session.submit([{ type: "set" }]);
    for (let i = 0; i < 3; i++) {
      vi.advanceTimersByTime(4000);
      sockets[0].receive({
        type: "heartbeat",
        permissions: { read: true, write: true },
      });
    }
    vi.advanceTimersByTime(3000);
    expect(sockets[0].readyState).toBe(3);
    expect(session.unresolved).toBe(true);
    const rejected = expect(promise).rejects.toThrow();
    session.stop();
    await rejected;
  });
});
