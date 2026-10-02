import { z } from "zod";
import {
  libraryIdentitySchema,
  sameLibraryIdentity,
  sameLibraryScope,
  libraryUUID,
  type LibraryIdentity,
} from "@/components/media/library-model";
import { transferCommandSchema } from "./transfer-model";
const common = {
  ...libraryIdentitySchema.shape,
  version: z.literal(1),
  key: libraryUUID,
};
export const transferIntentSchema = z
  .union([
    transferCommandSchema.options[0].safeExtend(common),
    transferCommandSchema.options[1].safeExtend(common),
  ])
  .refine(
    (intent) =>
      intent.action !== "create" ||
      sameLibraryScope(intent.scope, intent.body.source),
  );
export type TransferIntent = z.infer<typeof transferIntentSchema>;
type IntentStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;
function key(identity: LibraryIdentity) {
  return [
    "lanverse:media-transfer:v1",
    identity.origin,
    identity.actorId,
    identity.orgId,
    identity.libraryId,
  ]
    .map(encodeURIComponent)
    .join(":");
}
function failure() {
  return new Error(
    "迁移原键与原正文的存储不可用或损坏，请先恢复存储；当前不会发送新请求。",
  );
}
// Store only the original command and authenticated scope; never media content or URLs.
export function loadTransferIntent(
  storage: IntentStorage,
  identity: LibraryIdentity,
): TransferIntent | null {
  try {
    libraryIdentitySchema.parse({
      origin: identity.origin,
      actorId: identity.actorId,
      orgId: identity.orgId,
      libraryId: identity.libraryId,
      scope: identity.scope,
    });
    const raw = storage.getItem(key(identity));
    if (raw === null) return null;
    const original = transferIntentSchema.parse(JSON.parse(raw));
    if (!sameLibraryIdentity(original, identity)) throw failure();
    return original;
  } catch {
    throw failure();
  }
}
export function saveTransferIntent(
  storage: IntentStorage,
  intent: TransferIntent,
) {
  try {
    const checked = transferIntentSchema.parse(intent);
    const existing = loadTransferIntent(storage, checked),
      encoded = JSON.stringify(checked);
    if (existing && JSON.stringify(existing) !== encoded) throw failure();
    storage.setItem(key(checked), encoded);
    if (storage.getItem(key(checked)) !== encoded) throw failure();
  } catch {
    throw failure();
  }
}
export function clearTransferIntent(
  storage: IntentStorage,
  identity: LibraryIdentity,
) {
  try {
    storage.removeItem(key(identity));
    if (storage.getItem(key(identity)) !== null) throw failure();
  } catch {
    throw failure();
  }
}
