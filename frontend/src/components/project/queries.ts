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
  archived_at: z.iso.datetime({ offset: true }).nullable().optional(),
  delete_time: z.iso.datetime({ offset: true }).nullable().optional(),
  purge_after: z.iso.datetime({ offset: true }).nullable().optional(),
});
export type ProjectSummary = z.infer<typeof projectSummary>;
const projectDetail = projectSummary.extend({
  description: z.string(),
  style_subtype: styleSubtype.optional(),
  style_preset_id: z.string().uuid().nullable(),
  resolution: z.literal("1080p"),
  allow_overseas_models: z.boolean(),
  default_models: z.record(z.string(), z.string().uuid()),
  archived_at: z.iso.datetime({ offset: true }).nullable(),
  delete_time: z.iso.datetime({ offset: true }).nullable(),
  purge_after: z.iso.datetime({ offset: true }).nullable(),
  create_time: z.iso.datetime({ offset: true }),
  update_time: z.iso.datetime({ offset: true }),
});
export type ProjectDetail = z.infer<typeof projectDetail>;
export type ProjectChange =
  API.internalWorkspaceAdapterHttpProjectUpdateRequest;
export type ProjectTransition = "archive" | "unarchive" | "delete" | "restore";
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
export async function getProject(id: string, signal?: AbortSignal) {
  return parse(
    projectDetail.extend({ id: z.literal(id) }),
    await projects.getProject({ pid: id }, { signal }),
  );
}
export async function updateProject(
  id: string,
  body: ProjectChange,
  key: string,
) {
  return parse(
    projectDetail.extend({ id: z.literal(id) }),
    await projects.updateProject({ pid: id }, body, {
      headers: { "Idempotency-Key": key },
    }),
  );
}
export async function transitionProject(
  id: string,
  action: ProjectTransition,
  revision: number,
  key: string,
) {
  const command = {
    archive: projects.archiveProject,
    unarchive: projects.unarchiveProject,
    delete: projects.deleteProject,
    restore: projects.restoreProject,
  }[action];
  return parse(
    projectDetail.extend({ id: z.literal(id) }),
    await command(
      { pid: id },
      { expected_revision: revision },
      {
        headers: { "Idempotency-Key": key },
      },
    ),
  );
}
