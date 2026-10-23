import type { CanvasEditorState } from "./lib/canvas-document-adapter";
import { serviceGraphOperations } from "./service-graph";
import type { Operation } from "./service-document";
import { publishCanvasMedia, type CanvasMediaReceipt } from "./publish-media";

export type CanvasRecoveryReview = {
  revision: string;
  source: CanvasEditorState;
  current: CanvasEditorState;
  document: CanvasEditorState;
};
export type SavedCanvas = { document: CanvasEditorState; revision: string };
type Phase =
  "saved" | "editing" | "waiting" | "saving" | "uploading" | "recovery";
export type AutosaveSnapshot = {
  document: CanvasEditorState;
  replacement: CanvasEditorState;
  dirty: boolean;
  phase: Phase;
  error: string;
  writable: boolean;
};
function same(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (Array.isArray(a) && Array.isArray(b))
    return a.length === b.length && a.every((v, i) => same(v, b[i]));
  if (a && b && typeof a === "object" && typeof b === "object") {
    const left = a as Record<string, unknown>,
      right = b as Record<string, unknown>;
    const keys = Object.keys(left);
    return (
      keys.length === Object.keys(right).length &&
      keys.every((k) => same(left[k], right[k]))
    );
  }
  return false;
}
function merge(
  base: unknown,
  local: unknown,
  remote: unknown,
  key = "",
  reviewed = false,
): unknown {
  if (key === "serviceData" || key === "referenceProjection") return remote;
  if (same(base, local)) return remote;
  if (same(base, remote) || same(local, remote)) return local;
  if (
    key !== "position" &&
    base &&
    local &&
    remote &&
    typeof base === "object" &&
    typeof local === "object" &&
    typeof remote === "object" &&
    !Array.isArray(base) &&
    !Array.isArray(local) &&
    !Array.isArray(remote)
  ) {
    const b = base as Record<string, unknown>,
      l = local as Record<string, unknown>,
      r = remote as Record<string, unknown>;
    const result: Record<string, unknown> = {};
    for (const k of new Set([
      ...Object.keys(b),
      ...Object.keys(l),
      ...Object.keys(r),
    ])) {
      const value = merge(b[k], l[k], r[k], k, reviewed);
      if (value !== undefined) result[k] = value;
    }
    return result;
  }
  if (reviewed && key) return local;
  throw new Error("远端已修改同一内容，本页输入已保留，请处理冲突后继续");
}
/** Apply only compatible local intent. This transitional Go adapter is not a rich-text CRDT. */
export function rebaseCanvas(
  base: CanvasEditorState,
  local: CanvasEditorState,
  remote: CanvasEditorState,
  reviewed = false,
): CanvasEditorState {
  function objects<T extends { id: string }>(
    before: T[],
    mine: T[],
    theirs: T[],
    equivalent: (a: T, b: T) => boolean,
  ): T[] {
    const b = new Map(before.map((n) => [n.id, n])),
      l = new Map(mine.map((n) => [n.id, n]));
    const r = new Map(theirs.map((n) => [n.id, n]));
    const result: T[] = [];
    for (const id of new Set([...r.keys(), ...l.keys(), ...b.keys()])) {
      // Server DTO enrichment is not a new peer contribution. A successor deletion
      // may remove its own accepted node, but not a concurrently modified one.
      if (b.has(id) && !l.has(id) && r.has(id)) {
        try {
          if (equivalent(b.get(id)!, r.get(id)!)) continue;
        } catch {
          /* Retain the draft for explicit recovery. */
        }
      }
      const value = merge(b.get(id), l.get(id), r.get(id), "", reviewed);
      if (value !== undefined) result.push(value as T);
    }
    return result;
  }
  return {
    ...local,
    nodes: objects(
      base.nodes,
      local.nodes,
      remote.nodes,
      (a, b) =>
        serviceGraphOperations(
          { ...base, nodes: [a], connections: [] },
          { ...base, nodes: [b], connections: [] },
        ).length === 0,
    ),
    connections: objects(
      base.connections,
      local.connections,
      remote.connections,
      (a, b) =>
        serviceGraphOperations(
          { ...base, connections: [a] },
          { ...base, connections: [b] },
        ).length === 0,
    ),
  };
}
function localMedia(node: CanvasEditorState["nodes"][number] | undefined) {
  if (!node) return undefined;
  const source = node.metadata?.content;
  return ["image", "video", "audio"].includes(node.type) &&
    (source?.startsWith("data:") || source?.startsWith("blob:"))
    ? source
    : undefined;
}

