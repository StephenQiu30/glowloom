"use client";

import { useRef, useState } from "react";
import { ArrowRight } from "lucide-react";
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
  fundCredits,
  prepareModel,
  prepareRoute,
  reconcileGateway,
  type GatewayConfiguration,
  type GatewayModel,
  type GatewayPrice,
  type MediaDriverOption,
  type OnboardingReview,
} from "@/lib/ai-gateway-admin";
import {
  CheckField,
  FormDetails,
  InputField,
  SelectField,
  SubmitButton,
  micros,
  num,
  text,
  uuidPattern,
  type RunMutation,
} from "./gateway-fields";
import {
  MediaModelFields,
  MediaProbeFields,
  VideoMeterFields,
  readMediaModel,
  readMediaProbes,
  readVideoMetering,
} from "./gateway-media-fields";

export const protocolLabels: Record<string, string> = {
  chat: "Chat Completions",
  responses: "Responses",
  messages: "Messages",
  media: "媒体生成",
  local: "本地运行",
  [arkEmbedding.protocol]: "豆包文本向量",
};

const protocolOptions = Object.entries(protocolLabels).filter(
  ([key]) => key !== "local",
);
const currencyOptions = [
  ["CNY", "人民币 CNY"],
  ["USD", "美元 USD"],
] as const;

