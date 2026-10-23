import { WireAssembler, wireFrames } from "@glowloom/canvas-collab/wire";
import type {
  DurableReceipt,
  FrozenUpdate,
  SessionTransport,
} from "@glowloom/canvas-collab/session";
import {
  ApiError,
  browserConnectionCsrf,
  unwrap,
} from "@/components/canvas/runtime/lib/request";
import { getCanvasSnapshot } from "@/components/canvas/runtime/api/canvasDocumentBffController";
import {
  canvasGraphHash,
  createCanvasDocument,
  readCanvasDocument,
} from "@glowloom/canvas-collab/model";
import { canvasConnectionContract as contract } from "../constants/canvas-connection";

export type CollaborationConnectionStatus =
  | "connecting"
  | "connected"
  | "disconnected"
  | "expired"
  | "oversized"
  | "blocked";
export type CollaborationBinding = {
  actorKey: string;
  canvasId: string;
  epoch: string;
  bindingRevision: string;
  sessionId: string;
  connectionId: string;
  edit: boolean;
  lifecycle: string;
  syncProtocol: "yjs-v1";
  heartbeatIntervalMillis: number;
};
type Pending = {
  expected: string;
  resolve: (value: unknown) => void;
  reject: (error: Error) => void;
  timer: ReturnType<typeof setTimeout>;
};
type Options = {
  actorKey: string;
  canvasId: string;
  sessionId: string;
  ready: (binding: CollaborationBinding) => void | Promise<void>;
  changed: (status: CollaborationConnectionStatus) => void;
  message: (message: Record<string, unknown>) => void | Promise<void>;
  createSocket?: (url: string) => WebSocket;
  random?: () => number;
};
const unavailable = () =>
  new ApiError("连接未确认，输入和原提交身份已保留", 50300);

/** One authenticated connection, committed inbound messages only, no automatic mutation replay. */
export class CanvasCollaborationProvider implements SessionTransport {
  status: CollaborationConnectionStatus = "connecting";
  binding: CollaborationBinding | null = null;
  private socket: WebSocket | null = null;
  private generation = 0;
  private started = false;
  private stopped = false;
  private failures = 0;
  private protocolFailures = 0;
  private healthyHeartbeats = 0;
  private retry: ReturnType<typeof setTimeout> | null = null;
  private heartbeat: ReturnType<typeof setInterval> | null = null;
  private opening: ReturnType<typeof setTimeout> | null = null;
  private rpc = new Map<string, Pending>();
  private logical = new Map<string, Pending>();
  private serial: Promise<unknown> = Promise.resolve();
  private queued = 0;
  private operationBusy = false;
  private cooldownReady = false;

  constructor(private readonly options: Options) {}

  async confirmedSnapshot(receipt: DurableReceipt) {
    const generation = this.generation,
      binding = this.binding;
    if (!receipt.snapshotId || !binding || receipt.epoch !== binding.epoch)
      throw unavailable();
    const snapshot = unwrap(
      await getCanvasSnapshot({
        canvasId: this.options.canvasId,
        snapshotId: receipt.snapshotId,
      }),
    );
    if (
      this.stopped ||
      !this.started ||
      this.generation !== generation ||
      this.binding !== binding ||
      String(snapshot.canvasId) !== this.options.canvasId ||
      String(snapshot.sourceRevision) !== receipt.contentRevision ||
      String(snapshot.snapshotId) !== receipt.snapshotId ||
      String(snapshot.projectBindingRevision) !== binding.bindingRevision ||
      snapshot.graphHash !== receipt.graphHash
    )
      throw unavailable();
    const doc = createCanvasDocument({
      title: snapshot.title,
      graph: snapshot.graph,
    });
    try {
      const value = readCanvasDocument(doc);
      if (
        (await canvasGraphHash(value.graph)) !== receipt.graphHash ||
        this.stopped ||
        !this.started ||
        this.generation !== generation ||
        this.binding !== binding
      )
        throw unavailable();
      return value;
    } finally {
      doc.destroy();
    }
  }

  start() {
    if (this.started) return;
    this.started = true;
    this.stopped = false;
    window.addEventListener("online", this.available);
    window.addEventListener("offline", this.offline);
    window.document.addEventListener("visibilitychange", this.available);
    this.connect();
  }

  private setStatus(value: CollaborationConnectionStatus) {
    this.status = value;
    this.options.changed(value);
  }

