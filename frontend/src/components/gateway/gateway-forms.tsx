"use client";

import { useId, useRef, useState, type ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import {
  prepareModel,
  prepareRoute,
  type OnboardingReview,
  fundCredits,
  reconcileGateway,
  type GatewayConfiguration,
  type GatewayPrice,
} from "@/lib/ai-gateway-admin";

export type RunMutation = (
  action: () => Promise<unknown>,
  success: string,
) => Promise<boolean>;
export function InputField({
  label,
  name,
  type = "text",
  value,
  min,
  max,
  placeholder,
  required = true,
  description,
}: {
  label: string;
  name: string;
  type?: string;
  value?: string | number;
  min?: number;
  max?: number;
  placeholder?: string;
  required?: boolean;
  description?: string;
}) {
  const id = useId();
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      {name === "supplier" ? (
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={(event) => {
            const input = event.currentTarget
              .closest("form")
              ?.elements.namedItem(name);
            if (input instanceof HTMLInputElement)
              input.value = crypto.randomUUID();
          }}
        >
          生成{label}
        </Button>
      ) : null}
      <Input
        id={id}
        name={name}
        type={type}
        defaultValue={value}
        min={min}
        max={max}
        step={type === "number" ? 1 : undefined}
        required={required}
        placeholder={placeholder}
        autoComplete={type === "password" ? "new-password" : "off"}
      />
      {description ? <FieldDescription>{description}</FieldDescription> : null}
    </Field>
  );
}
function SelectField({
  label,
  name,
  value,
  children,
  required = true,
}: {
  label: string;
  name: string;
  value?: string;
  children: ReactNode;
  required?: boolean;
}) {
  const id = useId();
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <NativeSelect
        id={id}
        name={name}
        defaultValue={value}
        required={required}
        className="w-full"
      >
        {children}
      </NativeSelect>
    </Field>
  );
}
export function JSONField({
  label,
  name,
  value,
  description,
}: {
  label: string;
  name: string;
  value: string;
  description?: string;
}) {
  const id = useId();
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Textarea
        id={id}
        name={name}
        defaultValue={value}
        rows={7}
        spellCheck={false}
      />
      {description ? <FieldDescription>{description}</FieldDescription> : null}
    </Field>
  );
}
const priceFields = [
  ["input", "输入价"],
  ["cache_read", "缓存读取价"],
  ["cache_write", "缓存写入价"],
  ["output", "输出价"],
  ["request", "按次价"],
] as const;
function PriceFields({ kind }: { kind: "销售" | "采购" }) {
  return (
    <FieldGroup className="grid sm:grid-cols-2 lg:grid-cols-3">
      {priceFields.map(([name, label]) => (
        <InputField
          key={name}
          name={name}
          label={`${kind}${label}`}
          type="number"
          min={0}
          max={Number.MAX_SAFE_INTEGER}
          value={0}
        />
      ))}
      <SelectField label={`${kind}币种`} name="currency" value="CNY">
        <NativeSelectOption value="CNY">CNY</NativeSelectOption>
        <NativeSelectOption value="USD">USD</NativeSelectOption>
      </SelectField>
      <InputField
        name="fx"
        label={`${kind}汇率`}
        value="1"
        description="兑换人民币的正十进制数"
      />
      <InputField name="multiplier" label={`${kind}倍率`} value="1" />
      <InputField
        name="discount"
        label={`${kind}折扣`}
        value="1"
        description="大于 0，且不超过 1"
      />
    </FieldGroup>
  );
}
function text(form: FormData, key: string) {
  return String(form.get(key) ?? "").trim();
}
function num(form: FormData, key: string) {
  return Number(text(form, key));
}
function price(form: FormData): GatewayPrice {
  return {
    input: num(form, "input"),
    cache_read: num(form, "cache_read"),
    cache_write: num(form, "cache_write"),
    output: num(form, "output"),
    request: num(form, "request"),
    currency: text(form, "currency"),
    fx: text(form, "fx"),
    multiplier: text(form, "multiplier"),
    discount: text(form, "discount"),
  };
}

