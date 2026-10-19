import { RequestError } from "@/lib/request";
import { isProjectID } from "./project";
import { ProjectIntentStore } from "./project-intent-store";
import {
  blankProjectInput,
  type ProjectCreationContext,
  type ProjectCreationIntent,
  type ProjectCreationState,
  type IntentTarget,
} from "./project-creation-state";

export function requireCreationRecovery(value: unknown) {
  const result = value as API.ProjectCreationRecoveryResponse | null;
  if (
    result?.state === "unrecorded" &&
    result.project_id === null &&
    result.canvas_id === null &&
    result.project_status === null
  )
    return { state: "unrecorded" as const };
  if (
    result?.state === "created" &&
    isProjectID(result.project_id) &&
    isProjectID(result.canvas_id) &&
    ["active", "archived", "copying"].includes(result.project_status ?? "")
  )
    return {
      state: "created" as const,
      target: { projectId: result.project_id, canvasId: result.canvas_id },
      status: result.project_status!,
    };
  throw new RequestError("invalid_receipt");
}
type CreationPorts = {
  intents: ProjectIntentStore;
  context: () => Promise<ProjectCreationContext>;
  isCurrent: (context: ProjectCreationContext) => boolean;
  resolve: (key: string) => Promise<unknown>;
  create: (body: API.CreateProjectRequest, key: string) => Promise<unknown>;
  readTarget: (target: IntentTarget) => Promise<void>;
  navigate: (target: IntentTarget) => void;
  allowedScopes: () => string[];
};
const idle: ProjectCreationState = { phase: "idle" };

