"use client";

import { useRef, useState, type ReactNode } from "react";
import { ClipboardCheck, Coins, ReceiptText } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  FieldDescription,
  FieldGroup,
  FieldLegend,
  FieldSet,
} from "@/components/ui/field";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import {
  arkEmbedding,
  submitCreditGrant,
  modelPreset,
  prepareModel,
  prepareRoute,
  reconcileGateway,
  type AiServiceConfiguration,
  type GatewayModel,
  type GatewayPreset,
  type GatewayPrice,
  type MediaDriverOption,
  type OnboardingReview,
  type TextRates,
} from "@/lib/ai-service-admin";
import {
  CheckField,
  mediaTariffSummary,
  FormDetails,
  InputField,
  SelectField,
  SetupSelectField,
  SubmitButton,
  micros,
  num,
  text,
  uuidPattern,
  type RunMutation,
} from "./admin-fields";
import {
  MediaModelFields,
  modeLabel,
  MediaProbeFields,
  VideoMeterFields,
  readMediaModel,
  readMediaProbes,
  readVideoMetering,
} from "./media-fields";

export const protocolLabels: Record<string, string> = {
  chat: "Chat Completions",
  responses: "Responses",
  messages: "Messages",
  media: "媒体生成",
  [arkEmbedding.protocol]: "豆包文本向量",
};

const protocolOptions = Object.entries(protocolLabels);
const currencyOptions = [
  ["CNY", "人民币 CNY"],
  ["USD", "美元 USD"],
] as const;

function rateInCurrency(micros: number | null | undefined) {
  if (micros == null || !Number.isSafeInteger(micros) || micros < 0)
    return undefined;
  const digits = String(micros).padStart(7, "0");
  const fraction = digits.slice(-6).replace(/0+$/, "");
  const amount = digits.slice(0, -6);
  return fraction ? `${amount}.${fraction}` : amount;
}

function formatTextRates(rate: TextRates) {
  const base = `输入 ${rate.input} · 缓存读 ${rate.cache_read} · 缓存写 ${rate.cache_write} · 输出 ${rate.output}`;
  return rate.reasoning ? `${base} · 推理 ${rate.reasoning}` : base;
}

