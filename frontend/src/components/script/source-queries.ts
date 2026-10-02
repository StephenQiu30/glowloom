import { z } from "zod";
import * as script from "@/gen/api/script";
import { ApiError } from "@/lib/request";
import {
  sourceIntentSchema,
  type ScriptScope,
  type SourceIntent,
} from "./source-intent";
import {
  readSourceDetail,
  readSourcePage,
  readSourceReceipt,
  readScriptWorkspace,
  scriptUUID,
  type ScriptWorkspace,
  type SourceSummary,
} from "./source-model";

export function scriptWorkspaceKey(projectId: string) {
  return ["script-workspace", projectId] as const;
}
export function scriptScopeKey(scope: ScriptScope) {
  return [
    "project",
    scope.projectId,
    "script",
    scope.origin,
    scope.actorId,
    scope.orgId,
  ] as const;
}
export async function getWorkspace(projectId: string, signal?: AbortSignal) {
  scriptUUID.parse(projectId);
  return readScriptWorkspace(
    await script.getScriptWorkspace({ pid: projectId }, { signal }),
    projectId,
  );
}
export function requireScriptScope(
  workspace: ScriptWorkspace,
  scope: ScriptScope,
) {
  if (
    workspace.current_actor_id !== scope.actorId ||
    workspace.current_org_id !== scope.orgId ||
    workspace.state.project_id !== scope.projectId
  )
    throw new ApiError(409, "scope_changed");
}
export async function listSources(
  scope: ScriptScope,
  versionId?: string,
  after = 0,
  signal?: AbortSignal,
  limit = 25,
) {
  scriptUUID.parse(scope.projectId);
  if (versionId) scriptUUID.parse(versionId);
  z.number().int().min(0).parse(after);
  z.number().int().min(1).max(100).parse(limit);
  if (after > 0 && !versionId) throw new ApiError(422, "invalid_request");
  const result = await script.listScriptSources(
    { pid: scope.projectId, version_id: versionId, after, limit },
    { signal },
  );
  return readSourcePage(result, { versionId, after, limit });
}
export async function getSource(
  scope: ScriptScope,
  lineageId: string,
  versionId?: string,
  signal?: AbortSignal,
) {
  scriptUUID.parse(scope.projectId);
  scriptUUID.parse(lineageId);
  if (versionId) scriptUUID.parse(versionId);
  return readSourceDetail(
    await script.getScriptSource(
      { pid: scope.projectId, lineage: lineageId, version_id: versionId },
      { signal },
    ),
    lineageId,
  );
}
export async function readAllSources(
  scope: ScriptScope,
  versionId?: string,
  signal?: AbortSignal,
) {
  const items: SourceSummary[] = [];
  const ids = new Set<string>();
  const lineages = new Set<string>();
  let frozenVersion = versionId;
  let after = 0;
  for (;;) {
    const page = await listSources(scope, frozenVersion, after, signal, 100);
    if (!frozenVersion) frozenVersion = page.version_id;
    for (const item of page.items) {
      if (ids.has(item.id) || lineages.has(item.source_lineage_id))
        throw new ApiError(502, "invalid_response");
      ids.add(item.id);
      lineages.add(item.source_lineage_id);
      items.push(item);
    }
    if (page.next_position === undefined)
      return { version_id: frozenVersion, items };
    after = page.next_position;
  }
}
export async function runSourceIntent(intent: SourceIntent) {
  const checked = sourceIntentSchema.parse(intent);
  const options = {
    headers: {
      "Idempotency-Key": checked.key,
      Origin: checked.origin,
      "X-Request-ID": crypto.randomUUID(),
    },
  };
  const params = { pid: checked.projectId };
  let result: unknown;
  switch (checked.action) {
    case "create":
      result = await script.createScriptSource(params, checked.body, options);
      break;
    case "update":
      result = await script.updateScriptSource(
        { ...params, lineage: checked.lineageId },
        checked.body,
        options,
      );
      break;
    case "delete":
      result = await script.deleteScriptSource(
        { ...params, lineage: checked.lineageId },
        checked.body,
        options,
      );
      break;
    case "import":
      result = await script.importScriptSources(params, checked.body, options);
      break;
    case "reorder":
      result = await script.reorderScriptSources(params, checked.body, options);
      break;
  }
  return readSourceReceipt(checked, result);
}
