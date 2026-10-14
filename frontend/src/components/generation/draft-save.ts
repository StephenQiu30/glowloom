import { RequestError } from "@/lib/request";
import type { ComposerSnapshot } from "./composer-session";
import { validatePromptDraft, type PromptDraft } from "./prompt-document";
export type SavedDraft = {
  draft_id: string;
  revision: number;
  draft_hash: string;
  input_hash: string;
  draft: PromptDraft;
};
export type SaveRequest = {
  key: string;
  draftId?: string;
  expectedRevision: number;
  snapshot: ComposerSnapshot;
};
export type LocalBranch = {
  contextKey: string;
  replicaId: string;
  localRecordRevision: number;
  snapshot: ComposerSnapshot;
  serverRevision: number;
  draftId?: string;
  ackedSeq: number;
  pendingSave?: SaveRequest;
  pendingSubmission?: {
    key: string;
    body: API.QuoteItemRequest;
    confirmRequested?: boolean;
    confirmKey?: string;
  };
};
export function requireSaved(value: API.DraftSnapshot): SavedDraft {
  validatePromptDraft(value.draft);
  if (
    !value.draft_id ||
    !Number.isSafeInteger(value.revision) ||
    value.revision! < 1 ||
    !value.draft_hash ||
    !value.input_hash
  )
    throw new RequestError("invalid_receipt");
  return value as SavedDraft;
}
/** Owns one writer. A response advances acknowledgements, never replaces live editor intent. */
export class DraftSaver {
  branch: LocalBranch;
  status: "dirty" | "saving" | "saved" | "unknown" | "conflict" = "dirty";
  localError = false;
  private flight: Promise<SavedDraft> | null = null;
  private journal = Promise.resolve();
  private saved?: SavedDraft;
  private disposed = false;
  constructor(
    private options: {
      snapshot: ComposerSnapshot;
      revision: number;
      draftId?: string;
      branch?: LocalBranch;
      send: (r: SaveRequest) => Promise<SavedDraft>;
      persist: (b: LocalBranch) => Promise<void>;
      changed?: () => void;
    },
  ) {
    this.branch = structuredClone(
      options.branch ?? {
        contextKey: options.snapshot.contextKey,
        replicaId: options.snapshot.replicaId,
        localRecordRevision: 0,
        snapshot: options.snapshot,
        serverRevision: options.revision,
        draftId: options.draftId,
        ackedSeq: -1,
      },
    );
  }
  edit(snapshot: ComposerSnapshot) {
    if (
      this.disposed ||
      snapshot.contextKey !== this.branch.contextKey ||
      snapshot.replicaId !== this.branch.replicaId ||
      snapshot.localSeq < this.branch.snapshot.localSeq
    )
      return;
    if (
      snapshot.localSeq === this.branch.snapshot.localSeq &&
      canonical(snapshot.draft) === canonical(this.branch.snapshot.draft)
    )
      return;
    this.branch.snapshot = structuredClone(snapshot);
    if (this.status === "saved") this.status = "dirty";
    void this.persist();
    this.options.changed?.();
  }
  private persist() {
    this.branch.localRecordRevision++;
    const value = structuredClone(this.branch);
    this.journal = this.journal
      .catch(() => {})
      .then(() => this.options.persist(value))
      .catch(() => {
        this.localError = true;
        this.options.changed?.();
      });
    return this.journal;
  }
  setSubmission(pending: LocalBranch["pendingSubmission"]) {
    this.branch.pendingSubmission = pending;
    return this.persist();
  }
  flush(): Promise<SavedDraft> {
    if (this.flight) return this.flight;
    if (this.disposed) return Promise.reject(new Error("owner_disposed"));
    if (this.status === "conflict")
      return Promise.reject(new Error("revision_conflict"));
    this.flight = this.drain().finally(() => {
      this.flight = null;
    });
    return this.flight;
  }
  private async drain(): Promise<SavedDraft> {
    while (
      this.branch.pendingSave ||
      this.branch.ackedSeq !== this.branch.snapshot.localSeq ||
      !this.saved
    ) {
      const request = this.branch.pendingSave ?? {
        key: crypto.randomUUID(),
        draftId: this.branch.draftId,
        expectedRevision: this.branch.serverRevision,
        snapshot: structuredClone(this.branch.snapshot),
      };
      this.branch.pendingSave = request;
      this.status = "saving";
      await this.persist();
      this.options.changed?.();
      try {
        const saved = await this.options.send(structuredClone(request));
        validatePromptDraft(saved.draft);
        if (
          saved.revision !== request.expectedRevision + 1 ||
          (request.draftId && saved.draft_id !== request.draftId) ||
          JSON.stringify(saved.draft) !== JSON.stringify(request.snapshot.draft)
        ) {
          // JSON object key ordering is not semantically significant.
          if (
            saved.revision !== request.expectedRevision + 1 ||
            canonical(saved.draft) !== canonical(request.snapshot.draft)
          )
            throw new RequestError("invalid_receipt");
        }
        this.saved = saved;
        this.branch.draftId = saved.draft_id;
        this.branch.serverRevision = saved.revision;
        this.branch.ackedSeq = request.snapshot.localSeq;
        this.branch.pendingSave = undefined;
        this.status =
          this.branch.ackedSeq === this.branch.snapshot.localSeq
            ? "saved"
            : "dirty";
        await this.persist();
        this.options.changed?.();
      } catch (error) {
        const definite =
          error instanceof RequestError &&
          error.status >= 400 &&
          error.status < 500;
        this.status = definite ? "conflict" : "unknown";
        if (definite) this.branch.pendingSave = undefined;
        await this.persist();
        this.options.changed?.();
        throw error;
      }
    }
    return this.saved!;
  }
  dispose() {
    this.disposed = true;
  }
}
export function canonical(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(canonical).join(",")}]`;
  if (value !== null && typeof value === "object")
    return `{${Object.entries(value)
      .filter(([, v]) => v !== undefined)
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([k, v]) => `${JSON.stringify(k)}:${canonical(v)}`)
      .join(",")}}`;
  return JSON.stringify(value);
}
const dbName = "glowloom-prompt-branches";
async function database() {
  return new Promise<IDBDatabase>((resolve, reject) => {
    const request = indexedDB.open(dbName, 1);
    request.onupgradeneeded = () =>
      request.result.createObjectStore("branches", {
        keyPath: ["contextKey", "replicaId"],
      });
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });
}
async function transaction<T>(
  mode: IDBTransactionMode,
  run: (store: IDBObjectStore, set: (value: T) => void) => void,
): Promise<T> {
  const db = await database();
  try {
    return await new Promise<T>((resolve, reject) => {
      const tx = db.transaction("branches", mode);
      let result: T;
      tx.oncomplete = () => resolve(result);
      tx.onerror = () => reject(tx.error);
      tx.onabort = () => reject(tx.error ?? new Error("local_record_conflict"));
      run(tx.objectStore("branches"), (value) => (result = value));
    });
  } finally {
    db.close();
  }
}
export const branchJournal = {
  async write(branch: LocalBranch) {
    await transaction<void>("readwrite", (store) => {
      const request = store.get([branch.contextKey, branch.replicaId]);
      request.onsuccess = () => {
        const previous = request.result as LocalBranch | undefined;
        if (
          previous &&
          previous.localRecordRevision >= branch.localRecordRevision
        ) {
          store.transaction.abort();
          return;
        }
        store.put(branch);
      };
    });
  },
  async list(contextKey: string) {
    return transaction<LocalBranch[]>("readonly", (store, set) => {
      const req = store.getAll();
      req.onsuccess = () =>
        set(
          (req.result as LocalBranch[]).filter(
            (b) => b.contextKey === contextKey,
          ),
        );
    });
  },
  async remove(branch: LocalBranch) {
    return transaction<boolean>("readwrite", (store, set) => {
      const req = store.get([branch.contextKey, branch.replicaId]);
      req.onsuccess = () => {
        if (req.result?.localRecordRevision !== branch.localRecordRevision) {
          set(false);
          return;
        }
        store.delete([branch.contextKey, branch.replicaId]);
        set(true);
      };
    });
  },
  async clear() {
    await transaction<void>("readwrite", (store) => {
      store.clear();
    });
  },
};

// Swag's optional string maps nil pointers to undefined on write; the server returns explicit nulls.
export function draftRequest(draft: PromptDraft): API.PromptDraft {
  return {
    ...draft,
    mode: draft.mode ?? undefined,
    modelVersion: draft.modelVersion ?? undefined,
  };
}
