import { z } from "zod";
import {
  getAdminModel,
  getAdminProvider,
  listAdminCapabilities,
  listAdminModels,
  listAdminProviders,
} from "@/gen/api/settings";
import { ApiError } from "@/lib/request";

export const ADMIN_KEY = ["catalog-admin"] as const;
const id = z.string().uuid();
const status = z.enum(["active", "disabled"]);
const timestamp = z.iso.datetime({ offset: true });
const jsonObject = z.record(z.string(), z.json());
export const providerSchema = z.object({
  id,
  key: z.string().min(1),
  name: z.string().min(1),
  adapter_key: z.string().min(1),
  region: z.enum(["domestic", "overseas"]),
  status,
  concurrency_limit: z.number().int().positive(),
  rate_limit_per_min: z.number().int().positive(),
  revision: z.number().int().positive(),
  create_time: timestamp,
  update_time: timestamp,
});
const credentialSchema = z.object({
  id,
  provider_id: id,
  label: z.string(),
  last4: z.string().length(4),
  status,
  last_test_result: z
    .enum(["ok", "auth_failed", "unreachable", "timeout", "unsupported"])
    .nullable(),
  last_tested_at: timestamp.nullable(),
  create_time: timestamp,
  update_time: timestamp,
});
export const providerDetailSchema = z.object({
  provider: providerSchema,
  credential: credentialSchema.nullable(),
  credential_schema: z
    .array(
      z.object({
        name: z
          .string()
          .regex(/^[a-z][a-z0-9_]{0,63}$/)
          .refine(
            (value) =>
              !["constructor", "prototype", "__proto__"].includes(value),
          ),
        type: z.string(),
        required: z.boolean(),
      }),
    )
    .max(16)
    .refine(
      (fields) =>
        new Set(fields.map((field) => field.name)).size === fields.length,
    ),
});
export const adminModelSchema = z.object({
  id,
  provider_id: id,
  model_key: z.string().min(1),
  display_name: z.string().min(1),
  capability: z.string().min(1),
  current_version_id: id,
  revision: z.number().int().positive(),
  status,
  create_time: timestamp,
  update_time: timestamp,
});
export const modelDetailSchema = z.object({
  model: adminModelSchema,
  versions: z.array(
    z.object({
      id,
      version_no: z.number().int().positive(),
      provider_model_id: z.string().min(1),
      modes: z.array(z.string()),
      limits: jsonObject,
      param_schema: z.array(jsonObject),
      supports_query: z.boolean(),
      supports_cancel: z.boolean(),
      supports_callback: z.boolean(),
      expected_max_ms: z.number().int().positive(),
      moderation: z.enum(["provider", "platform", "both"]),
      queue: z.string().min(1),
      create_time: timestamp,
    }),
  ),
  prices: z.array(
    z.object({
      id,
      version_no: z.number().int().positive(),
      unit: z.enum([
        "per_image",
        "per_second",
        "per_request",
        "per_1k_tokens",
        "per_1k_chars",
      ]),
      rule: jsonObject,
      currency: z.string().length(3),
      fx_rate_to_cny: z.string().nullable(),
      effective_from: timestamp,
      create_time: timestamp,
    }),
  ),
});
export const capabilitiesSchema = z.array(
  z.object({
    id,
    key: z.string(),
    output_type: z.string(),
    modes: z.array(z.string()),
    input_roles: z.array(z.string()),
  }),
);
export const credentialTestAcceptedSchema = z.object({
  accepted: z.literal(true),
  provider_id: id,
  credential_id: id,
  event_id: id,
  test_id: id,
});
export type Provider = z.infer<typeof providerSchema>;
export type ProviderDetail = z.infer<typeof providerDetailSchema>;
export type AdminModel = z.infer<typeof adminModelSchema>;
export type ModelDetail = z.infer<typeof modelDetailSchema>;
export type Capability = z.infer<typeof capabilitiesSchema>[number];

export function parseAdmin<T>(schema: z.ZodType<T>, response: unknown): T {
  const parsed = schema.safeParse(response);
  if (!parsed.success) throw new ApiError(502, "invalid_response");
  return parsed.data;
}
export async function queryProviders(signal?: AbortSignal, cursor?: string) {
  return parseAdmin(
    z.object({
      items: z.array(
        z.object({
          provider: providerSchema,
          credential: credentialSchema.nullable(),
          model_count: z.number().int().nonnegative(),
        }),
      ),
      next_cursor: z.string().nullable(),
    }),
    await listAdminProviders({ limit: 100, cursor }, { signal }),
  );
}
export async function queryAdminModels(signal?: AbortSignal, cursor?: string) {
  return parseAdmin(
    z.object({
      items: z.array(adminModelSchema),
      next_cursor: z.string().nullable(),
    }),
    await listAdminModels({ limit: 100, cursor }, { signal }),
  );
}
export async function queryProvider(id: string, signal?: AbortSignal) {
  const detail = parseAdmin(
    providerDetailSchema,
    await getAdminProvider({ id }, { signal }),
  );
  if (
    detail.provider.id !== id ||
    (detail.credential && detail.credential.provider_id !== id)
  )
    throw new ApiError(502, "invalid_response");
  return detail;
}
export async function queryAdminModel(id: string, signal?: AbortSignal) {
  const detail = parseAdmin(
    modelDetailSchema,
    await getAdminModel({ id }, { signal }),
  );
  if (detail.model.id !== id) throw new ApiError(502, "invalid_response");
  return detail;
}
export async function queryCapabilities(signal?: AbortSignal) {
  return parseAdmin(
    capabilitiesSchema,
    await listAdminCapabilities({ signal }),
  );
}

export function adminWriteOptions() {
  return { headers: { "Idempotency-Key": crypto.randomUUID() } };
}
export function adminError(error: unknown) {
  return error instanceof ApiError
    ? error.message
    : "操作未完成，请重新读取后重试。";
}
