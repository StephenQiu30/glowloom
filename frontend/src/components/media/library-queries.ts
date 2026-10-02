import { z } from "zod";
import * as media from "@/gen/api/media";
import { ApiError } from "@/lib/request";
import { libraryIntentSchema, type LibraryIntent } from "./library-intent";
import {
  libraryFilterSchema,
  libraryAssetSchema,
  libraryUUID,
  readLibraryPage,
  readLibraryDetail,
  readLibraryReceipt,
  defaultLibraryFilter,
  type LibraryScope,
  type LibraryFilter,
  type LibraryIdentity,
  type LibraryAsset,
} from "./library-model";

export function libraryKey(identity: LibraryIdentity) {
  return [
    "media-library",
    identity.origin,
    identity.actorId,
    identity.orgId,
    identity.libraryId,
    identity.scope.kind,
    ...(identity.scope.kind === "project" ? [identity.scope.project_id] : []),
  ] as const;
}
function scopeParams(scope: LibraryScope) {
  return {
    scope: scope.kind,
    ...(scope.kind === "project" ? { project_id: scope.project_id } : {}),
  };
}
export async function listLibrary(
  scope: LibraryScope,
  filter: LibraryFilter = defaultLibraryFilter(scope),
  signal?: AbortSignal,
  identity?: LibraryIdentity,
) {
  const query = libraryFilterSchema.parse(filter);
  if (scope.kind === "project" && query.favorite_only)
    throw new ApiError(422, "invalid_request");
  const params = {
    ...scopeParams(scope),
    page: query.page,
    page_size: query.page_size,
    ...(query.kind ? { kind: query.kind } : {}),
    ...(query.category ? { category: query.category } : {}),
    ...(query.folder === "root"
      ? { root_only: true }
      : query.folder !== "all"
        ? { folder_id: query.folder }
        : {}),
    ...(query.favorite_only ? { favorite_only: true } : {}),
    ...(query.recent_only ? { recent_only: true } : {}),
    catalog_state: query.catalog_state,
    ...(query.search ? { search: query.search } : {}),
    order: query.order,
  };
  const page = readLibraryPage(
    await media.listMediaLibrary(params, { signal }),
    scope,
    query,
    identity,
  );
  if (
    page.items.some(
      (item) =>
        item.catalog_state !== query.catalog_state ||
        (query.kind && item.kind !== query.kind) ||
        (query.category && item.category !== query.category) ||
        (query.folder === "root" && item.folder_id !== null) ||
        (query.folder !== "all" &&
          query.folder !== "root" &&
          item.folder_id !== query.folder) ||
        (query.favorite_only && !item.favorite),
    )
  )
    throw new ApiError(502, "invalid_response");
  return page;
}
export async function freshLibrary(
  identity: LibraryIdentity,
  signal?: AbortSignal,
) {
  if (window.location.origin !== identity.origin)
    throw new ApiError(409, "scope_changed");
  return listLibrary(
    identity.scope,
    { ...defaultLibraryFilter(identity.scope), page_size: 1 },
    signal,
    identity,
  );
}
export async function getLibraryDetail(
  identity: LibraryIdentity,
  id: string,
  signal?: AbortSignal,
) {
  libraryUUID.parse(id);
  return readLibraryDetail(
    await media.getMediaLibraryItem(
      { item_id: id, ...scopeParams(identity.scope) },
      { signal },
    ),
    id,
  );
}
export async function applyLibraryIntent(intent: LibraryIntent) {
  const checked = libraryIntentSchema.parse(intent);
  const result: unknown = await media.applyMediaLibraryCommand(checked.body, {
    headers: { "Idempotency-Key": checked.key, Origin: checked.origin },
  });
  return readLibraryReceipt(result, checked, checked.body);
}
const signedURL = z.url().refine((value) => {
  const url = new URL(value);
  return (
    ["http:", "https:"].includes(url.protocol) && !url.username && !url.password
  );
});
const expires = z.iso
  .datetime({ offset: true })
  .refine((value) => Date.parse(value) > Date.now() + 5000);
const previewSchema = z
  .object({
    asset: libraryAssetSchema.refine((asset) => asset.kind !== "document"),
    url: signedURL,
    expires_at: expires,
    renditions: z
      .array(
        z
          .object({
            kind: z.enum([
              "thumb_256",
              "thumb_640",
              "poster",
              "proxy_720p",
              "waveform",
            ]),
            url: signedURL,
            expires_at: expires,
            width: z.number().int().positive().nullable(),
            height: z.number().int().positive().nullable(),
          })
          .strict(),
      )
      .max(5),
  })
  .strict();
export async function previewLibrary(
  identity: LibraryIdentity,
  asset: LibraryAsset,
  signal?: AbortSignal,
) {
  const result = previewSchema.safeParse(
    await media.previewLibraryMediaAsset(
      { item_id: asset.id, ...scopeParams(identity.scope) },
      { signal },
    ),
  );
  if (
    !result.success ||
    result.data.asset.id !== asset.id ||
    result.data.asset.kind !== asset.kind ||
    result.data.asset.revision !== asset.revision ||
    new Set(result.data.renditions.map((rend) => rend.kind)).size !==
      result.data.renditions.length
  )
    throw new ApiError(502, "invalid_response");
  return result.data;
}
export async function downloadLibraryOriginal(
  identity: LibraryIdentity,
  asset: LibraryAsset,
  signal?: AbortSignal,
) {
  const body: unknown = await media.downloadLibraryMediaAsset(
    { item_id: asset.id, ...scopeParams(identity.scope) },
    { responseType: "blob", signal, timeout: 300000 },
  );
  if (
    !(body instanceof Blob) ||
    body.size !== asset.byte_size ||
    body.type.split(";")[0] !== asset.mime_type.split(";")[0] ||
    /[/\\\p{Cc}]/u.test(asset.file_name)
  )
    throw new ApiError(502, "invalid_response");
  const sha256 = Array.from(
    new Uint8Array(
      await crypto.subtle.digest("SHA-256", await body.arrayBuffer()),
    ),
    (byte) => byte.toString(16).padStart(2, "0"),
  ).join("");
  return { blob: body, sha256, fileName: asset.file_name };
}
