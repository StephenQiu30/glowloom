import type { TaskContext } from "./definition";
export type RunStatus =
  | "created"
  | "ready"
  | "running"
  | "waiting_input"
  | "waiting_external"
  | "paused"
  | "succeeded"
  | "failed"
  | "canceled"
  | "unknown";
export type Evidence = {
  kind: "command" | "host" | "manual";
  status: "passed" | "failed" | "skipped" | "not_run";
  inputDigest: string;
  summary: string;
  artifactRef: string;
  stage?: number;
};
export type Run = {
  id: string;
  actor: string;
  workflowId: string;
  context: TaskContext;
  status: RunStatus;
  version: number;
  attempt: number;
  stage: number;
  externalHandle: string | null;
  evidence: Evidence[];
  acceptedBy: string | null;
  reacceptRequired?: boolean;
  pausedFrom?: RunStatus;
  pendingCheck?: { key: string; commandId: string } | null;
};
export type Action =
  | {
      type:
        | "start"
        | "pause"
        | "resume"
        | "cancel"
        | "fail"
        | "unknown"
        | "checkRejected";
    }
  | { type: "bind"; handle: string }
  | { type: "external"; handle: string; result: string }
  | { type: "evidence"; evidence: Evidence }
  | { type: "accept"; actor: string }
  | { type: "advance"; stages: number }
  | { type: "refresh"; context: TaskContext }
  | { type: "verifyStart"; key: string; commandId: string };
export function evolveRun(
  run: Run,
  action: Action,
  now: number,
  current: boolean,
): Run {
  const next = {
    ...run,
    evidence: [...run.evidence],
    version: run.version + 1,
  };
  const terminal = ["succeeded", "canceled"].includes(run.status);
  if (action.type === "external") {
    if (terminal || run.status === "paused") return next;
    if (!["running", "waiting_external", "unknown"].includes(run.status))
      throw new Error("Unexpected external event");
    next.externalHandle = action.handle;
    next.status = "waiting_input";
    next.evidence.push({
      kind: "host",
      status: "passed",
      inputDigest: run.context.digest,
      summary: action.result.slice(0, 12000),
      artifactRef: action.handle,
      stage: run.stage,
    });
    return next;
  }
  if (terminal) throw new Error("Run is terminal");
  if (
    ["start", "advance", "accept", "resume", "verifyStart"].includes(
      action.type,
    ) &&
    (!current || run.context.expiresAt <= now)
  )
    throw new Error("Input or authorization stale");
  switch (action.type) {
    case "start":
      if (!["created", "ready"].includes(run.status))
        throw new Error(`Cannot start ${run.status}`);
      next.status = "running";
      next.attempt++;
      next.externalHandle = null;
      break;
    case "pause":
      if (
        !["running", "waiting_external", "waiting_input", "ready"].includes(
          run.status,
        )
      )
        throw new Error("Cannot pause");
      next.status = "paused";
      next.pausedFrom = run.status;
      break;
    case "resume":
      if (run.status !== "paused") throw new Error("Not paused");
      next.status = ["running", "waiting_external", "unknown"].includes(
        run.pausedFrom ?? "",
      )
        ? "unknown"
        : (run.pausedFrom ?? "waiting_input");
      break;
    case "cancel":
      next.status = "canceled";
      break;
    case "unknown":
      next.status = "unknown";
      break;
    case "fail":
      next.status = "failed";
      break;
    case "bind":
      if (
        !["running", "waiting_external", "unknown"].includes(run.status) ||
        (run.externalHandle && run.externalHandle !== action.handle)
      )
        throw new Error("handle_conflict");
      next.externalHandle = action.handle;
      next.status = "waiting_external";
      break;
    case "refresh":
      if (
        !["created", "ready", "waiting_input", "failed"].includes(run.status) ||
        action.context.workflowRevision !== run.context.workflowRevision ||
        action.context.expiresAt <= now
      )
        throw new Error("context_refresh_blocked");
      next.context = action.context;
      next.reacceptRequired = !!run.acceptedBy || run.reacceptRequired;
      next.acceptedBy = null;
      break;
    case "checkRejected":
      if (!run.pendingCheck) throw new Error("No pending check");
      next.pendingCheck = null;
      if (run.status !== "paused") next.status = "waiting_input";
      break;
    case "verifyStart":
      if (
        !["waiting_input", "ready", "created"].includes(run.status) ||
        run.pendingCheck
      )
        throw new Error("check_already_pending");
      next.pendingCheck = { key: action.key, commandId: action.commandId };
      next.status = "waiting_external";
      break;
    case "evidence":
      if (
        action.evidence.inputDigest !== run.context.digest ||
        !action.evidence.artifactRef ||
        action.evidence.summary.length > 16000
      )
        throw new Error("Stale evidence");
      next.evidence.push({ ...action.evidence, stage: run.stage });
      next.pendingCheck = null;
      if (run.status !== "paused") next.status = "waiting_input";
      break;
    case "accept":
      if (
        (run.context.stageIds[run.stage] !== "accept" &&
          !run.reacceptRequired) ||
        run.status !== "waiting_input" ||
        !run.evidence.some(
          (e) =>
            e.kind === "command" &&
            e.status === "passed" &&
            e.inputDigest === run.context.digest,
        ) ||
        run.evidence.some(
          (e) =>
            e.kind === "command" &&
            e.status === "failed" &&
            e.inputDigest === run.context.digest,
        )
      )
        throw new Error("Matching passed evidence required");
      next.acceptedBy = action.actor;
      next.reacceptRequired = false;
      break;
    case "advance": {
      const phase = run.context.stageIds[run.stage];
      const evidence = run.evidence.filter(
        (e) => e.inputDigest === run.context.digest && e.stage === run.stage,
      );
      if (
        run.status !== "waiting_input" ||
        (phase === "accept"
          ? !run.acceptedBy
          : !evidence.some((e) => e.status === "passed")) ||
        (phase === "validate" &&
          !evidence.some((e) => e.kind === "command" && e.status === "passed"))
      )
        throw new Error("Stage evidence required");
      if (run.stage + 1 >= run.context.stageIds.length) {
        if (!run.acceptedBy) throw new Error("Acceptance required");
        next.status = "succeeded";
      } else {
        next.stage++;
        next.status = "ready";
        next.externalHandle = null;
      }
      break;
    }
  }
  return next;
}
