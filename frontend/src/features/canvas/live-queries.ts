import { z } from "zod";
import * as auth from "@/gen/api/auth";
import * as canvases from "@/gen/api/canvases";
import * as projects from "@/gen/api/projects";
import { ApiError } from "@/lib/request";
import type { CanvasCommand, CanvasDocument } from "./live-state";

const userSchema = z.object({
  id: z.string().uuid(),
  org_id: z.string().uuid(),
  login_name: z.string(),
  display_name: z.string(),
  role: z.string(),
  must_change_password: z.boolean(),
});
const sessionSchema = z.object({
  user: userSchema,
  must_change_password: z.boolean(),
});
export type Session = z.infer<typeof sessionSchema>;
const viewportSchema = z.object({
  x: z.number().finite(),
  y: z.number().finite(),
  zoom: z.number().min(0.1).max(4),
});
const nodeSchema = z.object({
  id: z.string().uuid(),
  node_type: z.literal("text"),
  node_action: z.literal("resource"),
  config: z.object({ text: z.string() }),
  x: z.number().finite(),
  y: z.number().finite(),
  width: z.number().optional(),
  height: z.number().optional(),
});
const edgeSchema = z.object({
  id: z.string().uuid(),
  edge_type: z.literal("annotation"),
  source_node_id: z.string().uuid(),
  target_node_id: z.string().uuid(),
});
const summarySchema = z.object({
  id: z.string().uuid(),
  project_id: z.string().uuid(),
  name: z.string(),
  scope: z.record(z.string(), z.unknown()),
  revision: z.number().int().positive(),
  viewport: viewportSchema,
});
const documentSchema = summarySchema.extend({
  nodes: z.array(nodeSchema),
  edges: z.array(edgeSchema),
});
const canvasListSchema = z.object({
  items: z.array(summarySchema),
  next_cursor: z.string().nullable(),
});
const projectListSchema = z.object({
  items: z.array(
    z.object({
      id: z.string().uuid(),
      name: z.string(),
      status: z.enum(["active", "archived"]),
      revision: z.number().int().positive(),
    }),
  ),
  next_cursor: z.string().nullable(),
});

// 在生成 REST DTO 与编辑器模型之间校验边界，畸形响应不转换成空列表或保存成功。
function parse<T>(schema: z.ZodType<T>, value: unknown): T {
  const result = schema.safeParse(value);
  if (!result.success) throw new ApiError(502, "invalid_response");
  return result.data;
}
const writeOptions = (key: string) => ({ headers: { "Idempotency-Key": key } });
export async function getCurrentUser(signal?: AbortSignal) {
  return parse(sessionSchema, await auth.currentUser({ signal }));
}
export async function login(login_name: string, password: string) {
  return parse(sessionSchema, await auth.login({ login_name, password }));
}
export async function logout(key: string) {
  await auth.logout(writeOptions(key));
}
export async function changePassword(
  current_password: string,
  new_password: string,
  key: string,
) {
  return parse(
    z.object({
      must_change_password: z.literal(false),
      revision: z.number().int().positive(),
    }),
    await auth.changePassword(
      { current_password, new_password },
      writeOptions(key),
    ),
  );
}
export async function listProjects(cursor?: string, signal?: AbortSignal) {
  return parse(
    projectListSchema,
    await projects.listProjects(
      { limit: 100, ...(cursor ? { cursor } : {}) },
      { signal },
    ),
  );
}
export async function listCanvases(
  pid: string,
  _cursor?: string,
  signal?: AbortSignal,
) {
  return parse(
    canvasListSchema,
    await canvases.listCanvases({ pid }, { signal }),
  );
}
export async function createCanvas(
  pid: string,
  name: string,
  key: string,
): Promise<CanvasDocument> {
  return parse(
    documentSchema,
    await canvases.createCanvas(
      { pid },
      { name, scope: {} },
      writeOptions(key),
    ),
  );
}
export async function getCanvas(
  id: string,
  signal?: AbortSignal,
): Promise<CanvasDocument> {
  return parse(documentSchema, await canvases.getCanvas({ id }, { signal }));
}
export async function saveCanvasCommands(
  id: string,
  expected_revision: number,
  commands: CanvasCommand[],
  key: string,
): Promise<CanvasDocument> {
  return parse(
    documentSchema,
    await canvases.applyCanvasCommands(
      { id },
      { expected_revision, commands },
      writeOptions(key),
    ),
  );
}
