import { isProjectID, normalizeProjectName } from "./project";

export type IntentPhase =
  "prepared" | "submitting" | "unknown" | "created" | "rejected";
export type ProjectCreationContext = {
  actorId: string;
  scopeId: string;
  sessionGeneration: number;
  scopeGeneration: number;
  fenceEpoch: number;
};
export type IntentTarget = { projectId: string; canvasId: string };
export type ProjectCreationIntent = {
  actorId: string;
  scopeId: string;
  contractVersion: 1;
  key: string;
  body: API.CreateProjectRequest;
  phase: IntentPhase;
  projectId: string | null;
  canvasId: string | null;
  hasUncertainDispatch: boolean;
  localRevision: number;
  fenceEpoch: number;
  createdAt: string;
  updatedAt: string;
};
export type ProjectCreationState =
  | { phase: "scope_required"; scopeIds: string[] }
  | { phase: "idle" | "checking_context" | "preparing" }
  | {
      phase:
        | "preflight"
        | "preflight_failed"
        | "submitting"
        | "unknown"
        | "unrecorded";
      intent: ProjectCreationIntent;
      message?: string;
    }
  | {
      phase:
        | "opening"
        | "open_failed"
        | "created_storage_failed"
        | "ack_failed"
        | "ready";
      target: IntentTarget;
      intentKey: string;
      message?: string;
    }
  | { phase: "blocked" | "rejected" | "storage_failed"; message: string };

export function blankProjectInput(): API.CreateProjectRequest {
  return {
    name: "未命名画布",
    description: "",
    aspect_ratio: "16:9",
    style_type: "realistic",
  };
}

export function requireCreationInput(value: unknown): API.CreateProjectRequest {
  if (!value || typeof value !== "object")
    throw new Error("creation_intent_corrupt");
  const body = value as Record<string, unknown>;
  if (
    Object.keys(body).length !== 4 ||
    typeof body.name !== "string" ||
    typeof body.description !== "string" ||
    !["16:9", "9:16", "1:1"].includes(body.aspect_ratio as string) ||
    body.style_type !== "realistic"
  )
    throw new Error("creation_intent_corrupt");
  if (normalizeProjectName(body.name) !== body.name)
    throw new Error("creation_intent_corrupt");
  return structuredClone(body) as API.CreateProjectRequest;
}

export function requireCreationIntent(value: unknown): ProjectCreationIntent {
  const intent = value as ProjectCreationIntent | null;
  if (
    !intent ||
    intent.contractVersion !== 1 ||
    !isProjectID(intent.actorId) ||
    !isProjectID(intent.scopeId) ||
    !isProjectID(intent.key) ||
    !["prepared", "submitting", "unknown", "created", "rejected"].includes(
      intent.phase,
    ) ||
    !Number.isSafeInteger(intent.localRevision) ||
    intent.localRevision < 1 ||
    !Number.isSafeInteger(intent.fenceEpoch) ||
    intent.fenceEpoch < 0 ||
    typeof intent.hasUncertainDispatch !== "boolean" ||
    !Number.isFinite(Date.parse(intent.createdAt)) ||
    !Number.isFinite(Date.parse(intent.updatedAt))
  )
    throw new Error("creation_intent_corrupt");
  requireCreationInput(intent.body);
  if (
    intent.phase === "created"
      ? !isProjectID(intent.projectId) || !isProjectID(intent.canvasId)
      : intent.projectId !== null || intent.canvasId !== null
  )
    throw new Error("creation_intent_corrupt");
  if (intent.phase === "rejected" && intent.hasUncertainDispatch)
    throw new Error("creation_intent_corrupt");
  return structuredClone(intent);
}
