import { z } from "zod";
import { scriptUUID } from "./source-model";
import {
  sameScriptScope,
  scriptScopeSchema,
  type ScriptScope,
} from "./source-intent";
import { MAX_DOCUMENT_BYTES, validateDocumentFile } from "./document-media";
export const documentUploadIntentSchema = scriptScopeSchema
  .extend({
    version: z.literal(1),
    key: scriptUUID,
    fileName: z.string().min(1),
    byteSize: z.number().int().positive().max(MAX_DOCUMENT_BYTES),
    sha256: z.string().regex(/^[0-9a-f]{64}$/),
    localReviewConfirmed: z.literal(true),
  })
  .strict();
export type DocumentUploadIntent = z.infer<typeof documentUploadIntentSchema>;
type IntentStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;
function storageKey(scope: ScriptScope) {
  return [
    "lanverse:script-document-upload:v1",
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
    "浏览器存储不可用或原文上传意图损坏。请恢复存储并核验原键，当前不会发送新的上传。",
  );
}
export function loadDocumentUpload(
  storage: IntentStorage,
  scope: ScriptScope,
): DocumentUploadIntent | null {
  try {
    scriptScopeSchema.parse({
      origin: scope.origin,
      actorId: scope.actorId,
      orgId: scope.orgId,
      projectId: scope.projectId,
    });
    const body = storage.getItem(storageKey(scope));
    if (body === null) return null;
    const intent = documentUploadIntentSchema.parse(JSON.parse(body));
    if (!sameScriptScope(intent, scope)) throw failure();
    return intent;
  } catch {
    throw failure();
  }
}
export function saveDocumentUpload(
  storage: IntentStorage,
  intent: DocumentUploadIntent,
) {
  try {
    const checked = documentUploadIntentSchema.parse(intent);
    const existing = loadDocumentUpload(storage, checked);
    if (existing && JSON.stringify(existing) !== JSON.stringify(checked))
      throw failure();
    const encoded = JSON.stringify(checked);
    storage.setItem(storageKey(checked), encoded);
    if (storage.getItem(storageKey(checked)) !== encoded) throw failure();
  } catch {
    throw failure();
  }
}
export function clearDocumentUpload(
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
export async function documentFileFingerprint(file: File) {
  validateDocumentFile(file);
  const sha256 = Array.from(
    new Uint8Array(
      await crypto.subtle.digest("SHA-256", await file.arrayBuffer()),
    ),
    (byte) => byte.toString(16).padStart(2, "0"),
  ).join("");
  return { fileName: file.name, byteSize: file.size, sha256 };
}
export async function requireOriginalDocument(
  file: File,
  intent: DocumentUploadIntent,
) {
  const current = await documentFileFingerprint(file);
  if (
    current.fileName !== intent.fileName ||
    current.byteSize !== intent.byteSize ||
    current.sha256 !== intent.sha256
  )
    throw new Error(
      "所选文件与原上传的文件名、大小或 SHA 不同。请重新选择确切原件，原键保持不变。",
    );
}
