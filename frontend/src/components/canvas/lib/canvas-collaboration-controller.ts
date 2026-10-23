import type * as Y from "yjs";
import {
  CollaborationSession,
  type CollaborationScope,
  type CommittedState,
  type CommittedUpdate,
  type ControlCommand,
  type JournalWriter,
  type RecoveryRecord,
  type RecoverySource,
  type SessionTransport,
} from "@glowloom/canvas-collab/session";
import {
  archiveRecovery,
  recoveryFields,
  type RecoveryField,
} from "@glowloom/canvas-collab/recovery";
import {
  collaborationJournal,
  type CollaborationJournal,
} from "@glowloom/canvas-collab/journal";
import {
  canvasUndoManager,
  applyCanvasHistory,
  applyRecoveredCanvasChanges,
  nodeTextBinding,
  type CanvasTextDraft,
  type NodeTextBinding,
} from "@glowloom/canvas-collab/model";
import * as api from "@/components/canvas/runtime/api/canvasDocumentBffController";
import { ApiError, unwrap } from "@/components/canvas/runtime/lib/request";
import {
  CanvasCollaborationProvider,
  type CollaborationBinding,
  type CollaborationConnectionStatus,
} from "./canvas-collaboration-provider";
import { claimCanvasJournal } from "./canvas-journal-lease";
import { fromCanvasGraph } from "./canvas-document-adapter";
import { canvasGraphFingerprint } from "./canvas-json";
import type { CanvasScope } from "../hooks/use-canvas-document";
import type {
  CanvasPendingForms,
  CanvasLocalPersistence,
} from "../types/canvas-recovery";

type Graph = CanvasContract.CanvasDocumentGraphDTO;
export type CanvasYjsRecovery = RecoveryRecord<Graph>;
type ProviderOptions = ConstructorParameters<
  typeof CanvasCollaborationProvider
>[0];
type Provider = SessionTransport &
  Pick<CanvasCollaborationProvider, "start" | "stop" | "probe" | "status">;
type Release = () => void | Promise<void>;
type Options = {
  commands?: {
    send: (
      command: ControlCommand,
    ) => Promise<CanvasContract.CanvasCommandResultVO>;
    query: (commandId: string) => Promise<CanvasContract.CanvasCommandStatusVO>;
  };
  scope: CanvasScope;
  identity: string;
  canvasId: string;
  initialDocument: CanvasContract.CanvasVO;
  journal?: Pick<
    CollaborationJournal,
    "list" | "open" | "removeRecord" | "transplant"
  >;
  claim?: (scope: Omit<CollaborationScope, "epoch">) => Promise<Release | null>;
  createProvider?: (options: ProviderOptions) => Provider;
  onAccessInvalidated?: () => void;
  onRestoreConfirmed?: () => void;
};
export type CanvasYjsState = {
  identity: string;
  generation: number;
  canvasId: string;
  status:
    | "CLEAN"
    | "DIRTY"
    | "SAVING"
    | "UNKNOWN"
    | "FAILED"
    | "CONFLICT"
    | "READ_ONLY";
  baseline: CanvasContract.CanvasVO;
  draft: Graph;
  editorGraph: Graph;
  sequence: number;
  confirmedSequence: number;
  inFlight: ControlCommand | null;
  message?: string;
  invalidDraft?: string;
  errorCode?: number;
  requestId?: string;
};
type View = {
  state: CanvasYjsState;
  connectionStatus: CollaborationConnectionStatus;
  localPersistence: CanvasLocalPersistence;
  recoveryChoices: CanvasYjsRecovery[];
  recoverySources: (RecoverySource<Graph> & { fields: RecoveryField[] })[];
  recoveryPending: boolean;
  recoveryBusy: boolean;
  recoveryRequired: boolean;
  sealedInput: boolean;
  pendingForms: CanvasPendingForms;
  pendingTextDrafts: Record<string, CanvasTextDraft>;
  textRecoveryPending: boolean;
  restoring: boolean;
  undo: { canUndo: boolean; canRedo: boolean };
};
const memoryWriter: JournalWriter<Graph> = {
  put: async () => {
    throw new Error("设备恢复记录不可用");
  },
  close: async () => {},
};
const recoveryMessages: Record<string, string> = {
  REPLAY_BUDGET: "自动同步恢复次数已用尽，请核对原提交结果",
  RECOVERY_UNPROTECTED: "设备记录未能保存，请保留此页面并核对原提交结果",
};
const errorMessage = (error: unknown) =>
  error instanceof Error
    ? (recoveryMessages[error.message] ?? error.message)
    : "画布修改待确认";

/** Owns one canvas editing session. The renderer and Tiptap share its one optimistic Y.Doc. */
export class CanvasCollaborationController {
  private readonly journal: NonNullable<Options["journal"]>;
  private readonly listeners = new Set<() => void>();
  private readonly leases = new Map<string, Release>();
  private readonly bindings = new Map<string, NodeTextBinding>();
  private readonly draftReaders = new Set<() => void>();
  private readonly draftOwners = new Map<
    () => void,
    CollaborationSession<Graph>
  >();
  private readonly archivedSessions = new Map<
    CollaborationSession<Graph>,
    string
  >();
  private readonly lateDrafts = new Map<
    CollaborationSession<Graph>,
    Map<
      string,
      {
        document: Parameters<CollaborationSession<Graph>["setTextDraft"]>[1];
        source?: Graph["nodes"][number];
      }
    >
  >();
  private readonly localDeletedNodes = new Set<string>();
  private applyingGraphHistory = false;
  private session: CollaborationSession<Graph> | null = null;
  private provider: Provider | null = null;
  private writer: JournalWriter<Graph> = memoryWriter;
  private undoManager: Y.UndoManager | null = null;
  private boundDoc: Y.Doc | null = null;
  private active = false;
  private writable = true;
  private started = false;
  private loadingRecovery: number | null = null;
  private lifecycle = 0;
  private authorizationGeneration = 0;
  private stopping: Promise<void> | null = null;
  private stable = true;
  private applyingRecovery: CanvasYjsRecovery | undefined;
  private handoff:
    | {
        record: CanvasYjsRecovery;
        base?: CanvasYjsRecovery;
        session?: CollaborationSession<Graph>;
      }
    | undefined;
  private pendingRestoreNotification: string | null = null;
  private lastRestoreNotification: string | null = null;
  private suspendedRecovery: CanvasYjsRecovery | undefined;
  private scope: Omit<CollaborationScope, "epoch"> | null = null;
  private binding: CollaborationBinding | null = null;
  private localTimer: ReturnType<typeof setTimeout> | null = null;
  private saveTimer: ReturnType<typeof setTimeout> | null = null;
  private dirtyAt: number | null = null;
  private lastLocalGeneration = -1;
  private lastConfirmed = "";
  private errors = new Map<string, string>();
  private operation: Promise<CanvasContract.CanvasCommandResultVO | null> | null =
    null;
  private resolvingDetached = false;
  private detachedAttempts = new Map<string, number>();
  private view: View;
  private retainedTextNodes = new Set<string>();