function PriceFields({
  kind,
  preset,
  embedding = false,
  media,
  videoTokens,
  videoResolution,
  defaultFX,
  advanced,
}: {
  kind: "销售" | "采购";
  preset?: GatewayPreset;
  media: boolean;
  embedding?: boolean;
  videoTokens?: boolean;
  videoResolution?: string;
  defaultFX?: string;
  advanced?: ReactNode;
}) {
  const [currency, setCurrency] = useState(preset?.currency ?? "CNY");
  const sameCurrency = currency === preset?.currency;
  const textTariff = !media && !embedding && preset?.text_tariff;
  const mediaTariff = media && preset?.media_tariff;
  const tokenPrice =
    sameCurrency && !media && !embedding ? preset.token_price : undefined;
  const requestPrice =
    sameCurrency && media && !videoTokens
      ? rateInCurrency(preset.request_micros)
      : undefined;
  const videoRate =
    videoTokens && currency === "CNY" && videoResolution
      ? preset?.video_estimate?.resolutions[videoResolution]
          ?.cny_per_million_without_video
      : undefined;
  const videoPrice =
    videoRate === undefined ? undefined : rateInCurrency(videoRate * 1_000_000);
  const hasDefault = Boolean(
    textTariff || mediaTariff || tokenPrice || requestPrice || videoPrice,
  );
  const unit = currency === "USD" ? "美元" : "元";
  const common = {
    type: "text",
    inputMode: "decimal" as const,
    pattern: "[0-9]+([.][0-9]{1,6})?",
    maxLength: 18,
    value: "0",
  };
  const fx =
    currency === "CNY"
      ? "1"
      : sameCurrency
        ? preset?.price_fx || defaultFX
        : undefined;
  const needsFX = Boolean(mediaTariff && currency === "USD" && !fx);
  const fxField = (
    <InputField
      key={currency}
      name="fx"
      label={`${kind}汇率`}
      inputMode="decimal"
      value={fx}
      description="1 美元折合的人民币金额"
    />
  );
  return (
    <FieldSet className="gap-3">
      <FieldLegend>{kind}价</FieldLegend>
      <FieldDescription>
        {kind === "销售" ? "用户计费" : "上游成本"} · {unit}{" "}
        {mediaTariff
          ? "· 按实际用量"
          : videoTokens
            ? "/ 百万视频 Token"
            : media
              ? "/ 次"
              : "/ 百万 Token"}
        {hasDefault && " · 默认使用供应商费率"}
        {!hasDefault &&
          preset &&
          (!sameCurrency && preset.currency
            ? " · 请填写当前币种的费率"
            : preset.price_unit === "per_1k_chars" ||
                preset.price_unit === "per_second"
              ? " · 供应商按字符或时长收费，需确认按次合同"
              : preset.video_estimate && videoTokens
                ? " · 选择供应商支持的分辨率"
                : preset.rates.length || preset.price_unit
                  ? " · 官方费率暂无法自动计费"
                  : " · 待补充供应商费率")}
      </FieldDescription>
      {mediaTariff ? (
        <>
          <p className="text-sm">{mediaTariffSummary(mediaTariff, currency)}</p>
        </>
      ) : textTariff ? (
        <>
          <dl className="grid gap-3 text-sm">
            {textTariff.tiers.map((tier) => (
              <div
                key={tier.from_input}
                className="flex flex-wrap gap-x-3 gap-y-1"
              >
                <dt className="font-medium">
                  {textTariff.schedule
                    ? "高峰"
                    : textTariff.tiers.length === 1
                      ? "标准"
                      : `≥ ${tier.from_input.toLocaleString("zh-CN")} 输入 Token`}
                </dt>
                <dd className="text-muted-foreground">
                  {formatTextRates(tier)}
                </dd>
              </div>
            ))}
            {textTariff.off_peak && (
              <div className="flex flex-wrap gap-x-3 gap-y-1">
                <dt className="font-medium">空闲</dt>
                <dd className="text-muted-foreground">
                  {formatTextRates(textTariff.off_peak)}
                </dd>
              </div>
            )}
          </dl>
        </>
      ) : (
        <FieldGroup className="grid gap-3 md:grid-cols-2">
          <SetupSelectField
            name="currency"
            label={`${kind}币种`}
            options={currencyOptions}
            value={currency}
            onValueChange={setCurrency}
          />
          {videoTokens ? (
            <InputField
              key={JSON.stringify([currency, videoResolution])}
              label={`${kind}视频 Token 单价`}
              name="output"
              {...common}
              value={videoPrice ?? common.value}
            />
          ) : embedding ? (
            <InputField label={`${kind}输入价`} name="input" {...common} />
          ) : media ? (
            <InputField
              key={currency}
              label={`${kind}按次价`}
              name="request"
              placeholder={`填写${kind}价`}
              {...common}
              value={requestPrice}
            />
          ) : (
            <FieldGroup key={currency} className="grid sm:grid-cols-2">
              <InputField
                label={`${kind}输入价`}
                name="input"
                {...common}
                value={rateInCurrency(tokenPrice?.input) ?? common.value}
              />
              <InputField
                label={`${kind}输出价`}
                name="output"
                {...common}
                value={rateInCurrency(tokenPrice?.output) ?? common.value}
              />
            </FieldGroup>
          )}
        </FieldGroup>
      )}
      {needsFX && fxField}
      {(Boolean(preset?.rates.length) || preset?.price_source_url) && (
        <FormDetails label="供应商参考价" keepMounted={false}>
          <div className="flex flex-col items-start gap-2 px-2 text-sm text-muted-foreground">
            {preset?.rates.length ? (
              <ul className="flex flex-col gap-1">
                {preset.rates.map((rate) => (
                  <li key={rate}>{rate}</li>
                ))}
              </ul>
            ) : null}
            {preset?.official_price_observed_on && (
              <p>价目核对日期 {preset.official_price_observed_on}</p>
            )}
            {preset?.price_source_url && (
              <a
                href={preset.price_source_url}
                target="_blank"
                rel="noreferrer"
                className="underline underline-offset-4"
              >
                供应商价目
              </a>
            )}
          </div>
        </FormDetails>
      )}
      <FormDetails
        label={
          advanced
            ? "更多设置"
            : media || embedding
              ? "汇率与折扣"
              : "缓存与其他费率"
        }
      >
        <FieldGroup>
          {advanced}
          {!textTariff && !media && !embedding && (
            <FieldGroup key={currency} className="grid sm:grid-cols-3">
              <InputField
                name="cache_read"
                label={`${kind}缓存读取价`}
                {...common}
                value={rateInCurrency(tokenPrice?.cache_read) ?? common.value}
                description={`${unit} / 百万 token`}
              />
              <InputField
                name="cache_write"
                label={`${kind}缓存写入价`}
                {...common}
                value={
                  rateInCurrency(
                    tokenPrice?.cache_write ?? tokenPrice?.input,
                  ) ?? common.value
                }
                description={`${unit} / 百万 token`}
              />
              <InputField
                name="request"
                label={`${kind}按次附加费`}
                {...common}
                description={`${unit} / 次`}
              />
            </FieldGroup>
          )}
          <FieldGroup className="grid sm:grid-cols-3">
            {!needsFX && fxField}
            <InputField
              name="multiplier"
              label={`${kind}倍率`}
              inputMode="decimal"
              value="1"
            />
            <InputField
              name="discount"
              label={`${kind}折扣`}
              inputMode="decimal"
              value="1"
              description="大于 0，最多为 1。"
            />
          </FieldGroup>
        </FieldGroup>
      </FormDetails>
    </FieldSet>
  );
}

