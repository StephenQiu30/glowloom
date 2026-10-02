import { z } from "zod";
import { validateCanvasMediaFiles } from "@/components/canvas/media-import";
import {
  libraryIdentitySchema,
  libraryUUID,
  sameLibraryIdentity,
  type LibraryIdentity,
} from "./library-model";
const safeName = z
  .string()
  .min(1)
  .refine(
    (value) =>
      value.isWellFormed() &&
      value === value.trim() &&
      !/[/\\\p{Cc}]/u.test(value) &&
      new TextEncoder().encode(value).length <= 255,
  );
export const libraryUploadIntentSchema = z
  .object({
    key: libraryUUID,
    fileName: safeName,
    byteSize: z
      .number()
      .int()
      .positive()
      .max(500 * 1024 * 1024),
    sha256: z.string().regex(/^[0-9a-f]{64}$/),
    local_review_confirmed: z.literal(true),
  })
  .strict();
export type LibraryUploadIntent = z.infer<typeof libraryUploadIntentSchema>;
const journalSchema = libraryIdentitySchema
  .extend({
    version: z.literal(1),
    uploads: z
      .array(libraryUploadIntentSchema)
      .max(25)
      .refine(
        (entries) =>
          new Set(entries.map((entry) => entry.key)).size === entries.length,
      ),
  })
  .strict();
type IntentStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;
function storageKey(identity: LibraryIdentity) {
  return [
    "lanverse:library-upload:v1",
    identity.origin,
    identity.actorId,
    identity.orgId,
    identity.libraryId,
  ]
    .map(encodeURIComponent)
    .join(":");
}
function fields(identity: LibraryIdentity) {
  return {
    origin: identity.origin,
    actorId: identity.actorId,
    orgId: identity.orgId,
    libraryId: identity.libraryId,
    scope: identity.scope,
  };
}
function failure() {
  return new Error(
    "浏览器存储不可用或原上传意图损坏。请恢复存储，当前不会发送新的上传。",
  );
}
export function loadLibraryUploads(
  storage: IntentStorage,
  identity: LibraryIdentity,
): LibraryUploadIntent[] {
  try {
    libraryIdentitySchema.parse(fields(identity));
    const saved = storage.getItem(storageKey(identity));
    if (saved === null) return [];
    const journal = journalSchema.parse(JSON.parse(saved));
    if (!sameLibraryIdentity(journal, identity)) throw failure();
    return journal.uploads;
  } catch {
    throw failure();
  }
}
export function saveLibraryUploads(
  storage: IntentStorage,
  identity: LibraryIdentity,
  uploads: LibraryUploadIntent[],
) {
  try {
    const journal = journalSchema.parse({
      ...fields(identity),
      version: 1,
      uploads,
    });
    const existing = loadLibraryUploads(storage, identity);
    for (const upload of journal.uploads) {
      const prior = existing.find((entry) => entry.key === upload.key);
      if (prior && JSON.stringify(prior) !== JSON.stringify(upload))
        throw failure();
    }
    if (!uploads.length) {
      storage.removeItem(storageKey(identity));
      if (storage.getItem(storageKey(identity)) !== null) throw failure();
    } else {
      const encoded = JSON.stringify(journal);
      storage.setItem(storageKey(identity), encoded);
      if (storage.getItem(storageKey(identity)) !== encoded) throw failure();
    }
  } catch {
    throw failure();
  }
}
export function validateLibraryFiles(files: readonly File[]) {
  const binaries = files.filter((file) => !/\.(txt|docx)$/i.test(file.name));
  const errors = binaries.length
    ? validateCanvasMediaFiles(binaries, 25, 25).errors.map((message) =>
        message.replace("画布剩余节点容量", "批次容量"),
      )
    : [];
  if (files.length < 1 || files.length > 25)
    errors.push("每批请选择1至25个文件。");
  let total = 0;
  for (const file of files) {
    total += file.size;
    if (!safeName.safeParse(file.name).success)
      errors.push("原文件名必须安全且不超过255 UTF-8字节。");
    if (/\.(txt|docx)$/i.test(file.name)) {
      const expected = /\.txt$/i.test(file.name)
        ? "text/plain"
        : "application/vnd.openxmlformats-officedocument.wordprocessingml.document";
      if (
        file.size < 1 ||
        file.size > 20 * 1024 * 1024 ||
        (file.type &&
          file.type !== "application/octet-stream" &&
          file.type !== expected)
      )
        errors.push(`${file.name}：文档必须是20MiB以内的非空TXT或DOCX原件。`);
    }
  }
  if (total > 700 * 1024 * 1024) errors.push("每批总容量不能超过700MiB。");
  return { valid: errors.length === 0, errors };
}
export async function uploadFingerprint(file: File) {
  const validation = validateLibraryFiles([file]);
  if (!validation.valid) throw new Error(validation.errors.join(" "));
  const digest = await crypto.subtle.digest(
    "SHA-256",
    await file.arrayBuffer(),
  );
  return {
    fileName: file.name,
    byteSize: file.size,
    sha256: Array.from(new Uint8Array(digest), (byte) =>
      byte.toString(16).padStart(2, "0"),
    ).join(""),
  };
}
export async function requireOriginalUpload(
  file: File,
  intent: LibraryUploadIntent,
) {
  const current = await uploadFingerprint(file);
  if (
    current.fileName !== intent.fileName ||
    current.byteSize !== intent.byteSize ||
    current.sha256 !== intent.sha256
  )
    throw new Error(
      "所选文件与原上传的文件名、大小或 SHA 不同。请选择确切原件，原键保持不变。",
    );
}
