import { z } from "zod";
import { sameFolderScope, type FolderScope } from "./folder-intent";

const uuid = z
  .uuid()
  .refine((value) => value !== "00000000-0000-0000-0000-000000000000");
export const projectSettingsChangeSchema = z
  .object({
    expected_revision: z.number().int().positive().max(2147483647),
    name: z
      .string()
      .refine(
        (value) =>
          value.trim() === value &&
          Array.from(value).length >= 1 &&
          Array.from(value).length <= 50 &&
          !value.includes("\0"),
      )
      .optional(),
    description: z
      .string()
      .refine((value) => !value.includes("\0"))
      .optional(),
    style_preset_id: z.uuid().optional(),
    allow_overseas_models: z.boolean().optional(),
    cover_asset_id: uuid.nullable().optional(),
  })
  .strict();
export const projectCoverIntentSchema = z
  .object({
    version: z.literal(1),
    origin: z.url().refine((value) => new URL(value).origin === value),
    actorId: uuid,
    orgId: uuid,
    projectId: uuid,
    key: uuid,
    body: projectSettingsChangeSchema,
  })
  .strict();
export type ProjectCoverIntent = z.infer<typeof projectCoverIntentSchema>;
type IntentStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;
function storageKey(scope: FolderScope, projectId: string) {
  return [
    "lanverse:project-settings:v1",
    scope.origin,
    scope.actorId,
    scope.orgId,
    projectId,
  ]
    .map(encodeURIComponent)
    .join(":");
}
function storageFailure() {
  return new Error(
    "浏览器存储不可用或原设置请求无法安全恢复。请恢复存储后核验原请求。",
  );
}
export function loadProjectCoverIntent(
  storage: IntentStorage,
  scope: FolderScope,
  projectId: string,
): ProjectCoverIntent | null {
  try {
    const encoded = storage.getItem(storageKey(scope, projectId));
    if (encoded === null) return null;
    if (encoded.length > 1_048_576) throw storageFailure();
    const intent = projectCoverIntentSchema.parse(JSON.parse(encoded));
    if (!sameFolderScope(scope, intent) || intent.projectId !== projectId)
      throw storageFailure();
    return intent;
  } catch {
    throw storageFailure();
  }
}
export function saveProjectCoverIntent(
  storage: IntentStorage,
  intent: ProjectCoverIntent,
) {
  try {
    const checked = projectCoverIntentSchema.parse(intent);
    const encoded = JSON.stringify(checked);
    if (new TextEncoder().encode(encoded).byteLength > 1_048_576)
      throw storageFailure();
    const key = storageKey(checked, checked.projectId);
    storage.setItem(key, encoded);
    if (storage.getItem(key) !== encoded) throw storageFailure();
  } catch {
    throw storageFailure();
  }
}
export function clearProjectCoverIntent(
  storage: IntentStorage,
  scope: FolderScope,
  projectId: string,
) {
  try {
    const key = storageKey(scope, projectId);
    storage.removeItem(key);
    if (storage.getItem(key) !== null) throw storageFailure();
  } catch {
    throw storageFailure();
  }
}