function readPrice(form: FormData, preset?: GatewayPreset): GatewayPrice {
  const currency =
    (preset?.text_tariff || preset?.media_tariff
      ? preset.currency
      : text(form, "currency")) ?? "";
  return {
    text_tariff: preset?.text_tariff ?? undefined,
    media_tariff: preset?.media_tariff ?? undefined,
    input: micros(form, "input"),
    output: micros(form, "output"),
    request: micros(form, "request"),
    cache_read: micros(form, "cache_read"),
    cache_write: micros(form, "cache_write"),
    currency,
    fx: text(form, "fx") || (currency === "CNY" ? "1" : ""),
    multiplier: text(form, "multiplier") || "1",
    discount: text(form, "discount") || "1",
  };
}

type ModelFormProps = {
  run: RunMutation;
  busy: boolean;
  onPrepared: (id: string) => void;
};
export function ModelSettings({
  run,
  busy,
  onPrepared,
  preset,
  driver,
}: ModelFormProps & { preset?: GatewayPreset; driver?: MediaDriverOption }) {
  const [protocol, setProtocol] = useState(
    preset && preset.capability !== "text.generate" ? "media" : "chat",
  );
  const [capability, setCapability] = useState(
    preset?.capability ?? "image.generate",
  );
  const [billingUnit, setBillingUnit] = useState(
    !preset || preset.adapter === "volcengine_ark" ? "video_tokens" : "request",
  );
  const videoResolutions = Object.keys(
    preset?.video_estimate?.resolutions ?? {},
  );
  const [videoResolution, setVideoResolution] = useState("");
  const attempt = useRef<{ payload: string; id: string } | null>(null);
  const media = protocol === "media";
  const embedding = protocol === arkEmbedding.protocol;
  const video = media && capability === "video.generate";
  const videoSeconds =
    preset?.media_tariff?.meter === "output_video_seconds_v1";
  const imageBudget =
    preset?.media_tariff?.meter === "reported_image_amount_v1" ||
    preset?.media_tariff?.meter === "image_tokens_v1";
  const videoTokens = video && !videoSeconds && billingUnit === "video_tokens";
  const mediaTariff = preset?.media_tariff;
  const presetPurchaseCap =
    videoSeconds && mediaTariff?.max_quantity
      ? rateInCurrency(
          Math.round(
            Number(mediaTariff.unit_price) *
              mediaTariff.max_quantity *
              1_000_000,
          ),
        )
      : "10";
  const fields =
    preset?.fields.filter(
      (field) =>
        driver?.parameters[capability]?.includes(field.field) &&
        !(
          videoTokens &&
          [
            "resolution",
            "duration",
            "duration_ms",
            "ratio",
            "aspect_ratio",
          ].includes(field.field)
        ),
    ) ?? [];
  const purchaseCap = (
    <InputField
      key={videoTokens ? "video-procurement-cap" : "default-procurement-cap"}
      label={imageBudget ? "单次授权上限（元）" : "单次采购上限（元）"}
      name="purchase_cap"
      value={videoTokens ? undefined : presetPurchaseCap}
      description={
        imageBudget ? "确认时冻结此金额对应的积分，核销按实际用量。" : undefined
      }
      inputMode="decimal"
      maxLength={18}
      pattern="[0-9]+([.][0-9]{1,6})?"
    />
  );
  const modelFields = (
    <>
      <FieldGroup className="grid sm:grid-cols-2">
        <InputField
          label="销售模型名称"
          name="name"
          value={preset?.name}
          placeholder="例如 doubao/seed"
          maxLength={160}
        />
        <SetupSelectField
          label="模型协议"
          name="protocol"
          options={
            preset
              ? protocolOptions.filter(([id]) =>
                  preset.capability === "text.generate"
                    ? ["chat", "responses", "messages"].includes(id)
                    : id === "media",
                )
              : protocolOptions
          }
          value={protocol}
          onValueChange={setProtocol}
        />
      </FieldGroup>
      {media && preset ? (
        <FieldSet>
          <FieldLegend variant="label">支持的生成模式</FieldLegend>
          <div className="flex flex-wrap gap-3">
            {preset.modes
              .filter(
                (mode) =>
                  !videoTokens ||
                  mode === "text2video" ||
                  (mode === "image2video" &&
                    !preset.upstream.startsWith("doubao-seedance-2-5-")),
              )
              .map((mode) => (
                <CheckField
                  key={mode}
                  name="preset_mode"
                  value={mode}
                  label={modeLabel(mode)}
                  defaultChecked
                />
              ))}
          </div>
          {fields.length > 0 && (
            <FormDetails label="生成参数与默认值">
              {fields.map((field) => (
                <p key={field.field} className="text-sm">
                  {field.label} · {String(field.default ?? field.type)}
                </p>
              ))}
            </FormDetails>
          )}
        </FieldSet>
      ) : media ? (
        <MediaModelFields
          capability={capability}
          onCapabilityChange={setCapability}
        />
      ) : (
        !embedding && (
          <FieldSet>
            <FieldLegend variant="label">文本能力</FieldLegend>
            <FieldGroup className="grid sm:grid-cols-2">
              <CheckField label="支持工具调用" name="tools" />
              <CheckField label="支持流式输出" name="stream" />
            </FieldGroup>
          </FieldSet>
        )
      )}
    </>
  );
  const limits = (
    <FieldGroup className="grid sm:grid-cols-2">
      {!videoTokens && !videoSeconds && !imageBudget && purchaseCap}
      {!media && (
        <>
          <InputField
            label="最大输入 token"
            name="max_input"
            type="number"
            min={1}
            max={1048576}
            value={32768}
          />
          {!embedding && (
            <InputField
              label="最大输出 token"
              name="max_output"
              type="number"
              min={1}
              max={1048576}
              value={4096}
            />
          )}
        </>
      )}
      {media && (
        <CheckField
          label="已审核供应商成功输出策略"
          name="reviewed"
          defaultChecked={videoSeconds}
        />
      )}
    </FieldGroup>
  );
  return (
    <form
      aria-label="准备模型与销售价"
      onSubmit={(event) => {
        event.preventDefault();
        const form = new FormData(event.currentTarget);
        void run(async () => {
          const draft = !media
            ? undefined
            : preset
              ? {
                  capability,
                  modes: form.getAll("preset_mode").map(String),
                  parameters: Object.fromEntries(
                    fields.map(
                      ({
                        field,
                        type,
                        min,
                        max,
                        enum: options,
                        default: value,
                      }) => [
                        field,
                        { type, min, max, enum: options, default: value },
                      ],
                    ),
                  ),
                  output_policy:
                    form.get("reviewed") === "on"
                      ? "provider_success_v1"
                      : undefined,
                }
              : readMediaModel(form);
          const model = {
            ...(draft && videoTokens ? readVideoMetering(form, draft) : draft),
            preset_key: preset?.key,
            name: text(form, "name"),
            protocol,
            price: readPrice(form, preset),
            max_input: Number(text(form, "max_input") || "32768"),
            max_output: embedding
              ? 1
              : videoTokens
                ? num(form, "max_output")
                : Number(text(form, "max_output") || "4096"),
            purchase_cap: micros(form, "purchase_cap"),
            tools: !embedding && !media && form.get("tools") === "on",
            stream: !embedding && !media && form.get("stream") === "on",
          };
          const payload = JSON.stringify(model);
          if (attempt.current?.payload !== payload)
            attempt.current = { payload, id: crypto.randomUUID() };
          const { id } = attempt.current;
          await prepareModel(model, id);
          attempt.current = null;
          onPrepared(id);
        }, "模型预设与销售价已保存。");
      }}
    >
      <FieldGroup>
        {!preset && modelFields}
        {video && !videoSeconds && (
          <SetupSelectField
            label="视频计费方式"
            value={billingUnit}
            onValueChange={setBillingUnit}
            options={[
              ["request", "已有按次合同"],
              ["video_tokens", "按实际视频 Token"],
            ]}
          />
        )}
        {videoTokens && (
          <VideoMeterFields
            resolutions={videoResolutions}
            resolution={videoResolution}
            onResolutionChange={setVideoResolution}
          />
        )}
        {!preset && <FormDetails label="能力与使用限制">{limits}</FormDetails>}
        <PriceFields
          kind="销售"
          advanced={
            preset && (
              <>
                {modelFields}
                {limits}
              </>
            )
          }
          preset={preset}
          embedding={embedding}
          media={media}
          videoTokens={videoTokens}
          videoResolution={videoResolution}
        />
        {(videoTokens || videoSeconds || imageBudget) && purchaseCap}
        <div className="flex justify-end">
          <SubmitButton busy={busy} label="保存模型" />
        </div>
      </FieldGroup>
    </form>
  );
}

