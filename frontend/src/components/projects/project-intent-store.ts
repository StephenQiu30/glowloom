import { getSessionGeneration, getScopeGeneration } from "@/lib/request";
import { isProjectID } from "./project";
import {
  requireCreationInput,
  requireCreationIntent,
  type ProjectCreationContext,
  type ProjectCreationIntent,
  type IntentPhase,
  type IntentTarget,
} from "./project-creation-state";

export type IntentDatabase = { fence: number; intents: unknown[] };
export interface IntentTransactions {
  transact<T>(write: boolean, run: (state: IntentDatabase) => T): Promise<T>;
}

class NativeIntentTransactions implements IntentTransactions {
  private open(): Promise<IDBDatabase> {
    return new Promise((resolve, reject) => {
      if (typeof indexedDB === "undefined") {
        reject(new Error("creation_storage_unavailable"));
        return;
      }
      let blocked = false;
      const request = indexedDB.open("glowloom-project-intents", 1);
      const timeout = setTimeout(() => {
        blocked = true;
        reject(new Error("creation_storage_blocked"));
      }, 5000);
      request.onupgradeneeded = () => {
        if (blocked) {
          request.transaction?.abort();
          return;
        }
        const db = request.result;
        db.createObjectStore("intents", { keyPath: ["actorId", "scopeId"] });
        db.createObjectStore("fence");
      };
      request.onerror = () => {
        clearTimeout(timeout);
        reject(request.error ?? new Error("creation_storage_unavailable"));
      };
      request.onblocked = () => {
        clearTimeout(timeout);
        blocked = true;
        reject(new Error("creation_storage_blocked"));
      };
      request.onsuccess = () => {
        clearTimeout(timeout);
        if (blocked) {
          request.result.close();
          return;
        }
        request.result.onversionchange = () => request.result.close();
        resolve(request.result);
      };
    });
  }
  async transact<T>(
    write: boolean,
    run: (state: IntentDatabase) => T,
  ): Promise<T> {
    const db = await this.open();
    try {
      return await new Promise<T>((resolve, reject) => {
        const tx = db.transaction(
          ["intents", "fence"],
          write ? "readwrite" : "readonly",
        );
        const records = tx.objectStore("intents"),
          fence = tx.objectStore("fence");
        const list = records.getAll(),
          epoch = fence.get("epoch");
        let completed = 0,
          result: T,
          error: unknown;
        const ready = () => {
          if (++completed !== 2) return;
          try {
            const state: IntentDatabase = {
              fence: epoch.result ?? 0,
              intents: list.result,
            };
            if (!Number.isSafeInteger(state.fence) || state.fence < 0)
              throw new Error("creation_storage_corrupt");
            result = run(state);
            if (write) {
              records.clear();
              for (const record of state.intents) records.put(record);
              fence.put(state.fence, "epoch");
            }
          } catch (reason) {
            error = reason;
            tx.abort();
          }
        };
        list.onsuccess = ready;
        epoch.onsuccess = ready;
        tx.oncomplete = () => resolve(result);
        tx.onabort = tx.onerror = () =>
          reject(error ?? tx.error ?? new Error("creation_storage_failed"));
      });
    } finally {
      db.close();
    }
  }
}

type Generations = Pick<
  ProjectCreationContext,
  "sessionGeneration" | "scopeGeneration"
