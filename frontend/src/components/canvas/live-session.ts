import {
  applyChanges,
  revision,
  type Operation,
  type Change,
} from "./service-document";

type Receipt = {
  op_id: string;
  status: string;
  revision?: string;
  code?: string;
};
export type Submission = {
  client_id: string;
  operation_version: 1;
  base_revision: string;
  operations?: Operation[];
  undo_of?: string;
};
type Pending = {
  id: string;
  body: Submission;
  receipt?: Receipt;
  resolve: (id: string) => void;
  reject: (error: Error) => void;
};
type Packet = {
  type: string;
  document?: API.DocumentDTO;
  revision?: string;
  from_revision?: string;
  to_revision?: string;
  changes?: Change[];
  permissions?: API.Permissions;
  actor_id?: string;
  receipt?: Receipt;
  op_id?: string;
  code?: string;
  status?: number;
  retryable?: boolean;
};
export type LiveStatus =
  "connecting" | "ready" | "saving" | "unconfirmed" | "readonly" | "error";
/** A single durable in-flight intent. Reconnect resolves the immutable original body. */
export class CanvasLiveSession {
  private socket: WebSocket | null = null;
  private stopped = false;
  private timer?: ReturnType<typeof setTimeout>;
  private deadline?: ReturnType<typeof setTimeout>;
  private receiptDeadline?: ReturnType<typeof setTimeout>;
  private pending?: Pending;
  private baseline?: API.DocumentDTO;
  private ready = false;
  private failures = 0;
  private lastFailure = "";
  readonly clientID = crypto.randomUUID();
  constructor(
    private config: {
      canvasID: string;
      actorID: string;
      scopeID: string;
      csrf: () => string | null;
      snapshot: (document: API.DocumentDTO) => void;
      status: (status: LiveStatus, message?: string) => void;
      socket?: (url: string) => WebSocket;
    },
  ) {}
  start() {
    if (this.stopped) return;
    this.ready = false;
    this.config.status(this.pending ? "unconfirmed" : "connecting");
    const url = new URL(
      `/api/canvases/${this.config.canvasID}/live`,
      window.location.origin,
    );
    url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
    const socket = this.config.socket?.(url.href) ?? new WebSocket(url);
    this.socket = socket;
    socket.onopen = () =>
      this.send({
        type: "join",
        protocol_version: 3,
        client_id: this.clientID,
        scope_id: this.config.scopeID,
        csrf_token: this.config.csrf(),
        last_revision: "0",
      });
    socket.onmessage = (event) => {
      if (this.stopped || this.socket !== socket) return;
      try {
        this.receive(JSON.parse(String(event.data)) as Packet);
      } catch (error) {
        this.halt(error instanceof Error ? error.message : "画布消息无效");
      }
    };
    socket.onclose = (event) => {
      if (this.stopped || this.socket !== socket) return;
      this.ready = false;
      clearTimeout(this.receiptDeadline);
      if (++this.failures >= 3) {
        this.halt(
          `画布连接失败：${this.lastFailure || event?.code || "连接已关闭"}`,
        );
        return;
      }
      this.config.status(
        this.pending ? "unconfirmed" : "connecting",
        "连接中断，正在重新确认",
      );
      this.timer = setTimeout(() => this.start(), 2000);
    };
    socket.onerror = () => socket.close();
    this.armDeadline();
  }
  private send(value: unknown) {
    if (this.socket?.readyState === WebSocket.OPEN)
      this.socket.send(JSON.stringify(value));
  }
  private armDeadline() {
    clearTimeout(this.deadline);
    this.deadline = setTimeout(() => this.socket?.close(), 15000);
  }
  private halt(message: string) {
    this.ready = false;
    this.stopped = true;
    clearTimeout(this.timer);
    clearTimeout(this.deadline);
    clearTimeout(this.receiptDeadline);
    this.socket?.close();
    this.config.status(this.pending ? "unconfirmed" : "error", message);
  }
  private receive(packet: Packet) {
    this.armDeadline();
    if (packet.type === "error") {
      if (packet.retryable) {
        this.lastFailure = packet.code ?? "service_unavailable";
        this.socket?.close();
        return;
      }
      this.halt(`画布服务拒绝请求：${packet.code ?? "unknown"}`);
      return;
    }
    if (packet.type === "resync_required") {
      this.socket?.close();
      return;
    }
    if (packet.type === "snapshot") {
      if (!packet.document || packet.document.id !== this.config.canvasID)
        throw new Error("画布身份不匹配");
      revision(packet.document.revision);
      this.baseline = packet.document;
      // An unresolved local edit is not replaced by the reconnect snapshot.
      if (!this.pending) this.config.snapshot(this.baseline);
      this.finish();
    } else if (packet.type === "changes") {
      if (
        !this.baseline ||
        packet.from_revision !== this.baseline.revision ||
        !Array.isArray(packet.changes)
      )
        throw new Error("画布历史不连续");
      this.baseline = applyChanges(this.baseline, packet.changes);
      if (this.baseline.revision !== packet.to_revision)
        throw new Error("画布历史版本不匹配");
      if (!this.pending) this.config.snapshot(this.baseline);
      this.finish();
    } else if (packet.type === "ready") {
      if (
        !this.baseline ||
        packet.actor_id !== this.config.actorID ||
        // Go's live packet uses omitempty,string; only zero is omitted.
        (packet.revision ?? "0") !== this.baseline.revision
      )
        throw new Error("画布会话身份不匹配");
      this.ready = true;
      this.failures = 0;
      this.lastFailure = "";
      this.baseline.permissions = packet.permissions;
      if (this.pending) {
        this.send({
          type: "resolve",
          op_id: this.pending.id,
          body: this.pending.body,
        });
        this.receiptDeadline = setTimeout(() => this.socket?.close(), 15000);
      } else
        this.config.status(packet.permissions?.write ? "ready" : "readonly");
    } else if (
      packet.type === "receipt_not_found" &&
      this.pending &&
      this.pending.id === packet.op_id
    ) {
      if (!this.baseline?.permissions?.write) {
        this.halt("写入权限已失效，原修改尚未保存");
        return;
      }
      this.send({
        type: "submit",
        op_id: this.pending.id,
        body: this.pending.body,
      });
    } else if (packet.type === "ack") {
      if (!this.pending || packet.receipt?.op_id !== this.pending.id) return;
      this.pending.receipt = packet.receipt;
      this.finish();
    } else if (packet.type === "heartbeat" && this.baseline) {
      if (packet.permissions) this.baseline.permissions = packet.permissions;
      if (!packet.permissions?.write && !this.pending)
        this.config.status("readonly");
    }
  }
  private finish() {
    const p = this.pending,
      baseline = this.baseline;
    if (!p?.receipt || !baseline) return;
    if (p.receipt.status === "rejected") {
      clearTimeout(this.receiptDeadline);
      this.pending = undefined;
      this.config.snapshot(baseline);
      this.config.status(baseline.permissions?.write ? "ready" : "readonly");
      p.reject(new Error(`修改未保存：${p.receipt.code ?? "rejected"}`));
      return;
    }
    if (
      p.receipt.status !== "accepted" ||
      BigInt(revision(baseline.revision)) < BigInt(revision(p.receipt.revision))
    )
      return;
    this.pending = undefined;
    clearTimeout(this.receiptDeadline);
    this.config.snapshot(baseline);
    this.config.status(baseline.permissions?.write ? "ready" : "readonly");
    p.resolve(p.id);
  }
  submit(
    operations: Operation[],
    undoOf?: string,
    baseRevision?: string,
  ): Promise<string> {
    if (!this.ready || this.pending || !this.baseline?.permissions?.write)
      return Promise.reject(new Error("画布尚未就绪，请等待同步完成"));
    const body: Submission = {
      client_id: this.clientID,
      operation_version: 1,
      base_revision: revision(baseRevision ?? this.baseline.revision),
      ...(undoOf ? { undo_of: undoOf } : { operations }),
    };
    return new Promise((resolve, reject) => {
      this.pending = { id: crypto.randomUUID(), body, resolve, reject };
      this.config.status("saving");
      this.send({ type: "submit", op_id: this.pending.id, body });
      this.receiptDeadline = setTimeout(() => this.socket?.close(), 15000);
      this.armDeadline();
    });
  }
  get unresolved() {
    return Boolean(this.pending);
  }
  stop() {
    this.stopped = true;
    this.ready = false;
    clearTimeout(this.timer);
    clearTimeout(this.deadline);
    clearTimeout(this.receiptDeadline);
    this.socket?.close();
    this.pending?.reject(new Error("画布已关闭，未确认请求需查证"));
    this.pending = undefined;
  }
}