export function RouteForm({
  configuration,
  run,
  busy,
  onPrepared,
  initialModelID,
  fixedModel = false,
  defaultScope,
}: {
  configuration: AiServiceConfiguration;
  run: RunMutation;
  busy: boolean;
  onPrepared: (review: OnboardingReview) => void;
  initialModelID?: string;
  fixedModel?: boolean;
  defaultScope?: string;
}) {
  const models = configuration.models.filter(
    ({ enabled, draft }) => enabled || draft,
  );
  const [selected, setSelected] = useState(initialModelID ?? "");
  const model =
    models.find(({ model }) => model.id === selected)?.model ??
    (fixedModel ? undefined : models[0]?.model);

  if (!model)
    return (
      <p className="text-sm text-muted-foreground">
        请先保存模型预设与销售价。
      </p>
    );
  const knownRoutes = configuration.routes.filter(
    ({ route }) => route.model_id === model.id,
  );
  const sources = knownRoutes.length
    ? knownRoutes.map(({ provider_key, route }) => ({
        provider_key,
        upstream_model: route.upstream_model,
      }))
    : configuration.onboarding.filter(
        (entry) =>
          entry.model_id === model.id &&
          ["prepared", "verifying", "failed"].includes(entry.state),
      );
  const source = sources[0];
  const consistent =
    source &&
    sources.every(
      (entry) =>
        entry.provider_key === source.provider_key &&
        entry.upstream_model === source.upstream_model,
    );
  const preset = modelPreset(
    configuration.model_presets,
    model,
    consistent ? source.provider_key : undefined,
    consistent ? source.upstream_model : undefined,
  );
  return (
    <PreparedRouteForm
      key={model.id}
      preset={preset}
      model={model}
      models={models}
      chooseModel={fixedModel ? undefined : setSelected}
      configuration={configuration}
      run={run}
      busy={busy}
      onPrepared={onPrepared}
      defaultScope={defaultScope}
    />
  );
}

