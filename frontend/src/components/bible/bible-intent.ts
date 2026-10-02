import { z } from "zod";
import { ApiError } from "@/lib/request";
import {
  bibleCommandSchema,
  bibleIdentitySchema,
  bibleUUID,
  type BibleIdentity,
} from "./bible-model";

export const bibleIntentSchema = bibleIdentitySchema
  .extend({
    version: z.literal(1),
    key: bibleUUID,
    command: bibleCommandSchema,
  })
  .strict();
export type BibleIntent = z.infer<typeof bibleIntentSchema>;
type IntentStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;
export function sameBibleIdentity(a: BibleIdentity, b: BibleIdentity) {
  return (
    a.origin === b.origin &&
    a.actorId === b.actorId &&
    a.orgId === b.orgId &&
    a.projectId === b.projectId
  );
}
function identityFields(identity: BibleIdentity) {
  return {
    origin: identity.origin,
    actorId: identity.actorId,
    orgId: identity.orgId,
    projectId: identity.projectId,
  };
}
function storageKey(identity: BibleIdentity) {
  return [
    "lanverse:bible-write:v1",
    identity.origin,
    identity.actorId,
    identity.orgId,
    identity.projectId,
  ]
    .map(encodeURIComponent)
    .join(":");
}
function unavailable() {
  return new Error(
    "浏览器存储不可用或原设定意图损坏。请恢复存储并核验原键，当前不会发送新修改。",
  );
}
export function loadBibleIntent(
  storage: IntentStorage,
  identity: BibleIdentity,
): BibleIntent | null {
  try {
    bibleIdentitySchema.parse(identityFields(identity));
    const saved = storage.getItem(storageKey(identity));
    if (saved === null) return null;
    if (new TextEncoder().encode(saved).byteLength > (1 << 20) + 4096)
      throw unavailable();
    const intent = bibleIntentSchema.parse(JSON.parse(saved));
    if (!sameBibleIdentity(intent, identity)) throw unavailable();
    return intent;
  } catch {
    throw unavailable();
  }
}
export function saveBibleIntent(storage: IntentStorage, input: BibleIntent) {
  try {
    const intent = bibleIntentSchema.parse(input),
      previous = loadBibleIntent(storage, intent),
      serialized = JSON.stringify(intent);
    if (previous && JSON.stringify(previous) !== serialized)
      throw unavailable();
    storage.setItem(storageKey(intent), serialized);
    if (storage.getItem(storageKey(intent)) !== serialized) throw unavailable();
  } catch {
    throw unavailable();
  }
}
export function clearBibleIntent(
  storage: IntentStorage,
  identity: BibleIdentity,
) {
  try {
    storage.removeItem(storageKey(identity));
    if (storage.getItem(storageKey(identity)) !== null) throw unavailable();
  } catch {
    throw unavailable();
  }
}
export function unknownBibleWrite(error: unknown) {
  return (
    !(error instanceof ApiError) ||
    error.status === 0 ||
    error.status === 408 ||
    error.status >= 500
  );
}
