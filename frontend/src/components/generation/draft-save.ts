import {
  RequestError,
  getSessionGeneration,
  getScopeGeneration,
  getCSRFToken,
  sessionCancellation,
} from "@/lib/request";
import { getSession, listMySpaces } from "@/api/identity";
import { readPromptDraft } from "@/api/promptEditor";
import { requireSession } from "@/components/auth/contracts";
import type { ComposerSnapshot } from "./composer-session";
import {
  emptyPromptDraft,
  validatePromptDraft,
  type PromptDraft,
} from "./prompt-document";
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
  inputRecovery?: true;
  inputScope?: "space";
  ownerState?: unknown;
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
      saved?: SavedDraft;
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
        ackedSeq: options.saved ? options.snapshot.localSeq : -1,
      },
    );
    this.saved = options.saved;
    if (options.saved) this.status = "saved";
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
    if (this.disposed) return Promise.resolve();
    this.branch.localRecordRevision++;
    const value = structuredClone(this.branch);
    this.journal = this.journal
      .catch(() => {})
      .then(() => (this.disposed ? undefined : this.options.persist(value)))
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
      if (this.disposed) throw new Error("owner_disposed");
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
      if (this.disposed) throw new Error("owner_disposed");
      try {
        const saved = await this.options.send(structuredClone(request));
        validatePromptDraft(saved.draft);
        if (
          saved.revision !== request.expectedRevision + 1 ||
          (request.draftId && saved.draft_id !== request.draftId) ||
          canonical(saved.draft) !== canonical(request.snapshot.draft)
        )
          throw new RequestError("invalid_receipt");
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
  rebase(saved: SavedDraft) {
    if (this.flight) throw new Error("save_in_flight");
    this.saved = saved;
    this.branch.serverRevision = saved.revision;
    this.branch.draftId = saved.draft_id;
    this.branch.pendingSave = undefined;
    this.status = "dirty";
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
// A single noncredential fence in the existing store invalidates queued writes
// across connections. It is never a branch or proof of server authorization.
const fenceKey = ["", ""];
type JournalFence = {
  contextKey: "";
  replicaId: "";
  epoch: string;
  account: string | null;
  blockedSession?: string;
  activeSession?: string;
  mode?: "active" | "input-only" | "cleared";
};
let lease: JournalFence | null = null;
let cleanupEpoch = 0;
// Keep failed strong intent stronger than a later passive cleanup in this process.
// A stored marker cannot prove that logout did not happen while storage was down.
// New-sid recovery also requires this module to have witnessed passive cleanup.
let pendingStrongClear = false;
let pendingLogout: string | undefined;
let passiveRecovery: { epoch: string; account: string } | undefined;
function inputOnly(branch: LocalBranch): LocalBranch | null {
  if (!branch.draftId || branch.contextKey.split("/").length !== 3) return null;
  try {
    validatePromptDraft(branch.snapshot.draft);
  } catch {
    return null;
  }
  const draft = emptyPromptDraft();
  draft.document.content = branch.snapshot.draft.document.content.map(
    (paragraph) => ({
      type: "paragraph",
      content: (paragraph.content ?? []).filter(
        (node) => node.type === "text" || node.type === "hardBreak",
      ),
    }),
  );
  return {
    inputRecovery: true,
    inputScope: branch.inputScope,
    contextKey: branch.contextKey,
    replicaId: branch.replicaId,
    localRecordRevision: branch.localRecordRevision + 1,
    snapshot: {
      contextKey: branch.contextKey,
      replicaId: branch.replicaId,
      localSeq: branch.snapshot.localSeq,
      draft,
    },
    serverRevision: branch.serverRevision,
    draftId: branch.draftId,
    ackedSeq: -1,
  };
}
export const branchJournal = {
  // An observed external identity change revokes eligibility immediately, even
  // when both the ensuing server read and durable cleanup are unavailable.
  invalidateObservedIdentity() {
    ++cleanupEpoch;
    lease = null;
    passiveRecovery = undefined;
    pendingStrongClear = true;
  },
  async checkpoint() {
    return transaction<string | null>("readonly", (store, set) => {
      const req = store.get(fenceKey);
      req.onsuccess = () => set(req.result?.epoch ?? null);
    });
  },
  /** The Provider captures the durable fence before obtaining server proof. */
  async activateAccount(
    account: string,
    sessionID: string,
    generation: number,
    checkpoint?: string | null,
  ) {
    const epoch = cleanupEpoch;
    if (passiveRecovery?.account !== account) passiveRecovery = undefined;
    if (checkpoint === undefined) {
      checkpoint = await branchJournal.checkpoint();
      const identity = requireSession(await getSession());
      if (identity.actor.id !== account || identity.session_id !== sessionID)
        throw sessionCancellation();
    }
    const next = await transaction<JournalFence>("readwrite", (store, set) => {
      const req = store.getAll();
      req.onsuccess = () => {
        if (generation !== getSessionGeneration() || epoch !== cleanupEpoch) {
          store.transaction.abort();
          return;
        }
        const values = req.result as (LocalBranch | JournalFence)[];
        const previous = values.find((v) => v.contextKey === "") as
          JournalFence | undefined;
        if (
          (previous?.epoch ?? null) !== checkpoint ||
          previous?.blockedSession === sessionID ||
          pendingLogout === sessionID
        ) {
          store.transaction.abort();
          return;
        }
        const fence: JournalFence = {
          contextKey: "",
          replicaId: "",
          epoch: previous?.epoch ?? crypto.randomUUID(),
          account,
          activeSession: sessionID,
          mode: "active",
        };
        const sameSession =
          !pendingStrongClear &&
          previous?.mode === "active" &&
          previous.account === account &&
          previous.activeSession === sessionID;
        const recoverInput =
          !pendingStrongClear &&
          previous?.mode === "input-only" &&
          passiveRecovery?.epoch === previous.epoch &&
          passiveRecovery.account === account &&
          !previous.blockedSession;
        if (!sameSession) fence.epoch = crypto.randomUUID();
        for (const value of values) {
          if (!value.contextKey) continue;
          const branch = value as LocalBranch;
          if (value.contextKey.startsWith(`${account}/`)) {
            if (sameSession) continue;
            // Old active/legacy rows are not evidence that cleanup committed.
            const input =
              recoverInput && branch.inputRecovery ? inputOnly(branch) : null;
            if (input) {
              store.put(input);
              continue;
            }
          }
          store.delete([value.contextKey, value.replicaId]);
        }
        store.put(fence);
        set(fence);
      };
    });
    if (generation !== getSessionGeneration() || epoch !== cleanupEpoch)
      throw sessionCancellation();
    lease = next;
    pendingStrongClear = false;
    pendingLogout = undefined;
    passiveRecovery = undefined;
  },
  async write(branch: LocalBranch) {
    const expected = lease;
    const generation = getSessionGeneration(),
      scope = getScopeGeneration();
    if (!expected || !branch.contextKey.startsWith(`${expected.account}/`))
      throw sessionCancellation();
    await transaction<void>("readwrite", (store) => {
      const fence = store.get(fenceKey);
      fence.onsuccess = () => {
        if (
          lease !== expected ||
          generation !== getSessionGeneration() ||
          scope !== getScopeGeneration() ||
          fence.result?.epoch !== expected.epoch ||
          fence.result?.account !== expected.account
        ) {
          store.transaction.abort();
          return;
        }
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
      };
    });
  },
  async list(contextKey: string) {
    const expected = lease;
    const generation = getSessionGeneration(),
      scope = getScopeGeneration();
    if (
      !expected ||
      !getCSRFToken() ||
      !contextKey.startsWith(`${expected.account}/`)
    )
      return [];
    const read = () =>
      transaction<LocalBranch[]>("readonly", (store, set) => {
        const req = store.getAll();
        req.onsuccess = () => {
          const values = req.result as (LocalBranch | JournalFence)[];
          const fence = values.find((v) => v.contextKey === "") as
            JournalFence | undefined;
          if (
            lease !== expected ||
            generation !== getSessionGeneration() ||
            scope !== getScopeGeneration() ||
            fence?.epoch !== expected.epoch ||
            fence.account !== expected.account
          ) {
            set([]);
            return;
          }
          set(
            values.filter((b) => b.contextKey === contextKey) as LocalBranch[],
          );
        };
      });
    const found = await read();
    if (!found.some((b) => b.inputRecovery)) return found;
    const [account, project, draft] = contextKey.split("/");
    const identity = requireSession(await getSession());
    if (identity.actor.id !== account) return [];
    try {
      if (
        found.every((branch) => branch.inputScope === "space") &&
        draft === "home-input"
      ) {
        const spaces = await listMySpaces();
        if (
          !spaces.items?.some((space) => space.id === project && space.writable)
        )
          throw new RequestError("forbidden", 403);
      } else {
        const saved = requireSaved(
          await readPromptDraft({ pid: project, id: draft }),
        );
        if (saved.draft_id !== draft) throw new RequestError("invalid_receipt");
      }
    } catch (error) {
      if (error instanceof RequestError && [403, 404].includes(error.status)) {
        for (const branch of found) await branchJournal.remove(branch);
        return [];
      }
      throw error;
    }
    // Permission reads may race logout, scope switches or another connection's clear.
    return read();
  },
  async remove(branch: LocalBranch) {
    const expected = lease;
    const generation = getSessionGeneration(),
      scope = getScopeGeneration();
    if (!expected || !branch.contextKey.startsWith(`${expected.account}/`))
      return false;
    return transaction<boolean>("readwrite", (store, set) => {
      const fence = store.get(fenceKey);
      fence.onsuccess = () => {
        if (
          lease !== expected ||
          generation !== getSessionGeneration() ||
          scope !== getScopeGeneration() ||
          fence.result?.epoch !== expected.epoch ||
          fence.result?.account !== expected.account
        ) {
          set(false);
          return;
        }
        const req = store.get([branch.contextKey, branch.replicaId]);
        req.onsuccess = () => {
          if (req.result?.localRecordRevision !== branch.localRecordRevision) {
            set(false);
            return;
          }
          store.delete([branch.contextKey, branch.replicaId]);
          set(true);
        };
      };
    });
  },
  async clear(options?: {
    preserveInput?: boolean;
    logoutSession?: string;
    retainWriter?: boolean;
  }) {
    const expected = lease;
    const recoveryAccount = expected?.account ?? passiveRecovery?.account;
    const generation = getSessionGeneration();
    const epoch = ++cleanupEpoch;
    lease = null;
    if (!options?.preserveInput) {
      pendingStrongClear = true;
      passiveRecovery = undefined;
    }
    if (options?.logoutSession) pendingLogout = options.logoutSession;
    window.dispatchEvent(new Event("glowloom:private-cache-cleared"));
    if (typeof BroadcastChannel !== "undefined") {
      const channel = new BroadcastChannel("glowloom-private-cache");
      channel.postMessage("clear");
      channel.close();
    }
    const next = await transaction<JournalFence>("readwrite", (store, set) => {
      const req = store.getAll();
      req.onsuccess = () => {
        const values = req.result as LocalBranch[];
        const previous = values.find((v) => v.contextKey === "") as
          JournalFence | undefined;
        store.clear();
        const keepWriter =
          options?.retainWriter &&
          expected &&
          previous?.epoch === expected.epoch &&
          previous.account === expected.account &&
          previous.mode === "active" &&
          !previous.blockedSession &&
          generation === getSessionGeneration();
        const fence: JournalFence = {
          contextKey: "",
          replicaId: "",
          epoch: crypto.randomUUID(),
          account: keepWriter ? expected.account : null,
          blockedSession: pendingLogout ?? previous?.blockedSession,
          activeSession: keepWriter ? expected.activeSession : undefined,
          mode: keepWriter
            ? "active"
            : options?.preserveInput &&
                !pendingStrongClear &&
                !previous?.blockedSession &&
                ((previous?.mode === "active" &&
                  expected?.epoch === previous.epoch &&
                  expected.account === previous.account &&
                  expected.activeSession === previous.activeSession) ||
                  (previous?.mode === "input-only" &&
                    passiveRecovery?.epoch === previous.epoch))
              ? "input-only"
              : "cleared",
        };
        store.put(fence);
        set(fence);
        if (fence.mode === "input-only")
          for (const value of values) {
            if (!value.contextKey) continue;
            const input = inputOnly(value);
            if (input) store.put(input);
          }
      };
    });
    if (epoch === cleanupEpoch) {
      pendingStrongClear = false;
      pendingLogout = undefined;
      passiveRecovery =
        next.mode === "input-only" && recoveryAccount
          ? { epoch: next.epoch, account: recoveryAccount }
          : undefined;
    }
    if (
      options?.retainWriter &&
      next.account &&
      !next.blockedSession &&
      generation === getSessionGeneration() &&
      epoch === cleanupEpoch
    )
      lease = next;
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