>;
type CleanupMode = "active_logout" | "identity_change" | "passive_expiry";
type Acknowledgement = "opened" | "target_unavailable" | "validation_rejected";
export class ProjectIntentStore {
  private active: ProjectCreationContext | null = null;
  private pendingStrongClear = false;
  private lifecycle = 0;
  private strongClearSequence = 0;
  constructor(
    private backend: IntentTransactions = new NativeIntentTransactions(),
    private generations: () => Generations = () => ({
      sessionGeneration: getSessionGeneration(),
      scopeGeneration: getScopeGeneration(),
    }),
  ) {}
  checkpoint() {
    return this.backend.transact(false, (state) => state.fence);
  }
  async activate(context: ProjectCreationContext) {
    const lifecycle = this.lifecycle;
    if (this.pendingStrongClear) {
      await this.invalidate("identity_change");
      throw new Error("creation_context_changed");
    }
    if (!isProjectID(context.actorId) || !isProjectID(context.scopeId))
      throw new Error("creation_context_changed");
    const current = this.generations();
    if (
      current.sessionGeneration !== context.sessionGeneration ||
      current.scopeGeneration !== context.scopeGeneration
    )
      throw new Error("creation_context_changed");
    await this.backend.transact(false, (state) => {
      if (state.fence !== context.fenceEpoch)
        throw new Error("creation_context_changed");
    });
    const after = this.generations();
    if (
      lifecycle !== this.lifecycle ||
      after.sessionGeneration !== context.sessionGeneration ||
      after.scopeGeneration !== context.scopeGeneration
    )
      throw new Error("creation_context_changed");
    this.active = { ...context };
  }
  isActive(context: ProjectCreationContext) {
    const active = this.active,
      current = this.generations();
    return (
      !this.pendingStrongClear &&
      !!active &&
      active.actorId === context.actorId &&
      active.scopeId === context.scopeId &&
      active.fenceEpoch === context.fenceEpoch &&
      active.sessionGeneration === context.sessionGeneration &&
      active.scopeGeneration === context.scopeGeneration &&
      current.sessionGeneration === context.sessionGeneration &&
      current.scopeGeneration === context.scopeGeneration
    );
  }
  private assert(context: ProjectCreationContext, state: IntentDatabase) {
    if (!this.isActive(context) || state.fence !== context.fenceEpoch)
      throw new Error("creation_context_changed");
  }

