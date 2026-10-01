import { z } from "zod";
import * as canvases from "@/gen/api/canvases";
import { ApiError } from "@/lib/request";
import {
  generationWireSchema,
  encodeGenerationConfig,
  type GenerationConfig,
} from "@/components/canvas/generation-config";

const node = z.object({
  id: z.string().uuid(),
  node_type: z.string(),
  config: z.object({ generation: generationWireSchema.optional() }),
});
const document = z.object({
  id: z.string().uuid(),
  project_id: z.string().uuid(),
  revision: z.number().int().positive(),
  name: z.string(),
  nodes: z
    .array(node)
    .nullable()
    .transform((value) => value ?? []),
});
export type DraftDocument = z.infer<typeof document>;
export type DraftSelection = {
  document: DraftDocument | null;
  nodeId: string | null;
  config: GenerationConfig | null;
};
function parse<T>(schema: z.ZodType<T>, value: unknown): T {
  const result = schema.safeParse(value);
  if (!result.success) throw new ApiError(502, "invalid_response");
  return result.data;
}
function scoped(value: unknown, projectId: string) {
  const result = parse(document, value);
  if (result.project_id !== projectId)
    throw new ApiError(502, "invalid_response");
  return result;
}
export async function loadGenerationDraft(
  projectId: string,
  capability: string,
  canvasId?: string,
  nodeId?: string,
  signal?: AbortSignal,
): Promise<DraftSelection> {
  let id = canvasId;
  if (!id) {
    const list = parse(
      z.object({
        items: z.array(z.object({ id: z.string().uuid(), name: z.string() })),
      }),
      await canvases.listCanvases({ pid: projectId }, { signal }),
    );
    id = list.items[0]?.id;
  }
  if (!id) {
    if (nodeId) throw new ApiError(404, "not_found");
    return { document: null, nodeId: null, config: null };
  }
  const value = scoped(await canvases.getCanvas({ id }, { signal }), projectId);
  const existing = nodeId
    ? value.nodes.find((node) => node.id === nodeId)
    : value.nodes.find(
        (node) =>
          node.node_type === "generation" &&
          node.config.generation?.capability === capability,
      );
  if (
    nodeId &&
    (!existing ||
      existing.node_type !== "generation" ||
      !existing.config.generation)
  )
    throw new ApiError(404, "not_found");
  return {
    document: value,
    nodeId: existing?.id ?? null,
    config: existing?.config.generation ?? null,
  };
}
export type DraftSaveRequest = {
  canvasId: string;
  revision: number;
  nodeId: string;
  config: GenerationConfig;
  existing: boolean;
  key: string;
};
export async function createDraftCanvas(projectId: string, key: string) {
  return scoped(
    await canvases.createCanvas(
      { pid: projectId },
      { name: "创作画布", scope: {} },
      { headers: { "Idempotency-Key": key } },
    ),
    projectId,
  );
}
export async function saveGenerationDraft(
  projectId: string,
  request: DraftSaveRequest,
) {
  const config = { generation: encodeGenerationConfig(request.config) };
  const commands = request.existing
    ? [{ type: "UpdateNodeConfig", id: request.nodeId, config }]
    : [
        {
          type: "AddNodes",
          nodes: [
            {
              id: request.nodeId,
              node_type: "generation",
              node_action: "tool",
              title: "创作",
              config,
              x: 0,
              y: 0,
              width: 360,
              height: 260,
              z_index: 0,
            },
          ],
        },
      ];
  const result = scoped(
    await canvases.applyCanvasCommands(
      { id: request.canvasId },
      { expected_revision: request.revision, commands },
      { headers: { "Idempotency-Key": request.key } },
    ),
    projectId,
  );
  const saved = result.nodes.find((node) => node.id === request.nodeId);
  if (!saved?.config.generation) throw new ApiError(502, "invalid_response");
  return {
    document: result,
    nodeId: saved.id,
    config: saved.config.generation,
  };
}
