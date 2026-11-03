export const PROTOCOL = "glowloom.agent-runtime/v1";
export const SDK_VERSION = "1.3.0";
export type ApprovalContext = {
  purpose: "planning" | "execution";
  candidate_id: string;
  manifest_hash: string;
  step_id?: string;
};
export type RuntimeIdentity = {
  runtime_session_id: string;
  run_id: string;
  project_id: string;
  actor_id: string;
  owner_epoch: number;
  control_revision: number;
  policy_pin: string;
  definition_pin: string;
  approval_context: ApprovalContext;
};
export type ModelPin = {
  release_id: string;
  context_window: number;
  max_tokens: number;
  input: ("text" | "image")[];
  params: Record<string, unknown>;
};
export type InputCommand = { request_id: string; message: string };
export type CommandReceipt = {
  request_id: string;
  submission_id: number;
  root_conversation_id: number;
};
export const isUUID = (value: unknown): value is string =>
  typeof value === "string" &&
  /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/i.test(value);
export function validateIdentity(identity: RuntimeIdentity): void {
  if (
    !identity ||
    ![
      identity.runtime_session_id,
      identity.run_id,
      identity.project_id,
      identity.actor_id,
      identity.approval_context?.candidate_id,
    ].every(isUUID) ||
    !Number.isSafeInteger(identity.owner_epoch) ||
    identity.owner_epoch < 1 ||
    !Number.isSafeInteger(identity.control_revision) ||
    identity.control_revision < 1 ||
    !["planning", "execution"].includes(identity.approval_context.purpose) ||
    !/^[a-f0-9]{64}$/.test(identity.approval_context.manifest_hash) ||
    !identity.policy_pin ||
    !identity.definition_pin ||
    (identity.approval_context.purpose === "execution" &&
      !identity.approval_context.step_id)
  )
    throw new Error("invalid_runtime_identity");
}
export function validateModel(model: ModelPin): void {
  if (
    !isUUID(model.release_id) ||
    !Number.isSafeInteger(model.context_window) ||
    model.context_window < 1024 ||
    !Number.isSafeInteger(model.max_tokens) ||
    model.max_tokens < 1 ||
    model.max_tokens >= model.context_window ||
    !model.input.includes("text") ||
    model.input.some((i) => i !== "text" && i !== "image") ||
    !model.params ||
    Array.isArray(model.params) ||
    typeof model.params !== "object" ||
    Buffer.byteLength(JSON.stringify(model.params)) > 8192
  )
    throw new Error("invalid_model_pin");
}
