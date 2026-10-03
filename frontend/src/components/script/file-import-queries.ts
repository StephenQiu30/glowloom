import * as api from "@/api/script";
import { scriptUUID } from "./source-model";
import type { ScriptScope } from "./source-intent";
import {
  fileImportIntentSchema,
  type FileImportIntent,
} from "./file-import-intent";
import {
  readFileImport,
  readFileImportPage,
  readFileImportReceipt,
} from "./file-import-model";
export function fileImportScopeKey(scope: ScriptScope) {
  return [
    "project",
    scope.projectId,
    "script-file-imports",
    scope.origin,
    scope.actorId,
    scope.orgId,
  ] as const;
}
export async function listFileImports(
  scope: ScriptScope,
  after = 0,
  signal?: AbortSignal,
) {
  return readFileImportPage(
    await api.listScriptFileImports(
      { pid: scope.projectId, after, limit: 25 },
      { signal },
    ),
    scope,
    after,
  );
}
export async function getFileImport(
  scope: ScriptScope,
  jobId: string,
  signal?: AbortSignal,
) {
  scriptUUID.parse(jobId);
  return readFileImport(
    await api.getScriptFileImport(
      { pid: scope.projectId, id: jobId },
      { signal },
    ),
    scope.projectId,
    jobId,
  );
}
export async function runFileImportIntent(intent: FileImportIntent) {
  const original = fileImportIntentSchema.parse(intent);
  const options = {
    headers: {
      "Idempotency-Key": original.key,
      Origin: original.origin,
      "X-Request-ID": crypto.randomUUID(),
    },
  };
  let response: unknown;
  if (original.action === "create")
    response = await api.createScriptFileImport(
      { pid: original.projectId },
      original.body,
      options,
    );
  else {
    const params = { pid: original.projectId, id: original.jobId };
    if (original.action === "cancel")
      response = await api.cancelScriptFileImport(
        params,
        original.body,
        options,
      );
    else if (original.action === "retry")
      response = await api.retryScriptFileImport(
        params,
        original.body,
        options,
      );
    else
      response = await api.reconcileScriptFileImport(
        params,
        original.body,
        options,
      );
  }
  return readFileImportReceipt(response, original.projectId, original);
}
