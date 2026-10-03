"use client";

import { useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation } from "@tanstack/react-query";
import { z } from "zod";
import {
  createAdminModel,
  publishAdminModelPrice,
  publishAdminModelVersion,
} from "@/api/settings";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { FieldGroup } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  adminError,
  adminModelSchema,
  adminWriteOptions,
  parseAdmin,
  type AdminModel,
  type Capability,
  type ModelDetail,
  type Provider,
} from "./admin-queries";
import { AdminField, AdminMessage, AdminSelect } from "./admin-ui";

const createSchema = z.object({
  model_key: z
    .string()
    .regex(/^[a-z][a-z0-9_.-]{0,127}$/, "使用小写字母开头的模型标识"),
  display_name: z.string().trim().min(1, "请输入名称").max(100),
  provider_id: z.string().uuid("请选择渠道"),
  capability: z.string().min(1, "请选择能力"),
});
export function ModelCreateForm({
  providers,
  capabilities,
  done,
}: {
  providers: Provider[];
  capabilities: Capability[];
  done: (model: AdminModel) => void;
}) {
  const form = useForm<z.infer<typeof createSchema>>({
    resolver: zodResolver(createSchema),
    defaultValues: {
      model_key: "",
      display_name: "",
      provider_id: "",
      capability: "",
    },
  });
  const mutation = useMutation({
    mutationFn: async (values: z.infer<typeof createSchema>) =>
      parseAdmin(
        adminModelSchema,
        await createAdminModel(values, adminWriteOptions()),
      ),
    onSuccess: done,
    retry: false,
  });
  return (
    <form
      onSubmit={form.handleSubmit((values) => mutation.mutate(values))}
      className="space-y-5"
    >
      <FieldGroup>
        <AdminField
          id="model-key"
          label="模型标识"
          error={form.formState.errors.model_key?.message}
        >
          <Input id="model-key" {...form.register("model_key")} />
        </AdminField>
        <AdminField
          id="model-name"
          label="显示名称"
          error={form.formState.errors.display_name?.message}
        >
          <Input id="model-name" {...form.register("display_name")} />
        </AdminField>
        <AdminField
          id="model-provider"
          label="渠道"
          error={form.formState.errors.provider_id?.message}
        >
          <Controller
            name="provider_id"
            control={form.control}
            render={({ field }) => (
              <AdminSelect
                id="model-provider"
                value={field.value}
                onChange={field.onChange}
                options={providers.map((p) => ({
                  value: p.id,
                  label: `${p.name}${p.status === "disabled" ? "（已禁用）" : ""}`,
                }))}
              />
            )}
          />
        </AdminField>
        <AdminField
          id="model-capability"
          label="能力"
          error={form.formState.errors.capability?.message}
        >
          <Controller
            name="capability"
            control={form.control}
            render={({ field }) => (
              <AdminSelect
                id="model-capability"
                value={field.value}
                onChange={field.onChange}
                options={capabilities.map((c) => ({
                  value: c.key,
                  label: `${c.key} · ${c.output_type}`,
                }))}
              />
            )}
          />
        </AdminField>
      </FieldGroup>
      <AdminMessage
        message={mutation.isError ? adminError(mutation.error) : ""}
        error
      />
      <Button
        type="submit"
        disabled={
          mutation.isPending || !providers.length || !capabilities.length
        }
      >
        创建模型
      </Button>
    </form>
  );
}
const object = z.record(z.string(), z.json());
function jsonText<T>(shape: z.ZodType<T>) {
  return z
    .string()
    .max(65536, "最多输入 64 KiB")
    .transform((value, ctx) => {
      let parsed: unknown;
      try {
        parsed = JSON.parse(value);
      } catch {
        ctx.addIssue({ code: "custom", message: "请输入合法 JSON" });
        return z.NEVER;
      }
      const result = shape.safeParse(parsed);
      if (!result.success) {
        ctx.addIssue({ code: "custom", message: "JSON 结构不符合要求" });
        return z.NEVER;
      }
      return result.data;
    });
}
const versionSchema = z.object({
  provider_model_id: z.string().trim().min(1, "请输入供应商模型标识").max(256),
  queue: z.string().trim().min(1, "请输入任务队列"),
  modes: z.array(z.string()).min(1, "至少选择一个模式"),
  expected_max_ms: z.number().int().min(1).max(2147483647),
  supports_query: z.boolean(),
  supports_cancel: z.boolean(),
  supports_callback: z.boolean(),
  moderation: z.enum(["provider", "platform", "both"]),
  limits: jsonText(object),
  param_schema: jsonText(z.array(object)),
});
function publicationSchema(
  modelId: string,
  revision: number,
  versionNo: number,
) {
  return z.object({
    id: z.string().uuid(),
    model_id: z.literal(modelId),
    revision: z.literal(revision + 1),
    version_no: z.literal(versionNo),
  });
}
export function ModelVersionForm({
  detail: initialDetail,
  capability,
  done,
}: {
  detail: ModelDetail;
  capability?: Capability;
  done: () => void;
}) {
  const [detail] = useState(() => initialDetail);
  const current =
    detail.versions.find((v) => v.id === detail.model.current_version_id) ??
    detail.versions[0];
  const next = Math.max(0, ...detail.versions.map((v) => v.version_no)) + 1;
  const form = useForm<
    z.input<typeof versionSchema>,
    unknown,
    z.output<typeof versionSchema>
  >({
    resolver: zodResolver(versionSchema),
    defaultValues: {
      provider_model_id: current?.provider_model_id ?? "",
      queue: current?.queue ?? "",
      modes: current?.modes ?? [],
      expected_max_ms: current?.expected_max_ms ?? 120000,
      supports_query: current?.supports_query ?? false,
      supports_cancel: current?.supports_cancel ?? false,
      supports_callback: current?.supports_callback ?? false,
      moderation: current?.moderation ?? "platform",
      limits: JSON.stringify(current?.limits ?? { max_outputs: 1 }, null, 2),
      param_schema: JSON.stringify(current?.param_schema ?? [], null, 2),
    },
  });
  const mutation = useMutation({
    mutationFn: async (values: z.output<typeof versionSchema>) =>
      parseAdmin(
        publicationSchema(detail.model.id, detail.model.revision, next),
        await publishAdminModelVersion(
          { id: detail.model.id },
          {
            ...values,
            expected_revision: detail.model.revision,
            version_no: next,
          },
          adminWriteOptions(),
        ),
      ),
    onSuccess: done,
    retry: false,
  });
  const modes = capability?.modes ?? current?.modes ?? [];
  return (
    <form
      onSubmit={form.handleSubmit((values) => mutation.mutate(values))}
      className="space-y-5"
    >
      <p className="text-sm text-muted-foreground">
        发布配置 v{next}，基于模型修订 {detail.model.revision}。历史配置保留。
      </p>
      <FieldGroup>
        <AdminField
          id="version-provider-model"
          label="供应商模型标识"
          error={form.formState.errors.provider_model_id?.message}
        >
          <Input
            id="version-provider-model"
            {...form.register("provider_model_id")}
          />
        </AdminField>
        <AdminField
          id="version-queue"
          label="任务队列"
          error={form.formState.errors.queue?.message}
        >
          <Input id="version-queue" {...form.register("queue")} />
        </AdminField>
        <AdminField
          id="version-modes"
          label="支持的生成模式"
          error={form.formState.errors.modes?.message}
        >
          <Controller
            name="modes"
            control={form.control}
            render={({ field }) => (
              <div id="version-modes" className="flex flex-wrap gap-4">
                {modes.map((mode) => (
                  <Label key={mode} htmlFor={`version-mode-${mode}`}>
                    <Checkbox
                      id={`version-mode-${mode}`}
                      checked={field.value.includes(mode)}
                      onCheckedChange={(checked) =>
                        field.onChange(
                          checked === true
                            ? [...field.value, mode]
                            : field.value.filter((v) => v !== mode),
                        )
                      }
                    />
                    {mode}
                  </Label>
                ))}
              </div>
            )}
          />
        </AdminField>
        <AdminField
          id="version-max-time"
          label="最长预期耗时（毫秒）"
          error={form.formState.errors.expected_max_ms?.message}
        >
          <Input
            id="version-max-time"
            type="number"
            min={1}
            {...form.register("expected_max_ms", { valueAsNumber: true })}
          />
        </AdminField>
        <div className="flex flex-wrap gap-4">
          {(
            ["supports_query", "supports_cancel", "supports_callback"] as const
          ).map((name) => (
            <Controller
              key={name}
              name={name}
              control={form.control}
              render={({ field }) => (
                <Label htmlFor={name}>
                  <Checkbox
                    id={name}
                    checked={field.value}
                    onCheckedChange={(value) => field.onChange(value === true)}
                  />
                  {
                    {
                      supports_query: "支持查询",
                      supports_cancel: "支持取消",
                      supports_callback: "支持回调",
                    }[name]
                  }
                </Label>
              )}
            />
          ))}
        </div>
        <AdminField id="version-moderation" label="内容审核">
          <Controller
            name="moderation"
            control={form.control}
            render={({ field }) => (
              <AdminSelect
                id="version-moderation"
                value={field.value}
                onChange={field.onChange}
                options={[
                  { value: "provider", label: "供应商" },
                  { value: "platform", label: "平台" },
                  { value: "both", label: "供应商与平台" },
                ]}
              />
            )}
          />
        </AdminField>
        <AdminField
          id="version-limits"
          label="输入与输出限制（JSON 对象）"
          error={form.formState.errors.limits?.message}
        >
          <Textarea
            id="version-limits"
            rows={6}
            className="font-mono"
            {...form.register("limits")}
          />
        </AdminField>
        <AdminField
          id="version-parameters"
          label="生成参数（JSON 数组）"
          error={form.formState.errors.param_schema?.message}
        >
          <Textarea
            id="version-parameters"
            rows={8}
            className="font-mono"
            {...form.register("param_schema")}
          />
        </AdminField>
      </FieldGroup>
      <AdminMessage
        message={mutation.isError ? adminError(mutation.error) : ""}
        error
      />
      <Button type="submit" disabled={mutation.isPending || !modes.length}>
        发布配置 v{next}
      </Button>
    </form>
  );
}
const priceSchema = z
  .object({
    unit: z.enum([
      "per_image",
      "per_second",
      "per_request",
      "per_1k_tokens",
      "per_1k_chars",
    ]),
    currency: z.string().regex(/^[A-Z]{3}$/, "使用三个大写字母的币种代码"),
    fx_rate_to_cny: z.string().trim().optional(),
    effective_from: z
      .string()
      .refine(
        (value) => Number.isFinite(new Date(value).getTime()),
        "请选择生效时间",
      ),
    rule: jsonText(object),
  })
  .superRefine((values, ctx) => {
    if (
      values.currency !== "CNY" &&
      !/^(?:0|[1-9]\d*)(?:\.\d+)?$/.test(values.fx_rate_to_cny ?? "")
    )
      ctx.addIssue({
        code: "custom",
        path: ["fx_rate_to_cny"],
        message: "外币价格需要填写人民币汇率",
      });
  });
