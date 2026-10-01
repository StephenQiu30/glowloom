import { z } from "zod";

export const generationConfigSchema = z
  .object({
    version: z.literal(1),
    capability: z.string().trim().min(1).max(128),
    modelProfileId: z.string().uuid().nullable(),
    mode: z.string().max(128),
    prompt: z.string().max(10000),
    params: z.record(z.string(), z.json()).refine((params) => {
      const facts = new Set([
        "status",
        "results",
        "result",
        "task_id",
        "operation_id",
        "batch_id",
        "provider_request_id",
        "quote_micros",
        "receipt",
        "error",
      ]);
      return (
        !Object.keys(params).some((key) => facts.has(key.toLowerCase())) &&
        new TextEncoder().encode(JSON.stringify(params)).byteLength <= 65536
      );
    }, "参数包含执行状态或超过大小限制"),
    outputCount: z.number().int().min(1).max(8),
    inputs: z
      .array(
        z
          .object({
            role: z
              .string()
              .trim()
              .min(1)
              .max(128)
              .refine((role) => role !== "prompt"),
            mediaAssetId: z.string().uuid(),
          })
          .strict(),
      )
      .max(255),
  })
  .strict();
export type GenerationConfig = z.infer<typeof generationConfigSchema>;
export function createGenerationConfig(
  capability = "image.generate",
): GenerationConfig {
  return {
    version: 1,
    capability,
    modelProfileId: null,
    mode: "",
    prompt: "",
    params: {},
    outputCount: 1,
    inputs: [],
  };
}
export function encodeGenerationConfig(value: GenerationConfig) {
  const config = generationConfigSchema.parse(value);
  return {
    version: config.version,
    capability: config.capability,
    model_profile_id: config.modelProfileId,
    mode: config.mode,
    prompt: config.prompt,
    params: config.params,
    output_count: config.outputCount,
    inputs: config.inputs.map((input) => ({
      role: input.role,
      media_asset_id: input.mediaAssetId,
    })),
  };
}
export const generationWireSchema = z
  .object({
    version: z.literal(1),
    capability: z.string(),
    model_profile_id: z.string().uuid().nullable(),
    mode: z.string(),
    prompt: z.string(),
    params: z.record(z.string(), z.json()),
    output_count: z.number(),
    inputs: z.array(
      z
        .object({ role: z.string(), media_asset_id: z.string().uuid() })
        .strict(),
    ),
  })
  .strict()
  .transform((value) => ({
    version: value.version,
    capability: value.capability,
    modelProfileId: value.model_profile_id,
    mode: value.mode,
    prompt: value.prompt,
    params: value.params,
    outputCount: value.output_count,
    inputs: value.inputs.map((input) => ({
      role: input.role,
      mediaAssetId: input.media_asset_id,
    })),
  }))
  .pipe(generationConfigSchema);
export function decodeGenerationConfig(value: unknown): GenerationConfig {
  return generationWireSchema.parse(value);
}