  constructor(private readonly options: Options) {
    fromCanvasGraph(options.initialDocument.graph);
    this.journal = options.journal ?? collaborationJournal;
    this.view = {
      state: {
        identity: options.identity,
        generation: 1,
        canvasId: options.canvasId,
        status: options.initialDocument.access?.edit ? "CLEAN" : "READ_ONLY",
        baseline: options.initialDocument,
        draft: options.initialDocument.graph,
        editorGraph: options.initialDocument.graph,
        sequence: 0,
        confirmedSequence: 0,
        inFlight: null,
      },
      connectionStatus: "connecting",
      localPersistence: "CHECKING",
      recoveryChoices: [],
      recoverySources: [],
      recoveryPending: options.scope === "USER",
      recoveryBusy: false,
      recoveryRequired: false,
      sealedInput: false,
      pendingForms: {},
      pendingTextDrafts: {},
      textRecoveryPending: false,
      restoring: false,
      undo: { canUndo: false, canRedo: false },
    };
  }

  getSnapshot = () => this.view;
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };
  private publish() {
    if (!this.active) return;
    const session = this.session;
    if (session) {
      for (const nodeId of this.retainedTextNodes)
        if (!session.textDrafts[nodeId]) this.retainedTextNodes.delete(nodeId);
      const retainedSources = [...this.draftOwners.values()].flatMap(
        (owner) => {
          const id = this.archivedSessions.get(owner);
          return id ? [id] : [];
        },
      );
      session.releaseResolvedSources(retainedSources);
      if (this.boundDoc !== session.doc) this.bindDocument();
      const snapshot =
          this.binding?.edit &&
          this.writable &&
          this.binding.lifecycle === "ACTIVE"
            ? session.snapshot
            : session.committedSnapshot,
        confirmed = session.confirmed;
      const graph =
        canvasGraphFingerprint(snapshot.graph) ===
        canvasGraphFingerprint(this.view.state.draft)
          ? this.view.state.draft
          : snapshot.graph;
      const watermark = `${confirmed.epoch}:${confirmed.commitSeq}`;
      let baseline = this.view.state.baseline;
      if (watermark !== this.lastConfirmed) {
        this.lastConfirmed = watermark;
        const committed = session.committedSnapshot;
        baseline = {
          ...baseline,
          title: committed.title,
          graph: committed.graph,
          revision: confirmed.contentRevision,
          graphHash: confirmed.graphHash,
          projectBindingRevision: confirmed.bindingRevision,
          epoch: confirmed.epoch,
          commitSeq: confirmed.commitSeq,
        };
      }
      const invalidDraft =
        this.errors.values().next().value ??
        (Object.keys(session.forms).length
          ? "存在未应用的参数，请应用或放弃"
          : (this.stable || this.retainedTextNodes.size > 0) &&
              Object.keys(session.textDrafts).length
            ? "有保留正文尚未应用，请核对并应用后同步"
            : undefined);
      const control = session.control;
      let status: CanvasYjsState["status"] =
        session.status === "REJECTED" ? "CONFLICT" : session.status;
      if (control)
        status =
          control.status === "REJECTED"
            ? "FAILED"
            : control.status === "SENDING"
              ? "SAVING"
              : "UNKNOWN";
      if (invalidDraft && !["UNKNOWN", "SAVING", "CONFLICT"].includes(status))
        status = "FAILED";
      if (
        !this.writable ||
        !this.binding?.edit ||
        this.binding.lifecycle !== "ACTIVE"
      )
        status = "READ_ONLY";
      this.view = {
        ...this.view,
        recoveryPending:
          this.view.recoveryChoices.length > 0 || !!session.sealed,
        recoverySources: session.recoverySources.map((source) => ({
          ...source,
          fields: recoveryFields(source, snapshot),
        })),
        state: {
          ...this.view.state,
          baseline,
          draft: graph,
          editorGraph: graph,
          sequence: session.generation,
          confirmedSequence:
            session.status === "CLEAN"
              ? session.generation
              : this.view.state.confirmedSequence,
          status,
          invalidDraft,
          inFlight: control,
        },
        pendingForms: session.forms as CanvasPendingForms,
        pendingTextDrafts: structuredClone(session.textDrafts),
        textRecoveryPending: this.retainedTextNodes.size > 0,
        recoveryRequired:
          session.status === "UNKNOWN" || control?.status === "UNKNOWN",
        sealedInput: !!session.sealed,
        localPersistence: this.lateDrafts.size
          ? "MEMORY"
          : session.protected
            ? "PERSISTED"
            : session.journalError
              ? "UNAVAILABLE"
              : "MEMORY",
        undo: {
          canUndo:
            this.stable &&
            !Object.keys(session.textDrafts).length &&
            (this.undoManager?.canUndo() ?? false),
          canRedo:
            this.stable &&
            !Object.keys(session.textDrafts).length &&
            (this.undoManager?.canRedo() ?? false),
        },
      };
      if (session.generation !== this.lastLocalGeneration) {
        this.lastLocalGeneration = session.generation;
        if (!this.localTimer)
          this.localTimer = setTimeout(() => {
            this.localTimer = null;
            void this.protect();
          }, 200);
        this.schedule();
      }
    }
    this.view = { ...this.view, state: { ...this.view.state } };
    this.listeners.forEach((listener) => listener());
  }

  private bindDocument() {
    this.undoManager?.destroy();
    this.boundDoc = this.session!.doc;
    this.bindings.clear();
    this.localDeletedNodes.clear();
    this.undoManager = canvasUndoManager(this.boundDoc);
    this.view = {
      ...this.view,
      state: { ...this.view.state, generation: this.view.state.generation + 1 },
    };
  }

  async start() {
    if (this.stopping) await this.stopping;
    if (this.started) return;
    this.retainDetachedInSuspended();
    const lifecycle = ++this.lifecycle;
    this.started = this.active = true;
    window.addEventListener("pagehide", this.protectOnPageHide);
    window.document.addEventListener("visibilitychange", this.protectOnHidden);
    const authorization = this.authorizationGeneration;
    this.loadingRecovery = lifecycle;
    try {
      await this.loadRecovery(lifecycle, authorization);
    } finally {
      if (this.loadingRecovery === lifecycle) this.loadingRecovery = null;
      if (
        this.active &&
        this.lifecycle === lifecycle &&
        (!this.writable || this.authorizationGeneration !== authorization)
      ) {
        for (const release of this.leases.values()) await release();
        this.leases.clear();
        this.started = false;
        this.scope = null;
        this.applyingRecovery = undefined;
        if (this.writable) await this.start();
      }
    }
  }

  private async loadRecovery(lifecycle: number, authorization: number) {
    const current = () =>
      this.active &&
      this.writable &&
      this.lifecycle === lifecycle &&
      this.authorizationGeneration === authorization;
    if (!current()) return;
    this.view.recoveryPending = this.options.scope === "USER";
    if (this.options.scope !== "USER") {
      this.view = {
        ...this.view,
        state: { ...this.view.state, status: "READ_ONLY" },
        connectionStatus: "disconnected",
        localPersistence: "MEMORY",
        recoveryPending: false,
      };
      this.publish();
      return;
    }
    const base = {
      origin: window.location.origin,
      actorKey: this.options.identity,
      canvasId: this.options.canvasId,
    };
    let records: CanvasYjsRecovery[] = [];
    try {
      records = await this.journal.list<Graph>(base);
    } catch {
      this.view.localPersistence = "UNAVAILABLE";
    }
    if (!current()) return;
    const suspended = this.suspendedRecovery;
    if (suspended) {
      records = records.filter(
        (record) =>
          record.scope.sessionId !== suspended.scope.sessionId ||
          record.scope.epoch !== suspended.scope.epoch,
      );
      records.push(suspended);
    }
    let available: CanvasYjsRecovery[] = [];
    for (const record of records) {
      if (this.leases.has(record.scope.sessionId)) {
        available.push(record);
        continue;
      }
      try {
        const lease = await this.claim(record.scope);
        if (!current()) {
          await lease?.();
          return;
        }
        if (lease) {
          this.leases.set(record.scope.sessionId, lease);
          available.push(record);
        }
      } catch {
        this.view.localPersistence = "UNAVAILABLE";
        // A live controller still owns its private in-memory preimage when device coordination is unavailable.
        // A busy lock returns null above and must never be bypassed.
        if (current() && record === suspended) available.push(record);
      }
    }
    if (available.some((record) => record !== suspended)) {
      try {
        const latest = await this.journal.list<Graph>(base);
        if (!current()) return;
        available = available.flatMap((record) => {
          if (record === suspended) return [record];
          const held = latest.find(
            (value) =>
              value.scope.epoch === record.scope.epoch &&
              value.scope.sessionId === record.scope.sessionId,
          );
          return held ? [held] : [];
        });
      } catch {
        this.view.localPersistence = "UNAVAILABLE";
        available = available.filter((record) => record === suspended);
      }
    }
    if (available.length === 1) {
      this.applyingRecovery = available[0];
      this.scope = available[0].scope;
    } else {
      this.scope = { ...base, sessionId: crypto.randomUUID() };
      try {
        const lease = await this.claim(this.scope);
        if (!current()) {
          await lease?.();
          return;
        }
        if (lease) this.leases.set(this.scope.sessionId, lease);
      } catch {
        this.view.localPersistence = "UNAVAILABLE";
      }
      this.view.recoveryChoices = available;
    }
    if (!current()) return;
    this.connect();
  }

  private claim(scope: Omit<CollaborationScope, "epoch">) {
    return (this.options.claim ?? claimCanvasJournal)(scope);
  }

  private connect() {
    if (!this.writable) return;
    this.provider?.stop();
    const scope = this.scope!;
    const provider: Provider = (
      this.options.createProvider ??
      ((options) => new CanvasCollaborationProvider(options))
    )({
      ...scope,
      ready: async (binding) => {
        if (!this.active || this.provider !== provider) return;
        this.binding = binding;
        if (this.session && this.session.confirmed.epoch !== binding.epoch) {
          this.draftReaders.forEach((read) => read());
          if (this.hasPending()) {
            await this.retainEpochRecord(this.session.record(), this.writer);
            this.view.state.message =
              "画布已切换版本；未同步内容已保留，请导出或逐项处理";
            this.view.recoveryPending = true;
            this.publish();
          }
          await this.replaceSession();
        }
        if (
          this.applyingRecovery &&
          this.applyingRecovery.scope.epoch !== binding.epoch
        ) {
          await this.retainEpochRecord(this.applyingRecovery);
          this.applyingRecovery = undefined;
          this.suspendedRecovery = undefined;
          this.view.state.message =
            "设备草稿来自另一画布代际，请导出后处理，不能覆盖当前版本";
        }
        if (!this.session && !this.handoff) {
          await this.writer.close();
          const writer = this.leases.has(scope.sessionId)
            ? await this.journal.open<Graph>({ ...scope, epoch: binding.epoch })
            : memoryWriter;
          if (!this.active || this.provider !== provider) {
            await writer.close();
            return;
          }
          this.writer = writer;
        }
      },
      changed: (status) => {
        if (!this.active || this.provider !== provider) return;
        this.view.connectionStatus = status;
        if (status === "expired") {
          this.writable = false;
          this.options.onAccessInvalidated?.();
        }
        this.publish();
        if (status === "connected") {
          if (
            this.view.recoveryChoices.some(
              (record) =>
                record.scope.epoch !== this.binding?.epoch &&
                (record.inFlight || record.control),
            )
          )
            void this.reconcileDetached().catch(() => {});
          else if (this.session?.control)
            void this.resolveUnknown().catch(() => {});
          else if (this.session?.status === "UNKNOWN")
            void this.resolveUnknown(true).catch(() => {});
          else this.schedule();
        }
      },
      message: async (message) => {
        if (!this.active || this.provider !== provider) return;
        if (message.type === "WATERMARK") {
          this.binding = {
            ...this.binding!,
            edit: message.edit === true,
            lifecycle: String(message.lifecycle),
          };
          this.publish();
          return;
        }
        if (
          message.modelVersion !== 1 ||
          typeof message.update !== "string" ||
          typeof message.commitSeq !== "string" ||
          typeof message.contentRevision !== "string" ||
          typeof message.graphHash !== "string"
        )
          throw new Error("画布同步状态无效");
        if (!this.session) {
          if (message.type !== "SYNC_STATE")
            throw new Error("画布缺少初始状态");
          const initial: CommittedState = {
            epoch: String(message.epoch),
            commitSeq: message.commitSeq,
            contentRevision: message.contentRevision,
            graphHash: message.graphHash,
            update: message.update,
            bindingRevision: String(message.bindingRevision),
            modelVersion: 1,
            lifecycle: String(message.lifecycle),
            edit: message.edit === true,
          };
          if (this.handoff) {
            const handoff = this.handoff;
            const recovery = archiveRecovery(
              handoff.base ?? handoff.record,
              initial,
            );
            const writer = await this.journal.transplant(
              handoff.record,
              recovery,
            );
            if (!this.active || !this.writable || this.provider !== provider) {
              await writer.close();
              return;
            }
            this.writer = writer;
            this.applyingRecovery = recovery;
            this.handoff = undefined;
            if (handoff.session) {
              this.archivedSessions.set(
                handoff.session,
                recovery.recoverySources.at(-1)!.id,
              );
            }
            this.view.recoveryChoices = this.view.recoveryChoices.filter(
              (record) =>
                record.scope.sessionId !== handoff.record.scope.sessionId ||
                record.scope.epoch !== handoff.record.scope.epoch,
            );
          }
          if (
            this.applyingRecovery &&
            (!initial.edit || initial.lifecycle !== "ACTIVE") &&
            !this.applyingRecovery.sealed
          ) {
            const blocked = structuredClone(this.applyingRecovery);
            blocked.sealed = {
              reason: "READ_ONLY",
              snapshot: blocked.snapshot,
              inFlight: blocked.inFlight,
              pending: blocked.pending,
              intents: [...blocked.inFlightIntents, ...blocked.intents],
              control: blocked.control,
            };
            this.applyingRecovery = blocked;
          }
          this.session = new CollaborationSession<Graph>({
            scope,
            initial,
            journal: this.writer,
            transport: provider,
            recovery: this.applyingRecovery,
            changed: () => {
              this.publish();
            },
          });
          this.retainedTextNodes = new Set(
            Object.keys(this.session.textDrafts),
          );
          this.applyingRecovery = undefined;
          this.suspendedRecovery = undefined;
          for (const [old, drafts] of this.lateDrafts) {
            let sourceId = this.archivedSessions.get(old);
            for (const [id, draft] of drafts) {
              if (!sourceId) {
                sourceId = this.session.retainDetachedText(
                  old.record(),
                  id,
                  draft.document,
                  draft.source,
                );
                if (sourceId) this.archivedSessions.set(old, sourceId);
              }
              if (
                sourceId &&
                this.session.retainSourceText(
                  old.scope,
                  id,
                  draft.document,
                  draft.source,
                  sourceId,
                )
              )
                drafts.delete(id);
            }
            if (!drafts.size) this.lateDrafts.delete(old);
          }
          this.bindDocument();
        } else {
          this.draftReaders.forEach((read) => read());
          await this.session.receive(message as CommittedUpdate);
        }
        this.view.recoveryPending = this.view.recoveryChoices.length > 0;
        this.publish();
        if (this.pendingRestoreNotification && this.session) {
          this.notifyRestore(this.pendingRestoreNotification);
          this.pendingRestoreNotification = null;
        }
      },
    });
    this.provider = provider;
    provider.start();
  }

  private schedule() {
    if (this.saveTimer) clearTimeout(this.saveTimer);
    this.saveTimer = null;
    if (
      !this.active ||
      !this.writable ||
      !this.stable ||
      this.view.recoveryPending ||
      this.view.recoveryBusy ||
      this.view.restoring ||
      this.provider?.status !== "connected" ||
      this.session?.status !== "DIRTY" ||
      Object.keys(this.session.forms).length ||
      Object.keys(this.session.textDrafts).length ||
      this.errors.size ||
      this.session.control
    )
      return;
    this.dirtyAt ??= Date.now();
    this.saveTimer = setTimeout(
      () => {
        this.saveTimer = null;
        void this.saveNow(true).catch(() => {});
      },
      Math.max(0, Math.min(1000, 5000 - (Date.now() - this.dirtyAt))),
    );
  }

  updateDraft(graph: Graph, stable = true, before = this.view.state.draft) {
    if (
      !this.session ||
      !this.canEdit() ||
      this.view.recoveryPending ||
      this.view.recoveryBusy ||
      this.view.restoring
    )
      return;
    this.stable = stable;
    this.errors.delete("conversion");
    fromCanvasGraph(graph);
    const nextIds = new Set(graph.nodes.map((node) => node.id));
    const aliveIds = new Set(
      this.session.snapshot.graph.nodes.map((node) => node.id),
    );
    const removed = before.nodes.filter(
      (node) => !nextIds.has(node.id) && aliveIds.has(node.id),
    );
    this.session.change(
      { title: this.session.snapshot.title, graph },
      { title: this.session.snapshot.title, graph: before },
    );
    for (const node of removed) {
      this.session.setTextDraft(node.id, null);
      this.localDeletedNodes.add(node.id);
    }
    if (stable) this.undoManager?.stopCapturing();
    this.publish();
    this.schedule();
  }

  textBinding = (nodeId: string) => {
    if (
      !this.session ||
      !this.binding?.edit ||
      !this.writable ||
      this.binding.lifecycle !== "ACTIVE"
    )
      return undefined;
    const node = this.session.doc
      .getMap<Y.Map<unknown>>("canvas")
      .get("nodes")
      ?.get(nodeId) as Y.Map<unknown> | undefined;
    const type = (node?.get("data") as Y.Map<unknown> | undefined)?.get("type");
    if (!node || type === "group" || type === "config") return undefined;
    let binding = this.bindings.get(nodeId);
    if (binding?.isAttached?.() === false) {
      this.bindings.delete(nodeId);
      binding = undefined;
    }
    if (!binding) {
      const session = this.session;
      const shared = nodeTextBinding(session.doc, nodeId);
      const source = session.snapshot.graph.nodes.find(
        (value) => value.id === nodeId,
      );
      let lastDetachedDraft: string | undefined;
      const isAttached = () =>
        !session.closed &&
        session.doc
          .getMap<Y.Map<unknown>>("canvas")
          .get("nodes")
          ?.get(nodeId) === node;
      const protectDraft = (
        document: Parameters<CollaborationSession<Graph>["setTextDraft"]>[1],
      ) => {
        if (session.closed) {
          if (!document) return;
          const fingerprint = JSON.stringify(document);
          if (fingerprint === lastDetachedDraft) return;
          const sourceId = this.archivedSessions.get(session);
          if (
            sourceId &&
            this.session?.retainSourceText(
              session.scope,
              nodeId,
              document,
              source,
              sourceId,
            )
          ) {
            lastDetachedDraft = fingerprint;
            return;
          }
          const detachedId = this.session?.retainDetachedText(
            session.record(),
            nodeId,
            document,
            source,
          );
          if (detachedId) {
            this.archivedSessions.set(session, detachedId);
            lastDetachedDraft = fingerprint;
            return;
          }
          if (document) {
            const drafts = this.lateDrafts.get(session) ?? new Map();
            drafts.set(nodeId, { document: structuredClone(document), source });
            this.lateDrafts.set(session, drafts);
            lastDetachedDraft = fingerprint;
            this.view.localPersistence = "MEMORY";
            this.publish();
          }
          return;
        }
        if (
          !isAttached() &&
          (this.applyingGraphHistory || this.localDeletedNodes.has(nodeId))
        )
          return;
        if (!document && session.textDrafts[nodeId]) {
          const current = session.snapshot.graph.nodes.find(
            (value) => value.id === nodeId,
          )?.textDocument;
          // A newly mounted editor reads the shared body, not an older retained DOM draft.
          if (
            canvasGraphFingerprint(current ?? null) !==
            canvasGraphFingerprint(session.textDrafts[nodeId])
          )
            return;
        }
        session.setTextDraft(nodeId, document, source);
      };
      binding = {
        ...shared,
        isAttached,
        protectDraft,
        markChanged: () => {
          if (!isAttached()) return false;
          shared.markChanged();
          return true;
        },
        registerDraftReader: (reader) => {
          const capture = () => protectDraft(reader());
          this.draftReaders.add(capture);
          this.draftOwners.set(capture, session);
          return () => {
            capture();
            if (session.textDrafts[nodeId]) this.retainedTextNodes.add(nodeId);
            this.draftReaders.delete(capture);
            this.draftOwners.delete(capture);
            this.publish();
          };
        },
      };
      this.bindings.set(nodeId, binding);
    }
    return binding;
  };
  undo = () => this.applyHistory("undo");
  redo = () => this.applyHistory("redo");
  private applyHistory(direction: "undo" | "redo") {
    if (!this.canEdit() || !this.session || !this.undoManager || !this.stable)
      return;
    this.draftReaders.forEach((read) => read());
    if (Object.keys(this.session.textDrafts).length) return;
    const before = this.session.snapshot;
    this.applyingGraphHistory = true;
    try {
      const restored = applyCanvasHistory(
        this.session.doc,
        this.undoManager,
        direction,
        this.session.confirmed,
      );
      restored.nodes.forEach((id) => this.localDeletedNodes.delete(id));
      const alive = new Set(
        this.session.snapshot.graph.nodes.map((node) => node.id),
      );
      before.graph.nodes.forEach((node) => {
        if (!alive.has(node.id)) this.localDeletedNodes.add(node.id);
      });
    } finally {
      this.applyingGraphHistory = false;
      this.publish();
    }
  }
  private canEdit() {
    return (
      this.active &&
      this.writable &&
      !!this.binding?.edit &&
      this.binding.lifecycle === "ACTIVE" &&
      !this.view.recoveryPending &&
      !this.view.recoveryBusy &&
      !this.view.restoring &&
      !this.session?.sealed &&
      !this.session?.control
    );
  }
  setStable(value: boolean) {
    this.stable = value;
    this.schedule();
  }
  setPendingForm(nodeId: string, form: CanvasPendingForms[string] | undefined) {
    if (!this.session || !this.writable) return;
    const forms = { ...this.session.forms };
    if (form) forms[nodeId] = form;
    else delete forms[nodeId];
    this.session.setForms(forms);
  }
  draftError(message: string, key = "conversion") {
    this.errors.set(key, message);
    this.publish();
  }
  clearFormError(key: string) {
    this.errors.delete(key);
    this.publish();
    this.schedule();
  }
  async applyTextDraft(
    nodeId: string,
    reviewedDraft: CanvasTextDraft,
    reviewedCurrent:
      CanvasContract.CanvasDocumentNodeDTO["textDocument"] | null,
  ) {
    if (!this.session || !this.canEdit() || !this.stable)
      throw new Error("请先结束输入并恢复画布编辑资格");
    this.draftReaders.forEach((read) => read());
    const session = this.session;
    if (!session || !this.canEdit() || !this.stable)
      throw new Error("画布状态已变化，请重新核对保留正文");
    const before = session.snapshot;
    const node = before.graph.nodes.find((value) => value.id === nodeId);
    if (!node) throw new Error("节点已不存在，请通过逐项恢复保留正文");
    if (
      canvasGraphFingerprint(session.textDrafts[nodeId] ?? null) !==
        canvasGraphFingerprint(reviewedDraft) ||
      canvasGraphFingerprint(node.textDocument ?? null) !==
        canvasGraphFingerprint(reviewedCurrent)
    )
      throw new Error("保留正文或当前正文已变化，请重新核对");
    const after = structuredClone(before);
    after.graph.nodes.find((value) => value.id === nodeId)!.textDocument =
      structuredClone(reviewedDraft);
    this.undoManager?.stopCapturing();
    applyRecoveredCanvasChanges(session.doc, before, after);
    session.setTextDraft(nodeId, null);
    this.undoManager?.stopCapturing();
    this.publish();
    await session.protect();
    this.schedule();
    if (!Object.keys(session.textDrafts).length) await this.saveNow();
  }
  private async protect() {
    if (this.session) {
      await this.session.protect();
      this.publish();
    }
  }

  private protectOnPageHide = () => {
    if (!this.active) return;
    this.draftReaders.forEach((read) => read());
    void this.protect();
  };

  private protectOnHidden = () => {
    if (window.document.visibilityState === "hidden") this.protectOnPageHide();
  };

  async saveNow(automatic = false) {
    this.draftReaders.forEach((read) => read());
    const session = this.session;
    if (
      !session ||
      !this.canEdit() ||
      !this.stable ||
      this.errors.size ||
      Object.keys(session.forms).length ||
      Object.keys(session.textDrafts).length
    )
      throw new Error("请先处理未应用或未确认输入，再保存画布");
    try {
      const receipt = await session.flush({ automatic });
      if (session.status === "CLEAN") this.dirtyAt = null;
      return receipt;
    } catch (error) {
      this.view.state.message = errorMessage(error);
      if (error instanceof ApiError) {
        this.view.state.errorCode = error.code;
        this.view.state.requestId = error.requestId;
      }
      throw error;
    } finally {
      this.publish();
      this.schedule();
    }
  }

  async prepareGeneration() {
    this.draftReaders.forEach((read) => read());
    if (!this.session || !this.canEdit() || !this.stable)
      throw new Error("请先确认画布已保存");
    while (this.session.status === "DIRTY" || this.session.status === "SAVING")
      await this.saveNow();
    if (
      this.session.status !== "CLEAN" ||
      this.errors.size ||
      Object.keys(this.session.forms).length
    )
      throw new Error("请先处理未确认的画布输入");
    return {
      ...this.view.state.baseline,
      epoch: this.session.confirmed.epoch,
      commitSeq: this.session.confirmed.commitSeq,
    };
  }

  async runCommand(
    operation: "EDIT" | "DELETE" | "CHECKPOINT",
    extra: { title?: string } = {},
  ) {
    if (operation === "EDIT") {
      if (!this.canEdit() || !this.session || !extra.title?.trim()) return null;
      this.session.change({
        ...this.session.snapshot,
        title: extra.title.trim(),
      });
      const receipt = await this.saveNow();
      return receipt
        ? ({
            canvasId: this.options.canvasId,
            revision: receipt.contentRevision,
          } as CanvasContract.CanvasCommandResultVO)
        : null;
    }
    return this.control(operation, extra);
  }

  async restoreSnapshot(snapshotId: string) {
    if (this.view.restoring) return null;
    await this.prepareGeneration();
    this.view.restoring = true;
    this.publish();
    try {
      return await this.control("RESTORE", { snapshotId });
    } finally {
      this.view.restoring = false;
      this.publish();
    }
  }

  private async control(
    operation: ControlCommand["operation"],
    extra: Record<string, unknown>,
  ) {
    if (
      this.operation ||
      !this.session ||
      !this.writable ||
      !this.binding?.edit ||
      this.binding.lifecycle !== "ACTIVE" ||
      this.view.recoveryPending
    )
      return null;
    if (
      !this.stable ||
      this.session.status !== "CLEAN" ||
      Object.keys(this.session.forms).length ||
      this.errors.size
    )
      throw new Error("请先保存画布并处理未应用的输入");
    const confirmed = this.session.confirmed;
    const body = {
      ...(operation === "DELETE"
        ? { id: this.options.canvasId }
        : { canvasId: this.options.canvasId }),
      commandId: crypto.randomUUID(),
      expectedRevision: confirmed.contentRevision,
      expectedProjectBindingRevision: confirmed.bindingRevision,
      expectedEpoch: confirmed.epoch,
      expectedCommitSeq: confirmed.commitSeq,
      ...extra,
    };
    this.session.setControl({ operation, status: "SENDING", body });
    await this.protect();
    return this.writeControl(false);
  }

  private async writeControl(
    query: boolean,
  ): Promise<CanvasContract.CanvasCommandResultVO | null> {
    if (this.operation) return this.operation;
    const session = this.session,
      command = session?.control;
    if (!session || !command || !this.active || !this.writable) return null;
    if (!query && (!this.binding?.edit || this.binding.lifecycle !== "ACTIVE"))
      throw new Error("请先恢复画布编辑资格");
    const authorization = this.authorizationGeneration;
    const promise = (async () => {
      try {
        let result: CanvasContract.CanvasCommandResultVO;
        if (command.receipt) {
          result = command.receipt as CanvasContract.CanvasCommandResultVO;
        } else if (query) {
          const status = this.options.commands
            ? await this.options.commands.query(command.body.commandId)
            : unwrap(
                await api.getCanvasCommand({
                  commandId: command.body.commandId,
                }),
              );
          if (status.status !== "COMPLETED" || !status.result)
            throw new Error("原命令结果尚未确认");
          result = status.result;
        } else {
          const send =
            command.operation === "RESTORE"
              ? api.restoreCanvasGraph
              : command.operation === "DELETE"
                ? api.deleteCanvas
                : api.checkpointCanvasGraph;
          result = this.options.commands
            ? await this.options.commands.send(command)
            : unwrap(await send(command.body as never));
        }
        if (
          !this.active ||
          !this.writable ||
          this.session !== session ||
          this.authorizationGeneration !== authorization
        )
          return null;
        if (
          String(result.canvasId) !== this.options.canvasId ||
          result.commandId !== command.body.commandId
        )
          throw new Error("画布命令回执无效");
        if (command.operation === "RESTORE") {
          session.setControl({
            ...command,
            status: "UNKNOWN",
            receipt: result as unknown as Record<string, unknown>,
          });
          this.draftReaders.forEach((read) => read());
          const record = await session.persist();
          if (
            !this.active ||
            !this.writable ||
            this.session !== session ||
            this.authorizationGeneration !== authorization
          )
            return null;
          const base = structuredClone(record);
          base.control = null;
          base.sealed = {
            reason: "RESTORE_CONFIRMED",
            snapshot: base.snapshot,
            inFlight: null,
            pending: base.pending,
            intents: base.intents,
            control: record.control,
            controlOutcome: "COMMITTED",
            controlReceipt: result as unknown as Record<string, unknown>,
          };
          this.provider?.stop();
          await this.replaceSession();
          if (
            !this.active ||
            !this.writable ||
            this.authorizationGeneration !== authorization
          )
            return null;
          this.handoff = { record, base, session };
          this.pendingRestoreNotification = command.body.commandId;
          this.view.recoveryPending = true;
          this.connect();
        } else {
          session.setControl(null);
          await this.protect();
          if (command.operation === "DELETE") {
            this.writable = false;
            this.provider?.stop();
          }
        }
        return result;
      } catch (error) {
        if (this.active && this.session === session) {
          const definitive =
            !query &&
            error instanceof ApiError &&
            [
              40000, 40100, 40300, 40301, 40400, 40900, 40901, 41300, 42200,
            ].includes(error.code);
          session.setControl({
            ...(session.control ?? command),
            status:
              definitive || command.status === "REJECTED"
                ? "REJECTED"
                : "UNKNOWN",
          });
          this.view.state.message = errorMessage(error);
          await this.protect();
        }
        throw error;
      } finally {
        this.operation = null;
        this.publish();
      }
    })();
    this.operation = promise;
    return promise;
  }

  async resolveUnknown(automatic = false) {
    if (
      this.view.recoveryChoices.some(
        (record) => record.inFlight || record.control,
      )
    ) {
      this.detachedAttempts.clear();
      return this.reconcileDetached();
    }
    if (this.session?.control) return this.writeControl(true);
    if (this.session?.status === "UNKNOWN") {
      try {
        return await this.session.flush({
          automatic,
          queryOnly: !this.binding?.edit || this.binding.lifecycle !== "ACTIVE",
        });
      } finally {
        this.publish();
        this.schedule();
      }
    }
    return this.saveNow();
  }
  async retryOriginal() {
    const command = this.session?.control;
    if (!command) return this.saveNow();
    try {
      return await this.writeControl(true);
    } catch (error) {
      if (!(error instanceof ApiError) || error.code !== 40400) throw error;
    }
    return this.writeControl(false);
  }

  async resumeAfterFix() {
    const session = this.session,
      control = session?.control;
    if (control?.status === "REJECTED") {
      try {
        const status = this.options.commands
          ? await this.options.commands.query(control.body.commandId)
          : unwrap(
              await api.getCanvasCommand({ commandId: control.body.commandId }),
            );
        if (status.status === "COMPLETED") return this.writeControl(true);
        throw new Error("原命令结果尚未确认");
      } catch (error) {
        if (!(error instanceof ApiError) || error.code !== 40400) throw error;
        if (this.session === session && this.active) {
          session!.setControl(null);
          await this.protect();
        }
      }
    }
    this.clearFormError("conversion");
  }

  private async retainEpochRecord(
    input: CanvasYjsRecovery,
    currentWriter?: JournalWriter<Graph>,
  ) {
    const record = structuredClone(input);
    record.sealed ??= {
      reason: "EPOCH_CHANGED",
      snapshot: record.snapshot,
      inFlight: record.inFlight,
      pending: record.pending,
      control: record.control,
    };
    record.generation++;
    record.updatedAt = Date.now();
    const writer =
      currentWriter ?? (await this.journal.open<Graph>(record.scope));
    try {
      if (!this.active) throw new Error("编辑会话已关闭");
      await writer.put(record);
    } catch (error) {
      this.view.localPersistence = "UNAVAILABLE";
      throw error;
    } finally {
      if (!currentWriter) await writer.close();
    }
    this.view.recoveryChoices = [
      ...this.view.recoveryChoices.filter(
        (value) =>
          value.scope.sessionId !== record.scope.sessionId ||
          value.scope.epoch !== record.scope.epoch,
      ),
      record,
    ];
  }

  private async reconcileDetached() {
    if (this.resolvingDetached || !this.provider || !this.active) return;
    this.resolvingDetached = true;
    const provider = this.provider,
      lifecycle = this.lifecycle,
      authorization = this.authorizationGeneration;
    const current = () =>
      this.active &&
      this.writable &&
      this.lifecycle === lifecycle &&
      this.provider === provider &&
      this.authorizationGeneration === authorization;
    try {
      for (const input of this.view.recoveryChoices) {
        if (!current()) return;
        if (
          input.scope.epoch === this.binding?.epoch ||
          (!input.inFlight && !input.control)
        )
          continue;
        const id = `${input.scope.epoch}:${input.scope.sessionId}`;
        const attempts = this.detachedAttempts.get(id) ?? 0;
        if (attempts >= 3) continue;
        this.detachedAttempts.set(id, attempts + 1);
        const record = structuredClone(input);
        let restoredCommand: string | undefined;
        if (!record.sealed) throw new Error("设备稿尚未封存");
        if (record.inFlight) {
          const original = record.inFlight,
            receipt =
              record.acceptedReceipt ?? (await provider.query(original));
          if (!current()) return;
          if (
            receipt &&
            (receipt.updateId !== original.updateId ||
              receipt.epoch !== original.epoch ||
              receipt.payloadHash !== original.payloadHash)
          )
            throw new Error("原同步结果无效");
          record.sealed.originalOutcome = receipt ? "COMMITTED" : "NOT_FOUND";
          if (receipt) record.sealed.originalReceipt = receipt;
          record.inFlight = null;
          record.acceptedReceipt = null;
          record.pending = [];
        }
        if (record.control) {
          try {
            const status = record.control.receipt
              ? {
                  status: "COMPLETED",
                  result: record.control
                    .receipt as CanvasContract.CanvasCommandResultVO,
                }
              : this.options.commands
                ? await this.options.commands.query(
                    record.control.body.commandId,
                  )
                : unwrap(
                    await api.getCanvasCommand({
                      commandId: record.control.body.commandId,
                    }),
                  );
            if (
              status.status !== "COMPLETED" ||
              !status.result ||
              String(status.result.canvasId) !== this.options.canvasId ||
              status.result.commandId !== record.control.body.commandId
            )
              throw new Error("原命令结果尚未确认");
            record.sealed.controlOutcome = "COMMITTED";
            record.sealed.controlReceipt = status.result as unknown as Record<
              string,
              unknown
            >;
            if (record.control.operation === "RESTORE")
              restoredCommand = record.control.body.commandId;
          } catch (error) {
            if (!(error instanceof ApiError) || error.code !== 40400)
              throw error;
            record.sealed.controlOutcome = "NOT_FOUND";
          }
          record.control = null;
        }
        if (!current()) return;
        const writer = await this.journal.open<Graph>(record.scope);
        try {
          if (!current()) return;
          record.generation++;
          record.updatedAt = Date.now();
          await writer.put(record);
        } finally {
          await writer.close();
        }
        if (!current()) return;
        this.view.recoveryChoices = this.view.recoveryChoices.map((value) =>
          value === input ? record : value,
        );
        if (restoredCommand && this.session)
          this.notifyRestore(restoredCommand);
      }
    } finally {
      this.resolvingDetached = false;
      this.publish();
    }
  }

  async chooseRecovery(record: CanvasYjsRecovery) {
    if (!this.active || !this.writable || this.view.recoveryBusy) return;
    const source = this.view.recoveryChoices.find(
      (value) =>
        value.scope.sessionId === record.scope.sessionId &&
        value.scope.epoch === record.scope.epoch,
    );
    if (!source) return;
    record = source;
    if (record.scope.epoch !== this.binding?.epoch)
      throw new Error("这份草稿来自另一代际，请先导出，不能直接覆盖当前画布");
    this.draftReaders.forEach((read) => read());
    if (this.hasPending()) throw new Error("请先处理当前编辑会话的未确认输入");
    const lifecycle = this.lifecycle,
      authorization = this.authorizationGeneration;
    const current = () =>
      this.active &&
      this.writable &&
      this.lifecycle === lifecycle &&
      this.authorizationGeneration === authorization;
    this.view.recoveryBusy = true;
    this.publish();
    this.provider?.stop();
    try {
      await this.replaceSession();
      if (!current()) return;
      this.scope = record.scope;
      this.applyingRecovery = record;
      this.view.recoveryChoices = this.view.recoveryChoices.filter(
        (value) => value !== source,
      );
      this.view.recoveryPending = true;
      this.connect();
    } finally {
      this.view.recoveryBusy = false;
      if (
        !current() &&
        this.active &&
        this.writable &&
        this.started &&
        this.scope
      )
        this.connect();
      this.publish();
    }
  }

  async beginFieldRecovery(input?: CanvasYjsRecovery) {
    if (!this.active || !this.writable || this.view.recoveryBusy) return;
    if (!this.binding?.edit || this.binding.lifecycle !== "ACTIVE")
      throw new Error("请先恢复画布编辑资格");
    const old = input ? undefined : (this.session ?? undefined);
    if (!input && !old?.sealed) throw new Error("当前没有需要逐项处理的输入");
    this.draftReaders.forEach((read) => read());
    if (input && this.hasPending())
      throw new Error("请先处理当前编辑会话的未确认输入");
    let record = input
      ? this.view.recoveryChoices.find(
          (value) =>
            value.scope.epoch === input.scope.epoch &&
            value.scope.sessionId === input.scope.sessionId,
        )
      : old?.record();
    if (!record) throw new Error("设备稿已变化，请重新查看");
    if (record.inFlight || record.control)
      throw new Error("请先核对原提交结果");
    if (!this.leases.has(record.scope.sessionId))
      throw new Error("该草稿仍由另一页面编辑");
    const lifecycle = this.lifecycle,
      authorization = this.authorizationGeneration;
    this.view.recoveryBusy = true;
    this.publish();
    try {
      this.provider?.stop();
      if (old) {
        this.draftReaders.forEach((read) => read());
        record = await old.persist();
        if (old.generation !== record.generation) record = await old.persist();
        if (old.generation !== record.generation)
          throw new Error("输入仍在变化，请完成输入后重试");
      }
      await this.replaceSession();
      if (
        !this.active ||
        !this.writable ||
        this.lifecycle !== lifecycle ||
        this.authorizationGeneration !== authorization
      )
        return;
      this.scope = record.scope;
      this.handoff = { record, session: old };
      this.applyingRecovery = undefined;
      this.view.recoveryPending = true;
      this.connect();
    } finally {
      this.view.recoveryBusy = false;
      this.publish();
    }
  }

  async applyRecoveryFields(sourceId: string, selected: RecoveryField[]) {
    if (!this.binding?.edit || this.binding.lifecycle !== "ACTIVE")
      throw new Error("请先恢复画布编辑资格");
    if (
      !this.active ||
      !this.writable ||
      this.view.recoveryBusy ||
      !this.session
    )
      return;
    const session = this.session;
    this.view.recoveryBusy = true;
    this.publish();
    try {
      await session.applyRecovery(sourceId, selected);
      if (!this.active || !this.writable || this.session !== session) return;
      if (session.status === "DIRTY") await session.flush();
    } finally {
      this.view.recoveryBusy = false;
      this.publish();
      this.schedule();
    }
  }

  async discardRecoverySource(sourceId: string) {
    if (
      !this.active ||
      !this.writable ||
      this.view.recoveryBusy ||
      !this.session
    )
      return;
    await this.session.discardRecoverySource(sourceId);
    this.publish();
  }

  async discardRecovery(record: CanvasYjsRecovery) {
    if (!this.active || !this.writable || this.view.recoveryBusy) return;
    const source = this.view.recoveryChoices.find(
      (value) =>
        value.scope.sessionId === record.scope.sessionId &&
        value.scope.epoch === record.scope.epoch,
    );
    if (!source) return;
    record = source;
    if (!this.leases.has(record.scope.sessionId))
      throw new Error("该草稿仍由另一页面编辑");
    if (record.inFlight || record.control)
      throw new Error("请先核对原提交结果，再放弃设备稿");
    const lifecycle = this.lifecycle,
      authorization = this.authorizationGeneration;
    this.view.recoveryBusy = true;
    this.publish();
    try {
      await this.journal.removeRecord(record.scope);
      if (
        !this.active ||
        this.lifecycle !== lifecycle ||
        this.authorizationGeneration !== authorization
      )
        return;
      this.view.recoveryChoices = this.view.recoveryChoices.filter(
        (value) => value !== source,
      );
      this.view.recoveryPending =
        this.view.recoveryChoices.length > 0 || !!this.session?.sealed;
    } finally {
      this.view.recoveryBusy = false;
      this.publish();
    }
  }

  exportDraft() {
    return {
      session:
        this.session?.record() ??
        this.handoff?.record ??
        this.suspendedRecovery ??
        null,
      detachedDrafts: [...this.lateDrafts].map(([session, drafts]) => ({
        scope: session.scope,
        textDrafts: Object.fromEntries(
          [...drafts].map(([id, draft]) => [id, draft.document]),
        ),
        nodes: [...drafts.values()].flatMap((draft) =>
          draft.source ? [draft.source] : [],
        ),
      })),
      snapshot: {
        title: this.view.state.baseline.title,
        graph: this.view.state.draft,
      },
      recoveryChoices: this.view.recoveryChoices,
    };
  }
  hasPending() {
    return (
      !!this.handoff ||
      (!this.session && !!this.suspendedRecovery) ||
      this.lateDrafts.size > 0 ||
      (!!this.session &&
        (this.session.status !== "CLEAN" ||
          !!this.session.control ||
          !!this.session.sealed ||
          this.session.recoverySources.some(
            (source) =>
              recoveryFields(source, this.session!.snapshot).length > 0,
          ) ||
          !!Object.keys(this.session.forms).length ||
          !!Object.keys(this.session.textDrafts).length))
    );
  }
  setEnabled(enabled: boolean) {
    if (this.writable !== enabled) this.authorizationGeneration++;
    this.writable = enabled;
    if (!enabled) {
      if (this.session?.control?.status === "SENDING") {
        this.session.setControl({ ...this.session.control, status: "UNKNOWN" });
      }
      this.provider?.stop();
      void this.protect();
    } else if (this.active && !this.started && this.loadingRecovery === null) {
      void this.start().catch((error) => {
        this.view.state.message = errorMessage(error);
        this.publish();
      });
    } else if (
      this.started &&
      this.scope &&
      this.loadingRecovery === null &&
      !this.view.recoveryBusy
    ) {
      if (this.provider) this.provider.start();
      else this.connect();
    }
    this.publish();
  }
  probe = () => this.provider?.probe();
  async discardCurrent() {
    if (!this.session || this.session.inFlight || this.session.control)
      throw new Error("请先查询原提交结果，再放弃未同步内容");
    const record = this.session.record();
    await this.journal.removeRecord(record.scope);
    await this.replaceSession();
    this.errors.clear();
    this.view.recoveryChoices = this.view.recoveryChoices.filter(
      (value) => value.scope.sessionId !== record.scope.sessionId,
    );
    this.scope = { ...record.scope, sessionId: crypto.randomUUID() };
    const lease = await this.claim(this.scope);
    if (lease) this.leases.set(this.scope.sessionId, lease);
    this.view.recoveryPending = true;
    this.connect();
    this.publish();
  }
  private async replaceSession() {
    this.undoManager?.destroy();
    this.undoManager = null;
    this.session?.destroy();
    this.session = null;
    this.boundDoc = null;
    this.bindings.clear();
    this.lastConfirmed = "";
    this.lastLocalGeneration = -1;
    await this.writer.close();
    this.writer = memoryWriter;
  }
  private notifyRestore(commandId: string) {
    if (this.lastRestoreNotification === commandId) return;
    this.lastRestoreNotification = commandId;
    this.options.onRestoreConfirmed?.();
  }

  private retainDetachedInSuspended() {
    const record = this.suspendedRecovery;
    if (!record) return;
    for (const [old, drafts] of this.lateDrafts) {
      const sourceId = this.archivedSessions.get(old);
      const target = sourceId
        ? record.recoverySources.find((source) => source.id === sourceId)
        : ["origin", "actorKey", "canvasId", "epoch", "sessionId"].every(
              (key) =>
                old.scope[key as keyof CollaborationScope] ===
                record.scope[key as keyof CollaborationScope],
            )
          ? record
          : undefined;
      if (!target) continue;
      for (const [id, draft] of drafts) {
        if (!draft.document) continue;
        target.textDrafts[id] = structuredClone(draft.document);
        record.generation++;
        if ("textGenerations" in target)
          target.textGenerations[id] = record.generation;
        const readable = target.sealed?.snapshot ?? record.snapshot;
        if (
          draft.source &&
          !readable.graph.nodes.some((node) => node.id === id)
        )
          readable.graph.nodes.push(structuredClone(draft.source));
      }
      record.updatedAt = Date.now();
      this.lateDrafts.delete(old);
      this.view.localPersistence = "MEMORY";
    }
  }
  async stop() {
    if (this.stopping) return this.stopping;
    this.draftReaders.forEach((read) => read());
    this.active = false;
    this.started = false;
    window.removeEventListener("pagehide", this.protectOnPageHide);
    window.document.removeEventListener(
      "visibilitychange",
      this.protectOnHidden,
    );
    ++this.lifecycle;
    if (this.localTimer) clearTimeout(this.localTimer);
    this.localTimer = null;
    if (this.saveTimer) clearTimeout(this.saveTimer);
    this.provider?.stop();
    const stopping = (async () => {
      if (this.handoff) {
        this.suspendedRecovery = structuredClone(this.handoff.record);
        this.handoff = undefined;
      }
      if (this.session) {
        await this.session.protect();
        if (this.hasPending()) this.suspendedRecovery = this.session.record();
      }
      await this.replaceSession();
      this.retainDetachedInSuspended();
      for (const release of this.leases.values()) await release();
      this.leases.clear();
    })();
    this.stopping = stopping;
    try {
      await stopping;
    } finally {
      if (this.stopping === stopping) this.stopping = null;
    }
  }
}
