import { z } from "zod";
import * as projects from "@/gen/api/projects";
import { ApiError } from "@/lib/request";
import type { CreationBody } from "./creation";

export const PROJECTS_KEY = ["projects"] as const;
const styleSubtype = z.enum([
  "anime_jp",
  "guofeng_xianxia",
  "cartoon_3d",
  "manhwa",
]);
const projectSummary = z.object({
  id: z.string().uuid(),
  name: z.string(),
  aspect_ratio: z.enum(["9:16", "16:9"]),
  style_type: z.enum(["realistic", "stylized"]),
  status: z.enum(["active", "archived"]),
  is_delete: z.boolean(),
  revision: z.number().int().positive(),
});
const projectPage = z.object({
  items: z.array(projectSummary).max(200),
  next_cursor: z.string().min(1).nullable(),
});
const createdProject = z.object({
  id: z.string().uuid(),
  name: z.string(),
  description: z.string(),
  aspect_ratio: z.enum(["9:16", "16:9"]),
  style_type: z.enum(["realistic", "stylized"]),
  style_subtype: styleSubtype.optional(),
  style_preset_id: z.string().uuid().optional(),
  resolution: z.literal("1080p"),
  allow_overseas_models: z.literal(false),
  status: z.literal("active"),
  revision: z.number().int().positive(),
  create_time: z.iso.datetime({ offset: true }),
  update_time: z.iso.datetime({ offset: true }),
});
const presetPage = z.object({
  items: z
    .array(
      z.object({
        id: z.string().uuid(),
        name: z.string(),
        style_type: z.enum(["realistic", "stylized"]),
        style_subtype: styleSubtype.optional(),
      }),
    )
    .max(200),
  next_cursor: z.string().min(1).nullable(),
});
function parse<T>(schema: z.ZodType<T>, value: unknown): T {
  const result = schema.safeParse(value);
  if (!result.success) throw new ApiError(502, "invalid_response");
  return result.data;
}
export async function listProjects(
  params: API.listProjectsParams,
  signal?: AbortSignal,
) {
  return parse(projectPage, await projects.listProjects(params, { signal }));
}
export async function listStylePresets(cursor?: string, signal?: AbortSignal) {
  return parse(
    presetPage,
    await projects.listStylePresets({ limit: 20, cursor }, { signal }),
  );
}
export async function createProject(body: CreationBody, key: string) {
  return parse(
    createdProject,
    await projects.createProject(body, { headers: { "Idempotency-Key": key } }),
  );
}