function PreparedRouteForm({
  preset,
  model,
  models,
  chooseModel,
  configuration,
  run,
  busy,
  onPrepared,
  defaultScope,
}: {
  model: GatewayModel;
  preset?: GatewayPreset;
  models: AiServiceConfiguration["models"];
  chooseModel?: (id: string) => void;
  configuration: AiServiceConfiguration;
  run: RunMutation;
  busy: boolean;
  onPrepared: (review: OnboardingReview) => void;
  defaultScope?: string;
}) {
  const attempt = useRef<{ payload: string; id: string } | null>(null);
  const newSupplier = useRef<string>(undefined);
  const media = model.protocol === "media";
  const embedding = model.protocol === arkEmbedding.protocol;
  const drivers = configuration.media_drivers.filter(
    (option) =>
      model.capability !== undefined &&
      option.parameters[model.capability] !== undefined &&
      (!model.modes?.includes("text2audio") ||
        option.key === "volcengine_speech"),
  );
  const [driverKey, setDriverKey] = useState(preset?.adapter ?? "");
  const [supplier, setSupplier] = useState("");
  const [groupOverride, setGroupOverride] = useState<string>();
  const draftRoute = configuration.routes.find(
    ({ route }) => route.supplier === supplier,
  )?.route;
  const group = (groupOverride ?? draftRoute?.group ?? "").trim();
  const quota = configuration.quotas.find((item) => item.name === group);
  const noVerificationSlot =
    quota !== undefined &&
    (quota.concurrency <= 1 || quota.rpm <= 1) &&
    configuration.routes.some(
      ({ route, enabled }) =>
        enabled &&
        route.group === group &&
        configuration.models.some(
          ({ model, enabled }) => enabled && model.id === route.model_id,
        ) &&
        configuration.credentials.some(
          ({ id, state }) => id === route.credential && state === "active",
        ),
    );
  const driver = drivers.find((option) => option.key === driverKey);
  const replacements = configuration.routes.filter(
    ({ route, enabled }) => enabled && route.model_id === model.id,
  );
  const supplierRoutes = new Map(
    configuration.routes
      .filter((row) => !preset || row.provider_key === preset.adapter)
      .map(({ route }) => [route.supplier, route]),
  );
  const hasConnectionDefaults = Boolean(
    preset && (media ? driver?.base_url : preset.base_url),
  );
  const connectionFields = (
    <>
      {media && (
        <>
          <SetupSelectField
            label="媒体驱动"
            value={driverKey}
            onValueChange={setDriverKey}
            placeholder="选择已安装的驱动"
            options={drivers.map((option) => [option.key, option.name])}
            required
          />
          {!drivers.length && (
            <FieldDescription>
              当前未读取到支持该生成类型的媒体驱动。
            </FieldDescription>
          )}
        </>
      )}
      {(!media || driver) && (
        <FieldGroup className="grid sm:grid-cols-2">
          <InputField
            key={`${driverKey}:${supplier}`}
            label="供应商地址"
            name="base_url"
            type="url"
            placeholder="https://api.example.com/v1"
            readOnly={
              embedding ||
              Boolean(driver && !driver.custom_url) ||
              Boolean(preset?.text_tariff || preset?.media_tariff)
            }
            value={
              embedding
                ? arkEmbedding.baseURL
                : (driver?.base_url ??
                  (preset?.text_tariff || preset?.media_tariff
                    ? preset.base_url
                    : (draftRoute?.base_url ?? preset?.base_url)))
            }
          />
          {driver && (
            <div className="flex flex-col gap-2 text-sm">
              <span className="font-medium">服务地区</span>
              <Badge variant="secondary" className="w-fit">
                {driver.region === "domestic" ? "国内" : "海外"}
              </Badge>
            </div>
          )}
        </FieldGroup>
      )}
    </>
  );
  const quotaFields = (
    <FieldGroup key={quota?.name ?? supplier} className="grid sm:grid-cols-2">
      <InputField
        label="共享并发"
        name="concurrency"
        type="number"
        min={1}
        max={1000}
        value={quota?.concurrency ?? preset?.concurrency ?? 1}
      />
      <InputField
        label="每分钟请求上限"
        name="rpm"
        type="number"
        min={1}
        max={media ? 100000 : Number.MAX_SAFE_INTEGER}
        value={quota?.rpm ?? preset?.rpm ?? 60}
      />
      <InputField
        label="路由优先级"
        name="priority"
        type="number"
        min={0}
        value={0}
        description="数值越小，越优先尝试。"
      />
      {media && (
        <InputField
          label="请求超时（秒）"
          name="timeout"
          type="number"
          min={5}
          max={300}
          value={60}
        />
      )}
    </FieldGroup>
  );
  return (
    <form
      aria-label="准备供应商路由与采购价"
      onSubmit={(event) => {
        event.preventDefault();
        if (noVerificationSlot) return;
        const data = new FormData(event.currentTarget);
        void run(async () => {
          if (media && !driver) throw new Error("请选择已安装的媒体驱动。");
          const group = text(data, "group");
          const concurrency = num(data, "concurrency");
          const rpm = num(data, "rpm");
          const baseURL = text(data, "base_url");
          if (!supplier && !newSupplier.current)
            newSupplier.current = crypto.randomUUID();
          const route = {
            model_id: model.id,
            supplier: supplier || newSupplier.current!,
            group,
            base_url: baseURL,
            upstream_model: text(data, "upstream_model"),
            protocol: model.protocol,
            purchase: readPrice(data, preset),
            priority: num(data, "priority"),
            ...(driver
              ? {
                  driver_config: mediaConfig(
                    driver,
                    model,
                    baseURL,
                    data,
                    concurrency,
                    rpm,
                  ),
                }
              : {}),
          };
          const options = {
            scope: text(data, "scope_id"),
            project: text(data, "project_id"),
            replacement: text(data, "replacement"),
            mediaTests: media ? readMediaProbes(data, model) : undefined,
            concurrency,
            rpm,
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
        }, "采购价与验证计划已保存。");
      }}
    >
      <FieldGroup>
        {chooseModel && (
          <SetupSelectField
            label="销售模型版本"
            value={model.id}
            onValueChange={chooseModel}
            options={models.map(({ model, draft }) => [
              model.id,
              `${model.name} · ${draft ? "草稿" : "已启用"} · ${model.id.slice(0, 8)}`,
            ])}
          />
        )}
        <FieldGroup className="grid sm:grid-cols-2">
          <SetupSelectField
            label="供应商账号"
            value={supplier || "new"}
            onValueChange={(value) => {
              setSupplier(value === "new" ? "" : value);
              setGroupOverride(undefined);
            }}
            options={[
              ["new", "新建供应商账号"],
              ...[...supplierRoutes].map(
                ([id, route]) =>
                  [id, `${route.group} · ${id.slice(0, 8)}`] as const,
              ),
            ]}
            description="同一供应商账号请选择已有记录，以共享配额。"
          />
          <InputField
            label="供应商模型号"
            name="upstream_model"
            value={embedding ? arkEmbedding.model : preset?.upstream}
            readOnly={
              embedding || Boolean(preset?.text_tariff || preset?.media_tariff)
            }
            placeholder="供应商返回的精确型号"
            maxLength={160}
          />
        </FieldGroup>
        {!hasConnectionDefaults && connectionFields}
        {replacements.length > 0 && (
          <SelectField
            label="替换现有路由"
            name="replacement"
            options={[
              ["", "增加路由"],
              ...replacements.map(
                ({ route }) =>
                  [
                    route.id ?? "",
                    `${route.upstream_model} · ${route.group}`,
                  ] as const,
              ),
            ]}
            description="新版通过验证后才切换。"
          />
        )}
        <FieldGroup className="grid sm:grid-cols-2">
          <InputField
            key={supplier}
            label="共享配额组"
            name="group"
            value={draftRoute?.group}
            onChange={(event) => setGroupOverride(event.currentTarget.value)}
            placeholder="例如 ark-team"
            maxLength={160}
          />
          <InputField
            label="验证付款空间 ID"
            name="scope_id"
            pattern={uuidPattern}
            value={defaultScope}
            description="验证积分从这个空间扣除。"
          />
        </FieldGroup>
        {noVerificationSlot && (
          <Alert variant="destructive">
            <AlertTitle>共享配额暂时无法验证</AlertTitle>
            <AlertDescription>
              线上路由需要保留一个并发名额和每分钟一个请求额度。请选择有验证余量的供应商账号。
            </AlertDescription>
          </Alert>
        )}
        {!preset && (
          <FormDetails label="共享配额与超时">{quotaFields}</FormDetails>
        )}
        <PriceFields
          kind="采购"
          advanced={
            preset && (
              <>
                {hasConnectionDefaults && connectionFields}
                {quotaFields}
              </>
            )
          }
          preset={preset}
          embedding={embedding}
          media={media}
          videoTokens={Boolean(model.video_metering)}
          videoResolution={model.video_metering?.resolution}
          defaultFX={
            preset?.media_tariff && model.price.currency === preset.currency
              ? model.price.fx
              : undefined
          }
        />
        {media && (
          <>
            <Separator />
            <MediaProbeFields model={model} />
          </>
        )}
        <div className="flex justify-end">
          <SubmitButton
            busy={busy}
            disabled={noVerificationSlot || (media && !driver)}
            label="准备验证"
            icon={ClipboardCheck}
          />
        </div>
      </FieldGroup>
    </form>
  );
}

function mediaConfig(
  driver: MediaDriverOption,
  model: GatewayModel,
  baseURL: string,
  form: FormData,
  concurrency: number,
  rpm: number,
) {
  const capability = model.capability ?? "";
  const allowed = driver.parameters[capability] ?? [];
  const unsupported = Object.keys(model.parameters ?? {}).filter(
    (name) => !allowed.includes(name),
  );
  if (unsupported.length)
    throw new Error(
      `该驱动不支持参数 ${unsupported.join("、")}。请选择兼容驱动或准备新的模型预设。`,
    );
  return {
    adapter_key: driver.key,
    base_url: baseURL,
    region: driver.region,
    capabilities: [capability.replace(".generate", "")],
    timeout_seconds: num(form, "timeout"),
    concurrency_limit: concurrency,
    rate_limit_per_min: rpm,
  };
}
export function CreditsForm({
  run,
  busy,
  defaultScope,
}: {
  run: RunMutation;
  busy: boolean;
  defaultScope?: string;
}) {
  const [preview, setPreview] = useState<{
    scope: string;
    reference: string;
    credits: number;
    reason: string;
  } | null>(null);
  return (
    <form
      aria-label="提交积分赠送单"
      onChange={() => setPreview(null)}
      onSubmit={(event) => {
        event.preventDefault();
        const data = new FormData(event.currentTarget);
        const next = {
          scope: text(data, "scope"),
          reference: text(data, "reference"),
          credits: num(data, "credits"),
          reason: text(data, "reason"),
        };
        if (!preview) {
          setPreview(next);
          return;
        }
        void run(
          () =>
            submitCreditGrant({
              scope_id: next.scope,
              key: next.reference,
              credits: next.credits,
              reason: next.reason,
            }),
          "已提交赠送单，等待另一位管理员复核。",
        ).then((ok) => {
          if (ok) setPreview(null);
        });
      }}
    >
      <FieldGroup>
        <FieldGroup className="grid sm:grid-cols-2">
          <InputField
            label="积分空间 ID"
            name="scope"
            pattern={uuidPattern}
            value={defaultScope}
          />
          <InputField
            label="唯一业务凭据"
            name="reference"
            description="业务编号全局唯一。"
            maxLength={120}
          />
          <InputField
            label="赠送积分"
            name="credits"
            type="number"
            min={1}
            max={1000000000}
            step={1}
          />
        </FieldGroup>
        <InputField label="赠送原因" name="reason" maxLength={500} />
        {preview ? (
          <p role="status" className="break-all">
            向空间 {preview.scope} 申请赠送 {preview.credits} 通用积分，凭据为
            {preview.reference}。
          </p>
        ) : null}
        <SubmitButton
          busy={busy}
          icon={Coins}
          label={preview ? "提交复核" : "核对赠送信息"}
        />
      </FieldGroup>
    </form>
  );
}

export function ReconciliationForm({
  run,
  busy,
  unknown,
  initialAttempt = "",
}: {
  run: RunMutation;
  busy: boolean;
  unknown: AiServiceConfiguration["unknown"];
  initialAttempt?: string;
}) {
  const [billingResult, setBillingResult] = useState("");
  const [selected, setSelected] = useState(initialAttempt);
  const pending = unknown.find((record) => record.attempt_id === selected);
  const videoInvoice =
    pending?.state === "settled" &&
    pending.tariff_meter === "output_video_seconds_v1";
  const outcome = videoInvoice ? "charged" : billingResult;
  return (
    <form
      aria-label="凭供应商证据对账"
      onSubmit={(event) => {
        event.preventDefault();
        if (!outcome) return;
        const data = new FormData(event.currentTarget);
        const notCharged = outcome === "not_charged";
        void run(() => {
          const billedAt = text(data, "billed_at");
          const evidenceTime = billedAt
            ? new Date(billedAt).toISOString()
            : undefined;
          return reconcileGateway({
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
                    reasoning: num(data, "reasoning") || undefined,
                    characters: num(data, "characters") || undefined,
                    original_seconds:
                      text(data, "original_seconds") || undefined,
                    pixels: num(data, "pixels") || undefined,
                    reference_images:
                      num(data, "reference_images") || undefined,
                    billing_started_at: evidenceTime,
                    billing_received_at: evidenceTime,
                  },
                }
              : {}),
          });
        }, "原请求已按供应商证据对账。");
      }}
    >
      <FieldGroup>
        <FieldDescription>
          必须先核对供应商原始账单 /
          交易凭据。本操作只结算原请求，不会重新调用供应商。
        </FieldDescription>
        <SelectField
          label="待核对记录"
          value={selected}
          onChange={(event) => setSelected(event.currentTarget.value)}
          options={[
            ["", "手动填写原请求与尝试"],
            ...unknown.map(
              (row) =>
                [
                  row.attempt_id,
                  `请求 ${row.request_id.slice(0, 8)} · 尝试 ${row.attempt_id.slice(0, 8)}`,
                ] as const,
            ),
          ]}
        />
        <FieldGroup key={selected} className="grid sm:grid-cols-2">
          <InputField
            label="原请求 ID"
            name="request_id"
            readOnly={Boolean(pending)}
            value={pending?.request_id}
            pattern={uuidPattern}
          />
          <InputField
            label="原尝试 ID"
            name="attempt_id"
            readOnly={Boolean(pending)}
            value={pending?.attempt_id}
            pattern={uuidPattern}
          />
        </FieldGroup>
        <InputField
          key={`reference-${selected}`}
          label="原始供应商账单凭据"
          name="reference"
          value={pending?.upstream_id}
          description={
            pending?.upstream_id
              ? "请用该原始任务号核对供应商用量。"
              : undefined
          }
        />
        <InputField label="核对原因" name="reason" />
        {!videoInvoice && (
          <SelectField
            label="供应商账单结果"
            value={billingResult}
            required
            onChange={(event) => setBillingResult(event.currentTarget.value)}
            options={[
              ["", "选择账单结果"],
              ["not_charged", "供应商证实未收费"],
              ["charged", "供应商已收费，填写用量"],
            ]}
          />
        )}
        {outcome === "charged" ? (
          <FieldGroup className="grid sm:grid-cols-2">
            {(!pending?.tariff_meter
              ? [
                  ["input", "实际输入 token"],
                  ["cache_read", "实际缓存读取 token"],
                  ["cache_write", "实际缓存写入 token"],
                  ["output", "实际输出 token"],
                  ["reasoning", "实际推理 token"],
                ]
              : pending.tariff_meter === "image_tokens_v1"
                ? [
                    ["input", "实际文本输入 Token"],
                    ["output", "实际图像输出 Token"],
                  ]
                : pending.tariff_meter === "speech_characters_v1"
                  ? [["characters", "实际计费字符数"]]
                  : pending.tariff_meter === "ark_image_pixels_v1"
                    ? [
                        ["pixels", "实际输出像素数"],
                        ["reference_images", "提交的参考图数"],
                      ]
                    : []
            ).map(([name, label]) => (
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
            {(videoInvoice ||
              pending?.tariff_meter === "reported_image_amount_v1") && (
              <InputField
                name="provider_amount"
                label={
                  videoInvoice
                    ? "供应商实际扣款（元）"
                    : "供应商实际扣款（USD）"
                }
                required
                inputMode="decimal"
                pattern="[0-9]+([.][0-9]{1,24})?"
              />
            )}
            {pending?.tariff_meter === "original_audio_seconds_v1" && (
              <InputField
                name="original_seconds"
                label="原始音频时长（秒）"
                required
                inputMode="decimal"
                pattern="[0-9]+([.][0-9]{1,24})?"
              />
            )}
            {pending?.timed_tariff && (
              <InputField
                name="billed_at"
                label="供应商计费时间"
                required
                placeholder="2026-11-27T09:00:00+08:00"
                pattern=".+(Z|[+\-][0-9]{2}:[0-9]{2})"
              />
            )}
          </FieldGroup>
        ) : null}
        <SubmitButton
          busy={busy}
          label={videoInvoice ? "记录供应商实账" : "凭证据结算原请求"}
          icon={ReceiptText}
        />
      </FieldGroup>
    </form>
  );
}
