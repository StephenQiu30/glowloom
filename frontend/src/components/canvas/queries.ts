import * as transcriptions from "@/gen/api/mediaTranscriptions";
import {
  transcriptionJobSchema,
  transcriptionResultSchema,
  transcriptionSourceSchema,
  type TranscriptionJob,
  type TranscriptionSource,
} from "./transcription-model";
import {
  generationWireSchema,
  encodeGenerationConfig,
  decodeGenerationConfig,
} from "./generation-config";
import { directorWireSchema, decodeDirector } from "./director/model";
import { batchWireSchema, decodeBatchConfig } from "./batch-table";
import { timelineWireSchema, decodeTimeline } from "./timeline";
import { z } from "zod";
import * as canvases from "@/gen/api/canvases";
import * as projects from "@/gen/api/projects";
import * as media from "@/gen/api/media";
import { ApiError } from "@/lib/request";
import { nodeConfig } from "./document";
import {
  CanvasNodeType,
  type CanvasCommand,
  type CanvasDocument,
  type CanvasNodeData,
} from "./model";
const uuid = z.string().uuid();
const nodeSchema = z.object({
  id: uuid,
  node_type: z.enum([
    "text",
    "image",
    "video",
    "audio",
    "model",
    "group",
    "batch_table",
    "timeline",
    "director",
    "generation",
  ]),
  node_action: z.enum(["resource", "tool"]),
  title: z.string(),
  config: z.object({
    text: z.string().optional(),
    collapsed: z.boolean().optional(),
    batch_table: batchWireSchema.optional(),
    timeline: timelineWireSchema.optional(),
    director: directorWireSchema.optional(),
    generation: generationWireSchema
      .transform(encodeGenerationConfig)
      .optional(),
  }),
  x: z.number().finite(),
  y: z.number().finite(),
  width: z.number().optional(),
  height: z.number().optional(),
  parent_id: uuid.nullish(),
  z_index: z.number().int(),
  ref_type: z.string().optional(),
  ref_id: uuid.optional(),
});
const summarySchema = z.object({
  id: uuid,
  project_id: uuid,
  name: z.string(),
  revision: z.number().int().positive(),
  scope: z.record(z.string(), z.unknown()),
  viewport: z.object({
    x: z.number().finite(),
    y: z.number().finite(),
    zoom: z.number().min(0.05).max(4),
  }),
});
const documentSchema = summarySchema.extend({
  nodes: z.array(nodeSchema),
  edges: z.array(
    z.object({
      id: uuid,
      edge_type: z.literal("annotation"),
      source_node_id: uuid,
      target_node_id: uuid,
    }),
  ),
});
const projectListSchema = z.object({
  items: z.array(
    z.object({
      id: uuid,
      name: z.string(),
      status: z.enum(["active", "archived"]),
      revision: z.number().int().positive(),
    }),
  ),
  next_cursor: z.string().nullable(),
});
export const mediaSchema = z.object({
  id: uuid,
  project_id: uuid,
  kind: z.enum(["image", "video", "audio", "model"]),
  file_name: z.string(),
  mime_type: z.string(),
  byte_size: z.number(),
  width: z.number().optional(),
  height: z.number().optional(),
  duration_ms: z.number().optional(),
  revision: z.number().int().positive(),
});
export type MediaAsset = z.infer<typeof mediaSchema>;
function parse<T>(schema: z.ZodType<T>, value: unknown): T {
  const result = schema.safeParse(value);
  if (!result.success) throw new ApiError(502, "invalid_response");
  return result.data;
}
export function decodeDocument(response: unknown): CanvasDocument {
  const doc = parse(documentSchema, response);
  return {
    id: doc.id,
    projectId: doc.project_id,
    name: doc.name,
    revision: doc.revision,
    scope: doc.scope,
    viewport: { x: doc.viewport.x, y: doc.viewport.y, k: doc.viewport.zoom },
    connections: doc.edges.map((edge) => ({
      id: edge.id,
      fromNodeId: edge.source_node_id,
      toNodeId: edge.target_node_id,
    })),
    nodes: doc.nodes.map((node) => ({
      id: node.id,
      type: node.node_type as CanvasNodeType,
      title: node.title,
      position: { x: node.x, y: node.y },
      width: node.width ?? (node.node_type === "group" ? 640 : 320),
      height: node.height ?? (node.node_type === "group" ? 420 : 220),
      parentId: node.parent_id ?? undefined,
      zIndex: node.z_index,
      assetId: node.ref_type === "media_asset" ? node.ref_id : undefined,
      ...(node.node_type === "batch_table"
        ? { batchTable: decodeBatchConfig(node.config.batch_table) }
        : {}),
      ...(node.node_type === "generation"
        ? { generation: decodeGenerationConfig(node.config.generation) }
        : {}),
      ...(node.node_type === "director"
        ? { director: decodeDirector(node.config.director) }
        : {}),
      ...(node.node_type === "timeline"
        ? { timeline: decodeTimeline(node.config.timeline) }
        : {}),
      metadata:
        node.node_type === "text"
          ? { content: node.config.text ?? "" }
          : node.node_type === "group"
            ? {
                frame: {
                  collapsed: node.config.collapsed ?? false,
                  expandedWidth: node.width ?? 640,
                  expandedHeight: node.height ?? 420,
                },
              }
            : {},
    })),
  };
}
function encodeNode(node: CanvasNodeData) {
  return {
    id: node.id,
    node_type: node.type,
    node_action:
      node.type === CanvasNodeType.BatchTable ||
      node.type === CanvasNodeType.Director ||
      node.type === CanvasNodeType.Generation ||
      node.type === CanvasNodeType.Timeline
        ? "tool"
        : "resource",
    title: node.title,
    config: nodeConfig(node),
    ...node.position,
    width: node.width,
    height: node.height,
    z_index: node.zIndex,
    ...(node.parentId ? { parent_id: node.parentId } : {}),
    ...(node.assetId ? { ref_type: "media_asset", ref_id: node.assetId } : {}),
  };
}
export function encodeCommands(commands: CanvasCommand[]) {
  return commands.map((command) => {
    if (command.type === "AddNodes")
      return { type: command.type, nodes: command.nodes.map(encodeNode) };
    if (command.type === "Connect")
      return {
        type: command.type,
        edges: command.edges.map((edge) => ({
          id: edge.id,
          edge_type: "annotation",
          source_node_id: edge.fromNodeId,
          target_node_id: edge.toNodeId,
        })),
      };
    if (command.type === "SetViewport")
      return {
        type: command.type,
        viewport: {
          x: command.viewport.x,
          y: command.viewport.y,
          zoom: command.viewport.k,
        },
      };
    return command;
  });
}
const writeOptions = (key: string) => ({ headers: { "Idempotency-Key": key } });
export async function listProjects(cursor?: string, signal?: AbortSignal) {
  return parse(
    projectListSchema,
    await projects.listProjects(
      { limit: 100, ...(cursor ? { cursor } : {}) },
      { signal },
    ),
  );
}
export async function listCanvases(pid: string, signal?: AbortSignal) {
  return parse(
    z.object({
      items: z.array(summarySchema),
      next_cursor: z.string().nullable(),
    }),
    await canvases.listCanvases({ pid }, { signal }),
  );
}
export async function createCanvas(pid: string, name: string, key: string) {
  return decodeDocument(
    await canvases.createCanvas(
      { pid },
      { name, scope: {} },
      writeOptions(key),
    ),
  );
}
export async function getCanvas(id: string, signal?: AbortSignal) {
  return decodeDocument(await canvases.getCanvas({ id }, { signal }));
}
export async function saveCanvasCommands(
  id: string,
  expected_revision: number,
  commands: CanvasCommand[],
  key: string,
) {
  return decodeDocument(
    await canvases.applyCanvasCommands(
      { id },
      { expected_revision, commands: encodeCommands(commands) },
      writeOptions(key),
    ),
  );
}
export async function renameCanvas(
  id: string,
  expected_revision: number,
  name: string,
  key: string,
) {
  return decodeDocument(
    await canvases.renameCanvas(
      { id },
      { expected_revision, name },
      writeOptions(key),
    ),
  );
}
export async function deleteCanvas(
  id: string,
  expected_revision: number,
  key: string,
) {
  return parse(
    z.object({
      id: uuid,
      revision: z.number().int().positive(),
      deleted: z.literal(true),
    }),
    await canvases.deleteCanvas(
      { id },
      { expected_revision },
      writeOptions(key),
    ),
  );
}
export async function listMediaAssets(
  pid: string,
  cursor?: string,
  signal?: AbortSignal,
) {
  return parse(
    z.object({
      items: z.array(mediaSchema),
      next_cursor: z.string().nullable(),
    }),
    await media.listMediaAssets(
      { pid, limit: 50, ...(cursor ? { cursor } : {}) },
      { signal },
    ),
  );
}
export async function getMediaPreview(
  pid: string,
  asset_id: string,
  signal?: AbortSignal,
) {
  return parse(
    z.object({
      asset: mediaSchema,
      url: z.string().url(),
      expires_at: z.string(),
    }),
    await media.getMediaPreview({ pid, asset_id }, { signal }),
  );
}

