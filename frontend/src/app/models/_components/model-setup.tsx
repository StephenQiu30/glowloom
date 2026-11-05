"use client";
import { FieldGroup, Field, FieldLabel } from "@/components/ui/field";
import { Textarea } from "@/components/ui/textarea";
import { Checkbox } from "@/components/ui/checkbox";
import { Button } from "@/components/ui/button";
import { ChevronDown } from "lucide-react";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";

import { useState, useId } from "react";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

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
export function modelConfiguration(
  form: FormData,
  capability: string,
  adapter = "openrouter",
) {
  if (form.get("setup_mode") === "advanced")
    return {
      limits: JSON.parse(String(form.get("limits"))),
      param_schema: JSON.parse(String(form.get("parameters"))),
    };
  const arkImage =
    capability === "image.generate" &&
    ["volcengine_ark", "byteplus"].includes(adapter);
  const limits: Record<string, unknown> = {
    max_outputs: integer(
      form,
      "max_outputs",
      1,
      capability === "image.generate" && !arkImage ? 8 : 1,
    ),
  };
  const endpoint = String(form.get("provider_endpoint") ?? "").trim();
  if (endpoint && adapter === "openrouter" && capability !== "video.generate")
    limits.provider_endpoint = endpoint;
  const param_schema: Record<string, unknown>[] = [];
  if (arkImage) {
    const sizes = values(form, "sizes");
    if (!sizes.length) throw new Error("请填写账号模型支持的图片尺寸。");
    param_schema.push({
      field: "size",
      label: "图片尺寸",
      type: "string",
      component: "select",
      enum: sizes,
      default: sizes[0],
      required: true,
    });
  }
  for (const [name, label, source] of [
    ["resolution", "分辨率", "resolutions"],
    ["aspect_ratio", "画面比例", "aspect_ratios"],
  ]) {
    const options = values(form, source);
    if (options.length && !arkImage) {
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
  adapter = "openrouter",
}: {
  capability: string;
  adapter?: string;
  limits?: Record<string, unknown>;
  parameters?: unknown;
}) {
  const formFieldId = useId();
  const arkImage =
    capability === "image.generate" &&
    ["volcengine_ark", "byteplus"].includes(adapter);

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
              max={capability === "image.generate" && !arkImage ? 8 : 1}
              required
              defaultValue={1}
            />
          </Field>
          {adapter === "openrouter" && capability !== "video.generate" && (
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
              {arkImage ? (
                <Field>
                  <FieldLabel htmlFor={`${formFieldId}-sizes`}>
                    图片尺寸
                  </FieldLabel>
                  <Input
                    id={`${formFieldId}-sizes`}
                    name="sizes"
                    required
                    placeholder="按账号模型文档填写，如 2K、4K 或宽x高"
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
                </>
              )}
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
  const withProcurement = (rule: Record<string, unknown>) => {
    const selected = form.get("procurement_price");
    if (selected === null) return rule;
    const result = { ...rule };
    delete result.procurement_price_version_id;
    if (selected !== "none" && selected !== "") {
      result.procurement_price_version_id = selected;
    }
    return result;
  };
  if (form.get("price_mode") === "advanced") {
    const rule: unknown = JSON.parse(String(form.get("rule")));
    if (typeof rule !== "object" || rule === null || Array.isArray(rule))
      throw new Error("请填写有效的计费规则。");
    return withProcurement(rule as Record<string, unknown>);
  }
  const micros = (name: string) => {
    const raw = String(form.get(name) ?? "");
    if (!/^\d+(\.\d{1,6})?$/.test(raw))
      throw new Error("价格需为非负金额，最多保留六位小数。");
    const [whole, fraction = ""] = raw.split(".");
    const amount =
      BigInt(whole) * BigInt(1_000_000) + BigInt(fraction.padEnd(6, "0"));
    if (amount > BigInt(Number.MAX_SAFE_INTEGER))
      throw new Error("价格超出范围。");
    return Number(amount);
  };
  const policyRaw = form.get("price_policy");
  const policy: Record<string, unknown> = policyRaw
    ? JSON.parse(String(policyRaw))
    : {};
  if (
    !policy ||
    typeof policy !== "object" ||
    Array.isArray(policy) ||
    Object.keys(policy).some(
      (key) => !["credit_ratio", "markup_multiplier"].includes(key),
    )
  ) {
    throw new Error("价格规则需要重新读取。");
  }
  return withProcurement(
    form.get("unit") === "per_1k_tokens"
      ? {
          ...policy,
          input_micros_per_1k: micros("input_rate"),
          output_micros_per_1k: micros("output_rate"),
        }
      : { ...policy, base_micros: micros("base_rate") },
  );
}
export function PriceSetup({
  rule,
  unit = "per_request",
  prices = [],
}: {
  rule?: Record<string, unknown>;
  unit?: string;
  prices?: API.AdminPrice[];
}) {
  const formFieldId = useId();
  const simple =
    !rule ||
    Object.entries(rule).every(
      ([key, value]) =>
        key === "procurement_price_version_id" ||
        ([
          "base_micros",
          "input_micros_per_1k",
          "output_micros_per_1k",
        ].includes(key) &&
          typeof value === "number" &&
          Number.isSafeInteger(value) &&
          value >= 0) ||
        (["credit_ratio", "markup_multiplier"].includes(key) &&
          (typeof value === "number" || typeof value === "string") &&
          Number.isFinite(Number(value)) &&
          Number(value) > 0),
    );
  const [advanced, setAdvanced] = useState(!simple);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const valueInYuan = (key: string) =>
    typeof rule?.[key] === "number" ? String(rule[key] / 1_000_000) : "";
  const [baseRate, setBaseRate] = useState(() => valueInYuan("base_micros"));
  const [inputRate, setInputRate] = useState(() =>
    valueInYuan("input_micros_per_1k"),
  );
  const [outputRate, setOutputRate] = useState(() =>
    valueInYuan("output_micros_per_1k"),
  );
  const [ruleText, setRuleText] = useState(() =>
    JSON.stringify(rule ?? { base_micros: 0 }, null, 2),
  );
  const [procurement, setProcurement] = useState(
    typeof rule?.procurement_price_version_id === "string"
      ? rule.procurement_price_version_id
      : "none",
  );
  const supplierRates = prices.filter(
    (price) =>
      price.id &&
      price.unit === unit &&
      price.source_url &&
      price.reviewed_at &&
      !price.rule?.procurement_price_version_id &&
      price.rule?.markup_multiplier === undefined,
  );
  return (
    <FieldGroup className="gap-3">
      <Input
        type="hidden"
        name="price_mode"
        value={advanced ? "advanced" : "basic"}
      />
      {!advanced ? (
        <Input
          type="hidden"
          name="price_policy"
          value={JSON.stringify(
            Object.fromEntries(
              Object.entries(rule ?? {}).filter(([key]) =>
                ["credit_ratio", "markup_multiplier"].includes(key),
              ),
            ),
          )}
        />
      ) : null}
      {!advanced ? (
        unit === "per_1k_tokens" ? (
          <>
            <Field>
              <FieldLabel htmlFor={formFieldId + "-input"}>
                每千输入 Token（元）
              </FieldLabel>
              <Input
                id={formFieldId + "-input"}
                name="input_rate"
                type="number"
                min={0}
                step="0.000001"
                required
                value={inputRate}
                onChange={(event) => setInputRate(event.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor={formFieldId + "-output"}>
                每千输出 Token（元）
              </FieldLabel>
              <Input
                id={formFieldId + "-output"}
                name="output_rate"
                type="number"
                min={0}
                step="0.000001"
                required
                value={outputRate}
                onChange={(event) => setOutputRate(event.target.value)}
              />
            </Field>
          </>
        ) : (
          <Field>
            <FieldLabel htmlFor={formFieldId + "-base"}>价格（元）</FieldLabel>
            <Input
              id={formFieldId + "-base"}
              name="base_rate"
              type="number"
              min={0}
              step="0.000001"
              required
              value={baseRate}
              onChange={(event) => setBaseRate(event.target.value)}
              placeholder="0.00"
            />
          </Field>
        )
      ) : (
        <p className="text-xs text-muted-foreground">已保留当前高级规则。</p>
      )}
      <Field>
        <FieldLabel htmlFor={formFieldId + "-procurement"}>
          采购费率版本
        </FieldLabel>
        <Select
          name="procurement_price"
          value={
            supplierRates.some((price) => price.id === procurement)
              ? procurement
              : "none"
          }
          onValueChange={setProcurement}
        >
          <SelectTrigger id={formFieldId + "-procurement"} className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="none">暂不绑定</SelectItem>
            {supplierRates.map((price) => (
              <SelectItem key={price.id} value={price.id!}>
                版本 {price.version_no} · {price.currency}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground">
          先保存有来源和审核时间的采购费率，再发布绑定该版本的用户售价。
        </p>
      </Field>
      <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen}>
        <CollapsibleTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            className="group/collapsible justify-between"
          >
            高级计费规则
            <ChevronDown
              data-icon="inline-end"
              className="transition-transform group-data-[state=open]:rotate-180"
            />
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent forceMount hidden={!advancedOpen} className="pt-3">
          <FieldGroup className="gap-3">
            <Field orientation="horizontal">
              <Checkbox
                id={formFieldId + "-advanced"}
                checked={advanced}
                onCheckedChange={(checked) => setAdvanced(checked === true)}
              />
              <FieldLabel htmlFor={formFieldId + "-advanced"}>
                编辑完整计费规则
              </FieldLabel>
            </Field>
            {advanced ? (
              <Field>
                <FieldLabel htmlFor={formFieldId + "-rule"}>
                  人民币微元计费规则
                </FieldLabel>
                <Textarea
                  id={formFieldId + "-rule"}
                  name="rule"
                  required
                  value={ruleText}
                  onChange={(event) => setRuleText(event.target.value)}
                />
              </Field>
            ) : null}
          </FieldGroup>
        </CollapsibleContent>
      </Collapsible>
    </FieldGroup>
  );
}
