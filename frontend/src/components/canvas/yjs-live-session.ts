import {
  CollaborationSession,
  type CommittedState,
  type DurableReceipt,
  type FrozenUpdate,
} from "@glowloom/canvas-collab/session";
import {
  archiveRecovery,
  recoveryFields,
  type RecoveryField,
} from "@glowloom/canvas-collab/recovery";
import {
  nodeTextBinding,
  canvasUndoManager,
  applyCanvasHistory,
  type NodeTextBinding,
  type CanvasTextDraft,
} from "@glowloom/canvas-collab/model";
import {
  serviceSnapshot,
  serviceProjection,
  type ServiceDocument,
  type ServiceGraph,
} from "@glowloom/canvas-collab/service-document";
import type * as Y from "yjs";
import { RequestError } from "@/lib/request";
import { canvasGraphFingerprint } from "./lib/canvas-json";
import { serviceDocumentFromEditor, serviceGraph } from "./service-graph";
import type { CanvasEditorState } from "./lib/canvas-document-adapter";
import { publishCanvasMedia, type CanvasMediaReceipt } from "./publish-media";

type State = {
  documentId: string;
  epoch: string;
  writerTerm: string;
  commitSeq: string;
  state: string;
  graphHash: string;
  document: API.DocumentDTO;
};
export type YjsPeer = {
  actorId: string;
  clientId: string;
  cursor: { x: number; y: number } | null;
  updatedAt: string;
};
export type YjsVersion = {
  id: string;
  epoch: string;
  revision: string;
  createdAt: string;
  reason: string;
};
type Packet = {
  presence?: YjsPeer[];
  yjs_versions?: YjsVersion[];
  document?: API.DocumentDTO;
  type: string;
  yjs_state?: State;
  yjs_receipt?: DurableReceipt & { status: string; code?: string };
  actor_id?: string;
  op_id?: string;
  permissions?: API.Permissions;
  status?: number;
  code?: string;
};
export type YjsView = {
  document: CanvasEditorState | null;
  dto: API.DocumentDTO | null;
  connected: boolean;
  writable: boolean;
  dirty: boolean;
  migration: boolean;
  message: string;
  phase: string;
  recovery: { sourceId: string; fields: RecoveryField[] }[];
  canUndo: boolean;
  canRedo: boolean;
};
const problem = (code: string, definitive = false) =>
  Object.assign(new Error(code), { code, definitive });
const asShared = (document: API.DocumentDTO) =>
  document as unknown as ServiceDocument;
const localSource = (node: CanvasEditorState["nodes"][number]) =>
  ["image", "video", "audio"].includes(node.type) &&
  /^(blob:|data:)/.test(node.metadata?.content ?? "")
    ? node.metadata!.content
    : undefined;

