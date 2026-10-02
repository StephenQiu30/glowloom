import { z } from "zod";
import { ApiError } from "@/lib/request";
import { sourceCommandSchema, scriptUUID } from "./source-model";

const origin = z.url().refine((value) => new URL(value).origin === value);
export const scriptScopeSchema = z
  .object({
    origin,
    actorId: scriptUUID,
    orgId: scriptUUID,
    projectId: scriptUUID,
  })
  .strict();
export type ScriptScope = z.infer<typeof scriptScopeSchema>;
const common = {
  ...scriptScopeSchema.shape,
  version: z.literal(1),
  key: scriptUUID,
};
export const sourceIntentSchema = z.discriminatedUnion("action", [
  sourceCommandSchema.options[0].extend(common).strict(),
  sourceCommandSchema.options[1].extend(common).strict(),
  sourceCommandSchema.options[2].extend(common).strict(),
  sourceCommandSchema.options[3].extend(common).strict(),
  sourceCommandSchema.options[4].extend(common).strict(),
]);
export type SourceIntent = z.infer<typeof sourceIntentSchema>;
type IntentStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;
function storageKey(scope: ScriptScope) {
  return [
    "lanverse:script-source:v1",
    scope.origin,
    scope.actorId,
    scope.orgId,
    scope.projectId,
  ]
    .map(encodeURIComponent)
    .join(":");
}
function storageFailure() {
  return new Error(
    "浏览器存储不可用或原剧本意图无法安全恢复。请恢复存储后核验原键，当前不会发送新的修改。",
  );
}
export function sameScriptScope(left: ScriptScope, right: ScriptScope) {
  return (
    left.origin === right.origin &&
    left.actorId === right.actorId &&
    left.orgId === right.orgId &&
    left.projectId === right.projectId
  );
}
export function loadSourceIntent(
  storage: IntentStorage,
  scope: ScriptScope,
): SourceIntent | null {
  try {
    scriptScopeSchema.parse(scope);
    const encoded = storage.getItem(storageKey(scope));
    if (encoded === null) return null;
    const intent = sourceIntentSchema.parse(JSON.parse(encoded));
    if (!sameScriptScope(intent, scope)) throw storageFailure();
    return intent;
  } catch {
    throw storageFailure();
  }
}
export function saveSourceIntent(storage: IntentStorage, intent: SourceIntent) {
  try {
    const checked = sourceIntentSchema.parse(intent);
    const encoded = JSON.stringify(checked);
    const key = storageKey(checked);
    storage.setItem(key, encoded);
    if (storage.getItem(key) !== encoded) throw storageFailure();
  } catch {
    throw storageFailure();
  }
}
export function clearSourceIntent(storage: IntentStorage, scope: ScriptScope) {
  try {
    const key = storageKey(scope);
    storage.removeItem(key);
    if (storage.getItem(key) !== null) throw storageFailure();
  } catch {
    throw storageFailure();
  }
}
export function unknownSourceWrite(error: unknown) {
  return (
    !(error instanceof ApiError) ||
    ![400, 401, 403, 404, 409, 413, 415, 422, 429].includes(error.status)
  );
}