  private readonly offline = () => {
    if (!this.started || this.stopped) return;
    this.invalidate();
    this.setStatus("disconnected");
  };
  private readonly available = () => {
    if (
      !this.started ||
      window.document.visibilityState === "hidden" ||
      window.navigator.onLine === false
    )
      return;
    if (this.stopped) {
      if (this.status === "blocked" && this.cooldownReady) this.probe();
      return;
    }
    if (this.status === "connecting") return;
    this.invalidate();
    this.connect();
  };

  probe() {
    if (!this.started || this.status === "expired") return;
    this.stopped = false;
    this.cooldownReady = false;
    this.protocolFailures = 0;
    this.invalidate();
    this.connect();
  }

  private connect() {
    if (
      !this.started ||
      this.stopped ||
      (this.socket && this.socket.readyState < 2)
    )
      return;
    if (window.navigator.onLine === false) {
      this.setStatus("disconnected");
      return;
    }
    const generation = ++this.generation;
    this.setStatus("connecting");
    this.binding = null;
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    let socket: WebSocket;
    try {
      socket = (this.options.createSocket ?? ((url) => new WebSocket(url)))(
        `${protocol}//${window.location.host}/api/canvas/connection`,
      );
    } catch {
      this.reconnect();
      return;
    }
    this.socket = socket;
    const csrf = browserConnectionCsrf();
    const assembler = new WireAssembler();
    let incoming = Promise.resolve(),
      queuedBytes = 0,
      protocolFailure = false;
    const current = () =>
      !this.stopped &&
      this.started &&
      generation === this.generation &&
      this.socket === socket &&
      socket.readyState === 1;
    const invalid = () => {
      protocolFailure = true;
      socket.close();
    };
    this.opening = setTimeout(() => socket.close(), contract.ackTimeoutMillis);
    socket.onmessage = (event) => {
      if (!current()) return;
      let reply: Record<string, unknown>;
      try {
        const text = String(event.data);
        if (
          new TextEncoder().encode(text).byteLength > contract.maxMessageBytes
        )
          throw unavailable();
        reply = JSON.parse(text) as Record<string, unknown>;
        if (
          !reply ||
          typeof reply !== "object" ||
          typeof reply.type !== "string" ||
          typeof reply.code !== "number"
        )
          throw unavailable();
      } catch {
        invalid();
        return;
      }
      // Native committed delivery is independent of RPC request ids, including initial bootstrap.
      if (reply.type === "YJS_FRAME") {
        const size = new TextEncoder().encode(String(event.data)).byteLength;
        queuedBytes += size;
        if (
          reply.code !== 20000 ||
          !this.binding ||
          queuedBytes > 24 * 1024 * 1024
        ) {
          invalid();
          return;
        }
        incoming = incoming
          .then(async () => {
            if (!current()) return;
            const value = await assembler.accept(reply.data);
            if (!current() || !value) return;
            if (typeof value !== "object" || Array.isArray(value))
              throw unavailable();
            const message = value as Record<string, unknown>,
              binding = this.binding;
            if (
              !binding ||
              message.actorKey !== this.options.actorKey ||
              message.canvasId !== this.options.canvasId ||
              message.sessionId !== this.options.sessionId ||
              (message.epoch !== undefined && message.epoch !== binding.epoch)
            )
              throw unavailable();
            if (
              ["DURABLE_ACK", "QUERY_RESULT", "REJECT"].includes(
                String(message.type),
              )
            ) {
              const id = message.requestId;
              if (typeof id !== "string") throw unavailable();
              const pending = this.logical.get(id);
              if (!pending) return;
              if (
                message.type !== "REJECT" &&
                message.type !== pending.expected
              )
                throw unavailable();
              this.logical.delete(id);
              clearTimeout(pending.timer);
              if (message.type === "REJECT") {
                const problem = message.error as {
                  code?: string | number;
                  message?: string;
                  definitive?: boolean;
                };
                pending.reject(
                  Object.assign(
                    new Error(problem?.message ?? "画布提交待确认"),
                    {
                      code: problem?.code,
                      definitive: problem?.definitive === true,
                    },
                  ),
                );
                if (
                  typeof problem?.code === "number" &&
                  [401, 403, 404].includes(Math.floor(problem.code / 100))
                ) {
                  this.stopped = true;
                  this.setStatus("expired");
                  if (
                    problem.code === 40100 &&
                    csrf === browserConnectionCsrf()
                  ) {
                    window.dispatchEvent(new Event("glowloom:session-expired"));
                  }
                  socket.close();
                }
              } else pending.resolve(message.receipt);
            } else if (
              ["SYNC_STATE", "COMMITTED_UPDATE", "WATERMARK"].includes(
                String(message.type),
              )
            ) {
              await this.options.message(message);
            } else throw unavailable();
          })
          .catch(() => {
            if (current()) invalid();
          })
          .finally(() => {
            queuedBytes -= size;
          });
        return;
      }
      if (typeof reply.requestId !== "string") {
        invalid();
        return;
      }
      const pending = this.rpc.get(reply.requestId);
      if (!pending) return;
      this.rpc.delete(reply.requestId);
      clearTimeout(pending.timer);
      if (reply.code !== 20000) {
        const error = new ApiError(
          String(reply.message || "画布连接请求失败"),
          reply.code,
          reply.requestId,
        );
        pending.reject(error);
        if ([401, 403, 404].includes(Math.floor(error.code / 100))) {
          this.stopped = true;
          this.setStatus("expired");
          if (error.code === 40100 && csrf === browserConnectionCsrf())
            window.dispatchEvent(new Event("glowloom:session-expired"));
          socket.close();
        }
      } else if (reply.type !== pending.expected || reply.data == null) {
        pending.reject(unavailable());
        invalid();
      } else pending.resolve(reply.data);
    };
    socket.onopen = () => {
      if (!current()) {
        socket.close();
        return;
      }
      if (this.opening) clearTimeout(this.opening);
      void this.requestNow(
        "OPEN",
        "READY",
        {
          canvasId: this.options.canvasId,
          csrfToken: csrf,
          sessionId: this.options.sessionId,
        },
        true,
      )
        .then(async (value) => {
          if (!current()) return;
          const binding = value as CollaborationBinding;
          if (
            binding.syncProtocol !== "yjs-v1" ||
            binding.actorKey !== this.options.actorKey ||
            binding.canvasId !== this.options.canvasId ||
            binding.sessionId !== this.options.sessionId ||
            !/^[a-f0-9-]{36}$/.test(binding.epoch) ||
            !/^[a-f0-9-]{36}$/.test(binding.connectionId) ||
            !/^[0-9]+$/.test(binding.bindingRevision) ||
            typeof binding.edit !== "boolean" ||
            typeof binding.lifecycle !== "string" ||
            "grant" in binding
          )
            throw unavailable();
          this.binding = binding;
          await this.options.ready(binding);
          if (!current()) return;
          await this.requestNow("YJS_OPEN", "YJS_READY", {}, true, 35000);
          await incoming;
          if (!current()) return;
          this.failures = 0;
          this.healthyHeartbeats = 0;
          this.setStatus("connected");
          const interval = binding.heartbeatIntervalMillis;
          this.heartbeat = setInterval(
            () => {
              if (this.rpc.size || this.queued || this.status !== "connected")
                return;
              void this.request("HEARTBEAT", "HEARTBEAT_ACK", {}).then(
                () => {
                  if (
                    current() &&
                    ++this.healthyHeartbeats >=
                      contract.healthyHeartbeatsToReset
                  )
                    this.protocolFailures = 0;
                },
                () => socket.close(),
              );
            },
            Number.isFinite(interval) && interval >= 1000 && interval <= 60000
              ? interval
              : 15000,
          );
        })
        .catch(() => {
          if (current()) invalid();
        });
    };
    socket.onerror = () => {
      if (current()) socket.close();
    };
    socket.onclose = (event) => {
      if (generation !== this.generation) return;
      assembler.clear();
      this.socket = null;
      this.clearPending();
      this.clearTimers();
      if (this.stopped || !this.started) return;
      if (
        event.code === contract.sessionCloseCode ||
        event.code === contract.oversizeCloseCode
      ) {
        this.stopped = true;
        this.setStatus(
          event.code === contract.sessionCloseCode ? "expired" : "oversized",
        );
        return;
      }
      if (protocolFailure || event.code === contract.protocolCloseCode) {
        if (++this.protocolFailures >= contract.maxProtocolFailures) {
          this.stopped = true;
          this.setStatus("blocked");
          this.retry = setTimeout(() => {
            this.retry = null;
            this.cooldownReady = true;
            if (
              window.document.visibilityState !== "hidden" &&
              window.navigator.onLine !== false
            )
              this.probe();
          }, contract.protocolCooldownMillis);
          return;
        }
      }
      this.reconnect();
    };
  }

