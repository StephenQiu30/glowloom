import * as api from "@/api/media";
import { ApiError } from "@/lib/request";
import {
  libraryUUID,
  type LibraryIdentity,
} from "@/components/media/library-model";
import { freshLibrary } from "@/components/media/library-queries";
import { transferIntentSchema, type TransferIntent } from "./transfer-intent";
import {
  readTransfer,
  readTransferPage,
  readTransferReceipt,
} from "./transfer-model";
export function transferKey(identity: LibraryIdentity) {
  return [
    "media-transfers",
    identity.origin,
    identity.actorId,
    identity.orgId,
    identity.libraryId,
  ] as const;
}
function current(identity: LibraryIdentity) {
  if (window.location.origin !== identity.origin)
    throw new ApiError(409, "scope_changed");
}
export async function listTransfers(
  identity: LibraryIdentity,
  page = 1,
  signal?: AbortSignal,
) {
  current(identity);
  return readTransferPage(
    await api.listMediaTransfers(
      {
        scope: identity.scope.kind,
        ...(identity.scope.kind === "project"
          ? { project_id: identity.scope.project_id }
          : {}),
        page,
        page_size: 20,
      },
      { signal },
    ),
    identity,
    page,
  );
}
export async function getTransfer(
  identity: LibraryIdentity,
  id: string,
  signal?: AbortSignal,
) {
  current(identity);
  libraryUUID.parse(id);
  return readTransfer(
    await api.getMediaTransfer({ job_id: id }, { signal }),
    identity,
    id,
  );
}
export async function runTransferIntent(intent: TransferIntent) {
  const original = transferIntentSchema.parse(intent);
  current(original);
  await freshLibrary({
    origin: original.origin,
    actorId: original.actorId,
    orgId: original.orgId,
    libraryId: original.libraryId,
    scope: original.scope,
  });
  const options = {
    headers: { "Idempotency-Key": original.key, Origin: original.origin },
  };
  let result: unknown;
  if (original.action === "create")
    result = await api.createMediaTransfer(original.body, options);
  else {
    const params = { job_id: original.jobId };
    if (original.action === "cancel")
      result = await api.cancelMediaTransfer(params, original.body, options);
    else if (original.action === "retry")
      result = await api.retryMediaTransfer(params, original.body, options);
    else
      result = await api.reconcileMediaTransfer(params, original.body, options);
  }
  return readTransferReceipt(result, original);
}
