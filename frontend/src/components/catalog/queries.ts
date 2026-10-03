import { z } from "zod";
import { listProjectModels } from "@/api/models";
import { ApiError } from "@/lib/request";

export const MODELS_KEY = ["models"] as const;
const parameter = z.object({
  field: z.string().min(1),
  label: z.string().min(1),
  type: z.enum(["string", "integer", "number", "boolean"]),
  component: z.enum([
    "input",
    "textarea",
    "select",
    "slider",
    "switch",
    "segmented",
    "voice",
  ]),
  default: z.union([z.string(), z.number().finite(), z.boolean()]).optional(),
  enum: z.array(z.union([z.string(), z.number().finite()])).optional(),
  for_modes: z.array(z.string()).optional(),
  required: z.boolean().optional(),
  min: z.number().finite().optional(),
  max: z.number().finite().optional(),
  step: z.number().positive().optional(),
  description: z.string().optional(),
});
const model = z.object({
  id: z.string().uuid(),
  key: z.string().min(1),
  display_name: z.string(),
  capability: z.string(),
  status: z.string(),
  provider_id: z.string().uuid(),
  provider_name: z.string(),
  region: z.string(),
  provider_status: z.string(),
  credential_present: z.boolean(),
  input_roles: z.array(z.string()).optional().default([]),
  credential_test_result: z.string().nullable(),
  current_version: z
    .object({
      id: z.string().uuid(),
      version_no: z.number().int().positive(),
      modes: z.array(z.string()),
      limits: z.record(z.string(), z.json()),
      param_schema: z.array(parameter),
    })
    .nullable(),
  current_price: z
    .object({
      id: z.string().uuid(),
      version_no: z.number().int().positive(),
      unit: z.string(),
      rule: z.record(z.string(), z.json()),
      currency: z.string(),
      fx_rate_to_cny: z.string().nullable(),
      effective_from: z.iso.datetime({ offset: true }),
    })
    .nullable(),
});
export type Model = z.infer<typeof model>;
export async function queryModels(
  projectId: string,
  capability?: string,
  signal?: AbortSignal,
  cursor?: string,
) {
  const response = await listProjectModels(
    { project_id: projectId, capability, limit: 100, cursor },
    { signal },
  );
  const result = z
    .object({
      items: z.array(model).max(200),
      next_cursor: z.string().nullable(),
    })
    .safeParse(response);
  if (!result.success) throw new ApiError(502, "invalid_response");
  return result.data;
}