export function ModelPriceForm({
  detail: initialDetail,
  done,
}: {
  detail: ModelDetail;
  done: () => void;
}) {
  const [detail] = useState(() => initialDetail);
  const latest = detail.prices[0];
  const next = Math.max(0, ...detail.prices.map((p) => p.version_no)) + 1;
  const form = useForm<
    z.input<typeof priceSchema>,
    unknown,
    z.output<typeof priceSchema>
  >({
    resolver: zodResolver(priceSchema),
    defaultValues: {
      unit: latest?.unit ?? "per_image",
      currency: latest?.currency ?? "CNY",
      fx_rate_to_cny: latest?.fx_rate_to_cny ?? "",
      effective_from: "",
      rule: JSON.stringify(latest?.rule ?? { base_micros: 1000000 }, null, 2),
    },
  });
  const mutation = useMutation({
    mutationFn: async (values: z.output<typeof priceSchema>) =>
      parseAdmin(
        publicationSchema(detail.model.id, detail.model.revision, next),
        await publishAdminModelPrice(
          { id: detail.model.id },
          {
            ...values,
            fx_rate_to_cny: values.fx_rate_to_cny || undefined,
            effective_from: new Date(values.effective_from).toISOString(),
            expected_revision: detail.model.revision,
            version_no: next,
          },
          adminWriteOptions(),
        ),
      ),
    onSuccess: done,
    retry: false,
  });
  return (
    <form
      onSubmit={form.handleSubmit((values) => mutation.mutate(values))}
      className="space-y-5"
    >
      <p className="text-sm text-muted-foreground">
        发布价格 v{next}，基于模型修订 {detail.model.revision}。
      </p>
      <FieldGroup>
        <AdminField id="price-unit" label="计价单位">
          <Controller
            name="unit"
            control={form.control}
            render={({ field }) => (
              <AdminSelect
                id="price-unit"
                value={field.value}
                onChange={field.onChange}
                options={[
                  { value: "per_image", label: "每张图" },
                  { value: "per_second", label: "每秒" },
                  { value: "per_request", label: "每次请求" },
                  { value: "per_1k_tokens", label: "每千 Token" },
                  { value: "per_1k_chars", label: "每千字符" },
                ]}
              />
            )}
          />
        </AdminField>
        <div className="grid gap-4 sm:grid-cols-2">
          <AdminField
            id="price-currency"
            label="币种"
            error={form.formState.errors.currency?.message}
          >
            <Input id="price-currency" {...form.register("currency")} />
          </AdminField>
          <AdminField
            id="price-fx"
            label="人民币汇率（外币必填）"
            error={form.formState.errors.fx_rate_to_cny?.message}
          >
            <Input
              id="price-fx"
              inputMode="decimal"
              {...form.register("fx_rate_to_cny")}
            />
          </AdminField>
        </div>
        <AdminField
          id="price-effective"
          label="生效时间（本地时间）"
          error={form.formState.errors.effective_from?.message}
        >
          <Input
            id="price-effective"
            type="datetime-local"
            {...form.register("effective_from")}
          />
        </AdminField>
        <AdminField
          id="price-rule"
          label="价格规则（JSON 对象）"
          error={form.formState.errors.rule?.message}
        >
          <Textarea
            id="price-rule"
            rows={8}
            className="font-mono"
            {...form.register("rule")}
          />
        </AdminField>
        <p className="text-sm text-muted-foreground">
          金额使用微单位，1,000,000 代表一个货币单位。图像、时长和请求计价使用
          base_micros，可追加 by_mode、by_resolution 系数；Token 计价使用
          input_micros_per_1k 和 output_micros_per_1k。
        </p>
      </FieldGroup>
      <AdminMessage
        message={mutation.isError ? adminError(mutation.error) : ""}
        error
      />
      <Button type="submit" disabled={mutation.isPending}>
        发布价格 v{next}
      </Button>
    </form>
  );
}
