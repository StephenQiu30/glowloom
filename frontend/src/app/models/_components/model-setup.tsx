"use client";
import { FieldGroup, Field, FieldLabel } from "@/components/ui/field";
import { Textarea } from "@/components/ui/textarea";
import { Checkbox } from "@/components/ui/checkbox";

import { useState, useId } from "react";
import { Input } from "@/components/ui/input";

function values(form: FormData, name: string) {
  return [
    ...new Set(
      String(form.get(name) ?? "")
        .split(/[,，\s]+/)
        .filter(Boolean),
    ),
  ];
}
function integer(form: FormData, name: string, min: number, max: number) {
  const n = Number(form.get(name));
  if (!Number.isSafeInteger(n) || n < min || n > max)
    throw new Error("请检查数量、时长或 Token 上限。");
  return n;
}
export function modelConfiguration(form: FormData, capability: string) {
  if (form.get("setup_mode") === "advanced")
    return {
      limits: JSON.parse(String(form.get("limits"))),
      param_schema: JSON.parse(String(form.get("parameters"))),
    };
  const limits: Record<string, unknown> = {
    max_outputs: integer(
      form,
      "max_outputs",
      1,
      capability === "image.generate" ? 8 : 1,
    ),
  };
  const endpoint = String(form.get("provider_endpoint") ?? "").trim();
  if (endpoint && capability !== "video.generate")
    limits.provider_endpoint = endpoint;
  const param_schema: Record<string, unknown>[] = [];
  for (const [name, label, source] of [
    ["resolution", "分辨率", "resolutions"],
    ["aspect_ratio", "画面比例", "aspect_ratios"],
  ]) {
    const options = values(form, source);
    if (options.length) {
      limits[source] = options;
      param_schema.push({
        field: name,
        label,
        type: "string",
        component: "select",
        enum: options,
        default: options[0],
        required: true,
      });
    }
  }
  if (capability === "video.generate") {
    const durations = values(form, "durations").map(Number);
    if (
      !durations.length ||
      durations.some((n) => !Number.isInteger(n) || n < 1 || n > 3600)
    )
      throw new Error("请填写模型已开通的整数秒时长。");
    limits.durations_ms = durations.map((n) => n * 1000);
    param_schema.push({
      field: "duration_ms",
      label: "时长（毫秒）",
      type: "integer",
      component: "select",
      enum: limits.durations_ms,
      default: durations[0] * 1000,
      required: true,
    });
  }
  if (capability === "text.generate")
    param_schema.push({
      field: "max_tokens",
      label: "最大输出 Token",
      type: "integer",
      component: "input",
      required: true,
      min: 1,
      max: integer(form, "max_tokens", 1, 1000000),
      default: integer(form, "max_tokens", 1, 1000000),
    });
  const modes = form.getAll("modes").map(String);
  const roles: Record<string, unknown> = {};
  if (modes.includes("image2image") || modes.includes("omni_reference")) {
    roles.reference_image = {
      types: ["image"],
      min_count: 1,
      max_count: integer(form, "max_references", 1, 100),
      for_modes: modes.filter(
        (m) => m === "image2image" || m === "omni_reference",
      ),
    };
  }
  if (modes.includes("image2video"))
    roles.first_frame = {
      types: ["image"],
      min_count: 1,
      max_count: 1,
      for_modes: ["image2video"],
    };
  if (Object.keys(roles).length) limits.roles = roles;
  return { limits, param_schema };
}

