import type { CanvasNodeData } from "./types/canvas";
import {
  openCanvasPendingJournal,
  type CanvasPendingJournal,
  type CanvasPendingJournalFactory,
} from "./lib/canvas-yjs-pending-journal";
import { reconcileTimelineConnections } from "./lib/canvas-timeline-connections";
import {
  CollaborationSession,
  type CommittedState,
  type DurableReceipt,
  type FrozenUpdate,
  type RecoveryRecord,
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
import { DOMParser as PromptDOMParser } from "@tiptap/pm/model";
import type { ComposerSession } from "@/components/generation/composer-session";
import { RequestError } from "@/lib/request";
import { canvasGraphFingerprint } from "./lib/canvas-json";
import { directorFingerprint } from "./director/director-model";
import { validateDirectorDocument } from "./director/director-workspaces";
import { readTimeline, type Timeline } from "./timeline/timeline-model";
import { serviceDocumentFromEditor, serviceGraph } from "./service-graph";
import type { CanvasEditorState } from "./lib/canvas-document-adapter";
import { publishCanvasMedia, type CanvasMediaReceipt } from "./publish-media";
import {
  validatePromptDraft,
  emptyPromptDraft,
  type PromptDraft,
} from "@/components/generation/prompt-document";

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
export type RetainedCanvasInput = {
  draft: PromptDraft;
  baseline: PromptDraft | null;
  reason: string;
};
export type YjsView = {
  localPersistence:
    "CHECKING" | "PENDING" | "PROTECTED" | "UNAVAILABLE" | "MEMORY";
  recoveryRemaining: number;
  inputDrafts: Record<string, RetainedCanvasInput>;
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
// Display ordinals and mention labels are allocated by the authority on accept.
// Compare author-owned content so that this bookkeeping does not block undo.
function historyNodeFingerprint(node: ServiceGraph["nodes"][number]) {
  const value = structuredClone(node) as Record<string, unknown>;
  delete value.referenceLabels;
  delete value.labelState;
  const normalize = (part: {
    type?: string;
    attrs?: Record<string, unknown>;
    content?: unknown[];
  }) => {
    if (part.type === "mention" && part.attrs?.id) {
      delete part.attrs.label;
      delete part.attrs.token;
    }
    part.content?.forEach((child) =>
      normalize(child as Parameters<typeof normalize>[0]),
    );
  };
  if (node.textDocument)
    normalize((value.textDocument as typeof node.textDocument).content);
  return canvasGraphFingerprint(value);
}
const inputFingerprint = (draft: API.CanvasDraft | undefined) =>
  canvasGraphFingerprint(
    draft
      ? {
          composer: draft.composer ?? null,
          prompt: draft.prompt ?? "",
          negative_prompt: draft.negative_prompt ?? "",
          aspect_ratio: draft.aspect_ratio ?? "",
          candidate_count: draft.candidate_count ?? 0,
        }
      : null,
  );
const timelineFingerprint = (value: API.TimelineConfig | undefined) => {
  if (!value) return canvasGraphFingerprint(null);
  return canvasGraphFingerprint({
    ...value,
    imported_assets: [
      ...new Set([
        ...(value.imported_assets ?? []),
        ...(value.clips ?? []).flatMap((clip) =>
          clip.asset_id ? [clip.asset_id] : [],
        ),
      ]),
    ],
    clips: value.clips?.map((clip) => ({
      ...clip,
      asset_id: clip.asset_id ?? null,
      node_id: clip.node_id ?? null,
      source_duration_ms: clip.source_duration_ms ?? null,
      crop: clip.crop ?? null,
    })),
  });
};
const composerFingerprint = (draft: unknown) =>
  canvasGraphFingerprint(draft ?? null);
const readableInput = (composer: ComposerSession) => {
  const draft = composer.snapshot().draft;
  if (!composer.composing || composer.editor.isDestroyed) return draft;
  try {
    // IME can update the visible DOM before ProseMirror emits its transaction.
    draft.document = PromptDOMParser.fromSchema(composer.editor.schema)
      .parse(composer.editor.view.dom, { preserveWhitespace: "full" })
      .toJSON();
    validatePromptDraft(draft);
    return draft;
  } catch {
    return composer.snapshot().draft;
  }
};
const localSource = (node: CanvasEditorState["nodes"][number]) =>
  ["image", "video", "audio"].includes(node.type) &&
  /^(blob:|data:)/.test(node.metadata?.content ?? "")
    ? node.metadata!.content
    : undefined;

/** One page-memory Y.Doc queue and one socket. Heartbeats resolve immutable originals. */
export class CanvasYjsLiveSession {
  clientID = crypto.randomUUID();
  private journal: CanvasPendingJournal | null = null;
  private journalError: Error | null = null;
  private storedRecovery?: RecoveryRecord<ServiceGraph>;
  private restoringOriginal = false;
  private restoredOriginalMissing = false;
  private journalLifecycle = 0;
  private journalReady = false;
  private socket: WebSocket | null = null;
  private session: CollaborationSession<ServiceGraph> | null = null;
  private inputGuards = new Map<string, () => boolean>();
  private inputBindings = new Map<
    string,
    {
      composer: ComposerSession;
      baseline: PromptDraft | null;
      applying: boolean;
      conflict: boolean;
      cleanup: () => void;
    }
  >();
  private refreshingInputs = false;
  private draftReaders = new Map<
    string,
    {
      session: CollaborationSession<ServiceGraph>;
      read: () => CanvasTextDraft | null;
    }
  >();
  private undoManager: Y.UndoManager | null = null;
  private timelineUndoManagers = new Map<string, Y.UndoManager>();
  private editingTimeline: string | null = null;
  private destroyTimelineHistory() {
    for (const manager of this.timelineUndoManagers.values()) manager.destroy();
    this.timelineUndoManagers.clear();
    this.editingTimeline = null;
  }
  private timelineUndoManager(nodeID: string) {
    let manager = this.timelineUndoManagers.get(nodeID);
    if (!manager && this.session) {
      manager = canvasUndoManager(this.session.doc);
      const owned = manager;
      manager.captureTransaction = (transaction) =>
        this.editingTimeline === nodeID || transaction.origin === owned;
      this.timelineUndoManagers.set(nodeID, manager);
    }
    return manager;
  }
  timelineHistoryState = (nodeID: string) => {
    const manager = this.timelineUndoManagers.get(nodeID);
    return { canUndo: !!manager?.canUndo(), canRedo: !!manager?.canRedo() };
  };
  timelineHistory = (nodeID: string, direction: "undo" | "redo") => {
    if (
      !this.view.document?.nodes.some(
        (node) => node.id === nodeID && node.type === "timeline",
      )
    )
      return;
    this.applyHistory(this.timelineUndoManagers.get(nodeID) ?? null, direction);
  };
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
    localPersistence: "CHECKING",
    recoveryRemaining: 0,
    inputDrafts: {},
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
      /** Dependency seam for isolated transport tests; production defaults to IndexedDB. */
      recoveryJournal?: CanvasPendingJournalFactory | false;
    },
  ) {}
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };
  getSnapshot = () => this.view;
  registerInputGuard = (nodeID: string, guard: () => boolean) => {
    this.inputGuards.set(nodeID, guard);
    return () => {
      if (this.inputGuards.get(nodeID) === guard)
        this.inputGuards.delete(nodeID);
    };
  };
  readInputDraft = (nodeID: string): RetainedCanvasInput | null => {
    const form = this.session?.forms[nodeID] as
      { canvasInput?: RetainedCanvasInput } | undefined;
    return structuredClone(
      this.session
        ? (form?.canvasInput ?? null)
        : (this.view.inputDrafts[nodeID] ?? null),
    );
  };
  retainInputDraft = (
    nodeID: string,
    input: PromptDraft,
    baseline: PromptDraft | null = null,
    reason = "生成输入尚未同步",
  ) => {
    validatePromptDraft(input);
    const retained = {
      draft: structuredClone(input),
      baseline: structuredClone(baseline),
      reason,
    };
    if (
      composerFingerprint(this.readInputDraft(nodeID)) ===
      composerFingerprint(retained)
    )
      return;
    if (this.session && !this.session.closed) {
      const previous = this.session.forms[nodeID];
      this.session.setForms({
        ...this.session.forms,
        [nodeID]: {
          ...(previous && typeof previous === "object" ? previous : {}),
          canvasInput: retained,
        },
      });
    } else
      this.emit({
        inputDrafts: { ...this.view.inputDrafts, [nodeID]: retained },
      });
  };
  private clearInputDraft(nodeID: string) {
    if (!this.session || !this.readInputDraft(nodeID)) return;
    const forms = { ...this.session.forms };
    const form = { ...(forms[nodeID] as Record<string, unknown> | undefined) };
    delete form.canvasInput;
    if (Object.keys(form).length) forms[nodeID] = form;
    else delete forms[nodeID];
    this.session.setForms(forms);
  }
  /** Explicit conflict resolution; no automatic replay over a collaborator's input. */
  resolveInputDraft = (nodeID: string, choice: "server" | "local") => {
    const retained = this.readInputDraft(nodeID);
    if (!retained) return true;
    const binding = this.inputBindings.get(nodeID);
    if (binding?.composer.composing) throw problem("请先结束输入法编辑");
    const authoritative = this.view.document?.nodes.find(
      (node) => node.id === nodeID,
    )?.serviceData?.config?.draft?.composer as PromptDraft | undefined;
    if (!authoritative) throw problem("节点已删除或不可访问，本页草稿继续保留");
    const input = choice === "server" ? authoritative : retained.draft;
    if (
      choice === "local" &&
      (!this.view.writable ||
        !this.view.document ||
        !this.canAccept(this.view.document))
    )
      throw problem("当前无法重新提交，本页草稿继续保留");
    if (binding) {
      if (binding.composer.editor.isDestroyed)
        throw problem("请重新打开生成输入");
      const doc = binding.composer.editor.schema.nodeFromJSON(input.document);
      const tr = binding.composer.editor.state.tr.replaceWith(
        0,
        binding.composer.editor.state.doc.content.size,
        doc.content,
      );
      binding.applying = true;
      try {
        if (!binding.composer.applyAuthoritative(input, tr))
          throw problem("输入仍在编辑，本页草稿继续保留");
        binding.baseline = structuredClone(authoritative);
        binding.conflict = false;
      } finally {
        binding.applying = false;
      }
    }
    if (choice === "local") return this.changeInput(nodeID, input);
    this.clearInputDraft(nodeID);
    return true;
  };
  private inputEditing() {
    return (
      this.gesture ||
      [...this.inputGuards.values()].some((guard) => !guard()) ||
      [...this.draftReaders.values()].some(
        (entry) => entry.session === this.session && entry.read() !== null,
      )
    );
  }
  private hasUnsubmittedInput() {
    return [...this.inputBindings].some(([nodeID, binding]) => {
      if (binding.composer.editor.isDestroyed) return false;
      const input = this.view.document?.nodes.find((node) => node.id === nodeID)
        ?.serviceData?.config?.draft?.composer;
      return (
        composerFingerprint(readableInput(binding.composer)) !==
        composerFingerprint(input)
      );
    });
  }
  changeInput = (
    nodeID: string,
    input: PromptDraft,
    materializeEmpty = false,
  ) => {
    validatePromptDraft(input);
    const binding = this.inputBindings.get(nodeID);
    if (binding?.applying) return true;
    const graph = this.view.document;
    const node = graph?.nodes.find((item) => item.id === nodeID);
    // Boot emits the initial snapshot even in read-only mode. It is not an edit.
    if (
      !this.readInputDraft(nodeID) &&
      node?.serviceData &&
      composerFingerprint(
        node.serviceData.config?.draft?.composer ??
          (materializeEmpty ? null : emptyPromptDraft()),
      ) === composerFingerprint(input)
    )
      return true;
    if (
      binding?.conflict ||
      this.inputEditing() ||
      !graph ||
      !node?.serviceData ||
      !this.canAccept(graph)
    ) {
      this.retainInputDraft(
        nodeID,
        input,
        binding?.baseline ?? null,
        binding?.conflict
          ? "其他协作者已修改生成输入，请先处理本页草稿"
          : "生成输入当前不可提交，已保留本页草稿",
      );
      return false;
    }
    if (
      composerFingerprint(node.serviceData.config?.draft?.composer) ===
      composerFingerprint(input)
    ) {
      this.clearInputDraft(nodeID);
      return true;
    }
    this.change(
      {
        ...graph,
        nodes: graph.nodes.map((item) =>
          item.id !== nodeID
            ? item
            : {
                ...item,
                serviceData: {
                  ...item.serviceData!,
                  config: {
                    ...item.serviceData!.config,
                    draft: {
                      composer: structuredClone(
                        input,
                      ) as unknown as API.PromptDraft,
                      prompt: "",
                      negative_prompt: "",
                      aspect_ratio: "",
                      candidate_count: 0,
                    },
                  },
                },
              },
        ),
      },
      true,
    );
    const current = this.view.document?.nodes.find((item) => item.id === nodeID)
      ?.serviceData?.config?.draft?.composer;
    if (composerFingerprint(current) !== composerFingerprint(input)) {
      this.retainInputDraft(
        nodeID,
        input,
        binding?.baseline ?? null,
        "生成输入未能写入画布，已保留本页草稿",
      );
      return false;
    }
    if (binding) binding.baseline = structuredClone(input);
    this.clearInputDraft(nodeID);
    return true;
  };
  bindInputComposer = (nodeID: string, composer: ComposerSession) => {
    this.inputBindings.get(nodeID)?.cleanup();
    const boundNode = this.view.document?.nodes.find(
      (node) => node.id === nodeID,
    );
    const baseline =
      boundNode?.serviceData?.config?.draft?.composer ??
      (boundNode ? emptyPromptDraft() : null);
    if (baseline) validatePromptDraft(baseline);
    const binding = {
      composer,
      baseline: structuredClone(baseline ?? null),
      applying: false,
      conflict: !!this.readInputDraft(nodeID),
      cleanup: () => {},
    };
    const unregister = this.registerInputGuard(
      nodeID,
      () =>
        !composer.editor.isDestroyed &&
        !composer.composing &&
        !composer.pendingTarget &&
        !composer
          .snapshot()
          .draft.references.some((reference) =>
            composer.referencePending(reference.id),
          ) &&
        !binding.conflict,
    );
    const protect = () => {
      if (binding.applying || composer.editor.isDestroyed) return;
      const draft = readableInput(composer);
      // A clean editor disappearing during undo has no unsaved input to retain.
      if (
        !composer.composing &&
        composerFingerprint(draft) === composerFingerprint(binding.baseline)
      )
        return;
      if (composer.composing || binding.conflict)
        this.retainInputDraft(
          nodeID,
          draft,
          binding.baseline,
          binding.conflict
            ? "其他协作者已修改生成输入，请先处理本页草稿"
            : "输入法编辑尚未结束",
        );
    };
    composer.editor.on("transaction", protect);
    composer.editor.view.dom.addEventListener("input", protect);
    composer.editor.view.dom.addEventListener("compositionstart", protect);
    composer.editor.view.dom.addEventListener("compositionend", protect);
    binding.cleanup = () => {
      if (
        !composer.editor.isDestroyed &&
        composerFingerprint(readableInput(composer)) !==
          composerFingerprint(binding.baseline)
      )
        this.retainInputDraft(
          nodeID,
          readableInput(composer),
          binding.baseline,
          "生成输入尚未同步，已保留本页草稿",
        );
      protect();
      unregister();
      composer.editor.off("transaction", protect);
      if (!composer.editor.isDestroyed) {
        composer.editor.view.dom.removeEventListener("input", protect);
        composer.editor.view.dom.removeEventListener(
          "compositionstart",
          protect,
        );
        composer.editor.view.dom.removeEventListener("compositionend", protect);
      }
      if (this.inputBindings.get(nodeID) === binding)
        this.inputBindings.delete(nodeID);
    };
    this.inputBindings.set(nodeID, binding);
    this.refreshInputBindings();
    return binding.cleanup;
  };
  private refreshInputBindings() {
    if (this.refreshingInputs) return;
    this.refreshingInputs = true;
    try {
      for (const [nodeID, binding] of this.inputBindings) {
        const { composer } = binding;
        if (binding.applying || composer.editor.isDestroyed) continue;
        const boundNode = this.view.document?.nodes.find(
          (node) => node.id === nodeID,
        );
        const next =
          boundNode?.serviceData?.config?.draft?.composer ??
          (boundNode ? emptyPromptDraft() : null);
        if (next) validatePromptDraft(next);
        const current = readableInput(composer);
        if (!next) {
          if (
            composer.composing ||
            composerFingerprint(current) !==
              composerFingerprint(binding.baseline)
          )
            this.retainInputDraft(
              nodeID,
              current,
              binding.baseline,
              "节点已删除或不可访问，生成输入已保留",
            );
          binding.conflict = true;
          continue;
        }
        if (composerFingerprint(next) === composerFingerprint(current)) {
          binding.baseline = structuredClone(next);
          if (!composer.composing && !binding.conflict)
            this.clearInputDraft(nodeID);
          continue;
        }
        if (composerFingerprint(next) === composerFingerprint(binding.baseline))
          continue;
        if (
          composer.composing ||
          binding.conflict ||
          composerFingerprint(current) !== composerFingerprint(binding.baseline)
        ) {
          binding.conflict = true;
          this.retainInputDraft(
            nodeID,
            current,
            binding.baseline,
            "其他协作者已修改生成输入，请先处理本页草稿",
          );
          continue;
        }
        const doc = composer.editor.schema.nodeFromJSON(next.document);
        const tr = composer.editor.state.tr.replaceWith(
          0,
          composer.editor.state.doc.content.size,
          doc.content,
        );
        binding.applying = true;
        try {
          if (composer.applyAuthoritative(next, tr))
            binding.baseline = structuredClone(next);
        } finally {
          binding.applying = false;
        }
      }
    } finally {
      this.refreshingInputs = false;
    }
  }

  /** A quote may only consume an acknowledged input, never a local projection. */
  prepareInput = async (nodeID: string): Promise<API.InputSource> => {
    const initial = this.view.document?.nodes.find(
      (node) => node.id === nodeID,
    );
    if (!initial || !["text", "image", "video", "audio"].includes(initial.type))
      throw problem("此节点不支持生成");
    if (!initial.serviceData?.config?.draft?.composer)
      throw problem("请先完成生成输入的初始化");
    if (this.readInputDraft(nodeID))
      throw problem("请先处理本页保留的生成输入");
    if (
      initial.serviceData.config.draft.composer.references?.some(
        (reference) => reference.source?.kind === "pending_upload",
      )
    )
      throw problem("参考素材尚未上传完成，请完成或移除后再生成");
    const input = inputFingerprint(
      this.view.document?.nodes.find((node) => node.id === nodeID)?.serviceData
        ?.config?.draft,
    );
    const session = this.session;
    const epoch = session?.confirmed.epoch;
    const checkInput = () => {
      if (this.session !== session || session?.confirmed.epoch !== epoch)
        throw problem("画布版本已切换，请重新检查生成输入");
      const current = this.view.document?.nodes.find(
        (node) => node.id === nodeID,
      );
      if (
        !current ||
        inputFingerprint(current.serviceData?.config?.draft) !== input
      )
        throw problem("生成输入已变化，请重新提交");
    };
    const dto = await this.waitForSaved(checkInput);
    // Resolution of a promise yields to other input handlers before this resumes.
    checkInput();
    if (
      this.disposed ||
      !this.view.writable ||
      !this.online ||
      this.inputEditing() ||
      this.hasUnsubmittedInput() ||
      this.view.dirty ||
      this.draining ||
      session?.status !== "CLEAN" ||
      session.inFlight ||
      this.dto?.revision !== dto.revision ||
      session.confirmed.contentRevision !== dto.revision
    )
      throw problem("画布已变化，请重新检查生成输入");
    const node = dto.nodes?.find((item) => item.id === nodeID);
    const canvasRevision = Number(dto.revision);
    if (
      !Number.isSafeInteger(canvasRevision) ||
      canvasRevision < 1 ||
      !Number.isSafeInteger(node?.input_revision) ||
      node!.input_revision! < 1 ||
      inputFingerprint(node?.config?.draft) !== input
    )
      throw problem("生成输入的保存版本尚未确认");
    return {
      kind: "canvas_input",
      canvas_id: this.config.canvasID,
      node_id: nodeID,
      canvas_revision: canvasRevision,
      input_revision: node!.input_revision!,
    };
  };

  /** Capture alongside async intent; compare again before writing into this document. */
  getDocumentEpoch = (): string | null =>
    this.disposed ||
    !this.view.document ||
    this.epochPending ||
    this.switchingEpoch
      ? null
      : (this.session?.confirmed.epoch ?? null);

  private retainedDirectorEdits = new Map<string, API.DirectorConfig>();
  retainedDirector = (nodeID: string) => {
    const draft = this.retainedDirectorEdits.get(nodeID);
    return draft ? structuredClone(draft) : null;
  };
  retainDirector = (nodeID: string, draft: API.DirectorConfig) => {
    this.retainedDirectorEdits.set(nodeID, validateDirectorDocument(draft));
  };
  discardRetainedDirector = (nodeID: string) => {
    this.retainedDirectorEdits.delete(nodeID);
  };
  changeDirector = (
    nodeID: string,
    input: API.DirectorConfig,
    expected: API.DirectorConfig,
    epoch: string | null,
  ) => {
    const director = validateDirectorDocument(input);
    try {
      const graph = this.view.document,
        node = graph?.nodes.find((item) => item.id === nodeID);
      if (
        !epoch ||
        this.getDocumentEpoch() !== epoch ||
        !graph ||
        node?.type !== "director" ||
        !node.serviceData?.config?.director ||
        !this.canAccept(graph) ||
        this.inputEditing()
      )
        throw problem("导演场景当前不可编辑或已恢复到其他版本");
      if (
        directorFingerprint(node.serviceData.config.director) !==
        directorFingerprint(expected)
      )
        throw problem("导演场景已由其他协作者修改，请检查后重试");
      if (directorFingerprint(director) !== directorFingerprint(expected)) {
        this.change(
          {
            ...graph,
            nodes: graph.nodes.map((item) =>
              item.id !== nodeID
                ? item
                : {
                    ...item,
                    serviceData: {
                      ...item.serviceData!,
                      config: { ...item.serviceData!.config, director },
                    },
                  },
            ),
          },
          true,
        );
        if (
          directorFingerprint(
            this.view.document?.nodes.find((item) => item.id === nodeID)
              ?.serviceData?.config?.director,
          ) !== directorFingerprint(director)
        )
          throw problem("导演场景未能写入，请检查保存状态");
      }
      this.retainedDirectorEdits.delete(nodeID);
    } catch (error) {
      this.retainedDirectorEdits.set(nodeID, director);
      throw error;
    }
  };

  private retainedTimelineEdits = new Map<string, Timeline>();
  retainedTimeline = (nodeID: string) => {
    const value = this.retainedTimelineEdits.get(nodeID);
    return value ? structuredClone(value) : null;
  };
  discardRetainedTimeline = (nodeID: string) => {
    this.retainedTimelineEdits.delete(nodeID);
  };
  changeTimeline = (nodeID: string, input: Timeline, expected: Timeline) => {
    const timeline = readTimeline(input);
    try {
      const graph = this.view.document;
      const node = graph?.nodes.find((item) => item.id === nodeID);
      if (
        !graph ||
        node?.type !== "timeline" ||
        !node?.serviceData?.config?.timeline ||
        !this.canAccept(graph) ||
        this.inputEditing()
      )
        throw problem("时间线当前不可编辑，请结束其他编辑或恢复连接权限");
      if (
        timelineFingerprint(node.serviceData.config.timeline) !==
        timelineFingerprint(expected)
      )
        throw problem("时间线已由其他协作者修改，请检查最新内容后重试");
      if (timelineFingerprint(timeline) === timelineFingerprint(expected)) {
        this.retainedTimelineEdits.delete(nodeID);
        return;
      }
      const manager = this.timelineUndoManager(nodeID);
      manager?.stopCapturing();
      this.editingTimeline = nodeID;
      try {
        this.change(
          reconcileTimelineConnections({
            ...graph,
            nodes: graph.nodes.map((item) =>
              item.id !== nodeID
                ? item
                : {
                    ...item,
                    serviceData: {
                      ...item.serviceData!,
                      config: { ...item.serviceData!.config, timeline },
                    },
                  },
            ),
          }),
          true,
        );
      } finally {
        this.editingTimeline = null;
        manager?.stopCapturing();
      }
      if (
        timelineFingerprint(
          this.view.document?.nodes.find((item) => item.id === nodeID)
            ?.serviceData?.config?.timeline,
        ) !== timelineFingerprint(timeline)
      )
        throw problem("时间线未能写入，请检查保存状态");
      this.retainedTimelineEdits.delete(nodeID);
    } catch (cause) {
      // Keep rejected intent in the canvas session so closing the workspace does not lose it.
      this.retainedTimelineEdits.set(nodeID, timeline);
      throw cause;
    }
  };
  prepareTimeline = async (
    nodeID: string,
  ): Promise<
    Required<
      Pick<API.MediatoolCreateInput, "canvas_id" | "node_id" | "revision">
    >
  > => {
    if (this.retainedTimelineEdits.has(nodeID))
      throw problem("请先处理保留的时间线编辑");
    const current = this.view.document?.nodes.find((node) => node.id === nodeID)
      ?.serviceData?.config?.timeline;
    const timeline = readTimeline(current);
    if (!timeline.clips.length) throw problem("请先添加素材");
    const fingerprint = timelineFingerprint(current);
    const check = () => {
      if (this.retainedTimelineEdits.has(nodeID))
        throw problem("请先处理保留的时间线编辑");
      if (
        timelineFingerprint(
          this.view.document?.nodes.find((node) => node.id === nodeID)
            ?.serviceData?.config?.timeline,
        ) !== fingerprint
      )
        throw problem("时间线已变化，请重新导出");
    };
    const dto = await this.waitForSaved(check);
    check();
    const revision = Number(dto.revision);
    if (
      this.disposed ||
      !this.view.writable ||
      !this.online ||
      this.view.dirty ||
      this.draining ||
      this.inputEditing() ||
      this.hasUnsubmittedInput() ||
      this.session?.status !== "CLEAN" ||
      this.session.inFlight ||
      this.dto?.revision !== dto.revision ||
      this.session.confirmed.contentRevision !== dto.revision ||
      !Number.isSafeInteger(revision) ||
      revision < 1 ||
      timelineFingerprint(
        dto.nodes?.find((node) => node.id === nodeID)?.config?.timeline,
      ) !== fingerprint
    )
      throw problem("时间线的保存版本尚未确认");
    return { canvas_id: this.config.canvasID, node_id: nodeID, revision };
  };

  waitForSaved = (check: () => void = () => {}): Promise<API.DocumentDTO> => {
    return new Promise((resolve, reject) => {
      let finished = false;
      let requestedRevision = "";
      const originalSession = this.session;
      const originalEpoch = originalSession?.confirmed.epoch;
      let unsubscribe: () => void = () => {};
      const timer = setTimeout(
        () => finish(problem("保存尚未确认，请稍后重试")),
        15000,
      );
      const finish = (error?: Error, dto?: API.DocumentDTO) => {
        if (finished) return;
        finished = true;
        clearTimeout(timer);
        unsubscribe();
        if (error) reject(error);
        else resolve(structuredClone(dto!));
      };
      const inspect = () => {
        try {
          check();
          const session = this.session;
          if (
            this.disposed ||
            session !== originalSession ||
            session?.confirmed.epoch !== originalEpoch ||
            !this.online ||
            this.socket?.readyState !== WebSocket.OPEN ||
            !this.view.writable ||
            !session ||
            this.inputEditing() ||
            this.hasUnsubmittedInput() ||
            Object.keys(this.view.inputDrafts).length ||
            this.epochPending ||
            this.switchingEpoch ||
            this.uploads.size ||
            this.uploadErrors.size ||
            Object.keys(session.textDrafts).length ||
            session.status === "UNKNOWN" ||
            this.view.recovery.length
          )
            throw problem("请先完成或恢复当前画布编辑，再开始生成");
          if (session.status !== "CLEAN" || session.inFlight || this.draining)
            return;
          if (this.dto?.revision === session.confirmed.contentRevision) {
            finish(undefined, this.dto);
          } else if (requestedRevision !== session.confirmed.contentRevision) {
            requestedRevision = session.confirmed.contentRevision;
            this.socket?.send(JSON.stringify({ type: "sync" }));
          }
        } catch (error) {
          finish(error instanceof Error ? error : problem("保存确认失败"));
        }
      };
      unsubscribe = this.subscribe(inspect);
      inspect();
      if (!finished) this.schedule();
    });
  };
  private emit(patch: Partial<YjsView> = {}) {
    const session = this.session;
    const inputDrafts = session
      ? Object.fromEntries(
          Object.entries(session.forms).flatMap(([id, value]) => {
            const input = (
              value as { canvasInput?: RetainedCanvasInput } | undefined
            )?.canvasInput;
            return input ? [[id, structuredClone(input)]] : [];
          }),
        )
      : this.view.inputDrafts;
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
      !!this.restoreCommand ||
      Object.keys(inputDrafts).length > 0;
    this.view = {
      ...this.view,
      recoveryRemaining: this.journal?.remaining ?? 0,
      localPersistence:
        this.config.recoveryJournal === false
          ? "MEMORY"
          : this.journalError || session?.journalError
            ? "UNAVAILABLE"
            : session?.protected
              ? "PROTECTED"
              : this.journal
                ? "PENDING"
                : "CHECKING",
      inputDrafts,
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
        this.config.recoveryJournal !== false &&
        (this.journalError || session?.journalError)
          ? "本机草稿保存失败，请勿关闭页面"
          : (this.journal?.remaining ?? 0) > 0
            ? `另有 ${this.journal!.remaining} 份本机草稿，当前恢复完成后请重新打开处理`
            : recovery.length ||
                session?.sealed ||
                Object.keys(inputDrafts).length
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
    this.refreshInputBindings();
    for (const listener of this.listeners) listener();
  }
  private changed = () => {
    if (this.disposed) return;
    const session = this.session;
    if (session && this.config.recoveryJournal !== false)
      void session.protect().then(() => {
        if (!this.disposed && this.session === session) this.emit();
      });
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
    this.journalReady = this.config.recoveryJournal === false;
    if (this.journalReady) this.connect();
    else void this.prepareJournal();
  }
  private async prepareJournal() {
    const lifecycle = ++this.journalLifecycle;
    try {
      const journal = await (
        this.config.recoveryJournal || openCanvasPendingJournal
      )({
        origin: location.origin,
        actorID: this.config.actorID,
        scopeID: this.config.scopeID,
        projectID: this.config.projectID,
        canvasID: this.config.canvasID,
      });
      if (this.disposed || lifecycle !== this.journalLifecycle) {
        await journal.close();
        return;
      }
      this.journal = journal;
      this.clientID = journal.sessionId;
      this.storedRecovery = journal.record;
      this.journalError = null;
    } catch (error) {
      if (this.disposed || lifecycle !== this.journalLifecycle) return;
      this.journalError =
        error instanceof Error ? error : problem("本机草稿存储不可用");
    }
    this.journalReady = true;
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
    if (this.disposed || this.renewal || !this.journalReady) return;
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
      this.emit(
        !this.view.document && !this.view.message
          ? { message: "连接已中断，正在尝试重新连接。" }
          : {},
      );
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
    this.destroyTimelineHistory();
    this.undoManager?.destroy();
    this.session?.destroy();
    const journal = this.journal;
    this.session = new CollaborationSession<ServiceGraph>({
      scope: {
        origin: location.origin,
        actorKey: `USER:${this.config.actorID}`,
        canvasId: this.config.canvasID,
        sessionId: this.clientID,
      },
      initial,
      storage: this.config.recoveryJournal === false ? "memory" : "journal",
      journal: {
        put: async (record) => {
          if (!journal)
            throw this.journalError ?? problem("本机草稿存储不可用");
          await journal.put(record);
        },
      },
      transport: {
        submit: async (update) => {
          const active = this.session;
          if (
            !active ||
            this.disposed ||
            active.inFlight?.updateId !== update.updateId
          )
            throw problem("原提交会话已变化");
          if (
            this.config.recoveryJournal !== false &&
            !(await active.protect())
          )
            throw problem("本机草稿保存失败，请勿关闭页面");
          if (this.disposed || this.session !== active)
            throw problem("原提交会话已关闭");
          const receipt = await this.rpc("yjs_submit", update);
          if (!receipt) throw problem("缺少保存回执");
          return receipt;
        },
        query: async (update) => {
          const receipt = await this.rpc("yjs_resolve", update);
          if (this.restoringOriginal && !receipt)
            this.restoredOriginalMissing = true;
          return receipt;
        },
        confirmedSnapshot: (receipt) => this.confirmedSnapshot(receipt),
      },
      changed: this.changed,
      recovery,
    });
    this.undoManager = canvasUndoManager(this.session.doc);
    this.undoManager.captureTransaction = () => this.editingTimeline === null;
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
          this.captureTextDrafts();
          await this.session?.protect();
          this.session?.destroy();
          this.session = null;
          this.dto = null;
          this.draft = null;
          this.emit({
            document: null,
            dto: null,
            inputDrafts: {},
            recovery: [],
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
      if (!this.session) {
        const stored = this.storedRecovery;
        this.storedRecovery = undefined;
        if (stored?.inFlight) {
          this.restoringOriginal = true;
          this.makeSession(stored.confirmed, stored);
        } else
          this.makeSession(
            initial,
            stored ? archiveRecovery(stored, initial) : undefined,
          );
      }
      if (this.session!.confirmed.epoch !== state.epoch) {
        this.epochPending = initial;
        this.emit({ message: "画布版本已切换，正在查证原提交并保留本页输入" });
        queueMicrotask(() => void this.adoptEpoch());
        return;
      } else await this.session!.receive({ ...initial, snapshot: true });
      if (this.session?.status === "REJECTED" && !this.session.inFlight) {
        const record = archiveRecovery(this.session.record(), initial);
        if (this.config.recoveryJournal !== false)
          await this.journal!.put(record);
        this.restoringOriginal = false;
        this.restoredOriginalMissing = false;
        this.makeSession(initial, record);
      }
      this.emit({ migration: false, message: "" });
      if (this.config.recoveryJournal !== false) await this.session?.protect();
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
      this.draining ||
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
          await session.flush({ queryOnly: true });
        } catch {
          /* A durable rejection also settles the original. */
        }
      }
      if (this.disposed || this.session !== session) return;
      if (this.restoredOriginalMissing) {
        await this.archiveMissingOriginal(this.epochPending!);
        return;
      }
      if (session.inFlight) {
        this.emit({ message: "原版本的提交结果尚未确认，输入继续保留在本页" });
        return;
      }
      const initial = this.epochPending;
      const recovery = archiveRecovery(session.record(), initial);
      if (this.config.recoveryJournal !== false)
        await this.journal!.put(recovery);
      this.epochPending = null;
      this.restoringOriginal = false;
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
      await session.flush({
        queryOnly: !this.permitted || this.restoringOriginal,
      });
      if (this.restoringOriginal && !session.inFlight) {
        this.restoringOriginal = false;
        if (
          session.pending.length ||
          session.sealed ||
          Object.keys(session.forms).length ||
          Object.keys(session.textDrafts).length
        ) {
          const recovery = archiveRecovery(session.record(), session.confirmed);
          await this.journal!.put(recovery);
          this.makeSession(session.confirmed, recovery);
          this.render();
          return;
        }
      }
      completed = true;
      if (!this.disposed && this.session === session) {
        this.emit({ message: "" });
        // Accepted bytes can still lose a single-value intent. Request the
        // recovery baseline immediately, even if no peer makes another edit.
        if (session.sealed) this.socket?.send(JSON.stringify({ type: "sync" }));
      }
    } catch (cause) {
      if (!this.disposed && this.session === session) {
        if (this.restoredOriginalMissing) {
          try {
            await this.archiveMissingOriginal(session.confirmed);
          } catch (error) {
            this.journalError =
              error instanceof Error ? error : problem("本机草稿归档失败");
          }
          this.render();
          return;
        }
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

  private async archiveMissingOriginal(initial: CommittedState) {
    const record = this.session!.record();
    const original = record.inFlight;
    const settled: RecoveryRecord<ServiceGraph> = {
      ...record,
      inFlight: null,
      acceptedReceipt: null,
      sealed: {
        reason: "ORIGINAL_NOT_FOUND",
        snapshot: record.snapshot,
        inFlight: original,
        pending: record.pending,
        intents: [...record.inFlightIntents, ...record.intents],
        originalOutcome: "NOT_FOUND",
      },
    };
    const recovery = archiveRecovery(settled, initial);
    await this.journal!.put(recovery);
    this.journalError = null;
    this.epochPending = null;
    this.restoringOriginal = false;
    this.restoredOriginalMissing = false;
    this.makeSession(initial, recovery);
    this.emit({
      message: "原提交未被服务器接收，本机修改已保留，请查看恢复内容",
    });
    this.render();
  }

  canAccept = (state: CanvasEditorState) =>
    !!this.session &&
    this.view.writable &&
    this.session.pending.length < 128 &&
    new TextEncoder().encode(JSON.stringify(state)).byteLength <
      16 * 1024 * 1024;
  /** Expose durable media identity without changing the page-local upload preview. */
  connectionMediaNode = (node: CanvasNodeData): CanvasNodeData => {
    const source = localSource(node);
    if (!source) return node;
    const receipt = this.receipts.get(node.id);
    if (
      receipt?.source !== source ||
      !receipt.asset ||
      !node.documentData?.fileReferences?.some(
        (ref) => ref.purpose === "CONTENT" && ref.fileId === receipt.asset!.id,
      )
    )
      throw problem("来源素材尚未上传完成");
    const metadata = { ...node.metadata };
    delete metadata.content;
    return { ...node, metadata };
  };

  /** Domain connections may replace only the target's settled composer input. */
  canAcceptAudioConnectionInput = (nodeID: string): boolean => {
    const node = this.view.document?.nodes.find((item) => item.id === nodeID);
    if (
      !node?.serviceData ||
      !this.view.writable ||
      this.readInputDraft(nodeID)
    )
      return false;
    try {
      if (this.inputGuards.get(nodeID)?.() === false) return false;
      const binding = this.inputBindings.get(nodeID);
      if (!binding) return true;
      const composer = binding.composer;
      if (
        binding.applying ||
        binding.conflict ||
        composer.editor.isDestroyed ||
        composer.composing ||
        composer.pendingTarget
      )
        return false;
      return (
        composerFingerprint(readableInput(composer)) ===
        composerFingerprint(
          node.serviceData.config?.draft?.composer ?? emptyPromptDraft(),
        )
      );
    } catch {
      return false;
    }
  };

  /** Connection gestures must publish their complete graph, never the upload-ready subset. */
  canAcceptConnectionGraph = (state: CanvasEditorState): boolean => {
    if (
      !this.rendered ||
      this.session?.status === "UNKNOWN" ||
      this.session?.inFlight ||
      !this.canAccept(state)
    )
      return false;
    try {
      for (const node of state.nodes) {
        this.connectionMediaNode(node);
        const config = node.serviceData?.config;
        const composers = [config?.composer, config?.draft?.composer] as (
          PromptDraft | undefined
        )[];
        if (
          composers.some((composer) =>
            composer?.references?.some(
              (reference) => reference.source?.kind === "pending_upload",
            ),
          )
        )
          return false;
      }
      // Run the same full projection as change(), before the editor publishes it.
      serviceDocumentFromEditor(this.rendered, state);
      return true;
    } catch {
      return false;
    }
  };
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
    this.applyHistory(this.undoManager, direction);
  };
  private applyHistory(
    manager: Y.UndoManager | null,
    direction: "undo" | "redo",
  ) {
    if (!this.view.writable || !manager || !this.session) return;
    const stack = direction === "undo" ? manager.undoStack : manager.redoStack;
    const frame = stack
      .at(-1)
      ?.meta.get("canvas-frame") as typeof this.historyFrame;
    if (frame) {
      const from = direction === "undo" ? frame.after : frame.before,
        to = direction === "undo" ? frame.before : frame.after;
      const removing = new Set(
        from.graph.nodes
          .filter((node) => !to.graph.nodes.some((next) => next.id === node.id))
          .map((node) => node.id),
      );
      const knownEdges = new Set(from.graph.connections.map((edge) => edge.id));
      const foreignDependency =
        (
          this.session.snapshot.graph
            .connections as ServiceGraph["connections"][number][]
        ).some(
          (edge) =>
            !knownEdges.has(edge.id) &&
            (removing.has(edge.fromNodeId) || removing.has(edge.toNodeId)),
        ) ||
        (
          this.session.snapshot.graph.nodes as ServiceGraph["nodes"][number][]
        ).some(
          (node) =>
            node.parentId &&
            removing.has(node.parentId) &&
            !removing.has(node.id) &&
            !(from.graph.nodes as ServiceGraph["nodes"][number][]).some(
              (previous) =>
                previous.id === node.id && previous.parentId === node.parentId,
            ),
        );
      if (foreignDependency) {
        this.emit({
          message: "该对象已有其他编辑，无法单独撤销创建。协作者的内容已保留",
        });
        return;
      }
      for (const collection of ["nodes", "connections"] as const) {
        const fingerprint = (item: unknown) =>
          collection === "nodes"
            ? historyNodeFingerprint(item as ServiceGraph["nodes"][number])
            : canvasGraphFingerprint(item);
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
              fingerprint(current.get(id)!) !== fingerprint(item),
          )
        ) {
          this.emit({
            message: "该对象已有其他编辑，无法单独撤销创建。协作者的内容已保留",
          });
          return;
        }
      }
    }
    manager.stopCapturing();
    this.historyFrame = frame;
    try {
      applyCanvasHistory(this.session.doc, manager, direction, {
        epoch: this.session.confirmed.epoch,
        commitSeq: this.session.confirmed.commitSeq,
      });
    } finally {
      this.historyFrame = null;
    }
    this.render();
    this.schedule();
  }
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
  private async confirmedSnapshot(receipt: DurableReceipt) {
    // A later snapshot may overtake the original ACK. Only the immutable
    // accepted revision can prove whether its single-value intent prevailed.
    if (!/^(0|[1-9]\d*)$/.test(receipt.contentRevision))
      throw problem("原提交版本无效");
    const revision = BigInt(receipt.contentRevision);
    if (revision >= BigInt("9223372036854775807"))
      throw problem("原提交历史版本暂不可查证");
    const versions = await this.versions(String(revision + BigInt(1)));
    const original = versions.find(
      (version) =>
        version.epoch === receipt.epoch &&
        version.revision === receipt.contentRevision,
    );
    if (!original) throw problem("原提交历史版本不可用");
    const document = await this.version(original.id);
    if (
      document.id !== this.config.canvasID ||
      document.project_id !== this.config.projectID ||
      document.revision !== receipt.contentRevision
    )
      throw problem("原提交历史版本身份不符");
    return serviceSnapshot(asShared(document));
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
    if (!this.view.document) this.emit({ message: "", phase: "正在连接" });
    this.uploadErrors.clear();
    if (this.draft) this.change(this.draft, !this.gesture);
    if (!this.online) this.resume();
    else this.schedule();
  };
  stop() {
    this.captureTextDrafts();
    const session = this.session,
      journal = this.journal;
    if (journal)
      void (async () => {
        await session?.protect();
        await journal.close();
      })();
    this.journal = null;
    this.journalReady = false;
    this.journalLifecycle++;
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
    this.destroyTimelineHistory();
    this.undoManager?.destroy();
    this.session?.destroy();
    this.session = null;
    this.epochPending = null;
    this.draftReaders.clear();
    this.receipts.clear();
    this.online = false;
    this.permitted = false;
    this.emit();
    for (const binding of this.inputBindings.values()) binding.cleanup();
    this.inputGuards.clear();
  }
}
