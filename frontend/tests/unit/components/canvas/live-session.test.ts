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
function setup() {
  vi.useFakeTimers();
  const sockets: Socket[] = [],
    snapshot = vi.fn(),
    status = vi.fn();
  const session = new CanvasLiveSession({
    canvasID: "canvas",
    actorID: "actor",
    scopeID: "space",
    csrf: () => "csrf",
    snapshot,
    status,
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