  private reconnect() {
    this.setStatus("disconnected");
    if (this.retry) clearTimeout(this.retry);
    if (this.stopped || !this.started || window.navigator.onLine === false)
      return;
    const delay = Math.min(
      contract.retryMaxMillis,
      contract.retryBaseMillis * 2 ** Math.min(this.failures++, 4),
    );
    this.retry = setTimeout(
      () => {
        this.retry = null;
        this.connect();
      },
      delay * (0.5 + (this.options.random ?? Math.random)() / 2),
    );
  }

  private request(
    type: string,
    expected: string,
    payload: object,
  ): Promise<unknown> {
    if (this.queued >= contract.maxPendingRequests)
      return Promise.reject(unavailable());
    this.queued++;
    const generation = this.generation;
    const result = this.serial.then(() => {
      if (generation !== this.generation) throw unavailable();
      return this.requestNow(type, expected, payload);
    });
    this.serial = result.catch(() => {});
    return result.finally(() => {
      this.queued--;
    });
  }

  private requestNow(
    type: string,
    expected: string,
    payload: object,
    opening = false,
    timeout: number = contract.ackTimeoutMillis,
  ): Promise<unknown> {
    const socket = this.socket;
    if (
      this.stopped ||
      !socket ||
      socket.readyState !== 1 ||
      (!opening && this.status !== "connected")
    )
      return Promise.reject(unavailable());
    const requestId = crypto.randomUUID();
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.rpc.delete(requestId);
        reject(unavailable());
        socket.close();
      }, timeout);
      this.rpc.set(requestId, { expected, resolve, reject, timer });
      try {
        const text = JSON.stringify({ type, requestId, ...payload });
        if (
          new TextEncoder().encode(text).byteLength > contract.maxMessageBytes
        )
          throw unavailable();
        socket.send(text);
      } catch (error) {
        clearTimeout(timer);
        this.rpc.delete(requestId);
        reject(error);
        socket.close();
      }
    });
  }

  private async operation(
    type: "SUBMIT_UPDATE" | "QUERY_RESULT",
    update: FrozenUpdate,
  ): Promise<unknown> {
    if (this.status !== "connected" || this.logical.size || this.operationBusy)
      throw unavailable();
    this.operationBusy = true;
    try {
      if (
        type === "SUBMIT_UPDATE" &&
        update.update.length > Math.ceil((1024 * 1024) / 3) * 4
      )
        throw Object.assign(
          new Error("这次修改超过同步大小限制，请保留输入后缩小内容"),
          {
            code: "UPDATE_TOO_LARGE",
            definitive: true,
          },
        );
      const generation = this.generation,
        requestId = crypto.randomUUID();
      const frames = await wireFrames(
        type === "SUBMIT_UPDATE"
          ? { ...update, type, requestId }
          : { type, requestId, epoch: update.epoch, updateId: update.updateId },
        2 * 1024 * 1024,
      );
      if (generation !== this.generation) throw unavailable();
      return await new Promise((resolve, reject) => {
        const timer = setTimeout(() => {
          this.logical.delete(requestId);
          reject(unavailable());
          this.socket?.close();
        }, 35000);
        this.logical.set(requestId, {
          expected: type === "SUBMIT_UPDATE" ? "DURABLE_ACK" : "QUERY_RESULT",
          resolve,
          reject,
          timer,
        });
        void (async () => {
          for (const frame of frames) {
            if (generation !== this.generation) throw unavailable();
            await this.request("YJS_FRAME", "YJS_FORWARDED", { frame });
          }
        })().catch((error) => {
          this.logical.delete(requestId);
          clearTimeout(timer);
          reject(error);
        });
      });
    } finally {
      this.operationBusy = false;
    }
  }

  async submit(update: FrozenUpdate) {
    return (await this.operation("SUBMIT_UPDATE", update)) as DurableReceipt;
  }
  async query(update: FrozenUpdate) {
    return (await this.operation(
      "QUERY_RESULT",
      update,
    )) as DurableReceipt | null;
  }

  private clearPending() {
    for (const map of [this.rpc, this.logical]) {
      for (const pending of map.values()) {
        clearTimeout(pending.timer);
        pending.reject(unavailable());
      }
      map.clear();
    }
  }
  private clearTimers() {
    if (this.opening) clearTimeout(this.opening);
    if (this.retry) clearTimeout(this.retry);
    if (this.heartbeat) clearInterval(this.heartbeat);
    this.opening = this.retry = this.heartbeat = null;
  }
  private invalidate() {
    ++this.generation;
    this.clearTimers();
    this.clearPending();
    const socket = this.socket;
    this.socket = null;
    socket?.close();
  }
  stop() {
    this.started = false;
    this.stopped = true;
    window.removeEventListener("online", this.available);
    window.removeEventListener("offline", this.offline);
    window.document.removeEventListener("visibilitychange", this.available);
    this.invalidate();
  }
}