export function ModelSetup({
  capability,
  limits,
  parameters,
}: {
  capability: string;
  limits?: Record<string, unknown>;
  parameters?: unknown;
}) {
  const formFieldId = useId();

  const [advanced, setAdvanced] = useState(!!limits);
  return (
    <FieldGroup className="gap-3">
      <Field orientation="horizontal">
        <Checkbox
          id={`${formFieldId}-field-1`}

          checked={advanced}
          onCheckedChange={(e) => setAdvanced(e === true)}
        />
        <FieldLabel htmlFor={`${formFieldId}-field-1`}>
          <span>编辑完整参数与素材规则</span>
        </FieldLabel>
      </Field>
      <Input
        type="hidden"
        name="setup_mode"
        value={advanced ? "advanced" : "basic"}
      />
      {advanced ? (
        <>
          <Field>
            <FieldLabel htmlFor={`${formFieldId}-field-2`}>
              <span>输入与输出限制</span>
            </FieldLabel>
            <Textarea
              id={`${formFieldId}-field-2`}
              name="limits"
              required
              className="mt-1 min-h-28 w-full"
              defaultValue={JSON.stringify(
                limits ?? { max_outputs: 1 },
                null,
                2,
              )}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor={`${formFieldId}-field-3`}>
              <span>参数表单定义</span>
            </FieldLabel>
            <Textarea
              id={`${formFieldId}-field-3`}
              name="parameters"
              required
              className="mt-1 min-h-28 w-full"
              defaultValue={JSON.stringify(parameters ?? [], null, 2)}
            />
          </Field>
        </>
      ) : (
        <>
          {limits && (
            <p role="status" className="text-sm">
              基础配置会替换原有高级规则；保存后仍需创建发布并验证。
            </p>
          )}
          <Field>
            <FieldLabel htmlFor={`${formFieldId}-field-4`}>
              <span>每次最多生成数量</span>
            </FieldLabel>
            <Input
              id={`${formFieldId}-field-4`}
              name="max_outputs"
              type="number"
              min={1}
              max={capability === "image.generate" ? 8 : 1}
              required
              defaultValue={1}
            />
          </Field>
          {capability !== "video.generate" && (
            <Field>
              <FieldLabel htmlFor={`${formFieldId}-field-5`}>
                <span>OpenRouter 固定服务端点（可选）</span>
              </FieldLabel>
              <Input
                id={`${formFieldId}-field-5`}
                name="provider_endpoint"
                maxLength={128}
                placeholder="填写官方端点记录中的 provider_tag"
              />
              <span className="text-xs text-muted-foreground">
                留空由 OpenRouter
                首次路由；请求不启用失败回退，且必须支持所传参数。
              </span>
            </Field>
          )}
          {capability === "text.generate" ? (
            <Field>
              <FieldLabel htmlFor={`${formFieldId}-field-6`}>
                <span>最大输出 Token</span>
              </FieldLabel>
              <Input
                id={`${formFieldId}-field-6`}
                name="max_tokens"
                type="number"
                required
                min={1}
                max={1000000}
                placeholder="填写该模型允许的上限"
              />
            </Field>
          ) : (
            <>
              <Field>
                <FieldLabel htmlFor={`${formFieldId}-field-7`}>
                  <span>支持的分辨率</span>
                </FieldLabel>
                <Input
                  id={`${formFieldId}-field-7`}
                  name="resolutions"
                  placeholder="按模型文档填写，用逗号分隔"
                />
              </Field>
              <Field>
                <FieldLabel htmlFor={`${formFieldId}-field-8`}>
                  <span>支持的画面比例</span>
                </FieldLabel>
                <Input
                  id={`${formFieldId}-field-8`}
                  name="aspect_ratios"
                  placeholder="按模型文档填写，用逗号分隔"
                />
              </Field>
              <Field>
                <FieldLabel htmlFor={`${formFieldId}-field-9`}>
                  <span>参考图数量上限</span>
                </FieldLabel>
                <Input
                  id={`${formFieldId}-field-9`}
                  name="max_references"
                  type="number"
                  min={1}
                  max={100}
                  placeholder="参考图模式必填"
                />
              </Field>
              {capability === "video.generate" && (
                <Field>
                  <FieldLabel htmlFor={`${formFieldId}-field-10`}>
                    <span>支持的视频时长（秒）</span>
                  </FieldLabel>
                  <Input
                    id={`${formFieldId}-field-10`}
                    name="durations"
                    required
                    placeholder="按模型文档填写，用逗号分隔"
                  />
                </Field>
              )}
            </>
          )}
        </>
      )}
    </FieldGroup>
  );
}

export function priceRule(form: FormData) {
  if (form.get("price_mode") === "advanced")
    return JSON.parse(String(form.get("rule")));
  const micros = (name: string) => {
    const raw = String(form.get(name) ?? "");
    if (!/^\d+(\.\d{1,6})?$/.test(raw))
      throw new Error("价格需为非负金额，最多保留六位小数。");
    const n = Math.round(Number(raw) * 1_000_000);
    if (!Number.isSafeInteger(n)) throw new Error("价格超出范围。");
    return n;
  };
  return form.get("unit") === "per_1k_tokens"
    ? {
        input_micros_per_1k: micros("input_rate"),
        output_micros_per_1k: micros("output_rate"),
      }
    : { base_micros: micros("base_rate") };
}
export function PriceSetup({ rule }: { rule?: Record<string, unknown> }) {
  const formFieldId = useId();

  const [advanced, setAdvanced] = useState(!!rule);
  return (
    <FieldGroup className="gap-3">
      <Field orientation="horizontal">
        <Checkbox
          id={`${formFieldId}-field-11`}

          checked={advanced}
          onCheckedChange={(e) => setAdvanced(e === true)}
        />
        <FieldLabel htmlFor={`${formFieldId}-field-11`}>
          <span>编辑完整计费规则</span>
        </FieldLabel>
      </Field>
      <Input
        type="hidden"
        name="price_mode"
        value={advanced ? "advanced" : "basic"}
      />
      {advanced ? (
        <Field>
          <FieldLabel htmlFor={`${formFieldId}-field-12`}>
            <span>人民币微元计费规则</span>
          </FieldLabel>
          <Textarea
            id={`${formFieldId}-field-12`}
            name="rule"
            required
            className="mt-1 w-full"
            defaultValue={JSON.stringify(rule ?? { base_micros: 0 }, null, 2)}
          />
        </Field>
      ) : (
        <>
          <Field>
            <FieldLabel htmlFor={`${formFieldId}-field-13`}>
              <span>每单位价格（元；按千 Token 计费时忽略）</span>
            </FieldLabel>
            <Input
              id={`${formFieldId}-field-13`}
              name="base_rate"
              type="number"
              min={0}
              step="0.000001"
              defaultValue={0}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor={`${formFieldId}-field-14`}>
              <span>每千输入 Token（元）</span>
            </FieldLabel>
            <Input
              id={`${formFieldId}-field-14`}
              name="input_rate"
              type="number"
              min={0}
              step="0.000001"
            />
          </Field>
          <Field>
            <FieldLabel htmlFor={`${formFieldId}-field-15`}>
              <span>每千输出 Token（元）</span>
            </FieldLabel>
            <Input
              id={`${formFieldId}-field-15`}
              name="output_rate"
              type="number"
              min={0}
              step="0.000001"
            />
          </Field>
        </>
      )}
    </FieldGroup>
  );
}
