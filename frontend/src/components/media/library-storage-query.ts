import { z } from "zod";
import * as media from "@/gen/api/media";
import { ApiError } from "@/lib/request";
import {
  libraryScopeSchema,
  libraryUUID,
  sameLibraryScope,
  type LibraryIdentity,
} from "./library-model";

const bytes = z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER);
const usageSchema = z
  .object({
    current_actor_id: libraryUUID,
    current_org_id: libraryUUID,
    library_id: libraryUUID,
    scope: libraryScopeSchema,
    used_bytes: bytes,
    object_count: bytes,
    limit_bytes: bytes.nullable(),
    calculated_at: z.iso.datetime({ offset: true }),
  })
  .strict();
export type LibraryStorageUsage = z.infer<typeof usageSchema>;
export async function getLibraryStorageUsage(
  identity: LibraryIdentity,
  signal?: AbortSignal,
): Promise<LibraryStorageUsage> {
  if (window.location.origin !== identity.origin)
    throw new ApiError(409, "scope_changed");
  const result = usageSchema.safeParse(
    await media.getMediaLibraryStorageUsage(
      {
        scope: identity.scope.kind,
        ...(identity.scope.kind === "project"
          ? { project_id: identity.scope.project_id }
          : {}),
      },
      { signal },
    ),
  );
  if (
    !result.success ||
    result.data.current_actor_id !== identity.actorId ||
    result.data.current_org_id !== identity.orgId ||
    result.data.library_id !== identity.libraryId ||
    !sameLibraryScope(result.data.scope, identity.scope)
  )
    throw new ApiError(502, "invalid_response");
  return result.data;
}
