import { purgeIntentSchema, type PurgeIntent } from "./library-purge-model";
import {
  libraryIdentitySchema,
  sameLibraryIdentity,
  type LibraryIdentity,
} from "./library-model";
type IntentStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;
const key = (identity: LibraryIdentity) =>
  [
    "lanverse:media-purge:v1",
    identity.origin,
    identity.actorId,
    identity.orgId,
    identity.libraryId,
  ]
    .map(encodeURIComponent)
    .join(":");
const failure = () =>
  new Error(
    "永久清理原键与正文的存储不可用或损坏。请先恢复存储，当前不会发送新的清理。",
  );
export function loadPurgeIntent(
  storage: IntentStorage,
  identity: LibraryIdentity,
): PurgeIntent | null {
  try {
    libraryIdentitySchema.parse(identity);
    const raw = storage.getItem(key(identity));
    if (raw === null) return null;
    if (new TextEncoder().encode(raw).length > 1024 * 1024) throw failure();
    const intent = purgeIntentSchema.parse(JSON.parse(raw));
    if (!sameLibraryIdentity(intent, identity)) throw failure();
    return intent;
  } catch {
    throw failure();
  }
}
export function savePurgeIntent(storage: IntentStorage, intent: PurgeIntent) {
  try {
    const parsed = purgeIntentSchema.parse(intent),
      existing = loadPurgeIntent(storage, {
        origin: parsed.origin,
        actorId: parsed.actorId,
        orgId: parsed.orgId,
        libraryId: parsed.libraryId,
        scope: parsed.scope,
      }),
      encoded = JSON.stringify(parsed);
    if (new TextEncoder().encode(encoded).length > 1024 * 1024) throw failure();
    if (existing && JSON.stringify(existing) !== encoded) throw failure();
    storage.setItem(key(parsed), encoded);
    if (storage.getItem(key(parsed)) !== encoded) throw failure();
  } catch {
    throw failure();
  }
}
export function clearPurgeIntent(
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