  private record(
    state: IntentDatabase,
    context: ProjectCreationContext,
    key?: string,
    revision?: number,
  ) {
    this.assert(context, state);
    const raw = state.intents.find((value) => {
      const i = value as Partial<ProjectCreationIntent> | null;
      return i?.actorId === context.actorId && i.scopeId === context.scopeId;
    });
    if (!raw) return null;
    const intent = requireCreationIntent(raw);
    if (
      intent.fenceEpoch !== state.fence ||
      (key !== undefined && intent.key !== key) ||
      (revision !== undefined && intent.localRevision !== revision)
    )
      throw new Error("creation_intent_conflict");
    return intent;
  }
  private put(state: IntentDatabase, intent: ProjectCreationIntent) {
    const index = state.intents.findIndex((value) => {
      const i = value as Partial<ProjectCreationIntent>;
      return i.actorId === intent.actorId && i.scopeId === intent.scopeId;
    });
    if (index < 0) state.intents.push(intent);
    else state.intents[index] = intent;
  }
  beginOrResume(
    context: ProjectCreationContext,
    body: API.CreateProjectRequest,
  ) {
    return this.backend.transact(true, (state) => {
      const existing = this.record(state, context);
      if (existing)
        return { intent: existing, disposition: "existing" as const };
      const now = new Date().toISOString();
      const intent: ProjectCreationIntent = {
        actorId: context.actorId,
        scopeId: context.scopeId,
        contractVersion: 1,
        key: crypto.randomUUID(),
        body: requireCreationInput(body),
        phase: "prepared",
        projectId: null,
        canvasId: null,
        hasUncertainDispatch: false,
        localRevision: 1,
        fenceEpoch: state.fence,
        createdAt: now,
        updatedAt: now,
      };
      this.put(state, intent);
      return { intent: structuredClone(intent), disposition: "new" as const };
    });
  }
  readForActor(context: ProjectCreationContext) {
    return this.backend.transact(false, (state) => {
      this.assert(context, state);
      return state.intents
        .filter(
          (value) =>
            (value as Partial<ProjectCreationIntent>)?.actorId ===
            context.actorId,
        )
        .map(requireCreationIntent);
    });
  }
  beginDispatch(
    context: ProjectCreationContext,
    key: string,
    revision: number,
    source: "initial" | "explicit_continuation",
  ) {
    return this.backend.transact(true, (state) => {
      const intent = this.record(state, context, key, revision);
      if (
        !intent ||
        !(source === "initial"
          ? intent.phase === "prepared"
          : intent.phase === "prepared" || intent.phase === "unknown")
      )
        throw new Error("creation_intent_conflict");
      intent.phase = "submitting";
      if (source === "explicit_continuation")
        intent.hasUncertainDispatch = true;
      intent.localRevision++;
      intent.updatedAt = new Date().toISOString();
      this.put(state, intent);
      return intent;
    });
  }
  transition(
    context: ProjectCreationContext,
    key: string,
    revision: number,
    phase: IntentPhase,
    target?: IntentTarget,
  ) {
    return this.backend.transact(true, (state) => {
      const intent = this.record(state, context, key, revision);
      if (!intent) throw new Error("creation_intent_conflict");
      const created =
        phase === "created" &&
        ["prepared", "submitting", "unknown", "created"].includes(intent.phase);
      const unknown =
        phase === "unknown" && ["submitting", "unknown"].includes(intent.phase);
      const rejected =
        phase === "rejected" &&
        intent.phase === "submitting" &&
        !intent.hasUncertainDispatch;
      if (!created && !unknown && !rejected)
        throw new Error("creation_intent_conflict");
      if (created) {
        if (
          !target ||
          !isProjectID(target.projectId) ||
          !isProjectID(target.canvasId) ||
          (intent.phase === "created" &&
            (intent.projectId !== target.projectId ||
              intent.canvasId !== target.canvasId))
        )
          throw new Error("creation_intent_conflict");
        intent.projectId = target.projectId;
        intent.canvasId = target.canvasId;
      }
      if (unknown) intent.hasUncertainDispatch = true;
      intent.phase = phase;
      intent.localRevision++;
      intent.updatedAt = new Date().toISOString();
      this.put(state, intent);
      return intent;
    });
  }
  acknowledge(
    context: ProjectCreationContext,
    key: string,
    revision: number,
    reason: Acknowledgement,
  ) {
    return this.backend.transact(true, (state) => {
      const intent = this.record(state, context, key, revision);
      if (
        !intent ||
        !(
          reason === "target_unavailable" ||
          (reason === "opened" && intent.phase === "created") ||
          (reason === "validation_rejected" &&
            intent.phase === "rejected" &&
            !intent.hasUncertainDispatch)
        )
      )
        throw new Error("creation_intent_conflict");
      state.intents = state.intents.filter((value) => {
        const i = value as Partial<ProjectCreationIntent>;
        return i.actorId !== intent.actorId || i.scopeId !== intent.scopeId;
      });
    });
  }
  async invalidate(mode: CleanupMode) {
    ++this.lifecycle;
    this.active = null;
    if (mode === "passive_expiry") return;
    const sequence = ++this.strongClearSequence;
    this.pendingStrongClear = true;
    await this.backend.transact(true, (state) => {
      state.fence++;
      state.intents = [];
    });
    if (sequence === this.strongClearSequence) this.pendingStrongClear = false;
  }
  forgetRevokedScopes(context: ProjectCreationContext, allowed: string[]) {
    return this.backend.transact(true, (state) => {
      this.assert(context, state);
      if (
        !allowed.includes(context.scopeId) ||
        allowed.some((id) => !isProjectID(id))
      )
        throw new Error("creation_context_changed");
      state.intents = state.intents.filter((value) => {
        const i = value as Partial<ProjectCreationIntent>;
        return (
          i.actorId !== context.actorId ||
          (typeof i.scopeId === "string" && allowed.includes(i.scopeId))
        );
      });
    });
  }
}

export const projectIntentStore = new ProjectIntentStore();
