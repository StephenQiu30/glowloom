import { z } from "zod";
import { scriptUUID } from "./source-model";
import {
  sameScriptScope,
  scriptScopeSchema,
  type ScriptScope,
} from "./source-intent";
import {
  fileImportCreateSchema,
  fileImportControlSchema,
} from "./file-import-model";
const common = {
  ...scriptScopeSchema.shape,
  version: z.literal(1),
  key: scriptUUID,
};
export const fileImportIntentSchema = z.union([
  fileImportCreateSchema.extend(common).strict(),
  fileImportControlSchema.extend(common).strict(),
]);
export type FileImportIntent = z.infer<typeof fileImportIntentSchema>;
type IntentStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;
function storageKey(scope: ScriptScope) {
  return [
    "lanverse:script-file-import:v1",
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
    "导入原键与原正文的存储不可用或损坏，请先恢复存储；当前不会发送新请求。",
  );
}
export function loadFileImportIntent(
  storage: IntentStorage,
  scope: ScriptScope,
): FileImportIntent | null {
  try {
    scriptScopeSchema.parse({
      origin: scope.origin,
      actorId: scope.actorId,
      orgId: scope.orgId,
      projectId: scope.projectId,
    });
    const raw = storage.getItem(storageKey(scope));
    if (raw === null) return null;
    const original = fileImportIntentSchema.parse(JSON.parse(raw));
    if (!sameScriptScope(original, scope)) throw failure();
    return original;
  } catch {
    throw failure();
  }
}
export function saveFileImportIntent(
  storage: IntentStorage,
  intent: FileImportIntent,
) {
  try {
    const checked = fileImportIntentSchema.parse(intent);
    const encoded = JSON.stringify(checked);
    const existing = loadFileImportIntent(storage, checked);
    if (existing && JSON.stringify(existing) !== encoded) throw failure();
    storage.setItem(storageKey(checked), encoded);
    if (storage.getItem(storageKey(checked)) !== encoded) throw failure();
  } catch {
    throw failure();
  }
}
export function clearFileImportIntent(
  storage: IntentStorage,
  scope: ScriptScope,
) {
  try {
    storage.removeItem(storageKey(scope));
    if (storage.getItem(storageKey(scope)) !== null) throw failure();
  } catch {
    throw failure();
  }
}
