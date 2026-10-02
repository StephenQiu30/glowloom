import { z } from "zod";
import { ApiError } from "@/lib/request";
import {
  libraryCommandSchema,
  libraryIdentitySchema,
  libraryUUID,
  sameLibraryIdentity,
  sameLibraryScope,
  type LibraryIdentity,
} from "./library-model";

export const libraryIntentSchema = libraryIdentitySchema
  .extend({
    version: z.literal(1),
    key: libraryUUID,
    body: libraryCommandSchema,
  })
  .strict()
  .refine(
    (intent) =>
      sameLibraryScope(intent.scope, intent.body.scope) &&
      new TextEncoder().encode(JSON.stringify(intent.body)).length <=
        1024 * 1024,
  );
export type LibraryIntent = z.infer<typeof libraryIntentSchema>;
type IntentStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;
function identityFields(identity: LibraryIdentity) {
  return {
    origin: identity.origin,
    actorId: identity.actorId,
    orgId: identity.orgId,
    libraryId: identity.libraryId,
    scope: identity.scope,
  };
}
function storageKey(identity: LibraryIdentity) {
  return [
    "lanverse:media-library-command:v1",
    identity.origin,
    identity.actorId,
    identity.orgId,
    identity.libraryId,
  ]
    .map(encodeURIComponent)
    .join(":");
}
function unavailable() {
  return new Error(
    "浏览器存储不可用或原素材库意图损坏。请恢复存储并核验原键，当前不会发送新修改。",
  );
}
export function loadLibraryIntent(
  storage: IntentStorage,
  identity: LibraryIdentity,
): LibraryIntent | null {
  try {
    libraryIdentitySchema.parse(identityFields(identity));
    const saved = storage.getItem(storageKey(identity));
    if (saved === null) return null;
    const intent = libraryIntentSchema.parse(JSON.parse(saved));
    if (!sameLibraryIdentity(intent, identity)) throw unavailable();
    return intent;
  } catch {
    throw unavailable();
  }
}
export function saveLibraryIntent(
  storage: IntentStorage,
  intent: LibraryIntent,
) {
  try {
    const checked = libraryIntentSchema.parse(intent),
      existing = loadLibraryIntent(storage, checked);
    if (existing && JSON.stringify(existing) !== JSON.stringify(checked))
      throw unavailable();
    const encoded = JSON.stringify(checked);
    storage.setItem(storageKey(checked), encoded);
    if (storage.getItem(storageKey(checked)) !== encoded) throw unavailable();
  } catch {
    throw unavailable();
  }
}
export function clearLibraryIntent(
  storage: IntentStorage,
  identity: LibraryIdentity,
) {
  try {
    storage.removeItem(storageKey(identity));
    if (storage.getItem(storageKey(identity)) !== null) throw unavailable();
  } catch {
    throw unavailable();
  }
}
export function unknownLibraryWrite(error: unknown) {
  return (
    !(error instanceof ApiError) ||
    error.status === 0 ||
    error.status >= 500 ||
    error.status === 408
  );
}
