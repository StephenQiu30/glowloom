import { z } from "zod";
import { ApiError } from "@/lib/request";
import { folderCommandSchema, folderUUID } from "./folder-model";

const scopeSchema = z
  .object({
    origin: z.url().refine((value) => new URL(value).origin === value),
    actorId: folderUUID,
    orgId: folderUUID,
  })
  .strict();
export type FolderScope = z.infer<typeof scopeSchema>;
const common = {
  origin: scopeSchema.shape.origin,
  actorId: folderUUID,
  orgId: folderUUID,
  version: z.literal(1),
  key: folderUUID,
};
export const folderIntentSchema = z.discriminatedUnion("action", [
  folderCommandSchema.options[0].extend(common).strict(),
  folderCommandSchema.options[1].extend(common).strict(),
  folderCommandSchema.options[2].extend(common).strict(),
  folderCommandSchema.options[3].extend(common).strict(),
]);
export type FolderIntent = z.infer<typeof folderIntentSchema>;
type IntentStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;
function storageKey(scope: FolderScope) {
  return [
    "lanverse:project-folder:v1",
    scope.origin,
    scope.actorId,
    scope.orgId,
  ]
    .map(encodeURIComponent)
    .join(":");
}
function storageFailure() {
  return new Error(
    "浏览器存储不可用或目录意图无法安全恢复。请恢复存储后核验原请求，当前不会发送新的目录操作。",
  );
}
export function sameFolderScope(left: FolderScope, right: FolderScope) {
  return (
    left.origin === right.origin &&
    left.actorId === right.actorId &&
    left.orgId === right.orgId
  );
}
export function loadFolderIntent(
  storage: IntentStorage,
  scope: FolderScope,
): FolderIntent | null {
  try {
    scopeSchema.parse(scope);
    const encoded = storage.getItem(storageKey(scope));
    if (encoded === null) return null;
    const intent = folderIntentSchema.parse(JSON.parse(encoded));
    if (!sameFolderScope(scope, intent)) throw storageFailure();
    return intent;
  } catch {
    throw storageFailure();
  }
}
export function saveFolderIntent(storage: IntentStorage, intent: FolderIntent) {
  try {
    const checked = folderIntentSchema.parse(intent);
    const encoded = JSON.stringify(checked);
    const key = storageKey(checked);
    storage.setItem(key, encoded);
    if (storage.getItem(key) !== encoded) throw storageFailure();
  } catch {
    throw storageFailure();
  }
}
export function clearFolderIntent(storage: IntentStorage, scope: FolderScope) {
  try {
    const key = storageKey(scope);
    storage.removeItem(key);
    if (storage.getItem(key) !== null) throw storageFailure();
  } catch {
    throw storageFailure();
  }
}
export function unknownFolderWrite(error: unknown) {
  return (
    !(error instanceof ApiError) ||
    ![400, 401, 403, 404, 409, 422, 429].includes(error.status)
  );
}
