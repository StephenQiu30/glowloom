import { z } from "zod";
import * as media from "@/api/media";
import { ApiError } from "@/lib/request";
import {
  libraryAssetSchema,
  libraryUUID,
  type LibraryIdentity,
} from "./library-model";
import { freshLibrary } from "./library-queries";
import {
  libraryUploadIntentSchema,
  type LibraryUploadIntent,
} from "./library-upload-intent";

const projectAsset = libraryAssetSchema
  .omit({ width: true, height: true, duration_ms: true })
  .extend({
    project_id: libraryUUID,
    width: z.number().int().positive().optional(),
    height: z.number().int().positive().optional(),
    duration_ms: z.number().int().positive().optional(),
  })
  .strict();
export async function uploadLibraryOriginal(
  identity: LibraryIdentity,
  file: File,
  intent: LibraryUploadIntent,
  signal: AbortSignal,
  onProgress: (loaded: number, total?: number) => void,
) {
  libraryUploadIntentSchema.parse(intent);
  await freshLibrary(identity, signal);
  const options = {
    signal,
    timeout: 300000,
    headers: { "Idempotency-Key": intent.key, Origin: identity.origin },
    onUploadProgress: (event: { loaded: number; total?: number }) =>
      onProgress(event.loaded, event.total),
  };
  const value: unknown =
    identity.scope.kind === "personal"
      ? await media.uploadPersonalMediaAsset(
          { local_review_confirmed: true },
          file,
          options,
        )
      : await media.uploadMediaAsset(
          { pid: identity.scope.project_id },
          { local_review_confirmed: true },
          file,
          options,
        );
  const parsed = z
    .object({
      asset:
        identity.scope.kind === "personal" ? libraryAssetSchema : projectAsset,
      duplicate_of: libraryUUID.nullable(),
    })
    .strict()
    .safeParse(value);
  if (
    !parsed.success ||
    (parsed.data.duplicate_of !== null &&
      parsed.data.duplicate_of !== parsed.data.asset.id) ||
    (identity.scope.kind === "project" &&
      (!("project_id" in parsed.data.asset) ||
        parsed.data.asset.project_id !== identity.scope.project_id))
  )
    throw new ApiError(502, "invalid_response");
  const asset = parsed.data.asset;
  const extension = intent.fileName.split(".").at(-1)?.toLowerCase();
  const expected =
    extension && ["txt", "docx"].includes(extension)
      ? "document"
      : extension && ["png", "jpg", "jpeg", "webp", "gif"].includes(extension)
        ? "image"
        : extension && ["mp4", "mov", "webm"].includes(extension)
          ? "video"
          : extension && ["mp3", "wav", "m4a"].includes(extension)
            ? "audio"
            : extension && ["glb", "gltf"].includes(extension)
              ? "model"
              : undefined;
  if (asset.kind !== expected) throw new ApiError(502, "invalid_response");
  const originalMIME =
    extension === "gltf"
      ? "model/gltf+json"
      : extension === "glb"
        ? "model/gltf-binary"
        : extension === "gif"
          ? "image/gif"
          : undefined;
  if (
    originalMIME &&
    (asset.mime_type !== originalMIME || asset.byte_size !== intent.byteSize)
  )
    throw new ApiError(502, "invalid_response");
  if (
    /\.(txt|docx)$/i.test(intent.fileName) &&
    (asset.kind !== "document" || asset.byte_size !== intent.byteSize)
  )
    throw new ApiError(502, "invalid_response");
  return libraryAssetSchema.parse({
    id: asset.id,
    kind: asset.kind,
    file_name: asset.file_name,
    mime_type: asset.mime_type,
    byte_size: asset.byte_size,
    revision: asset.revision,
    width: asset.width ?? null,
    height: asset.height ?? null,
    duration_ms: asset.duration_ms ?? null,
  });
}