/** One page-memory Y.Doc queue and one socket. Heartbeats resolve immutable originals. */
export class CanvasYjsLiveSession {
  readonly clientID = crypto.randomUUID();
  private socket: WebSocket | null = null;
  private session: CollaborationSession<ServiceGraph> | null = null;
  private draftReaders = new Map<
    string,
    {
      session: CollaborationSession<ServiceGraph>;
      read: () => CanvasTextDraft | null;
    }
  >();
  private undoManager: Y.UndoManager | null = null;
  private historyFrame: {
    before: ReturnType<typeof serviceSnapshot>;
    after: ReturnType<typeof serviceSnapshot>;
  } | null = null;
  private dto: API.DocumentDTO | null = null;
  private rendered: API.DocumentDTO | null = null;
  private draft: CanvasEditorState | null = null;
  private gesture = false;
  private online = false;
  private permitted = false;
  private disposed = false;
  private failures = 0;
  private reconnectTimer?: ReturnType<typeof setTimeout>;
  private deadline?: ReturnType<typeof setTimeout>;
  private peers: YjsPeer[] = [];
  private presenceListeners = new Set<() => void>();
  private cursorTimer?: ReturnType<typeof setTimeout>;
  private cursor: { x: number; y: number } | null = null;
  subscribePresence = (listener: () => void) => {
    this.presenceListeners.add(listener);
    return () => {
      this.presenceListeners.delete(listener);
    };
  };
  getPresence = () => this.peers;
  private setPeers(peers: YjsPeer[]) {
    this.peers = peers.filter((peer) => peer.clientId !== this.clientID);
    for (const listener of this.presenceListeners) listener();
  }
  setCursor = (cursor: { x: number; y: number } | null) => {
    this.cursor = cursor;
    if (this.cursorTimer) return;
    this.cursorTimer = setTimeout(() => {
      this.cursorTimer = undefined;
      if (this.online && this.socket?.readyState === WebSocket.OPEN)
        this.socket.send(
          JSON.stringify({
            type: "yjs_presence",
            body: { cursor: this.cursor },
          }),
        );
    }, 100);
  };
  private controls = new Map<
    string,
    {
      resolve: (packet: Packet) => void;
      reject: (error: Error) => void;
      timer?: ReturnType<typeof setTimeout>;
    }
  >();
  private restoreCommand: {
    type: string;
    op_id: string;
    body: Record<string, unknown>;
  } | null = null;
  private pendingRPC?: {
    id: string;
    resolve: (receipt: DurableReceipt | null) => void;
    reject: (cause: Error) => void;
    timer: ReturnType<typeof setTimeout>;
  };
  private incoming: Promise<void> = Promise.resolve();
  private draining = false;
  private epochPending: CommittedState | null = null;
  private switchingEpoch = false;
  private scheduled = false;
  private listeners = new Set<() => void>();
  private receipts = new Map<string, CanvasMediaReceipt>();
  private uploads = new Map<
    string,
    { source: string; abort: AbortController }
  >();
  private uploadErrors = new Map<string, string>();
  private renewal: AbortController | null = null;
  private view: YjsView = {
    document: null,
    dto: null,
    connected: false,
    writable: false,
    dirty: false,
    migration: false,
    message: "",
    phase: "正在连接",
    recovery: [],
    canUndo: false,
    canRedo: false,
  };
  constructor(
    private config: {
      canvasID: string;
      projectID: string;
      actorID: string;
      scopeID: string;
      csrf: () => string | null;
      renew: (signal: AbortSignal) => Promise<void>;
      socket?: (url: string) => WebSocket;
    },
  ) {}
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };
  getSnapshot = () => this.view;
  private emit(patch: Partial<YjsView> = {}) {
    const session = this.session;
    const recovery =
      session?.recoverySources
        .map((source) => ({
          sourceId: source.id,
          fields: recoveryFields(source, session.snapshot),
        }))
        .filter((source) => source.fields.length) ?? [];
    const dirty =
      (!!session &&
        (session.status !== "CLEAN" ||
          !!session.inFlight ||
          !!session.sealed ||
          recovery.length > 0)) ||
      this.uploads.size > 0 ||
      this.uploadErrors.size > 0 ||
      this.gesture ||
      !!this.restoreCommand;
    this.view = {
      ...this.view,
      connected: this.online,
      writable:
        this.permitted &&
        !this.restoreCommand &&
        !this.epochPending &&
        !session?.sealed &&
        session?.status !== "REJECTED",
      dirty,
      recovery,
      canUndo: !!this.undoManager?.canUndo(),
      canRedo: !!this.undoManager?.canRedo(),
      phase:
        recovery.length || session?.sealed
          ? "输入已保留，需处理"
          : this.uploads.size
            ? "素材上传中"
            : this.uploadErrors.size
              ? "素材待重试"
              : this.gesture
                ? "正在编辑，尚未保存"
                : session?.status === "UNKNOWN"
                  ? "保存结果未确认"
                  : dirty
                    ? "等待保存确认"
                    : this.online
                      ? "已保存"
                      : "连接中断",
      ...patch,
    };
    for (const listener of this.listeners) listener();
  }
  private changed = () => {
    if (this.disposed) return;
    this.render();
    this.schedule();
  };
  private render() {
    if (!this.session || !this.dto) return;
    if (!this.gesture) {
      const projected = serviceProjection(
        this.session.snapshot,
        asShared(this.dto),
      ) as unknown as API.DocumentDTO;
      this.rendered = projected;
      const graph = serviceGraph(
        projected,
        this.config.canvasID,
        this.config.projectID,
      );
      // Media bytes remain page-local until upload; unrelated shared edits still render.
      if (this.draft) {
        const privateNodes = this.draft.nodes.filter((node) =>
          localSource(node),
        );
        for (const node of privateNodes) {
          const found = graph.nodes.findIndex((item) => item.id === node.id);
          if (found >= 0)
            graph.nodes[found] = {
              ...graph.nodes[found],
              metadata: {
                ...graph.nodes[found].metadata,
                content: node.metadata?.content,
              },
            };
          else graph.nodes.push(node);
        }
      }
      const same =
        JSON.stringify(graph.nodes) ===
          JSON.stringify(this.view.document?.nodes) &&
        JSON.stringify(graph.connections) ===
          JSON.stringify(this.view.document?.connections);
      this.emit({ document: same ? this.view.document : graph, dto: this.dto });
    } else this.emit();
  }
  start() {
    this.disposed = false;
    window.addEventListener("online", this.resume);
    window.addEventListener("pageshow", this.restored);
    this.connect();
  }
  private resume = () => {
    if (!this.online && !this.disposed) {
      this.failures = 0;
      void this.reconnect(true);
    }
  };
  private restored = (event: PageTransitionEvent) => {
    if (event.persisted) {
      this.permitted = false;
      void this.reconnect(true);
    }
  };
  private connect() {
    if (this.disposed || this.renewal) return;
    const url = new URL(
      `/api/canvases/${this.config.canvasID}/live`,
      location.origin,
    );
    url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
    const socket = this.config.socket?.(url.href) ?? new WebSocket(url);
    this.socket = socket;
    socket.onopen = () => {
      if (this.socket !== socket) return;
      socket.send(
        JSON.stringify({
          type: "join",
          protocol_version: 4,
          client_id: this.clientID,
          scope_id: this.config.scopeID,
          csrf_token: this.config.csrf(),
        }),
      );
    };
    socket.onmessage = (event) => {
      if (this.socket !== socket || this.disposed) return;
      this.armDeadline();
      this.incoming = this.incoming
        .then(async () => {
          if (this.socket !== socket || this.disposed) return;
          await this.receive(JSON.parse(String(event.data)) as Packet);
        })
        .catch((cause) => {
          this.emit({
            message: cause instanceof Error ? cause.message : "协作消息无效",
          });
          socket.close();
        });
    };
    socket.onerror = () => socket.close();
    socket.onclose = () => {
      if (this.socket !== socket || this.disposed) return;
      this.online = false;
      this.setPeers([]);
      clearTimeout(this.deadline);
      this.rejectRPC(problem("连接中断，原提交等待查证"));
      this.emit();
      this.scheduleReconnect();
    };
    this.armDeadline();
  }
  private armDeadline() {
    clearTimeout(this.deadline);
    this.deadline = setTimeout(() => this.socket?.close(), 20000);
  }
  private scheduleReconnect() {
    if (this.disposed) return;
    clearTimeout(this.reconnectTimer);
    // A server-only outage does not dispatch the browser's online event.
    // Retry the connection without replacing any pending business transaction.
    this.failures = Math.min(this.failures + 1, 5);
    this.reconnectTimer = setTimeout(
      () => void this.reconnect(true),
      Math.min(1000 * 2 ** this.failures, 30000),
    );
  }
  private async reconnect(renew: boolean) {
    if (this.disposed || this.renewal) return;
    clearTimeout(this.reconnectTimer);
    const old = this.socket;
    this.socket = null;
    old?.close();
    this.online = false;
    this.rejectRPC(problem("正在重新确认原提交"));
    if (renew) {
      const abort = new AbortController();
      this.renewal = abort;
      try {
        await this.config.renew(abort.signal);
      } catch (cause) {
        this.permitted = false;
        this.emit({
          message: cause instanceof Error ? cause.message : "请重新登录",
        });
        if (
          cause instanceof RequestError &&
          (cause.status === 0 ||
            cause.status >= 500 ||
            [408, 429].includes(cause.status))
        )
          this.scheduleReconnect();
        return;
      } finally {
        this.renewal = null;
      }
    }
    this.connect();
  }
  private makeSession(
    initial: CommittedState,
    recovery?: ReturnType<CollaborationSession<ServiceGraph>["record"]>,
  ) {
    this.captureTextDrafts();
    this.draftReaders.clear();
    // Replacing a rejected/deleted editor ends its gesture. Its readable
    // composition has already been archived; it must not block recovery saves.
    this.gesture = false;
    this.undoManager?.destroy();
    this.session?.destroy();
    this.session = new CollaborationSession<ServiceGraph>({
      scope: {
        origin: location.origin,
        actorKey: `USER:${this.config.actorID}`,
        canvasId: this.config.canvasID,
        sessionId: this.clientID,
      },
      initial,
      storage: "memory",
      transport: {
        submit: (update) =>
          this.rpc("yjs_submit", update).then((receipt) => {
            if (!receipt) throw problem("缺少保存回执");
            return receipt;
          }),
        query: (update) => this.rpc("yjs_resolve", update),
      },
      changed: this.changed,
      recovery,
    });
    this.undoManager = canvasUndoManager(this.session.doc);
    const recordFrame = ({
      stackItem,
    }: {
      stackItem: { meta: Map<unknown, unknown> };
    }) => {
      if (!this.historyFrame) return;
      const previous = stackItem.meta.get(
        "canvas-frame",
      ) as typeof this.historyFrame;
      stackItem.meta.set(
        "canvas-frame",
        structuredClone({
          before: previous?.before ?? this.historyFrame.before,
          after: this.historyFrame.after,
        }),
      );
    };
    this.undoManager.on("stack-item-added", recordFrame);
    this.undoManager.on("stack-item-updated", recordFrame);
  }
  private async receive(packet: Packet) {
    if (
      packet.actor_id &&
      packet.actor_id !== "00000000-0000-0000-0000-000000000000" &&
      packet.actor_id !== this.config.actorID
    )
      throw problem("账号已变更");
    if (packet.type === "yjs_presence") {
      this.setPeers(packet.presence ?? []);
      return;
    }
    const control = packet.op_id && this.controls.get(packet.op_id);
    if (control && packet.type === "error" && (packet.status ?? 503) < 500) {
      clearTimeout(control.timer);
      this.controls.delete(packet.op_id!);
      if (this.restoreCommand?.op_id === packet.op_id)
        this.restoreCommand = null;
      control.reject(problem(packet.code ?? "Control rejected"));
      this.emit();
      if (![401, 403, 404].includes(packet.status ?? 0)) return;
    } else if (
      control &&
      ["yjs_history", "yjs_version", "yjs_restore_ack"].includes(packet.type)
    ) {
      clearTimeout(control.timer);
      this.controls.delete(packet.op_id!);
      if (this.restoreCommand?.op_id === packet.op_id)
        this.restoreCommand = null;
      control.resolve(packet);
      this.emit();
      return;
    }
    if (packet.type === "error") {
      // A transient history/control failure must not reject an unrelated
      // in-flight business update. Restore keeps its original command ID.
      if (control && (packet.status ?? 503) >= 500) {
        this.emit({ message: packet.code ?? "协作暂不可用" });
        return;
      }
      if ([401, 403, 404].includes(packet.status ?? 0)) {
        this.permitted = false;
        if (packet.status !== 401) {
          this.session?.destroy();
          this.session = null;
          this.dto = null;
          this.draft = null;
          this.emit({
            document: null,
            dto: null,
            message: "画布访问权限已失效",
          });
          this.stop();
          return;
        }
        void this.reconnect(true);
        return;
      }
      this.rejectRPC(problem(packet.code ?? "协作暂不可用"));
      this.emit({ message: packet.code ?? "协作暂不可用" });
      return;
    }
    if (packet.type === "yjs_migration_required") {
      const state = packet.yjs_state!;
      this.dto = state.document;
      this.online = true;
      this.permitted = !!state.document.permissions?.write;
      this.emit({
        migration: true,
        dto: state.document,
        document: serviceGraph(
          state.document,
          this.config.canvasID,
          this.config.projectID,
        ),
        writable: false,
        phase: "待启用多人协作",
      });
      return;
    }
    if (packet.type === "yjs_snapshot") {
      const state = packet.yjs_state!;
      if (
        state.documentId !== this.config.canvasID ||
        state.document.id !== this.config.canvasID ||
        state.document.project_id !== this.config.projectID ||
        !/^(0|[1-9]\d*)$/.test(state.commitSeq)
      )
        throw problem("画布协作身份无效");
      this.dto = state.document;
      this.online = true;
      this.permitted = !!state.document.permissions?.write;
      this.failures = 0;
      const initial: CommittedState = {
        epoch: state.epoch,
        commitSeq: state.commitSeq,
        contentRevision: state.document.revision!,
        graphHash: state.graphHash,
        update: state.state,
        bindingRevision: state.document.revision!,
        modelVersion: 1,
        lifecycle: "ACTIVE",
        edit: this.permitted,
      };
      this.captureTextDrafts();
      if (!this.session) this.makeSession(initial);
      else if (this.session.confirmed.epoch !== state.epoch) {
        this.epochPending = initial;
        this.emit({ message: "画布版本已切换，正在查证原提交并保留本页输入" });
        queueMicrotask(() => void this.adoptEpoch());
        return;
      } else await this.session.receive({ ...initial, snapshot: true });
      if (this.session?.status === "REJECTED" && !this.session.inFlight) {
        const record = archiveRecovery(this.session.record(), initial);
        this.makeSession(initial, record);
      }
      this.emit({ migration: false, message: "" });
      this.render();
      this.schedule();
      return;
    }
    if (packet.type === "heartbeat") {
      if (this.restoreCommand && this.socket?.readyState === WebSocket.OPEN)
        this.socket.send(JSON.stringify(this.restoreCommand));
      this.online = true;
      this.permitted = !!packet.permissions?.write;
      this.emit();
      this.schedule();
      return;
    }
    if (packet.type === "yjs_ack" || packet.type === "yjs_not_found") {
      const pending = this.pendingRPC;
      if (!pending || pending.id !== packet.op_id) return;
      clearTimeout(pending.timer);
      this.pendingRPC = undefined;
      const receipt = packet.yjs_receipt;
      if (receipt?.status === "rejected")
        pending.reject(
          Object.assign(problem(receipt.code ?? "提交被拒绝", true), {
            originalOutcome: "REJECTED",
          }),
        );
      else pending.resolve(receipt ?? null);
    }
  }
  private async adoptEpoch() {
    const session = this.session;
    if (
      this.switchingEpoch ||
      this.disposed ||
      !this.epochPending ||
      !this.online ||
      !session
    )
      return;
    this.switchingEpoch = true;
    try {
      if (session.inFlight) {
        try {
          await session.flush();
        } catch {
          /* A durable rejection also settles the original. */
        }
      }
      if (this.disposed || this.session !== session) return;
      if (session.inFlight) {
        this.emit({ message: "原版本的提交结果尚未确认，输入继续保留在本页" });
        return;
      }
      const initial = this.epochPending;
      const recovery = archiveRecovery(session.record(), initial);
      this.epochPending = null;
      this.makeSession(initial, recovery);
      this.emit({ message: "" });
      this.render();
    } catch (cause) {
      this.emit({
        message: cause instanceof Error ? cause.message : "版本切换需要处理",
      });
    } finally {
      this.switchingEpoch = false;
    }
  }
  initialize = () => {
    if (this.dto && this.online)
      this.socket?.send(
        JSON.stringify({
          type: "yjs_initialize",
          body: { revision: this.dto.revision },
        }),
      );
  };
  private rpc(
    type: string,
    update: FrozenUpdate,
  ): Promise<DurableReceipt | null> {
    if (!this.online || this.socket?.readyState !== WebSocket.OPEN)
      return Promise.reject(problem("画布离线，修改保留在本页"));
    if (this.pendingRPC) return Promise.reject(problem("原提交尚未确认"));
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.rejectRPC(problem("回执超时，等待心跳查证"));
      }, 12000);
      this.pendingRPC = { id: update.updateId, resolve, reject, timer };
      this.socket!.send(
        JSON.stringify({ type, op_id: update.updateId, body: update }),
      );
    });
  }
  private rejectRPC(cause: Error) {
    const pending = this.pendingRPC;
    if (!pending) return;
    clearTimeout(pending.timer);
    this.pendingRPC = undefined;
    pending.reject(cause);
  }
  private schedule() {
    if (this.scheduled || this.disposed) return;
    this.scheduled = true;
    queueMicrotask(() => {
      this.scheduled = false;
      void this.drain();
    });
  }
  private async drain() {
    const session = this.session;
    if (this.epochPending) {
      void this.adoptEpoch();
      return;
    }
    if (
      this.draining ||
      !this.online ||
      !session ||
      (!this.permitted && !session.inFlight) ||
      this.gesture ||
      session.status === "REJECTED"
    )
      return;
    if (
      !session.inFlight &&
      (!session.pending.length || Object.keys(session.textDrafts).length)
    )
      return;
    this.draining = true;
    let completed = false;
    try {
      await session.flush({ queryOnly: !this.permitted });
      completed = true;
      if (!this.disposed && this.session === session)
        this.emit({ message: "" });
    } catch (cause) {
      if (!this.disposed && this.session === session) {
        this.emit({
          message: cause instanceof Error ? cause.message : "保存尚未确认",
        });
        if (session.sealed) this.socket?.send(JSON.stringify({ type: "sync" }));
      }
    } finally {
      this.draining = false;
      if (!this.disposed && this.session === session) {
        this.render();
        if (this.epochPending || (completed && session.status === "DIRTY"))
          this.schedule();
      }
    }
  }

  canAccept = (state: CanvasEditorState) =>
    !!this.session &&
    this.view.writable &&
    this.session.pending.length < 128 &&
    new TextEncoder().encode(JSON.stringify(state)).byteLength <
      16 * 1024 * 1024;
  change = (state: CanvasEditorState, stable: boolean) => {
    if (!this.session || !this.rendered || !this.canAccept(state)) return;
    this.draft = state;
    this.gesture = !stable;
    if (!stable) {
      this.emit();
      return;
    }
    const blocked = new Set<string>();
    for (const node of state.nodes) {
      const source = localSource(node);
      if (!source) continue;
      const receipt = this.receipts.get(node.id);
      if (
        receipt?.source === source &&
        receipt.asset &&
        node.documentData?.fileReferences?.some(
          (ref) => ref.fileId === receipt.asset!.id,
        )
      )
        continue;
      blocked.add(node.id);
      if (
        this.online &&
        this.uploads.size < 2 &&
        !this.uploads.has(node.id) &&
        !this.uploadErrors.has(node.id)
      ) {
        const abort = new AbortController();
        this.uploads.set(node.id, { source, abort });
        void publishCanvasMedia(
          { ...state, nodes: [node], connections: [] },
          this.config.projectID,
          this.receipts,
          abort.signal,
        )
          .then((result) => {
            if (this.disposed || abort.signal.aborted || !this.draft) return;
            this.draft = {
              ...this.draft,
              nodes: this.draft.nodes.map((current) =>
                current.id === node.id && localSource(current) === source
                  ? {
                      ...current,
                      documentData: {
                        ...current.documentData!,
                        fileReferences:
                          result.nodes[0].documentData?.fileReferences,
                      },
                    }
                  : current,
              ),
            };
          })
          .catch((cause) => {
            if (!abort.signal.aborted)
              this.uploadErrors.set(
                node.id,
                cause instanceof Error ? cause.message : "素材上传未完成",
              );
          })
          .finally(() => {
            this.uploads.delete(node.id);
            if (!this.disposed && this.draft)
              this.change(this.draft, !this.gesture);
          });
      }
    }
    for (const [id, upload] of this.uploads) {
      if (
        localSource(
          state.nodes.find((node) => node.id === id) ??
            ({ type: "text" } as CanvasEditorState["nodes"][number]),
        ) !== upload.source
      ) {
        upload.abort.abort();
        this.uploads.delete(id);
      }
    }
    const baseline = serviceGraph(
      this.rendered,
      this.config.canvasID,
      this.config.projectID,
    );
    const nodes = state.nodes.flatMap((node) =>
      blocked.has(node.id)
        ? baseline.nodes.filter((old) => old.id === node.id)
        : [node],
    );
    const ids = new Set(nodes.map((node) => node.id));
    const ready = {
      ...state,
      nodes,
      connections: state.connections.filter(
        (edge) => ids.has(edge.fromNodeId) && ids.has(edge.toNodeId),
      ),
    };
    try {
      const after = serviceDocumentFromEditor(this.rendered, ready);
      this.historyFrame = {
        before: this.session.snapshot,
        after: serviceSnapshot(asShared(after)),
      };
      try {
        this.session.change(
          this.historyFrame.after,
          serviceSnapshot(asShared(this.rendered)),
        );
      } finally {
        this.historyFrame = null;
      }
      this.render();
      this.schedule();
    } catch (cause) {
      this.emit({
        message: cause instanceof Error ? cause.message : "修改需要恢复",
      });
    }
  };
  textBinding = (nodeID: string): NodeTextBinding | undefined => {
    if (
      !this.session ||
      !this.session.snapshot.graph.nodes.some((node) => node.id === nodeID)
    )
      return undefined;
    try {
      const session = this.session,
        binding = nodeTextBinding(session.doc, nodeID);
      return {
        ...binding,
        isAttached: () =>
          this.session === session &&
          session.snapshot.graph.nodes.some((node) => node.id === nodeID),
        protectDraft: (document) => session.setTextDraft(nodeID, document),
        registerDraftReader: (read) => {
          const entry = { session, read };
          this.draftReaders.set(nodeID, entry);
          return () => {
            if (this.draftReaders.get(nodeID) === entry)
              this.draftReaders.delete(nodeID);
          };
        },
      };
    } catch {
      return undefined;
    }
  };
  private captureTextDrafts() {
    for (const [id, entry] of this.draftReaders)
      if (entry.session === this.session)
        entry.session.setTextDraft(id, entry.read());
  }
  rename = (name: string) => {
    const value = name.trim();
    if (
      !this.view.writable ||
      !this.session ||
      !value ||
      [...value].length > 128
    )
      throw problem("名称需为1至128个字符");
    const before = this.session.snapshot;
    this.historyFrame = { before, after: { ...before, title: value } };
    try {
      this.session.change(this.historyFrame.after, before);
    } finally {
      this.historyFrame = null;
    }
    this.schedule();
  };
  async applyRecovery(
    sourceId: string,
    fields: RecoveryField[],
    commitSeq: string,
  ) {
    if (!this.session || this.session.confirmed.commitSeq !== commitSeq)
      throw problem("服务器内容已变化，请重新查看差异");
    await this.session.applyRecovery(sourceId, fields);
    this.schedule();
  }
  get commitSeq() {
    return this.session?.confirmed.commitSeq ?? "0";
  }
  discardRecovery = async (sourceId: string) => {
    await this.session?.discardRecoverySource(sourceId);
    this.render();
  };
  history = (direction: "undo" | "redo") => {
    if (!this.view.writable || !this.undoManager || !this.session) return;
    const stack =
      direction === "undo"
        ? this.undoManager.undoStack
        : this.undoManager.redoStack;
    const frame = stack
      .at(-1)
      ?.meta.get("canvas-frame") as typeof this.historyFrame;
    if (frame) {
      const from = direction === "undo" ? frame.after : frame.before,
        to = direction === "undo" ? frame.before : frame.after;
      for (const collection of ["nodes", "connections"] as const) {
        const previous = new Map(
          from.graph[collection].map((item) => [item.id, item]),
        );
        const target = new Map(
          to.graph[collection].map((item) => [item.id, item]),
        );
        const current = new Map(
          this.session.snapshot.graph[collection].map((item) => [
            item.id,
            item,
          ]),
        );
        if (
          [...previous].some(
            ([id, item]) =>
              !target.has(id) &&
              current.has(id) &&
              canvasGraphFingerprint(current.get(id)) !==
                canvasGraphFingerprint(item),
          )
        ) {
          this.emit({
            message: "该对象已有其他编辑，无法单独撤销创建。协作者的内容已保留",
          });
          return;
        }
      }
    }
    this.undoManager.stopCapturing();
    this.historyFrame = frame;
    try {
      applyCanvasHistory(this.session.doc, this.undoManager, direction, {
        epoch: this.session.confirmed.epoch,
        commitSeq: this.session.confirmed.commitSeq,
      });
    } finally {
      this.historyFrame = null;
    }
    this.render();
    this.schedule();
  };
  private control(
    type: string,
    body: Record<string, unknown>,
    persistent = false,
  ): Promise<Packet> {
    if (!this.online || this.socket?.readyState !== WebSocket.OPEN)
      return Promise.reject(problem("Canvas is disconnected"));
    const id = crypto.randomUUID(),
      command = {
        type,
        op_id: id,
        body: persistent ? { ...body, id, sessionId: this.clientID } : body,
      };
    if (persistent) this.restoreCommand = command;
    return new Promise((resolve, reject) => {
      const timer = persistent
        ? undefined
        : setTimeout(() => {
            this.controls.delete(id);
            reject(problem("Request timed out"));
          }, 12000);
      this.controls.set(id, { resolve, reject, timer });
      this.socket!.send(JSON.stringify(command));
      this.emit();
    });
  }
  async versions(before = "0"): Promise<YjsVersion[]> {
    return (await this.control("yjs_history", { before })).yjs_versions ?? [];
  }
  async version(id: string): Promise<API.DocumentDTO> {
    const result = await this.control("yjs_version", { id });
    if (!result.document) throw problem("Version unavailable");
    return result.document;
  }
  async restoreVersion(versionId: string, reviewSeq: string) {
    if (
      this.view.dirty ||
      !this.view.writable ||
      !this.session ||
      reviewSeq !== this.commitSeq
    )
      throw problem("Canvas changed; review the version again");
    await this.control(
      "yjs_restore",
      { versionId, epoch: this.session.confirmed.epoch, commitSeq: reviewSeq },
      true,
    );
  }
  retry = () => {
    this.uploadErrors.clear();
    if (this.draft) this.change(this.draft, !this.gesture);
    if (!this.online) this.resume();
    else this.schedule();
  };
  stop() {
    this.disposed = true;
    clearTimeout(this.cursorTimer);
    this.setPeers([]);
    for (const control of this.controls.values()) {
      clearTimeout(control.timer);
      control.reject(problem("Canvas closed"));
    }
    this.controls.clear();
    this.restoreCommand = null;
    clearTimeout(this.reconnectTimer);
    clearTimeout(this.deadline);
    this.renewal?.abort();
    window.removeEventListener("online", this.resume);
    window.removeEventListener("pageshow", this.restored);
    const socket = this.socket;
    this.socket = null;
    socket?.close();
    this.rejectRPC(problem("页面已关闭"));
    for (const upload of this.uploads.values()) upload.abort.abort();
    this.uploads.clear();
    this.undoManager?.destroy();
    this.session?.destroy();
    this.session = null;
    this.epochPending = null;
    this.draftReaders.clear();
    this.receipts.clear();
  }
}
