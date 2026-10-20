import { getCanvas } from "@/api/canvases";
import {
  getCSRFToken,
  getCreationScope,
  getScopeGeneration,
  getSessionGeneration,
  RequestError,
} from "@/lib/request";

export type LiveStatus = "connecting" | "live" | "reconnecting" | "closed";
type Callbacks = {
  snapshot: (document: unknown, writable: boolean) => void;
  status: (status: LiveStatus, writable?: boolean) => void;
  revoked: () => void;
};
type Pending = {
  key: string;
  body: API.CommandsInput;
  resolve: (value: API.Result) => void;
  reject: (error: Error) => void;
  promise: Promise<API.Result>;
};

/** One document, one in-memory request. PostgreSQL receipts confirm persistence. */
export class CanvasLive {
  private socket?: WebSocket;
  private retry?: ReturnType<typeof setTimeout>;
  private watchdog?: ReturnType<typeof setTimeout>;
  private pending?: Pending;
  private stopped = true;
  private ready = false;
  private attempts = 0;
  private session = 0;
  private scope = 0;
  constructor(
    private id: string,
    private callbacks: Callbacks,
  ) {}

  start() {
    if (!this.stopped) return;
    this.stopped = false;
    this.session = getSessionGeneration();
    this.scope = getScopeGeneration();
    window.addEventListener("online", this.sync);
    document.addEventListener("visibilitychange", this.sync);
    this.connect();
  }
  stop() {
    this.stopped = true;
    this.ready = false;
    clearTimeout(this.retry);
    clearTimeout(this.watchdog);
    window.removeEventListener("online", this.sync);
    document.removeEventListener("visibilitychange", this.sync);
    const socket = this.socket;
    this.socket = undefined;
    socket?.close();
    this.pending?.reject(new RequestError("request_unconfirmed"));
    this.pending = undefined;
  }
  private current() {
    return (
      this.session === getSessionGeneration() &&
      this.scope === getScopeGeneration()
    );
  }
  private sync = () => {
    if (document.visibilityState !== "visible" || this.stopped) return;
    if (!this.current()) return this.revoke();
    if (this.ready) this.send({ type: "sync" });
    else if (!this.socket || this.socket.readyState === 3) {
      clearTimeout(this.retry);
      this.connect();
    }
  };
  private revoke() {
    if (this.stopped) return;
    this.stop();
    this.callbacks.status("closed");
    this.callbacks.revoked();
  }
  private touch(socket: WebSocket) {
    clearTimeout(this.watchdog);
    this.watchdog = setTimeout(() => {
      if (this.socket === socket) socket.close();
    }, 20_000);
  }
  private send(value: unknown) {
    if (this.socket?.readyState === 1) this.socket.send(JSON.stringify(value));
  }
  private connect() {
    if (this.stopped) return;
    if (!this.current()) return this.revoke();
    if (this.socket && this.socket.readyState < 2) return;
    this.ready = false;
    this.callbacks.status(this.attempts ? "reconnecting" : "connecting");
    const url = new URL(`/api/canvases/${this.id}/live`, window.location.href);
    url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
    const socket = new WebSocket(url.href);
    this.socket = socket;
    this.touch(socket);
    socket.onopen = () => {
      if (this.socket !== socket || this.stopped) return;
      this.send({
        type: "join",
        protocol_version: 1,
        scope_id: getCreationScope(),
        csrf_token: getCSRFToken(),
      });
    };
    socket.onmessage = (event) => {
      if (this.socket !== socket || this.stopped) return;
      if (!this.current()) return this.revoke();
      this.touch(socket);
      try {
        const p = JSON.parse(event.data) as Record<string, unknown>;
        if (p.type === "snapshot" && typeof p.writable === "boolean") {
          this.callbacks.snapshot(p.document, p.writable);
        } else if (p.type === "ready") {
          this.ready = true;
          this.attempts = 0;
          this.callbacks.status("live", p.writable === true);
          if (this.pending)
            this.send({
              type: "resolve",
              op_id: this.pending.key,
              body: this.pending.body,
            });
        } else if (
          p.type === "not_found" &&
          this.pending &&
          p.op_id === this.pending.key
        ) {
          this.send({
            type: "submit",
            op_id: this.pending.key,
            body: this.pending.body,
          });
        } else if (
          p.type === "ack" &&
          this.pending &&
          p.op_id === this.pending?.key &&
          p.result &&
          typeof p.revision === "number"
        ) {
          const pending = this.pending;
          this.pending = undefined;
          pending.resolve(p.result as API.Result);
        } else if (p.type === "error") {
          if (p.status === 401 && p.code === "access_expired") {
            // Reuse the existing HTTP renewal policy; socket pings never renew.
            void getCanvas({ id: this.id }).then(
              () => {
                if (this.socket === socket && !this.stopped) socket.close();
              },
              (error: unknown) => {
                if (this.socket !== socket || this.stopped) return;
                if (
                  error instanceof RequestError &&
                  [401, 403, 404].includes(error.status)
                )
                  this.revoke();
                else socket.close();
              },
            );
          } else if ([401, 403, 404].includes(Number(p.status))) {
            this.revoke();
          } else if (
            this.pending &&
            p.op_id === this.pending.key &&
            Number(p.status) >= 400 &&
            Number(p.status) < 500
          ) {
            const pending = this.pending;
            this.pending = undefined;
            pending.reject(new RequestError(String(p.code), Number(p.status)));
          } else socket.close(); // Unknown result: recheck the original receipt.
        }
      } catch {
        socket.close();
      }
    };
    socket.onerror = () => socket.close();
    socket.onclose = () => {
      if (this.socket !== socket || this.stopped) return;
      this.ready = false;
      clearTimeout(this.watchdog);
      this.callbacks.status("reconnecting");
      const delay =
        Math.min(10_000, 500 * 2 ** Math.min(this.attempts++, 5)) +
        Math.random() * 250;
      this.retry = setTimeout(() => {
        // HTTP can inspect an expired/invalid handshake and reuse session renewal;
        // browser WebSocket errors deliberately hide handshake response bodies.
        if (this.socket !== socket || this.stopped) return;
        void getCanvas({ id: this.id }).then(
          () => {
            if (this.socket === socket && !this.stopped) this.connect();
          },
          (error: unknown) => {
            if (this.socket !== socket || this.stopped) return;
            if (
              error instanceof RequestError &&
              [401, 403, 404].includes(error.status)
            )
              this.revoke();
            else this.connect();
          },
        );
      }, delay);
    };
  }
  submit(key: string, body: API.CommandsInput): Promise<API.Result> {
    if (this.stopped || !this.current())
      return Promise.reject(new RequestError("request_unconfirmed"));
    if (this.pending) {
      if (
        this.pending.key !== key ||
        JSON.stringify(this.pending.body) !== JSON.stringify(body)
      )
        return Promise.reject(new RequestError("save_pending", 409));
      return this.pending.promise;
    }
    if (!this.ready)
      return Promise.reject(new RequestError("request_unconfirmed"));
    // The recovery envelope is one byte longer than submit. Both must fit.
    const envelope = JSON.stringify({ type: "resolve", op_id: key, body });
    if (new TextEncoder().encode(envelope).byteLength > 1 << 20)
      return Promise.reject(new RequestError("canvas_request_too_large", 413));
    const frozen = structuredClone(body);
    let resolve!: Pending["resolve"], reject!: Pending["reject"];
    const promise = new Promise<API.Result>((yes, no) => {
      resolve = yes;
      reject = no;
    });
    this.pending = { key, body: frozen, promise, resolve, reject };
    this.send({ type: "submit", op_id: key, body: frozen });
    return promise;
  }
}