export function ModelForm({ run, busy }: { run: RunMutation; busy: boolean }) {
  const [protocol, setProtocol] = useState("chat");
  const protocolId = useId();
  const attempt = useRef<{ payload: string; id: string } | null>(null);
  return (
    <form
      aria-label="准备模型与销售价"
      onSubmit={(event) => {
        event.preventDefault();
        const form = new FormData(event.currentTarget);
        void run(async () => {
          const media =
            protocol === "media" ? JSON.parse(text(form, "media")) : {};
          if (media.output_policy && form.get("reviewed") !== "on")
            throw new Error("请确认已审核供应商成功输出策略");
          const model = {
            ...media,
            name: text(form, "name"),
            protocol,
            price: price(form),
            max_input: num(form, "max_input"),
            max_output: num(form, "max_output"),
            purchase_cap: num(form, "purchase_cap"),
            tools: form.get("tools") === "on",
            stream: form.get("stream") === "on",
          };
          const payload = JSON.stringify(model);
          if (attempt.current?.payload !== payload)
            attempt.current = { payload, id: crypto.randomUUID() };
          await prepareModel(model, attempt.current.id);
          attempt.current = null;
        }, "模型预设与销售价已保存为草稿。");
      }}
    >
      <FieldGroup>
        <InputField
          label="销售模型名称"
          name="name"
          placeholder="text/default"
          description="同名同协议的新版本通过验证后才切换。"
        />
        <Field>
          <FieldLabel htmlFor={protocolId}>模型协议</FieldLabel>
          <NativeSelect
            id={protocolId}
            value={protocol}
            onChange={(event) => setProtocol(event.target.value)}
            className="w-full"
          >
            {["chat", "responses", "messages", "media"].map((p) => (
              <NativeSelectOption key={p} value={p}>
                {p}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </Field>
        <PriceFields kind="销售" />
        <FieldDescription>
          价格单位：每百万 token 的微货币；按次价为每次请求的微货币。1 元 =
          1000000 微人民币。外部模型不能免费发布。
        </FieldDescription>
        <FieldGroup className="grid sm:grid-cols-3">
          <InputField
            label="最大输入量"
            name="max_input"
            type="number"
            value={32768}
            min={1}
            max={1048576}
          />
          <InputField
            label="最大输出量"
            name="max_output"
            type="number"
            value={4096}
            min={1}
            max={1048576}
          />
          <InputField
            label="单次采购上限（微人民币）"
            name="purchase_cap"
            type="number"
            min={0}
            value={10000000}
          />
        </FieldGroup>
        {protocol === "media" ? (
          <>
            <JSONField
              label="媒体发布配置 JSON"
              name="media"
              value={
                '{"capability":"image.generate","modes":["text2image"],"parameters":{}}'
              }
              description="填写 capability、modes、parameters。只有已审核供应商成功策略时才填写 output_policy: provider_success_v1。媒体只支持按次价。"
            />
            <Field orientation="horizontal">
              <FieldLabel>
                <Checkbox name="reviewed" />
                已审核供应商成功输出策略
              </FieldLabel>
            </Field>
          </>
        ) : (
          <FieldGroup className="flex-row">
            <Field orientation="horizontal">
              <FieldLabel>
                <Checkbox name="tools" />
                支持工具
              </FieldLabel>
            </Field>
            <Field orientation="horizontal">
              <FieldLabel>
                <Checkbox name="stream" />
                支持流式
              </FieldLabel>
            </Field>
          </FieldGroup>
        )}
        <Button type="submit" disabled={busy}>
          保存模型预设和销售价
        </Button>
      </FieldGroup>
    </form>
  );
}

export function RouteForm({
  configuration,
  run,
  busy,
  onPrepared,
}: {
  configuration: GatewayConfiguration;
  run: RunMutation;
  busy: boolean;
  onPrepared: (review: OnboardingReview) => void;
}) {
  const models = configuration.models.filter(
    ({ model, enabled, draft }) =>
      (enabled || draft) && model.protocol !== "local",
  );
  const [selected, setSelected] = useState("");
  const modelId = useId();
  const model =
    models.find(({ model }) => model.id === selected)?.model ??
    models[0]?.model;
  const attempt = useRef<{ payload: string; id: string } | null>(null);
  const replacements = configuration.routes.filter(
    ({ route, enabled }) => enabled && route.model_id === model?.id,
  );

  return (
    <form
      aria-label="准备供应商路由与采购价"
      onSubmit={(event) => {
        event.preventDefault();
        if (!model) return;
        const data = new FormData(event.currentTarget);
        void run(async () => {
          const driver = text(data, "driver_config");
          const media = text(data, "media_tests");
          const route = {
            model_id: model.id,
            supplier: text(data, "supplier"),
            group: text(data, "group"),
            base_url: text(data, "base_url"),
            upstream_model: text(data, "upstream_model"),
            protocol: model.protocol,
            purchase: price(data),
            priority: num(data, "priority"),
            ...(driver ? { driver_config: JSON.parse(driver) } : {}),
          };
          const options = {
            scope: text(data, "scope_id"),
            project: text(data, "project_id"),
            replacement: text(data, "replacement"),
            mediaTests: media ? JSON.parse(media) : undefined,
            concurrency: num(data, "concurrency"),
            rpm: num(data, "rpm"),
          };
          const payload = JSON.stringify({ route, options });
          if (attempt.current?.payload !== payload)
            attempt.current = { payload, id: crypto.randomUUID() };
          const review = await prepareRoute(
            { ...route, id: attempt.current.id },
            options,
          );
          attempt.current = null;
          onPrepared(review);
        }, "采购路由与验证计划已保存。");
      }}
    >
      <FieldGroup>
        <Field>
          <FieldLabel htmlFor={modelId}>销售模型版本</FieldLabel>
          <NativeSelect
            id={modelId}
            value={model?.id ?? ""}
            onChange={(event) => setSelected(event.currentTarget.value)}
            className="w-full"
            required
          >
            <NativeSelectOption value="" disabled>
              请先准备模型与销售价
            </NativeSelectOption>
            {models.map(({ model, draft }) => (
              <NativeSelectOption key={model.id} value={model.id}>
                {model.name} · {model.protocol} · {draft ? "草稿" : "已启用"} ·{" "}
                {model.id}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </Field>
        <FieldGroup className="grid sm:grid-cols-2">
          <InputField label="供应商 ID" name="supplier" placeholder="UUID" />
          <InputField
            label="验证付款空间 ID"
            name="scope_id"
            placeholder="已授权并有余额的测试空间 UUID"
          />
          <InputField
            label="共享配额组"
            name="group"
            description="同一供应商账号共享并发和 RPM。"
          />
          <InputField
            label="供应商地址"
            name="base_url"
            type="url"
            placeholder="https://api.deepseek.com/v1"
          />
          <InputField
            label="供应商模型号"
            name="upstream_model"
            placeholder="供应商实际返回的精确型号"
          />
        </FieldGroup>
        {replacements.length > 0 && (
          <SelectField
            key={model?.id}
            label="替换现有路由"
            name="replacement"
            value=""
            required={false}
          >
            <NativeSelectOption value="">增加路由</NativeSelectOption>
            {replacements.map(({ route }) => (
              <NativeSelectOption key={route.id} value={route.id}>
                {route.upstream_model} · {route.group} · {route.id}
              </NativeSelectOption>
            ))}
          </SelectField>
        )}
        <PriceFields kind="采购" />
        <FieldDescription>
          采购价按实际供应商合同填写。销售价沿模型版本。
        </FieldDescription>
        <FieldGroup className="grid sm:grid-cols-3">
          <InputField
            label="优先级"
            name="priority"
            type="number"
            min={0}
            value={0}
          />
          <InputField
            label="共享并发"
            name="concurrency"
            type="number"
            min={1}
            max={1000}
            value={1}
          />
          <InputField
            label="共享 RPM"
            name="rpm"
            type="number"
            min={1}
            value={60}
          />
        </FieldGroup>
        {model?.protocol === "media" && (
          <FieldGroup key={model.id}>
            <InputField
              label="验证项目 ID"
              name="project_id"
              placeholder="测试空间内的项目 UUID"
            />
            <JSONField
              label="媒体驱动配置 JSON"
              name="driver_config"
              value=""
              description="填写已支持的 ProviderConfig，地址须与路由相同。"
            />
            <JSONField
              label="媒体验证输入 JSON"
              name="media_tests"
              value={JSON.stringify(
                model.modes?.map((mode) => ({
                  model: model.name,
                  capability: model.capability,
                  mode,
                  prompt: "Glowloom 模型接入验证",
                  params: {},
                  output_count: 1,
                })) ?? [],
                null,
                2,
              )}
              description="每个模式填写一条测试。引用须来自测试项目，并包含资产 ID、角色和版本。"
            />
          </FieldGroup>
        )}
        <Button type="submit" disabled={busy || !model}>
          保存采购价与验证计划
        </Button>
      </FieldGroup>
    </form>
  );
}

export function CreditsForm({
  run,
  busy,
}: {
  run: RunMutation;
  busy: boolean;
}) {
  const [preview, setPreview] = useState<{
    scope: string;
    reference: string;
    credits: number;
  } | null>(null);
  return (
    <form
      aria-label="管理员增加积分"
      onChange={() => setPreview(null)}
      onSubmit={(event) => {
        event.preventDefault();
        const data = new FormData(event.currentTarget);
        const next = {
          scope: text(data, "scope"),
          reference: text(data, "reference"),
          credits: num(data, "credits"),
        };
        if (!preview) {
          setPreview(next);
          return;
        }
        void run(
          () => fundCredits(next.scope, next.reference, next.credits),
          "积分已记入唯一账本。",
        ).then((ok) => {
          if (ok) setPreview(null);
        });
      }}
    >
      <FieldGroup>
        <InputField label="积分空间 ID" name="scope" placeholder="UUID" />
        <InputField
          label="唯一业务凭据"
          name="reference"
          description="填入可持久核对的原始业务编号，重复提交相同凭据不得重复入账。"
        />
        <InputField
          label="增加积分"
          name="credits"
          type="number"
          min={1}
          max={1000000000}
        />
        {preview ? (
          <p role="status" className="break-all">
            确认给空间 {preview.scope} 增加 {preview.credits} 积分，业务凭据：
            {preview.reference}。
          </p>
        ) : null}
        <Button type="submit" disabled={busy}>
          {preview ? "确认增加积分" : "核对入账信息"}
        </Button>
      </FieldGroup>
    </form>
  );
}

export function ReconciliationForm({
  run,
  busy,
  unknown,
}: {
  run: RunMutation;
  busy: boolean;
  unknown: GatewayConfiguration["unknown"];
}) {
  const [notCharged, setNotCharged] = useState(true);
  return (
    <form
      aria-label="凭供应商证据对账"
      onSubmit={(event) => {
        event.preventDefault();
        const data = new FormData(event.currentTarget);
        void run(
          () =>
            reconcileGateway({
              request_id: text(data, "request_id"),
              attempt_id: text(data, "attempt_id"),
              reference: text(data, "reference"),
              reason: text(data, "reason"),
              not_charged: notCharged,
              ...(!notCharged
                ? {
                    usage: {
                      input: num(data, "input"),
                      cache_read: num(data, "cache_read"),
                      cache_write: num(data, "cache_write"),
                      output: num(data, "output"),
                    },
                  }
                : {}),
            }),
          "原请求已按供应商证据对账。",
        );
      }}
    >
      <FieldGroup>
        <FieldDescription>
          必须先核对供应商原始账单 /
          交易凭据。本操作只结算原请求，不会重新调用供应商。
        </FieldDescription>
        {unknown.length ? (
          <ul className="flex flex-col gap-2">
            {unknown.map((row) => (
              <li key={row.attempt_id} className="break-all">
                请求 {row.request_id} / 尝试 {row.attempt_id} / 空间{" "}
                {row.scope_id}
              </li>
            ))}
          </ul>
        ) : (
          <p>暂无待核对请求。</p>
        )}
        <InputField label="原请求 ID" name="request_id" />
        <InputField label="原尝试 ID" name="attempt_id" />
        <InputField label="原始供应商账单凭据" name="reference" />
        <InputField label="核对原因" name="reason" />
        <Field orientation="horizontal">
          <FieldLabel>
            <Checkbox
              checked={notCharged}
              onCheckedChange={(checked) => setNotCharged(checked === true)}
            />
            供应商证实未收费
          </FieldLabel>
        </Field>
        {!notCharged ? (
          <FieldGroup className="grid sm:grid-cols-2">
            {[
              ["input", "实际输入 token"],
              ["cache_read", "实际缓存读取 token"],
              ["cache_write", "实际缓存写入 token"],
              ["output", "实际输出 token"],
            ].map(([name, label]) => (
              <InputField
                key={name}
                name={name}
                label={label}
                type="number"
                min={0}
                max={2 ** 40}
                value={0}
              />
            ))}
          </FieldGroup>
        ) : null}
        <Button type="submit" disabled={busy}>
          凭证据结算原请求
        </Button>
      </FieldGroup>
    </form>
  );
}
