import { z } from "zod";
import { ApiError } from "@/lib/request";

const uuid = z
  .string()
  .uuid()
  .refine((value) => value !== "00000000-0000-0000-0000-000000000000");
export const copyName = z
  .string()
  .trim()
  .min(1, "请填写副本名称。")
  .refine((value) => Array.from(value).length <= 50, "名称最多 50 个字符。");
const scopeSchema = z.object({
  origin: z.url().refine((value) => new URL(value).origin === value),
  actorId: uuid,
  orgId: uuid,
  sourceId: uuid,
});
export type CopyScope = z.infer<typeof scopeSchema>;
const common = scopeSchema.extend({ version: z.literal(1), key: uuid });
const control = z
  .object({ expected_revision: z.number().int().positive() })
  .strict();
export const copyIntentSchema = z.discriminatedUnion("action", [
  common
    .extend({
      action: z.literal("create"),
      body: control.extend({ target_name: copyName }).strict(),
    })
    .strict(),
  common
    .extend({
      action: z.enum(["cancel", "retry", "reconcile"]),
      jobId: uuid,
      body: control,
    })
    .strict(),
]);
export type CopyIntent = z.infer<typeof copyIntentSchema>;
type IntentStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;

function storageKey(scope: CopyScope) {
  return [
    "lanverse:project-copy:v1",
    scope.origin,
    scope.actorId,
    scope.orgId,
    scope.sourceId,
  ]
    .map(encodeURIComponent)
    .join(":");
}
function storageFailure() {
  return new Error(
    "浏览器存储不可用或复制意图无法安全恢复。请恢复存储后核验原请求，当前不会发送新的复制操作。",
  );
}
export function sameCopyScope(left: CopyScope, right: CopyScope) {
  return (
    left.origin === right.origin &&
    left.actorId === right.actorId &&
    left.orgId === right.orgId &&
    left.sourceId === right.sourceId
  );
}
export function loadCopyIntent(
  storage: IntentStorage,
  scope: CopyScope,
): CopyIntent | null {
  try {
    scopeSchema.parse(scope);
    const stored = storage.getItem(storageKey(scope));
    if (stored === null) return null;
    const intent = copyIntentSchema.parse(JSON.parse(stored));
    if (!sameCopyScope(scope, intent)) throw storageFailure();
    return intent;
  } catch {
    throw storageFailure();
  }
}
export function saveCopyIntent(storage: IntentStorage, intent: CopyIntent) {
  try {
    const checked = copyIntentSchema.parse(intent);
    const encoded = JSON.stringify(checked);
    const key = storageKey(checked);
    // Write ahead and verify persistence before any mutation leaves this tab.
    storage.setItem(key, encoded);
    if (storage.getItem(key) !== encoded) throw storageFailure();
  } catch {
    throw storageFailure();
  }
}
export function clearCopyIntent(storage: IntentStorage, scope: CopyScope) {
  try {
    const key = storageKey(scope);
    storage.removeItem(key);
    if (storage.getItem(key) !== null) throw storageFailure();
  } catch {
    throw storageFailure();
  }
}
export function unknownCopyWrite(error: unknown) {
  return (
    !(error instanceof ApiError) ||
    ![400, 401, 403, 404, 409, 422, 429].includes(error.status)
  );
}
