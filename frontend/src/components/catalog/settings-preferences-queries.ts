import { z } from "zod";
import { getProjectModelDefaults, listPromptPreferences } from "@/api/settings";
import { ApiError } from "@/lib/request";

export const PROJECT_DEFAULTS_KEY = ["project-model-defaults"] as const;
export const PROMPT_PREFERENCES_KEY = ["workspace-prompt-preferences"] as const;
export const promptOperations = [
  "chapter_assets_extract",
  "storyboard_plan",
  "storyboard_repair",
  "storyboard_first_frame",
  "storyboard_video",
  "character_extract",
  "character_turnaround",
  "short_drama_outline",
  "skill_draft",
] as const;
const operation = z.enum(promptOperations);
export const promptMode = z.enum(["inherit", "append", "rewrite"]);
export const projectDefaultsSchema = z.object({
  project_id: z.string().uuid(),
  revision: z.number().int().positive(),
  default_models: z.record(
    z.string().min(1).max(128),
    z.string().min(1).max(128),
  ),
});
export type ProjectModelDefaults = z.infer<typeof projectDefaultsSchema>;
export const promptCustomizationSchema = z.object({
  id: z.string().uuid(),
  operation,
  mode: promptMode,
  content: z.string().refine((value) => [...value].length <= 12000),
  base_template_id: z.string().uuid(),
  revision: z.number().int().positive(),
  update_time: z.iso.datetime({ offset: true }),
});
const definition = z.object({
  operation,
  label: z.string().min(1),
  category: z.string().min(1),
  description: z.string(),
  output_type: z.string().min(1),
  schema_key: z.string(),
  content: z.string(),
  output_contract: z.string().min(1),
  template_id: z.string().uuid(),
  template_version: z.number().int().positive(),
  variables: z.array(
    z.object({
      label: z.string().min(1),
      placeholder: z.string().regex(/^\{\{[^{}]+\}\}$/),
    }),
  ),
});
export const promptPreferenceSchema = z
  .object({
    definition,
    customization: promptCustomizationSchema.nullable(),
    outdated: z.boolean(),
  })
  .refine(
    (value) =>
      !value.customization ||
      value.customization.operation === value.definition.operation,
  );
export type PromptPreference = z.infer<typeof promptPreferenceSchema>;
export type PromptCustomization = z.infer<typeof promptCustomizationSchema>;

// Defaults seed only a new draft; an explicit draft model remains the caller's choice.
export async function queryProjectModelDefaults(
  projectId: string,
  signal?: AbortSignal,
) {
  const result = projectDefaultsSchema.safeParse(
    await getProjectModelDefaults({ pid: projectId }, { signal }),
  );
  if (!result.success || result.data.project_id !== projectId)
    throw new ApiError(502, "invalid_response");
  return result.data;
}
export async function queryPromptPreferences(signal?: AbortSignal) {
  const result = z
    .object({ items: z.array(promptPreferenceSchema).length(9) })
    .safeParse(await listPromptPreferences({ signal }));
  if (
    !result.success ||
    new Set(result.data.items.map((item) => item.definition.operation)).size !==
      9
  )
    throw new ApiError(502, "invalid_response");
  return result.data.items;
}

export function preferenceError(error: unknown) {
  if (error instanceof ApiError && error.code === "baseline_changed")
    return "默认模板已更新，请重新读取并确认创作要求后保存。";
  if (error instanceof ApiError && error.code === "catalog_unavailable")
    return "所选模型已不可用，请重新读取项目模型目录后选择。";
  return error instanceof ApiError
    ? error.message
    : "保存结果尚未确认，请重试或重新读取。";
}