/** Page-memory only: one immutable in-flight commit and a bounded successor intent. */
export class CanvasAutosaveSession {
  private accepted: SavedCanvas;
  private authority: SavedCanvas;
  private draft: CanvasEditorState;
  private surface: CanvasEditorState;
  private replacement: CanvasEditorState;
  private listeners = new Set<() => void>();
  private snapshot!: AutosaveSnapshot;
  private stable = true;
  private canEdit = false;
  private canSend = false;
  private pending = false;
  private reviewedRevision?: string;
  private disposed = false;
  private scheduled = false;
  private error = "";
  private queued = 0;
  private receipts = new Map<string, CanvasMediaReceipt>();
  private uploads = new Map<
    string,
    { source: string; abort: AbortController }
  >();
  private uploadErrors = new Map<string, string>();
  constructor(
    private config: SavedCanvas & {
      projectID: string;
      commit: (
        operations: Operation[],
        revision: string,
        strictBase?: boolean,
      ) => Promise<SavedCanvas>;
      publish?: typeof publishCanvasMedia;
      maxBytes?: number;
      maxTransactions?: number;
      maxSubmissionBytes?: number;
    },
  ) {
    this.accepted = this.authority = {
      document: config.document,
      revision: config.revision,
    };
    this.draft = this.surface = this.replacement = config.document;
    this.notify();
  }
  activate() {
    this.disposed = false;
    this.notify();
  }
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };
  getSnapshot = () => this.snapshot;
  private changed() {
    try {
      return (
        serviceGraphOperations(this.accepted.document, this.draft).length > 0
      );
    } catch {
      return true;
    }
  }
  private notify() {
    if (this.disposed) return;
    const dirty = this.changed() || this.pending || !this.stable;
    this.snapshot = {
      document: this.draft,
      replacement: this.replacement,
      dirty,
      writable:
        this.canEdit &&
        !this.error &&
        (!this.stable || this.queued < (this.config.maxTransactions ?? 128)),
      phase: this.error
        ? "recovery"
        : this.pending
          ? "saving"
          : this.uploads.size || this.uploadErrors.size
            ? "uploading"
            : !dirty
              ? "saved"
              : !this.stable
                ? "editing"
                : "waiting",
      error: this.error || [...this.uploadErrors.values()][0] || "",
    };
    for (const listener of this.listeners) listener();
  }
  receive(
    document: CanvasEditorState,
    revision: string,
    canEdit: boolean,
    canSend: boolean,
  ) {
    if (this.disposed) return;
    this.canEdit = canEdit;
    this.canSend = canSend;
    if (BigInt(revision) >= BigInt(this.authority.revision))
      this.authority = { document, revision };
    if (
      !this.pending &&
      !this.error &&
      BigInt(revision) > BigInt(this.accepted.revision)
    ) {
      try {
        if (this.reviewedRevision && revision !== this.reviewedRevision)
          throw new Error("服务器内容已更新，请重新查看差异后再应用");
        this.draft = rebaseCanvas(this.accepted.document, this.draft, document);
        if (this.stable) this.surface = this.replacement = this.draft;
        this.accepted = this.authority;
      } catch (cause) {
        this.error = (cause as Error).message;
      }
    }
    this.notify();
    this.schedule();
  }
  canAccept(document: CanvasEditorState) {
    const bytes = (value: unknown) =>
      new TextEncoder().encode(JSON.stringify(value)).byteLength;
    if (
      !this.snapshot.writable ||
      bytes(document) > (this.config.maxBytes ?? 16 * 1024 * 1024)
    )
      return false;
    try {
      // Leave headroom under the server's 1 MiB frame limit for the immutable envelope.
      if (
        bytes(serviceGraphOperations(this.accepted.document, document)) >
        (this.config.maxSubmissionBytes ?? 900 * 1024)
      )
        return false;
    } catch {
      // Upload candidates are not yet serializable; their bytes stay in the page
      // and are independently bounded above, then published before graph admission.
    }
    return true;
  }
  edit(document: CanvasEditorState, stable: boolean) {
    if (this.disposed) return;
    let presentationOnly = same(this.surface, document);
    if (!presentationOnly) {
      try {
        presentationOnly =
          serviceGraphOperations(this.surface, document).length === 0;
      } catch {
        /* Validate actual edits in the submission path. */
      }
    }
    if (presentationOnly) {
      this.surface = document;
      if (this.stable !== stable) {
        if (stable && this.changed()) this.queued++;
        this.stable = stable;
        this.notify();
        this.schedule();
      }
      return;
    }
    if (!this.canAccept(document)) {
      this.replacement = { ...this.draft };
      this.error ||= "当前不能接纳更多编辑，已有输入仍保留在本页";
      this.notify();
      return;
    }
    if (stable) this.queued++;
    try {
      this.draft = rebaseCanvas(this.surface, document, this.draft);
    } catch (cause) {
      this.draft = document;
      this.error = (cause as Error).message;
    }
    for (const id of this.receipts.keys()) {
      if (!document.nodes.some((node) => node.id === id))
        this.receipts.delete(id);
    }
    const previousSurface = this.surface;
    this.surface = document;
    for (const [id, upload] of this.uploads) {
      if (
        localMedia(document.nodes.find((node) => node.id === id)) !==
        upload.source
      ) {
        upload.abort.abort();
        this.uploads.delete(id);
        this.receipts.delete(id);
        this.uploadErrors.delete(id);
      }
    }
    for (const id of this.uploadErrors.keys()) {
      const before = previousSurface.nodes.find((node) => node.id === id);
      const after = document.nodes.find((node) => node.id === id);
      if (!after || !before || localMedia(before) !== localMedia(after)) {
        this.uploadErrors.delete(id);
        this.receipts.delete(id);
      }
    }
    this.stable = stable;
    this.notify();
    this.schedule();
  }
  private schedule() {
    if (this.disposed || this.scheduled) return;
    this.scheduled = true;
    queueMicrotask(() => {
      this.scheduled = false;
      void this.drain();
    });
  }
  private prepareMedia() {
    const publish = this.config.publish ?? publishCanvasMedia;
    const blocked = new Set<string>();
    for (const node of this.draft.nodes) {
      const source = localMedia(node);
      if (!source) continue;
      const receipt = this.receipts.get(node.id);
      if (
        receipt?.source === source &&
        receipt.asset &&
        node.documentData?.fileReferences?.some(
          (r) => r.fileId === receipt.asset!.id,
        )
      )
        continue;
      blocked.add(node.id);
      if (
        !this.canSend ||
        this.uploads.has(node.id) ||
        this.uploadErrors.has(node.id) ||
        this.uploads.size >= 2
      )
        continue;
      const abort = new AbortController();
      this.uploads.set(node.id, { source, abort });
      const input = { ...this.draft, nodes: [node], connections: [] };
      void publish(input, this.config.projectID, this.receipts, abort.signal)
        .then((result) => {
          if (this.disposed || abort.signal.aborted) return;
          const published = result.nodes[0];
          this.draft = {
            ...this.draft,
            nodes: this.draft.nodes.map((current) => {
              if (current.id !== node.id || localMedia(current) !== source)
                return current;
              return {
                ...current,
                documentData: {
                  ...(current.documentData ?? published.documentData!),
                  fileReferences: published.documentData?.fileReferences,
                },
              };
            }),
          };
          if (this.stable) this.surface = this.replacement = this.draft;
        })
        .catch((cause: unknown) => {
          if (!this.disposed && !abort.signal.aborted)
            this.uploadErrors.set(
              node.id,
              cause instanceof Error ? cause.message : "素材上传未完成",
            );
        })
        .finally(() => {
          if (this.disposed) return;
          if (this.uploads.get(node.id)?.abort !== abort) return;
          this.uploads.delete(node.id);
          this.notify();
          this.schedule();
        });
    }
    // Do not let an unpublishable media node hold up independent completed edits.
    const nodes = this.draft.nodes.flatMap((node) => {
      if (!blocked.has(node.id)) return [node];
      const original = this.accepted.document.nodes.find(
        (n) => n.id === node.id,
      );
      return original ? [original] : [];
    });
    const ids = new Set(nodes.map((n) => n.id));
    return {
      ...this.draft,
      nodes,
      connections: this.draft.connections.filter(
        (e) => ids.has(e.fromNodeId) && ids.has(e.toNodeId),
      ),
    };
  }
  private async drain() {
    if (
      this.disposed ||
      this.pending ||
      !this.canSend ||
      !this.canEdit ||
      !this.stable ||
      this.error
    )
      return;
    let target: CanvasEditorState;
    let operations: Operation[];
    try {
      target = this.prepareMedia();
      operations = serviceGraphOperations(this.accepted.document, target);
      if (
        new TextEncoder().encode(JSON.stringify(operations)).byteLength >
        (this.config.maxSubmissionBytes ?? 900 * 1024)
      )
        throw new Error(
          "这次修改超过单次保存容量，本页输入已保留，请缩小内容后重新应用",
        );
    } catch (cause) {
      this.error = cause instanceof Error ? cause.message : "编辑无法保存";
      this.notify();
      return;
    }
    if (!operations.length) {
      if (!this.changed()) this.queued = 0;
      this.notify();
      return;
    }
    this.pending = true;
    this.queued = 0;
    this.notify();
    try {
      const immutable = structuredClone(operations);
      const saved = this.reviewedRevision
        ? await this.config.commit(immutable, this.reviewedRevision, true)
        : await this.config.commit(immutable, this.accepted.revision);
      if (this.disposed) return;
      if (BigInt(saved.revision) >= BigInt(this.authority.revision))
        this.authority = saved;
      const current = this.authority;
      // Preserve every successor change; a receipt only acknowledges its frozen target.
      const next = rebaseCanvas(target, this.draft, current.document);
      this.accepted = current;
      this.draft = next;
      if (this.stable) this.surface = this.replacement = next;
    } catch (cause) {
      if (this.disposed) return;
      this.error =
        cause instanceof Error ? cause.message : "保存未完成，本页输入已保留";
    } finally {
      if (!this.disposed) {
        this.pending = false;
        this.reviewedRevision = undefined;
        this.notify();
        this.schedule();
      }
    }
  }
  retry() {
    if (this.disposed || this.pending || this.error) return;
    this.uploadErrors.clear();
    this.notify();
    this.schedule();
  }
  reviewRecovery(): CanvasRecoveryReview {
    if (!this.error || this.pending || !this.canEdit)
      throw new Error("请先完成原保存结果查证并确认编辑权限");
    const document = rebaseCanvas(
      this.accepted.document,
      this.draft,
      this.authority.document,
      true,
    );
    serviceGraphOperations(this.authority.document, document);
    return {
      revision: this.authority.revision,
      source: this.draft,
      current: this.authority.document,
      document,
    };
  }

  applyRecovery(review: CanvasRecoveryReview) {
    if (this.disposed || this.pending || !this.canEdit || !this.error)
      throw new Error("当前不能重新应用输入");
    if (
      review.revision !== this.authority.revision ||
      review.source !== this.draft
    )
      throw new Error("内容已更新，请重新查看差异后再应用");
    const error = this.error;
    this.error = "";
    // Check against the reviewed authority, not a rejected historical baseline.
    const previous = this.accepted;
    const queued = this.queued;
    this.queued = 0;
    this.accepted = this.authority;
    this.notify();
    if (!this.canAccept(review.document)) {
      this.accepted = previous;
      this.queued = queued;
      this.error = error;
      this.notify();
      throw new Error("恢复内容超过容量，请保留本页并缩小单次修改");
    }
    this.reviewedRevision = review.revision;
    this.draft = this.surface = this.replacement = review.document;
    this.queued = 0;
    this.stable = true;
    this.notify();
    this.schedule();
  }
  useAccepted() {
    if (this.disposed || this.pending) return;
    for (const upload of this.uploads.values()) upload.abort.abort();
    this.uploads.clear();
    this.uploadErrors.clear();
    this.receipts.clear();
    this.draft = this.surface = this.replacement = this.authority.document;
    this.accepted = this.authority;
    this.reviewedRevision = undefined;
    this.error = "";
    this.queued = 0;
    this.stable = true;
    this.notify();
  }
  dispose() {
    this.disposed = true;
    for (const upload of this.uploads.values()) upload.abort.abort();
    this.uploads.clear();
    this.receipts.clear();
    this.listeners.clear();
  }
}