export class ProjectCreationController {
  private state: ProjectCreationState = idle;
  private listeners = new Set<() => void>();
  private sequence = 0;
  private activeRun: number | null = null;
  private disposed = false;
  private restoreFlight: Promise<void> | null = null;
  constructor(private ports: CreationPorts) {}
  snapshot = () => this.state;
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };
  resume() {
    this.disposed = false;
    this.activeRun = null;
    this.restoreFlight = null;
    this.sequence++;
  }
  dispose() {
    this.disposed = true;
    this.activeRun = null;
    this.restoreFlight = null;
    this.sequence++;
  }
  private publish(state: ProjectCreationState) {
    this.state = state;
    for (const listener of this.listeners) listener();
  }
  private assert(context: ProjectCreationContext, run: number) {
    if (
      this.disposed ||
      this.activeRun !== run ||
      !this.ports.isCurrent(context) ||
      !this.ports.intents.isActive(context)
    )
      throw new Error("creation_context_changed");
  }
  private async perform(action: (run: number) => Promise<void>) {
    if (this.disposed || this.activeRun !== null) return;
    const run = ++this.sequence;
    this.activeRun = run;
    try {
      await action(run);
    } catch (reason) {
      if (!this.disposed && this.activeRun === run)
        this.publish({ phase: "blocked", message: creationMessage(reason) });
    } finally {
      if (this.activeRun === run) this.activeRun = null;
    }
  }
  private async records(context: ProjectCreationContext, run: number) {
    await this.ports.intents.forgetRevokedScopes(
      context,
      this.ports.allowedScopes(),
    );
    this.assert(context, run);
    const records = await this.ports.intents.readForActor(context);
    this.assert(context, run);
    return records;
  }
  restore = () => {
    const flight = this.perform(async (run) => {
      const context = await this.ports.context();
      this.assert(context, run);
      const records = await this.records(context, run),
        intent = records.find((i) => i.scopeId === context.scopeId);
      if (intent) await this.lookup(context, intent, run, false, false);
      else if (records.length)
        this.publish({
          phase: "scope_required",
          scopeIds: records.map((i) => i.scopeId),
        });
    });
    this.restoreFlight = flight;
    void flight.then(() => {
      if (this.restoreFlight === flight) this.restoreFlight = null;
    });
    return flight;
  };
  start = async () => {
    const restoring = this.restoreFlight;
    if (restoring) {
      await restoring;
      if (this.state.phase !== "idle") return;
    }
    return this.perform(async (run) => {
      this.publish({ phase: "checking_context" });
      const context = await this.ports.context();
      this.assert(context, run);
      const records = await this.records(context, run);
      if (
        !records.some((i) => i.scopeId === context.scopeId) &&
        records.length
      ) {
        this.publish({
          phase: "scope_required",
          scopeIds: records.map((i) => i.scopeId),
        });
        return;
      }
      this.publish({ phase: "preparing" });
      const acquired = await this.ports.intents.beginOrResume(
        context,
        blankProjectInput(),
      );
      this.assert(context, run);
      await this.lookup(
        context,
        acquired.intent,
        run,
        acquired.disposition === "new",
      );
    });
  };
  resolve = () =>
    this.perform(async (run) => {
      const context = await this.ports.context();
      this.assert(context, run);
      const intent = (await this.records(context, run)).find(
        (i) => i.scopeId === context.scopeId,
      );
      if (intent) await this.lookup(context, intent, run, false);
    });
  continueOriginal = () =>
    this.perform(async (run) => {
      if (this.state.phase !== "unrecorded") return;
      const expected = this.state.intent,
        context = await this.ports.context();
      this.assert(context, run);
      const intent = (await this.records(context, run)).find(
        (i) => i.scopeId === context.scopeId,
      );
      if (
        !intent ||
        intent.key !== expected.key ||
        intent.localRevision !== expected.localRevision
      )
        throw new Error("creation_intent_conflict");
      await this.dispatch(context, intent, run, "explicit_continuation");
    });
  reopen = () =>
    this.perform(async (run) => {
      const snapshot = this.state;
      if (!("target" in snapshot)) return;
      const context = await this.ports.context();
      this.assert(context, run);
      const intent = (await this.records(context, run)).find(
        (i) => i.scopeId === context.scopeId,
      );
      if (!intent || intent.key !== snapshot.intentKey) {
        await this.ports.readTarget(snapshot.target);
        this.assert(context, run);
        this.ports.navigate(snapshot.target);
        this.publish({
          phase: "ready",
          target: snapshot.target,
          intentKey: snapshot.intentKey,
        });
        return;
      }
      await this.acceptTarget(context, intent, snapshot.target, run);
    });
  private async lookup(
    context: ProjectCreationContext,
    original: ProjectCreationIntent,
    run: number,
    initial: boolean,
    openImmediately = true,
  ) {
    let intent = original;
    if (intent.phase === "rejected") {
      await this.ports.intents.acknowledge(
        context,
        intent.key,
        intent.localRevision,
        "validation_rejected",
      );
      this.assert(context, run);
      this.publish({
        phase: "rejected",
        message: "原创建参数已被明确拒绝。再次点击新建画布可发起新的创建。",
      });
      return;
    }
    if (intent.phase === "submitting") {
      intent = await this.ports.intents.transition(
        context,
        intent.key,
        intent.localRevision,
        "unknown",
      );
      this.assert(context, run);
    }
    this.publish({ phase: initial ? "preflight" : "unknown", intent });
    try {
      const recovery = requireCreationRecovery(
        await this.ports.resolve(intent.key),
      );
      this.assert(context, run);
      if (recovery.state === "created") {
        if (recovery.status === "copying") {
          this.publish({
            phase: "blocked",
            message: "项目仍在准备中，请稍后确认原创建结果。",
          });
          return;
        }
        await this.acceptTarget(
          context,
          intent,
          recovery.target,
          run,
          openImmediately,
        );
        return;
      }
      if (intent.phase === "created") {
        this.publish({
          phase: "open_failed",
          target: { projectId: intent.projectId!, canvasId: intent.canvasId! },
          intentKey: intent.key,
          message: "暂未找到原创建回执，保留已知目标，仅重试读取。",
        });
        return;
      }
      if (initial) await this.dispatch(context, intent, run, "initial");
      else this.publish({ phase: "unrecorded", intent });
    } catch (reason) {
      this.assert(context, run);
      if (this.state.phase === "submitting") {
        this.publish({
          phase: "unknown",
          intent: this.state.intent,
          message: creationMessage(reason),
        });
        return;
      }
      if (
        reason instanceof RequestError &&
        reason.status === 410 &&
        reason.code === "creation_target_unavailable"
      ) {
        await this.ports.intents.acknowledge(
          context,
          intent.key,
          intent.localRevision,
          "target_unavailable",
        );
        this.assert(context, run);
        this.publish({
          phase: "blocked",
          message: "原画布已不可用。可以返回列表查看其他创作。",
        });
        return;
      }
      this.publish({
        phase: initial ? "preflight_failed" : "unknown",
        intent,
        message: creationMessage(reason),
      });
    }
  }
  private async dispatch(
    context: ProjectCreationContext,
    original: ProjectCreationIntent,
    run: number,
    source: "initial" | "explicit_continuation",
  ) {
    const intent = await this.ports.intents.beginDispatch(
      context,
      original.key,
      original.localRevision,
      source,
    );
    this.assert(context, run);
    this.publish({ phase: "submitting", intent });
    let raw: unknown;
    try {
      raw = await this.ports.create(structuredClone(intent.body), intent.key);
    } catch (reason) {
      this.assert(context, run);
      let current = (await this.records(context, run)).find(
        (i) => i.scopeId === context.scopeId,
      );
      if (!current || current.key !== intent.key) {
        this.publish({
          phase: "blocked",
          message: "原创建状态已由其他页面处理，请返回列表确认。",
        });
        return;
      }
      if (current.phase === "created") {
        await this.acceptTarget(
          context,
          current,
          { projectId: current.projectId!, canvasId: current.canvasId! },
          run,
        );
        return;
      }
      if (
        reason instanceof RequestError &&
        reason.status === 422 &&
        !current.hasUncertainDispatch &&
        current.phase === "submitting"
      ) {
        current = await this.ports.intents.transition(
          context,
          current.key,
          current.localRevision,
          "rejected",
        );
        this.assert(context, run);
        await this.ports.intents.acknowledge(
          context,
          current.key,
          current.localRevision,
          "validation_rejected",
        );
        this.assert(context, run);
        this.publish({
          phase: "rejected",
          message: "创建参数被拒绝，本次未创建。请稍后再次明确新建。",
        });
        return;
      }
      if (current.phase === "submitting")
        current = await this.ports.intents.transition(
          context,
          current.key,
          current.localRevision,
          "unknown",
        );
      this.assert(context, run);
      this.publish({
        phase: "unknown",
        intent: current,
        message: creationMessage(reason),
      });
      return;
    }
    this.assert(context, run);
    const receipt = raw as { id?: unknown; canvas_id?: unknown } | null;
    if (
      !receipt ||
      !isProjectID(receipt.id) ||
      !isProjectID(receipt.canvas_id)
    ) {
      const unknown = await this.ports.intents.transition(
        context,
        intent.key,
        intent.localRevision,
        "unknown",
      );
      this.assert(context, run);
      this.publish({
        phase: "unknown",
        intent: unknown,
        message: "创建回执不完整，请确认原创建结果。",
      });
      return;
    }
    await this.acceptTarget(
      context,
      intent,
      { projectId: receipt.id, canvasId: receipt.canvas_id },
      run,
    );
  }
  private async acceptTarget(
    context: ProjectCreationContext,
    original: ProjectCreationIntent,
    target: IntentTarget,
    run: number,
    openImmediately = true,
  ) {
    let intent = original;
    try {
      const latest = (await this.records(context, run)).find(
        (i) => i.scopeId === context.scopeId,
      );
      if (!latest || latest.key !== intent.key) {
        this.publish({
          phase: "open_failed",
          target,
          intentKey: original.key,
          message:
            "原创建已由其他页面处理。可只读确认并重新打开，不影响后续创建。",
        });
        return;
      }
      intent = latest;
      if (intent.phase !== "created")
        intent = await this.ports.intents.transition(
          context,
          intent.key,
          intent.localRevision,
          "created",
          target,
        );
      else if (
        intent.projectId !== target.projectId ||
        intent.canvasId !== target.canvasId
      )
        throw new RequestError("invalid_receipt");
      this.assert(context, run);
    } catch (reason) {
      this.assert(context, run);
      this.publish({
        phase: "created_storage_failed",
        target,
        intentKey: original.key,
        message: creationMessage(reason),
      });
      return;
    }
    if (!openImmediately) {
      this.publish({
        phase: "open_failed",
        target,
        intentKey: original.key,
        message: "已找回原画布，选择重新打开后继续创作。",
      });
      return;
    }
    this.publish({ phase: "opening", target, intentKey: original.key });
    try {
      await this.ports.readTarget(target);
      this.assert(context, run);
    } catch (reason) {
      this.assert(context, run);
      this.publish({
        phase: "open_failed",
        target,
        intentKey: original.key,
        message: creationMessage(reason),
      });
      return;
    }
    try {
      await this.ports.intents.acknowledge(
        context,
        intent.key,
        intent.localRevision,
        "opened",
      );
      this.assert(context, run);
    } catch (reason) {
      this.assert(context, run);
      this.publish({
        phase: "ack_failed",
        target,
        intentKey: original.key,
        message: creationMessage(reason),
      });
      return;
    }
    this.ports.navigate(target);
    this.publish({ phase: "ready", target, intentKey: original.key });
  }
}

export function creationMessage(reason: unknown) {
  if (reason instanceof RequestError) {
    if (reason.status === 401)
      return "登录已失效。重新登录后先确认原创建结果。";
    if (reason.status === 403 || reason.status === 404)
      return "当前归属、权限或对象已变化，请刷新归属后确认。";
    if (reason.status === 409)
      return "原请求或主关联发生冲突，请只读确认结果。";
  }
  const code = reason instanceof Error ? reason.message : "";
  if (code.includes("storage"))
    return "无法保存创建状态，请检查站点存储后重试。";
  if (code.includes("corrupt"))
    return "本地创建记录格式无法恢复，请从我的画布核对已有项目。";
  if (code.includes("context")) return "身份或创作归属已变化，已停止此次操作。";
  return "结果尚未确认，请确认原创建结果；已创建的画布仅重新打开。";
}
