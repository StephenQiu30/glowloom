import { z } from "zod";
import * as media from "@/gen/api/media";
import { ApiError } from "@/lib/request";
import { scriptUUID } from "./source-model";
import type { ScriptScope } from "./source-intent";

export const MAX_DOCUMENT_BYTES = 20 * 1024 * 1024;
export const documentAssetSchema = z
  .object({
    id: scriptUUID,
    project_id: scriptUUID,
    kind: z.literal("document"),
    file_name: z.string().min(1),
    mime_type: z.enum([
      "text/plain",
      "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    ]),
    byte_size: z.number().int().positive().max(MAX_DOCUMENT_BYTES),
    revision: z.number().int().positive(),
  })
  .strict();
export type DocumentAsset = z.infer<typeof documentAssetSchema>;
export function documentMediaKey(scope: ScriptScope) {
  return [
    "project",
    scope.projectId,
    "script-documents",
    scope.origin,
    scope.actorId,
    scope.orgId,
  ] as const;
}
function invalid(): never {
  throw new ApiError(502, "invalid_response");
}
export function readDocumentPage(value: unknown, pid: string) {
  const parsed = z
    .object({
      items: z.array(documentAssetSchema).max(50),
      next_cursor: scriptUUID.nullable(),
    })
    .strict()
    .safeParse(value);
  if (
    !parsed.success ||
    parsed.data.items.some((asset) => asset.project_id !== pid) ||
    new Set(parsed.data.items.map((asset) => asset.id)).size !==
      parsed.data.items.length
  )
    invalid();
  return parsed.data;
}
export function readDocumentUpload(value: unknown, pid: string) {
  const parsed = z
    .object({ asset: documentAssetSchema, duplicate_of: scriptUUID.nullish() })
    .strict()
    .safeParse(value);
  if (
    !parsed.success ||
    parsed.data.asset.project_id !== pid ||
    (parsed.data.duplicate_of &&
      parsed.data.duplicate_of !== parsed.data.asset.id)
  )
    invalid();
  return parsed.data.asset;
}
export async function listDocuments(
  scope: ScriptScope,
  cursor?: string,
  signal?: AbortSignal,
) {
  return readDocumentPage(
    await media.listMediaAssets(
      { pid: scope.projectId, kind: "document", limit: 50, cursor },
      { signal },
    ),
    scope.projectId,
  );
}
export function validateDocumentFile(file: File) {
  if (
    !file.name ||
    file.name.trim() !== file.name ||
    new TextEncoder().encode(file.name).length > 255 ||
    /[/\\\p{Cc}]/u.test(file.name) ||
    !/\.(txt|docx)$/i.test(file.name) ||
    file.size < 1 ||
    file.size > MAX_DOCUMENT_BYTES
  )
    throw new Error(
      "请选择不超过 20 MiB 的非空 TXT 或 DOCX 原件，文件名不能含路径。",
    );
}
export async function findDocument(
  scope: ScriptScope,
  id: string,
  signal?: AbortSignal,
): Promise<DocumentAsset> {
  scriptUUID.parse(id);
  const cursors = new Set<string>();
  const identities = new Set<string>();
  let cursor: string | undefined;
  do {
    const page = await listDocuments(scope, cursor, signal);
    for (const item of page.items) {
      if (identities.has(item.id)) invalid();
      identities.add(item.id);
    }
    const found = page.items.find((item) => item.id === id);
    if (found) return found;
    cursor = page.next_cursor ?? undefined;
    if (cursor) {
      if (cursors.has(cursor)) invalid();
      cursors.add(cursor);
    }
  } while (cursor);
  throw new ApiError(404, "not_found");
}
export async function uploadDocument(
  pid: string,
  file: File,
  key: string,
  origin: string,
  signal: AbortSignal,
  progress: (loaded: number, total?: number) => void,
) {
  scriptUUID.parse(pid);
  scriptUUID.parse(key);
  validateDocumentFile(file);
  const asset = readDocumentUpload(
    await media.uploadMediaAsset(
      { pid },
      { local_review_confirmed: true },
      file,
      {
        signal,
        timeout: 300_000,
        headers: {
          "Idempotency-Key": key,
          Origin: origin,
          "X-Request-ID": crypto.randomUUID(),
        },
        onUploadProgress: (event) => progress(event.loaded, event.total),
      },
    ),
    pid,
  );
  if (asset.byte_size !== file.size) invalid();
  return asset;
}
export async function downloadDocument(
  scope: ScriptScope,
  asset: DocumentAsset,
  signal?: AbortSignal,
): Promise<Blob> {
  if (documentAssetSchema.parse(asset).project_id !== scope.projectId)
    throw new ApiError(404, "not_found");
  const body: unknown = await media.downloadDocumentAsset(
    { pid: scope.projectId, asset_id: asset.id },
    { responseType: "blob", signal },
  );
  if (
    !(body instanceof Blob) ||
    body.size !== asset.byte_size ||
    body.type !== asset.mime_type
  )
    invalid();
  return body;
}
