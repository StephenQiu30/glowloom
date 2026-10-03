import { ApiError } from "@/lib/request";
import { getBibleVersion } from "@/api/bible";
import { bibleUUID, readBibleVersion } from "@/components/bible/bible-model";
import {
  getBibleDetail,
  getBibleSnapshot,
  listBible,
} from "@/components/bible/bible-queries";
import { scriptScopeSchema, type ScriptScope } from "./source-intent";
import {
  getWorkspace,
  requireScriptScope,
  scriptScopeKey,
} from "./source-queries";

export type CharacterBinding = {
  character_id: string;
  character_version_id?: string;
};
export const characterBindingKey = (scope: ScriptScope) =>
  [...scriptScopeKey(scope), "character-bindings"] as const;
async function readBindingPage(
  scope: ScriptScope,
  cursor: string | undefined,
  limit: number,
  signal?: AbortSignal,
) {
  scriptScopeSchema.parse(scope);
  if (typeof window !== "undefined" && window.location.origin !== scope.origin)
    throw new ApiError(403, "scope_changed");
  const [workspace, page] = await Promise.all([
    getWorkspace(scope.projectId, signal),
    listBible(scope.projectId, "character", { cursor, limit }, signal, scope),
  ]);
  requireScriptScope(workspace, scope);
  return page;
}
export async function listCharacterBindings(
  scope: ScriptScope,
  cursor?: string,
  signal?: AbortSignal,
) {
  const page = await readBindingPage(scope, cursor, 25, signal);
  if (cursor && cursor === page.next_cursor)
    throw new ApiError(502, "invalid_character_cursor");
  const confirmed = page.entries.filter(
    ({ head }) =>
      !head.deleted && !head.redirect_id && head.confirmed_version_id,
  );
  const items = await Promise.all(
    confirmed.map(async ({ head }) => {
      const pin = head.confirmed_version_id!;
      const version = readBibleVersion(
        await getBibleVersion(
          {
            pid: scope.projectId,
            kind: "character",
            id: head.id,
            version: pin,
          },
          { signal },
        ),
        scope,
        "character",
        head.id,
        pin,
      );
      if (version.kind !== "character")
        throw new ApiError(502, "invalid_character_binding");
      return {
        id: head.id,
        name: version.character.name,
        aliases: version.character.aliases ?? [],
        confirmedVersionId: pin,
      };
    }),
  );
  return {
    items,
    next_cursor: page.next_cursor,
  };
}
export async function resolveCharacterBinding(
  scope: ScriptScope,
  id: string,
  signal?: AbortSignal,
) {
  scriptScopeSchema.parse(scope);
  bibleUUID.parse(id);
  if (typeof window !== "undefined" && window.location.origin !== scope.origin)
    throw new ApiError(403, "scope_changed");
  const [workspace, detail] = await Promise.all([
    getWorkspace(scope.projectId, signal),
    getBibleDetail(scope, "character", id, signal),
  ]);
  requireScriptScope(workspace, scope);
  const pin = detail.head.confirmed_version_id;
  if (
    detail.head.deleted ||
    detail.head.redirect_id ||
    detail.resolved_id !== id ||
    !pin
  )
    throw new ApiError(409, "character_binding_unavailable");
  const version =
    detail.current.id === pin
      ? detail.current
      : await getBibleSnapshot(scope, "character", id, pin, signal);
  if (version.kind !== "character")
    throw new ApiError(502, "invalid_character_binding");
  return {
    character_id: id,
    character_version_id: pin,
    name: version.character.name,
    aliases: version.character.aliases ?? [],
  };
}