function PriceFields({
  kind,
  embedding = false,
  media,
  videoTokens,
}: {
  kind: "销售" | "采购";
  media: boolean;
  embedding?: boolean;
  videoTokens?: boolean;
}) {
  const [currency, setCurrency] = useState("CNY");
  const unit = currency === "USD" ? "美元" : "元";
  const common = {
    type: "text",
    inputMode: "decimal" as const,
    pattern: "[0-9]+([.][0-9]{1,6})?",
    maxLength: 18,
    value: "0",
  };
  return (
    <FieldSet>
      <FieldLegend>{kind}定价</FieldLegend>
      <FieldDescription>
        {videoTokens
          ? `按 ${unit} / 百万视频 Token 计价。`
          : media
            ? `每次请求按 ${unit} 计价。`
            : embedding
              ? `输入按 ${unit} / 百万 token 计价。`
              : `输入和输出按 ${unit} / 百万 token 计价。`}
      </FieldDescription>
      <FieldGroup>
        <SelectField
          name="currency"
          label={`${kind}币种`}
          options={currencyOptions}
          value={currency}
          onChange={(event) => setCurrency(event.currentTarget.value)}
        />
        {videoTokens ? (
          <InputField
            label={`${kind}视频 Token 单价`}
            name="output"
            {...common}
          />
        ) : embedding ? (
          <InputField label={`${kind}输入价`} name="input" {...common} />
        ) : media ? (
          <InputField label={`${kind}按次价`} name="request" {...common} />
        ) : (
          <FieldGroup className="grid sm:grid-cols-2">
            <InputField label={`${kind}输入价`} name="input" {...common} />
            <InputField label={`${kind}输出价`} name="output" {...common} />
          </FieldGroup>
        )}
        <FormDetails
          label={media || embedding ? "汇率与折扣" : "缓存与其他费率"}
        >
          <FieldGroup>
            {!media && !embedding && (
              <FieldGroup className="grid sm:grid-cols-3">
                <InputField
                  name="cache_read"
                  label={`${kind}缓存读取价`}
                  {...common}
                  description={`${unit} / 百万 token`}
                />
                <InputField
                  name="cache_write"
                  label={`${kind}缓存写入价`}
                  {...common}
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
              <InputField
                name="fx"
                label={`${kind}汇率`}
                inputMode="decimal"
                value="1"
                description="兑换人民币的汇率。"
              />
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
      </FieldGroup>
    </FieldSet>
  );
}

function readPrice(form: FormData): GatewayPrice {
  return {
    input: micros(form, "input"),
    output: micros(form, "output"),
    request: micros(form, "request"),
    cache_read: micros(form, "cache_read"),
    cache_write: micros(form, "cache_write"),
    currency: text(form, "currency"),
    fx: text(form, "fx") || "1",
    multiplier: text(form, "multiplier") || "1",
    discount: text(form, "discount") || "1",
  };
}

export function ModelForm({
  run,
  busy,
  onPrepared,
}: {
  run: RunMutation;
  busy: boolean;
  onPrepared: (id: string) => void;
}) {
  const [protocol, setProtocol] = useState("chat");
  const [capability, setCapability] = useState("image.generate");
  const [billingUnit, setBillingUnit] = useState("video_tokens");
  const attempt = useRef<{ payload: string; id: string } | null>(null);
  const media = protocol === "media";
  const embedding = protocol === arkEmbedding.protocol;
  const video = media && capability === "video.generate";
  const videoTokens = video && billingUnit === "video_tokens";
  return (
    <form
      aria-label="准备模型与销售价"
      onSubmit={(event) => {
        event.preventDefault();
        const form = new FormData(event.currentTarget);
        void run(async () => {
          const draft = media ? readMediaModel(form) : undefined;
          const model = {
            ...(draft && videoTokens ? readVideoMetering(form, draft) : draft),
            name: text(form, "name"),
            protocol,
            price: readPrice(form),
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
        <FieldGroup className="grid sm:grid-cols-2">
          <InputField
            label="销售模型名称"
            name="name"
            placeholder="例如 doubao/seed"
            maxLength={160}
            description="同名同协议的新版本通过验证后才会切换。"
          />
          <SelectField
            label="模型协议"
            name="protocol"
            options={protocolOptions}
            value={protocol}
            onChange={(event) => setProtocol(event.currentTarget.value)}
          />
        </FieldGroup>
        {media ? (
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
        {video && (
          <SelectField
            label="视频计费方式"
            value={billingUnit}
            onChange={(event) => setBillingUnit(event.currentTarget.value)}
            options={[
              ["request", "已有按次合同"],
              ["video_tokens", "按实际视频 Token"],
            ]}
          />
        )}
        {videoTokens && <VideoMeterFields />}
        <FormDetails label="能力与使用限制">
          <FieldGroup className="grid sm:grid-cols-2">
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
              <CheckField label="已审核供应商成功输出策略" name="reviewed" />
            )}
          </FieldGroup>
        </FormDetails>
        <Separator />
        <PriceFields
          kind="销售"
          embedding={embedding}
          media={media}
          videoTokens={videoTokens}
        />
        <InputField
          key={
            videoTokens ? "video-procurement-cap" : "default-procurement-cap"
          }
          label="单次采购上限（元）"
          name="purchase_cap"
          value={videoTokens ? undefined : "10"}
          inputMode="decimal"
          maxLength={18}
          pattern="[0-9]+([.][0-9]{1,6})?"
          description="限制一次模型调用的最高采购费用。"
        />
        <div>
          <SubmitButton busy={busy}>
            保存并继续采购配置
            <ArrowRight data-icon="inline-end" aria-hidden="true" />
          </SubmitButton>
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
  defaultScope,
}: {
  configuration: GatewayConfiguration;
  run: RunMutation;
  busy: boolean;
  onPrepared: (review: OnboardingReview) => void;
  initialModelID?: string;
  defaultScope?: string;
}) {
  const models = configuration.models.filter(
    ({ model, enabled, draft }) =>
      (enabled || draft) && model.protocol !== "local",
  );
  const [selected, setSelected] = useState(initialModelID ?? "");
  const model =
    models.find(({ model }) => model.id === selected)?.model ??
    models[0]?.model;

  if (!model)
    return (
      <p className="text-sm text-muted-foreground">
        请先保存模型预设与销售价。
      </p>
    );
  return (
    <PreparedRouteForm
      key={model.id}
      model={model}
      models={models}
      chooseModel={setSelected}
      configuration={configuration}
      run={run}
      busy={busy}
      onPrepared={onPrepared}
      defaultScope={defaultScope}
    />
  );
}

function PreparedRouteForm({
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
  models: GatewayConfiguration["models"];
  chooseModel: (id: string) => void;
  configuration: GatewayConfiguration;
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
      option.parameters[model.capability] !== undefined,
  );
  const [driverKey, setDriverKey] = useState("");
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
      .filter(({ route }) => route.protocol !== "local")
      .map(({ route }) => [route.supplier, route]),
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
            purchase: readPrice(data),
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
        <SelectField
          label="销售模型版本"
          value={model.id}
          onChange={(event) => chooseModel(event.currentTarget.value)}
          options={models.map(({ model, draft }) => [
            model.id,
            `${model.name} · ${draft ? "草稿" : "已启用"} · ${model.id.slice(0, 8)}`,
          ])}
        />
        <FieldGroup className="grid sm:grid-cols-2">
          <SelectField
            label="供应商账号"
            value={supplier}
            onChange={(event) => {
              setSupplier(event.currentTarget.value);
              setGroupOverride(undefined);
            }}
            options={[
              ["", "新建供应商账号"],
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
            value={embedding ? arkEmbedding.model : undefined}
            readOnly={embedding}
            placeholder="供应商返回的精确型号"
            maxLength={160}
          />
        </FieldGroup>
        {media && (
          <>
            <SelectField
              label="媒体驱动"
              value={driverKey}
              onChange={(event) => setDriverKey(event.currentTarget.value)}
              options={[
                ["", "选择已安装的驱动"],
                ...drivers.map((option) => [option.key, option.name] as const),
              ]}
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
              readOnly={embedding || Boolean(driver && !driver.custom_url)}
              value={
                embedding
                  ? arkEmbedding.baseURL
                  : (driver?.base_url ?? draftRoute?.base_url)
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
        <FormDetails label="共享配额与超时">
          <FieldGroup
            key={quota?.name ?? supplier}
            className="grid sm:grid-cols-2"
          >
            <InputField
              label="共享并发"
              name="concurrency"
              type="number"
              min={1}
              max={1000}
              value={quota?.concurrency ?? 1}
            />
            <InputField
              label="每分钟请求上限"
              name="rpm"
              type="number"
              min={1}
              max={media ? 100000 : Number.MAX_SAFE_INTEGER}
              value={quota?.rpm ?? 60}
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
        </FormDetails>
        <Separator />
        <PriceFields
          kind="采购"
          embedding={embedding}
          media={media}
          videoTokens={Boolean(model.video_metering)}
        />
        {media && (
          <>
            <Separator />
            <MediaProbeFields model={model} />
          </>
        )}
        <div>
          <SubmitButton
            busy={busy}
            disabled={noVerificationSlot || (media && !driver)}
          >
            保存并查看验证预算
            <ArrowRight data-icon="inline-end" aria-hidden="true" />
          </SubmitButton>
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
            description="填入可持久核对的原始业务编号，重复提交相同凭据不得重复入账。"
          />
          <InputField
            label="增加积分"
            name="credits"
            type="number"
            min={1}
            max={1000000000}
          />
        </FieldGroup>
        {preview ? (
          <p role="status" className="break-all">
            确认给空间 {preview.scope} 增加 {preview.credits} 积分，业务凭据：
            {preview.reference}。
          </p>
        ) : null}
        <SubmitButton busy={busy}>
          {preview ? "确认增加积分" : "核对入账信息"}
        </SubmitButton>
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
  const [billingResult, setBillingResult] = useState("");
  const [selected, setSelected] = useState("");
  const pending = unknown.find((record) => record.attempt_id === selected);
  return (
    <form
      aria-label="凭供应商证据对账"
      onSubmit={(event) => {
        event.preventDefault();
        if (!billingResult) return;
        const data = new FormData(event.currentTarget);
        const notCharged = billingResult === "not_charged";
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
        {billingResult === "charged" ? (
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
        <SubmitButton busy={busy}>凭证据结算原请求</SubmitButton>
      </FieldGroup>
    </form>
  );
}
