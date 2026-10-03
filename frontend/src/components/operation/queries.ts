import { z } from "zod";
import * as operations from "@/api/operations";
import { ApiError } from "@/lib/request";
import type { QuoteResponse } from "./use-quote";

export const OPERATIONS_KEY = ["operations"] as const;
const micros = z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER);
const quoteDetail = z.object({
  unit: z.string().optional(),
  quantity: z.number().nonnegative().finite().optional(),
  unit_price_micros: micros.optional(),
  outputs: z.number().int().nonnegative().optional(),
  multipliers: z
    .record(z.string(), z.number().nonnegative().finite())
    .optional(),
  currency: z.string().optional(),
  fx_rate_to_cny: z.string().optional(),
  estimated_input_tokens: micros.optional(),
  max_output_tokens: micros.optional(),
  input_unit_price_micros: micros.optional(),
  output_unit_price_micros: micros.optional(),
});
const status = z.enum([
  "draft",
  "quoted",
  "expired",
  "confirmed",
  "submitting",
  "submitted",
  "unknown",
  "reconciling",
  "manual",
  "ingesting",
  "completed",
  "cancelling",
  "succeeded",
  "failed",
  "cancelled",
]);
const canvasSource = z.object({
  canvas_id: z.string().uuid(),
  node_id: z.string().uuid(),
  row_id: z.string().uuid().optional(),
  revision: z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER),
});
export type GenerationSource = z.infer<typeof canvasSource>;
const task = z.object({
  id: z.string().uuid(),
  project_id: z.string().uuid(),
  batch_id: z.string().uuid().nullable(),
  capability: z.string(),
  mode: z.string(),
  model_key: z.string(),
  model_name: z.string(),
  target_type: z.string(),
  target_id: z.string().uuid().nullable(),
  origin: z.string(),
  source: canvasSource.nullable().optional(),
  status,
  quote_micros: micros.nullable(),
  settled_micros: micros.nullable(),
  failure_code: z.string().nullable(),
  retryable: z.boolean().nullable(),
  cancel_requested: z.boolean(),
  create_time: z.iso.datetime({ offset: true }),
  update_time: z.iso.datetime({ offset: true }),
  started_at: z.iso.datetime({ offset: true }).nullable(),
  finished_at: z.iso.datetime({ offset: true }).nullable(),
});
const taskDetail = task.extend({
  params: z.record(z.string(), z.json()),
  output_count: z.number().int().min(1).max(8),
  quote_detail: z.record(z.string(), z.json()).nullable(),
  quote_expires_at: z.iso.datetime({ offset: true }).nullable(),
  reused_from_id: z.string().uuid().nullable(),
  inputs: z.array(
    z.object({
      sequence: z.number().int(),
      role: z.string(),
      text: z.string().nullable(),
      media_asset_id: z.string().uuid().nullable(),
      mask_asset_id: z.string().uuid().nullable(),
    }),
  ),
  outputs: z.array(
    z.object({
      id: z.string().uuid(),
      sequence: z.number().int(),
      kind: z.string(),
      media_asset_id: z.string().uuid().nullable(),
      moderation_status: z.string(),
      create_time: z.iso.datetime({ offset: true }),
    }),
  ),
  events: z.array(
    z.object({
      id: z.string().uuid(),
      from_status: z.string().nullable(),
      to_status: z.string(),
      reason: z.string().nullable(),
      create_time: z.iso.datetime({ offset: true }),
    }),
  ),
});
export type Task = z.infer<typeof task>;
export type GenerationItem = {
  source?: GenerationSource;
  model_key: string;
  capability: string;
  mode: string;
  prompt: string;
  params: Record<string, z.core.util.JSONType>;
  output_count: number;
  force_regenerate: boolean;
  media_inputs: { role: string; media_asset_id: string }[];
};
function parse<T>(schema: z.ZodType<T>, value: unknown): T {
  const result = schema.safeParse(value);
  if (!result.success) throw new ApiError(502, "invalid_response");
  return result.data;
}

export async function quoteGeneration(
  projectId: string,
  items: GenerationItem[],
  key: string,
  labels: string[] = [],
): Promise<QuoteResponse> {
  const response = await operations.createGenerationQuotes(
    { pid: projectId },
    { items },
    { headers: { "Idempotency-Key": key } },
  );
  const value = parse(
    z.object({
      batch_id: z.string().uuid().optional(),
      expires_at: z.iso.datetime({ offset: true }),
      items: z
        .array(
          z.object({
            operation_id: z.string().uuid().optional(),
            model_key: z.string(),
            mode: z.string(),
            quote_micros: micros.optional(),
            quote_detail: quoteDetail.optional(),
            reused_from_id: z.string().uuid().optional(),
            region: z.string().optional(),
            error_code: z.string().optional(),
          }),
        )
        .min(1)
        .max(150),
      total_micros: micros,
      available_micros: micros,
      confirmable: z.boolean(),
    }),
    response,
  );
  if (value.items.length !== items.length)
    throw new ApiError(502, "invalid_response");
  return {
    batch_id: value.batch_id ?? null,
    expires_at: value.expires_at,
    total_micros: value.total_micros,
    available_micros: value.available_micros,
    confirmable: value.confirmable,
    items: value.items.map((item, index) => ({
      operation_id: item.operation_id ?? null,
      target_label: labels[index] ?? `创作 ${index + 1}`,
      model_key: item.model_key,
      mode: item.mode,
      quote_micros: item.quote_micros,
      quote_detail: item.quote_detail,
      reused: !!item.reused_from_id,
      region: item.region,
      errors: item.error_code
        ? [
            {
              code: item.error_code,
              message: `此项未能报价（${item.error_code}）`,
            },
          ]
        : [],
    })),
  };
}
export async function confirmGeneration(
  projectId: string,
  quote: QuoteResponse,
  excluded: string[],
  key: string,
) {
  if (quote.batch_id)
    return operations.confirmBatchQuote(
      { id: quote.batch_id },
      { project_id: projectId, exclude_operation_ids: excluded },
      { headers: { "Idempotency-Key": key } },
    );
  const chosen = quote.items.filter(
    (item) => item.operation_id && !excluded.includes(item.operation_id),
  );
  if (chosen.length !== 1 || !chosen[0].operation_id)
    throw new ApiError(422, "invalid_request");
  return operations.confirmOperationQuote(
    { id: chosen[0].operation_id },
    { project_id: projectId },
    { headers: { "Idempotency-Key": key } },
  );
}
export async function queryTasks(
  projectId: string,
  filters: {
    status?: string;
    capability?: string;
    cursor?: string;
    canvas_id?: string;
    node_id?: string;
    row_id?: string;
  } = {},
  signal?: AbortSignal,
) {
  return parse(
    z.object({
      items: z.array(task).max(200),
      next_cursor: z.string().nullable(),
    }),
    await operations.listProjectOperations(
      { pid: projectId, limit: 50, ...filters },
      { signal },
    ),
  );
}
export async function queryTask(
  projectId: string,
  id: string,
  signal?: AbortSignal,
) {
  return parse(
    taskDetail,
    await operations.getOperation({ id, project_id: projectId }, { signal }),
  );
}
export async function requestTaskCancel(
  projectId: string,
  id: string,
  key: string,
) {
  return operations.cancelOperation(
    { id },
    { project_id: projectId },
    { headers: { "Idempotency-Key": key } },
  );
}
