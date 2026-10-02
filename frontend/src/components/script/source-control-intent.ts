import { z } from "zod";
import { scriptUUID } from "./source-model";
import {
  sameScriptScope,
  scriptScopeSchema,
  type ScriptScope,
} from "./source-intent";
import { sourceControlCommandSchema } from "./source-write-model";
export const sourceControlIntentSchema = sourceControlCommandSchema
  .extend({
    ...scriptScopeSchema.shape,
    version: z.literal(1),
    key: scriptUUID,
  })
  .strict();
export type SourceControlIntent = z.infer<typeof sourceControlIntentSchema>;
type IntentStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;
function storageKey(scope: ScriptScope) {
  return [
    "lanverse:script-source-control:v1",
    scope.origin,
    scope.actorId,
    scope.orgId,
    scope.projectId,
  ]
    .map(encodeURIComponent)
    .join(":");
}
function failure() {
  return new Error(
    "浏览器存储不可用或原保存控制意图损坏。请恢复原键与原正文；当前不会提交新控制。",
  );
}
export function loadSourceControl(
  storage: IntentStorage,
  scope: ScriptScope,
): SourceControlIntent | null {
  try {
    const body = storage.getItem(storageKey(scope));
    if (body === null) return null;
    const result = sourceControlIntentSchema.parse(JSON.parse(body));
    if (!sameScriptScope(result, scope)) throw failure();
    return result;
  } catch {
    throw failure();
  }
}
export function saveSourceControl(
  storage: IntentStorage,
  intent: SourceControlIntent,
) {
  try {
    const checked = sourceControlIntentSchema.parse(intent);
    const existing = loadSourceControl(storage, checked);
    const encoded = JSON.stringify(checked);
    if (existing && JSON.stringify(existing) !== encoded) throw failure();
    storage.setItem(storageKey(checked), encoded);
    if (storage.getItem(storageKey(checked)) !== encoded) throw failure();
  } catch {
    throw failure();
  }
}
export function clearSourceControl(storage: IntentStorage, scope: ScriptScope) {
  try {
    storage.removeItem(storageKey(scope));
    if (storage.getItem(storageKey(scope)) !== null) throw failure();
  } catch {
    throw failure();
  }
}