export async function uploadCanvasMedia(
  projectId: string,
  file: File,
  key: string,
  {
    signal,
    onProgress,
  }: {
    signal: AbortSignal;
    onProgress: (progress: { loaded: number; total?: number }) => void;
  },
): Promise<MediaAsset> {
  const response = parse(
    z.object({ asset: mediaSchema, duplicate_of: uuid.nullable() }),
    await media.uploadMediaAsset(
      { pid: projectId },
      { local_review_confirmed: true },
      file,
      {
        signal,
        headers: { "Idempotency-Key": key },
        timeout: 300_000,
        onUploadProgress: (event) =>
          onProgress({ loaded: event.loaded, total: event.total }),
      },
    ),
  );
  if (response.asset.project_id !== projectId)
    throw new ApiError(502, "invalid_response");
  return response.asset;
}

// Local speech jobs use the same generated request and project-scoped cache.
export const MEDIA_TRANSCRIPTIONS_KEY = [
  "canvas",
  "media-transcriptions",
] as const;
function transcriptionJob(value: unknown, projectId: string, id?: string) {
  const result = parse(transcriptionJobSchema, value);
  if (result.project_id !== projectId || (id && result.id !== id))
    throw new ApiError(502, "invalid_response");
  return result;
}
export async function createMediaTranscription(
  projectId: string,
  source: TranscriptionSource,
  language: string,
  key: string,
) {
  const frozen = transcriptionSourceSchema.parse(source);
  const value = transcriptionJob(
    await transcriptions.createMediaTranscription(
      { pid: projectId },
      { ...frozen, language },
      writeOptions(key),
    ),
    projectId,
  );
  if (
    value.source.canvas_id !== frozen.canvas_id ||
    value.source.node_id !== frozen.node_id ||
    value.source.revision !== frozen.revision ||
    value.language !== language
  )
    throw new ApiError(502, "invalid_response");
  return value;
}
export async function listMediaTranscriptions(
  projectId: string,
  canvasId: string,
  nodeId?: string,
  cursor?: string,
  signal?: AbortSignal,
) {
  const value = parse(
    z.object({
      items: z.array(transcriptionJobSchema).max(25),
      next_cursor: z.string().nullable(),
    }),
    await transcriptions.listMediaTranscriptions(
      {
        pid: projectId,
        canvas_id: canvasId,
        ...(nodeId ? { node_id: nodeId } : {}),
        limit: 25,
        ...(cursor ? { cursor } : {}),
      },
      { signal },
    ),
  );
  if (
    value.items.some(
      (item) =>
        item.project_id !== projectId ||
        item.source.canvas_id !== canvasId ||
        (nodeId && item.source.node_id !== nodeId),
    )
  )
    throw new ApiError(502, "invalid_response");
  return value;
}
export async function getMediaTranscriptionResult(
  job: TranscriptionJob,
  signal?: AbortSignal,
) {
  const value = parse(
    transcriptionResultSchema,
    await transcriptions.getMediaTranscriptionResult(
      { job_id: job.id, project_id: job.project_id },
      { signal },
    ),
  );
  if (
    job.status !== "succeeded" ||
    value.job_id !== job.id ||
    value.revision !== job.revision ||
    value.sha256 !== job.result_sha256
  )
    throw new ApiError(502, "invalid_response");
  return value;
}
export async function controlMediaTranscription(
  job: TranscriptionJob,
  action: "cancel" | "retry",
  key: string,
) {
  const invoke =
    action === "cancel"
      ? transcriptions.cancelMediaTranscription
      : transcriptions.retryMediaTranscription;
  return transcriptionJob(
    await invoke(
      { job_id: job.id },
      { project_id: job.project_id, revision: job.revision },
      writeOptions(key),
    ),
    job.project_id,
    job.id,
  );
}
export async function downloadMediaTranscriptionSubtitles(
  job: TranscriptionJob,
  signal?: AbortSignal,
) {
  const value: unknown =
    await transcriptions.downloadMediaTranscriptionSubtitles(
      { job_id: job.id, project_id: job.project_id },
      { responseType: "blob", signal },
    );
  if (
    !(value instanceof Blob) ||
    value.size < 1 ||
    value.size > 8 * 1024 * 1024 ||
    !value.type.startsWith("application/x-subrip")
  )
    throw new ApiError(502, "invalid_response");
  return value;
}
