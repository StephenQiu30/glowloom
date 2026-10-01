import { z } from "zod";
import { ApiError } from "@/lib/request";
import {
  depthRevision,
  depthSHA,
  depthSourceSchema,
  depthUUID,
} from "./media-depth-model";

const scopeSchema = z.object({
  origin: z.url().refine((value) => {
    const url = new URL(value);
    return ["http:", "https:"].includes(url.protocol) && url.origin === value;
  }),
  actorId: depthUUID,
  orgId: depthUUID,
  projectId: depthUUID,
  canvasId: depthUUID,
  nodeId: depthUUID,
});
export type DepthScope = z.infer<typeof scopeSchema>;
const common = scopeSchema.extend({ version: z.literal(1), key: depthUUID });
const controlBody = z
  .object({ project_id: depthUUID, revision: depthRevision })
  .strict();
export const depthIntentSchema = z
  .discriminatedUnion("action", [
    common
      .extend({ action: z.literal("create"), body: depthSourceSchema })
      .strict(),
    common
      .extend({
        action: z.enum(["cancel", "retry", "reconcile"]),
        jobId: depthUUID,
        body: controlBody,
      })
      .strict(),
    common
      .extend({
        action: z.literal("review"),
        jobId: depthUUID,
        body: controlBody
          .extend({ sha256: depthSHA, local_review_confirmed: z.literal(true) })
          .strict(),
      })
      .strict(),
  ])
  .refine((intent) =>
    intent.action === "create"
      ? intent.body.canvas_id === intent.canvasId &&
        intent.body.node_id === intent.nodeId
      : intent.body.project_id === intent.projectId,
  );
export type DepthIntent = z.infer<typeof depthIntentSchema>;
type IntentStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;
function storageKey(scope: DepthScope) {
  return [
    "lanverse:media-depth:v1",
    scope.origin,
    scope.actorId,
    scope.orgId,
    scope.projectId,
    scope.canvasId,
    scope.nodeId,
  ]
    .map(encodeURIComponent)
    .join(":");
}
function storageFailure() {
  return new Error(
    "浏览器存储不可用或深度请求无法安全恢复。请恢复存储后核验原请求，当前不能提交新的深度操作。",
  );
}
export function sameDepthScope(left: DepthScope, right: DepthScope) {
  return (
    left.origin === right.origin &&
    left.actorId === right.actorId &&
    left.orgId === right.orgId &&
    left.projectId === right.projectId &&
    left.canvasId === right.canvasId &&
    left.nodeId === right.nodeId
  );
}
export function loadDepthIntent(
  storage: IntentStorage,
  scope: DepthScope,
): DepthIntent | null {
  try {
    scopeSchema.parse(scope);
    const stored = storage.getItem(storageKey(scope));
    if (stored === null) return null;
    const intent = depthIntentSchema.parse(JSON.parse(stored));
    if (!sameDepthScope(scope, intent)) throw storageFailure();
    return intent;
  } catch {
    throw storageFailure();
  }
}
export function saveDepthIntent(storage: IntentStorage, intent: DepthIntent) {
  try {
    const checked = depthIntentSchema.parse(intent),
      encoded = JSON.stringify(checked),
      key = storageKey(checked);
    storage.setItem(key, encoded);
    if (storage.getItem(key) !== encoded) throw storageFailure();
  } catch {
    throw storageFailure();
  }
}
export function clearDepthIntent(storage: IntentStorage, scope: DepthScope) {
  try {
    const key = storageKey(scope);
    storage.removeItem(key);
    if (storage.getItem(key) !== null) throw storageFailure();
  } catch {
    throw storageFailure();
  }
}
export function unknownDepthWrite(error: unknown) {
  return (
    !(error instanceof ApiError) ||
    ![400, 401, 403, 404, 409, 422, 429].includes(error.status)
  );
}
