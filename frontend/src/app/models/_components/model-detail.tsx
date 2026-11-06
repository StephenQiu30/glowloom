"use client";
import { PriceEvidenceFields } from "./price-evidence-fields";
import { isProcurementPrice, pricingUnits } from "./preset-pricing-review";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { LoadingState } from "@/components/layout/loading-state";
import { ContentState } from "@/components/layout/content-state";
import { creationModeLabel } from "@/components/generation/creation-contract";
import { ChevronDown } from "lucide-react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  AlertDialog,
  AlertDialogTrigger,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
  AlertDialogAction,
} from "@/components/ui/alert-dialog";
import { Checkbox } from "@/components/ui/checkbox";
import {
  FieldSet,
  FieldLegend,
  Field,
  FieldLabel,
  FieldGroup,
} from "@/components/ui/field";
import { useEffect, useState, useId } from "react";
import {
  ModelSetup,
  PriceSetup,
  modelConfiguration,
  priceRule,
} from "./model-setup";
import { ProviderTrial } from "@/components/catalog/provider-trial";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  getAdminModel,
  getProviderConfig,
  listModelReleases,
  listCapabilityDefaults,
  publishAdminModelVersion,
  publishAdminModelPrice,
  createModelRelease,
  activateModelRelease,
  setAdminModelStatus,
} from "@/api/settings";
import {
  catalogMessage,
  MutationStatus,
  useCatalogMutation,
} from "@/components/catalog/mutation";

export type ModelDetailView = "configuration" | "pricing" | "release";

export function ModelDetail({
  id,
  capabilities,
  providers = [],
  initialView = "configuration",
  pricingReview = false,
  onSaved,
}: {
  id: string;
  capabilities: API.CapabilitySummary[];
  providers?: API.ProviderListItem[];
  initialView?: ModelDetailView;
  pricingReview?: boolean;
  onSaved: () => void;
}) {
  const formFieldId = useId();

  const [view, setView] = useState<string>(initialView);
  const [priceUnit, setPriceUnit] = useState<string | null>(null);
  const [priceCurrency, setPriceCurrency] = useState<string | null>(null);
  const [pricePurpose, setPricePurpose] = useState<"procurement" | "sale">();
  const [advancedOpen, setAdvancedOpen] = useState<boolean | null>(null);
  const [detail, setDetail] = useState<API.AdminModelDetail | null>(null);
  const [config, setConfig] = useState<API.ConnectionConfig | null>(null);
  const [releases, setReleases] = useState<API.ModelRelease[]>([]);
  const [defaults, setDefaults] = useState<API.CapabilityDefault[]>([]);
  const [error, setError] = useState("");
  const [readAttempt, setReadAttempt] = useState(0);
  function reloadDetail() {
    onSaved();
    setReadAttempt((attempt) => attempt + 1);
  }
  const mutation = useCatalogMutation(reloadDetail, `model.${id}`);
  useEffect(() => {
    const abort = new AbortController();
    const model = getAdminModel({ id }, { signal: abort.signal });
    Promise.all([
      model,
      listModelReleases({ id }, { signal: abort.signal }),
      listCapabilityDefaults({ signal: abort.signal }),
    ])
      .then(([d, r, defaults]) => {
        if (!abort.signal.aborted) {
          setDetail(d);
          setReleases(r.items ?? []);
          setDefaults(defaults.items ?? []);
        }
      })
      .catch((e) => {
        if (!abort.signal.aborted) setError(catalogMessage(e));
      });
    model
      .then((d) =>
        d.model?.provider_id
          ? getProviderConfig(
              { id: d.model.provider_id },
              { signal: abort.signal },
            )
          : null,
      )
      .then((c) => {
        if (!abort.signal.aborted) setConfig(c);
      })
      .catch((e) => {
        if (!abort.signal.aborted) setError(catalogMessage(e));
      });
    return () => abort.abort();
  }, [id, readAttempt]);
  const model = detail?.model;
  const archived = model?.status === "archived";
  const providerStatus = providers.find(
    (item) => item.provider?.id === model?.provider_id,
  )?.provider?.status;
  const latest = detail?.versions?.[0];
  const latestPrice = detail?.prices?.[0];
  const procurementPrices = detail?.prices?.filter(isProcurementPrice) ?? [];
  const procurement = procurementPrices[0];
  const sale = detail?.prices?.find(
    (price) => typeof price.rule?.procurement_price_version_id === "string",
  );
  const hasSeparatePrices =
    pricingReview || !!detail?.candidate?.model || !!sale;
  const purpose = pricePurpose ?? (procurement ? "sale" : "procurement");
  const editablePrice = hasSeparatePrices
    ? purpose === "procurement"
      ? procurement
      : sale
    : latestPrice;
  const currency = hasSeparatePrices
    ? (priceCurrency ?? editablePrice?.currency ?? "CNY")
    : "CNY";
  const unit =
    priceUnit ??
    editablePrice?.unit ??
    (hasSeparatePrices ? (procurement?.unit ?? "") : "per_request");
  function choosePricePurpose(value: string) {
    if (value !== "procurement" && value !== "sale") return;
    setPricePurpose(value);
    setPriceCurrency(null);
    setPriceUnit(null);
  }
  const active = releases.find((r) => r.status === "active");
  const currentDefault = defaults.find(
    (d) => d.capability === model?.capability,
  );
  const modes = (
    capabilities.find((c) => c.key === model?.capability)?.modes ?? []
  ).filter((mode) =>
    [
      "text_to_text",
      "text2image",
      "image2image",
      "text2video",
      "image2video",
      "omni_reference",
    ].includes(mode),
  );
  const priceForm = detail ? (
    <form
      key={hasSeparatePrices ? purpose : "price"}
      onSubmit={async (e) => {
        e.preventDefault();
        const f = new FormData(e.currentTarget);
        try {
          const rule = priceRule(f);
          if (
            hasSeparatePrices &&
            purpose === "procurement" &&
            (rule.markup_multiplier !== undefined ||
              rule.procurement_price_version_id !== undefined)
          )
            throw new Error("采购价不能关联其他价目或包含销售加价。");
          if (
            hasSeparatePrices &&
            purpose === "sale" &&
            !procurementPrices.some(
              (price) =>
                price.id === rule.procurement_price_version_id &&
                price.unit === unit,
            )
          )
            throw new Error("请选择同一计量方式的已审核采购价。");
          await mutation.run("model.price", id, (request) =>
            publishAdminModelPrice(
              { id },
              {
                expected_revision: model?.revision,
                version_no: (latestPrice?.version_no ?? 0) + 1,
                unit: String(f.get("unit")) as API.PriceUnit,
                source_url: String(f.get("source_url")),
                reviewed_at: new Date(
                  String(f.get("reviewed_at")),
                ).toISOString(),
                rule,
                currency,
                fx_rate_to_cny:
                  currency === "CNY"
                    ? undefined
                    : String(f.get("fx_rate") ?? ""),
                effective_from: new Date(
                  String(f.get("effective")),
                ).toISOString(),
              },
              { headers: { "Idempotency-Key": request } },
            ),
          );
        } catch (e) {
          setError(catalogMessage(e));
        }
      }}
    >
      <FieldGroup className="gap-3">
        {hasSeparatePrices && (
          <p className="text-sm text-muted-foreground">
            {editablePrice
              ? `v${editablePrice.version_no} · ${purpose === "procurement" ? "已核验" : active?.price_version_id === editablePrice.id ? "已发布" : "已保存"}`
              : "待补价"}
          </p>
        )}
        <Field>
          <FieldLabel htmlFor={`${formFieldId}-field-4`}>
            <span>计费方式</span>
          </FieldLabel>
          <Select
            name="unit"
            required
            value={unit}
            onValueChange={setPriceUnit}
          >
            <SelectTrigger
              id={`${formFieldId}-field-4`}
              className="h-10 w-full"
            >
              <SelectValue placeholder="选择计费方式" />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {Object.entries(pricingUnits)
                  .filter(
                    ([key]) =>
                      !hasSeparatePrices ||
                      purpose !== "sale" ||
                      procurementPrices.some((price) => price.unit === key),
                  )
                  .map(([key, label]) => (
                    <SelectItem key={key} value={key}>
                      {label}
                    </SelectItem>
                  ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
        {hasSeparatePrices && (
          <FieldGroup className="grid gap-3 sm:grid-cols-2">
            <Field>
              <FieldLabel htmlFor={`${formFieldId}-currency`}>币种</FieldLabel>
              <Select value={currency} onValueChange={setPriceCurrency}>
                <SelectTrigger id={`${formFieldId}-currency`}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem value="CNY">CNY</SelectItem>
                    <SelectItem value="USD">USD</SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
            {currency !== "CNY" && (
              <Field>
                <FieldLabel htmlFor={`${formFieldId}-fx`}>
                  兑人民币汇率
                </FieldLabel>
                <Input
                  id={`${formFieldId}-fx`}
                  name="fx_rate"
                  required
                  type="number"
                  min="0.000001"
                  step="0.000001"
                  defaultValue={editablePrice?.fx_rate_to_cny ?? undefined}
                />
              </Field>
            )}
          </FieldGroup>
        )}
        <PriceSetup
          key={editablePrice?.id ?? "new"}
          unit={unit}
          rule={editablePrice?.rule}
          prices={detail?.prices}
          currency={hasSeparatePrices ? currency : undefined}
          procurementOnly={hasSeparatePrices && purpose === "procurement"}
          requireProcurement={hasSeparatePrices && purpose === "sale"}
        />
        <PriceEvidenceFields price={editablePrice} />
        <Field>
          <FieldLabel htmlFor={`${formFieldId}-field-5`}>
            <span>生效时间</span>
          </FieldLabel>
          <Input
            id={`${formFieldId}-field-5`}
            type="datetime-local"
            name="effective"
            required
          />
        </Field>
        <Button disabled={archived || mutation.busy || !latest || !unit}>
          {hasSeparatePrices
            ? purpose === "procurement"
              ? "保存采购价"
              : "保存销售价"
            : "保存价格"}
        </Button>
      </FieldGroup>
    </form>
  ) : null;
  return (
    <section className="flex min-w-0 flex-col gap-3" aria-label="模型详情">
      <h2 className="text-lg font-medium">
        {model?.display_name ?? "模型详情"}
      </h2>
      {error && detail ? <FeedbackToast message={error} /> : null}
      <MutationStatus mutation={mutation} />
      {!detail && error ? (
        <ContentState
          state="error"
          title="模型配置加载失败"
          description={error}
          actions={
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setError("");
                setReadAttempt((attempt) => attempt + 1);
              }}
            >
              重试加载
            </Button>
          }
        />
      ) : !detail ? (
        <LoadingState>正在加载配置…</LoadingState>
      ) : (
        <>
          {archived && model ? (
            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button
                  variant="outline"
                  className="self-start"
                  disabled={mutation.busy || !!mutation.pending}
                >
                  恢复模型
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>恢复模型？</AlertDialogTitle>
                  <AlertDialogDescription>
                    模型将恢复为未启用，发布后才能接收新任务。
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>取消</AlertDialogCancel>
                  <AlertDialogAction
                    onClick={() =>
                      void mutation.run("model.status", id, (requestID) =>
                        setAdminModelStatus(
                          { id },
                          {
                            status: "disabled",
                            expected_revision: model.revision,
                          },
                          { headers: { "Idempotency-Key": requestID } },
                        ),
                      )
                    }
                  >
                    确认恢复
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          ) : null}
          <Tabs
            hidden={archived}
            value={archived ? "archived" : view}
            onValueChange={setView}
            className={archived ? "hidden" : "gap-4"}
          >
            <TabsList
              aria-label="模型详情视图"
              className="max-w-full max-md:min-h-11"
            >
              <TabsTrigger value="configuration">模型设置</TabsTrigger>
              <TabsTrigger value="pricing">价格</TabsTrigger>
              <TabsTrigger value="release">发布</TabsTrigger>
            </TabsList>
            <TabsContent
              value="configuration"
              forceMount
              hidden={view !== "configuration"}
            >
              <form
                onInvalidCapture={(event) => {
                  const target = event.target;
                  if (
                    (target instanceof HTMLInputElement ||
                      target instanceof HTMLTextAreaElement) &&
                    target.closest("[data-model-advanced]") &&
                    !(advancedOpen ?? !latest?.limits)
                  ) {
                    event.preventDefault();
                    setAdvancedOpen(true);
                    requestAnimationFrame(() => {
                      target.focus();
                      target.reportValidity();
                    });
                  }
                }}
                onSubmit={async (e) => {
                  e.preventDefault();
                  const f = new FormData(e.currentTarget);
                  try {
                    const configuration = modelConfiguration(
                      f,
                      model?.capability ?? "",
                      config?.config?.adapter_key,
                    );
                    await mutation.run("model.version", id, (request) =>
                      publishAdminModelVersion(
                        { id },
                        {
                          expected_revision: model?.revision,
                          version_no: (latest?.version_no ?? 0) + 1,
                          provider_model_id: String(f.get("upstream")),
                          modes: f.getAll("modes").map(String),
                          ...configuration,
                          expected_max_ms: Number(f.get("timeout")) * 1000,
                          moderation: "provider",
                          queue: "provider",
                          supports_query:
                            model?.capability === "video.generate",
                          supports_cancel: false,
                          supports_callback: false,
                        },
                        { headers: { "Idempotency-Key": request } },
                      ),
                    );
                  } catch (e) {
                    setError(catalogMessage(e));
                  }
                }}
              >
                <FieldGroup className="gap-3">
                  <Field>
                    <FieldLabel htmlFor={`${formFieldId}-field-1`}>
                      <span>上游模型 ID</span>
                    </FieldLabel>
                    <Input
                      id={`${formFieldId}-field-1`}
                      required
                      name="upstream"
                      maxLength={256}
                      defaultValue={
                        latest?.provider_model_id ??
                        detail.candidate?.model?.provider_model_id
                      }
                      placeholder="模型或推理接入点 ID"
                    />
                  </Field>
                  <FieldSet>
                    <FieldLegend>开放模式</FieldLegend>
                    <FieldGroup>
                      <div className="mt-2 flex flex-wrap gap-3">
                        {modes.map((mode, fieldIndex0) => (
                          <Field key={mode} orientation="horizontal">
                            <Checkbox
                              id={`${formFieldId}-field-2-${fieldIndex0}`}

                              name="modes"
                              value={mode}
                              defaultChecked={(
                                latest?.modes ?? detail.candidate?.model?.modes
                              )?.includes(mode)}
                            />
                            <FieldLabel
                              htmlFor={`${formFieldId}-field-2-${fieldIndex0}`}
                            >
                              <span>
                                {mode === "text_to_text"
                                  ? "文本生成"
                                  : creationModeLabel(mode)}
                              </span>
                            </FieldLabel>
                          </Field>
                        ))}
                      </div>
                    </FieldGroup>
                  </FieldSet>
                  <Field>
                    <FieldLabel htmlFor={`${formFieldId}-field-3`}>
                      <span>最长等待时间（秒）</span>
                    </FieldLabel>
                    <Input
                      id={`${formFieldId}-field-3`}
                      type="number"
                      name="timeout"
                      min={1}
                      max={86400}
                      required
                      defaultValue={(latest?.expected_max_ms ?? 600000) / 1000}
                    />
                  </Field>
                  <Collapsible
                    data-model-advanced
                    open={advancedOpen ?? !latest?.limits}
                    onOpenChange={setAdvancedOpen}
                  >
                    <CollapsibleTrigger asChild>
                      <Button
                        type="button"
                        variant="ghost"
                        className="group/collapsible justify-between"
                      >
                        高级参数
                        <ChevronDown
                          data-icon="inline-end"
                          className="transition-transform group-data-[state=open]:rotate-180"
                        />
                      </Button>
                    </CollapsibleTrigger>
                    <CollapsibleContent
                      forceMount
                      className="pt-3 data-[state=closed]:hidden"
                    >
                      <ModelSetup
                        key={latest?.id ?? "new"}
                        adapter={config?.config?.adapter_key}
                        capability={model?.capability ?? ""}
                        limits={latest?.limits}
                        parameters={latest?.param_schema}
                      />
                    </CollapsibleContent>
                  </Collapsible>
                  <Button disabled={mutation.busy}>保存配置</Button>
                </FieldGroup>
              </form>
            </TabsContent>
            <TabsContent value="pricing" forceMount hidden={view !== "pricing"}>
              {hasSeparatePrices ? (
                <Tabs
                  value={purpose}
                  onValueChange={choosePricePurpose}
                  className="gap-3"
                >
                  <TabsList
                    variant="line"
                    aria-label="价目类型"
                    className="max-md:min-h-11"
                  >
                    <TabsTrigger value="procurement">采购价</TabsTrigger>
                    <TabsTrigger value="sale">销售价</TabsTrigger>
                  </TabsList>
                  <TabsContent value="procurement">
                    {purpose === "procurement" && priceForm}
                  </TabsContent>
                  <TabsContent value="sale">
                    {purpose === "sale" &&
                      (procurement ? (
                        priceForm
                      ) : (
                        <p className="text-sm text-muted-foreground">
                          请先核验采购价
                        </p>
                      ))}
                  </TabsContent>
                </Tabs>
              ) : (
                priceForm
              )}
            </TabsContent>
            <TabsContent
              value="release"
              forceMount
              hidden={view !== "release"}
              className="flex flex-col gap-5 data-[state=inactive]:hidden"
            >
              <Collapsible className="flex flex-col gap-3">
                <CollapsibleTrigger asChild>
                  <Button
                    type="button"
                    variant="ghost"
                    className="group/collapsible h-auto w-full justify-between whitespace-normal"
                  >
                    <span>创建新发布</span>
                    <ChevronDown
                      data-icon="inline-end"
                      className="transition-transform group-data-[state=open]/collapsible:rotate-180"
                    />
                  </Button>
                </CollapsibleTrigger>
                <CollapsibleContent
                  forceMount
                  className="pt-3 data-[state=closed]:hidden"
                >
                  <form
                    onSubmit={async (e) => {
                      e.preventDefault();
                      const f = new FormData(e.currentTarget);
                      await mutation.run("release.create", id, (request) =>
                        createModelRelease(
                          { id },
                          {
                            request_id: request,
                            model_version_id: String(f.get("version")),
                            price_version_id: String(f.get("price")),
                            config_version_id: config?.id,
                            upstream_cost: {
                              currency: String(f.get("currency")),
                              max_micros: Number(f.get("cost")),
                              source: String(f.get("source")),
                              reviewed_at: new Date(
                                String(f.get("reviewed")),
                              ).toISOString(),
                            },
                          },
                        ),
                      );
                    }}
                  >
                    <FieldGroup className="mt-4 gap-3">
                      <Field>
                        <FieldLabel htmlFor={`${formFieldId}-field-6`}>
                          <span>配置版本</span>
                        </FieldLabel>
                        <Select
                          name="version"
                          required
                          defaultValue={detail.versions?.[0]?.id}
                        >
                          <SelectTrigger
                            id={`${formFieldId}-field-6`}
                            className="h-10 w-full"
                          >
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectGroup>
                              {detail.versions?.map((v) =>
                                v.id ? (
                                  <SelectItem key={v.id} value={v.id}>
                                    v{v.version_no} · {v.provider_model_id}
                                  </SelectItem>
                                ) : null,
                              )}
                            </SelectGroup>
                          </SelectContent>
                        </Select>
                      </Field>
                      <Field>
                        <FieldLabel htmlFor={`${formFieldId}-field-7`}>
                          <span>价格版本</span>
                        </FieldLabel>
                        <Select
                          key={(hasSeparatePrices ? sale : latestPrice)?.id}
                          name="price"
                          required
                          defaultValue={
                            hasSeparatePrices ? sale?.id : latestPrice?.id
                          }
                        >
                          <SelectTrigger
                            id={`${formFieldId}-field-7`}
                            className="h-10 w-full"
                          >
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectGroup>
                              {detail.prices
                                ?.filter(
                                  (p) =>
                                    !hasSeparatePrices ||
                                    typeof p.rule
                                      ?.procurement_price_version_id ===
                                      "string",
                                )
                                .map((p) =>
                                  p.id ? (
                                    <SelectItem key={p.id} value={p.id}>
                                      v{p.version_no} ·{" "}
                                      {pricingUnits[p.unit ?? ""] ?? p.unit}
                                    </SelectItem>
                                  ) : null,
                                )}
                            </SelectGroup>
                          </SelectContent>
                        </Select>
                      </Field>
                      <p className="text-sm">
                        绑定连接配置：v{config?.version ?? "—"}
                      </p>
                      <div className="grid gap-3 sm:grid-cols-2">
                        <Field>
                          <FieldLabel htmlFor={`${formFieldId}-field-8`}>
                            <span>上游币种</span>
                          </FieldLabel>
                          <Select name="currency" defaultValue="CNY">
                            <SelectTrigger
                              id={`${formFieldId}-field-8`}
                              className="h-10 w-full"
                            >
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectGroup>
                                <SelectItem value="CNY">人民币</SelectItem>
                                <SelectItem value="USD">美元</SelectItem>
                              </SelectGroup>
                            </SelectContent>
                          </Select>
                        </Field>
                        <Field>
                          <FieldLabel htmlFor={`${formFieldId}-field-9`}>
                            <span>单次成本上限（微元）</span>
                          </FieldLabel>
                          <Input
                            id={`${formFieldId}-field-9`}
                            name="cost"
                            type="number"
                            min={0}
                            max={1000000000}
                            required
                            placeholder="填写已审核上限"
                          />
                        </Field>
                      </div>
                      <Field>
                        <FieldLabel htmlFor={`${formFieldId}-field-10`}>
                          <span>成本来源</span>
                        </FieldLabel>
                        <Input
                          id={`${formFieldId}-field-10`}
                          name="source"
                          required
                          maxLength={500}
                          placeholder="官方定价页面或合同说明"
                        />
                      </Field>
                      <Field>
                        <FieldLabel htmlFor={`${formFieldId}-field-11`}>
                          <span>成本审核时间</span>
                        </FieldLabel>
                        <Input
                          id={`${formFieldId}-field-11`}
                          name="reviewed"
                          type="datetime-local"
                          required
                        />
                      </Field>
                      <Button
                        disabled={
                          mutation.busy ||
                          !config ||
                          !(hasSeparatePrices ? sale : latestPrice)
                        }
                      >
                        创建待验证发布
                      </Button>
                    </FieldGroup>
                  </form>
                </CollapsibleContent>
              </Collapsible>
              <section className="flex flex-col gap-3" aria-label="发布历史">
                <h3 className="font-medium">发布记录</h3>
                {!releases.length ? (
                  <p className="text-sm text-muted-foreground">尚未发布</p>
                ) : null}
                {releases.map((r, fieldIndex1) => {
                  const onboardingDraft =
                    r.status === "draft" &&
                    !Object.keys(r.upstream_cost ?? {}).length;
                  const missing =
                    r.required_modes?.filter(
                      (mode) => !r.validated_modes?.includes(mode),
                    ) ?? [];
                  const canActivate =
                    providerStatus !== "archived" &&
                    (r.status === "draft" ||
                      r.status === "retired" ||
                      (r.status === "active" &&
                        (model?.status === "disabled" ||
                          providerStatus === "disabled")));
                  const ready =
                    canActivate &&
                    !!r.required_modes?.length &&
                    !missing.length;
                  return (
                    <article
                      key={r.id}
                      className="space-y-2 rounded-lg border p-3 text-sm"
                    >
                      <p className="font-medium">
                        {r.status === "active"
                          ? "当前发布"
                          : r.status === "retired"
                            ? "历史发布"
                            : "待验证发布"}
                        {currentDefault?.release_id === r.id
                          ? " · 平台默认"
                          : ""}
                      </p>
                      {r.validated_modes?.length ? (
                        <p>
                          已验证：
                          {r.validated_modes.map(creationModeLabel).join("、")}
                        </p>
                      ) : null}
                      {missing.length > 0 && (
                        <p>
                          待验证：{missing.map(creationModeLabel).join("、")}
                        </p>
                      )}
                      {r.onboarding_check_id || onboardingDraft ? (
                        <p className="text-muted-foreground">
                          历史接入记录需重新创建发布并逐模式验证。
                        </p>
                      ) : null}
                      {r.status === "draft" && !onboardingDraft ? (
                        <ProviderTrial
                          release={r}
                          version={detail.versions?.find(
                            (v) => v.id === r.model_version_id,
                          )}
                          onVerified={reloadDetail}
                        />
                      ) : null}
                      {canActivate && !onboardingDraft ? (
                        <form
                          onSubmit={async (e) => {
                            e.preventDefault();
                            const f = new FormData(e.currentTarget);
                            await mutation.run(
                              "release.activate",
                              r.id!,
                              (request) =>
                                activateModelRelease(
                                  { id: r.id! },
                                  {
                                    request_id: request,
                                    expected_revision: r.revision,
                                    expected_model_revision: model?.revision,
                                    expected_active_release_id: active?.id,
                                    enable_provider: f.has("enable"),
                                    set_default: f.has("default"),
                                    expected_default_revision:
                                      currentDefault?.revision ?? 0,
                                  },
                                ),
                            );
                          }}
                        >
                          <FieldGroup className="gap-2">
                            <Field orientation="horizontal">
                              <Checkbox
                                id={`${formFieldId}-field-12-${fieldIndex1}`}
                                name="enable"
                                defaultChecked={providerStatus === "disabled"}
                              />
                              <FieldLabel
                                htmlFor={`${formFieldId}-field-12-${fieldIndex1}`}
                              >
                                <span>同时启用平台连接</span>
                              </FieldLabel>
                            </Field>
                            <Field orientation="horizontal">
                              <Checkbox
                                id={`${formFieldId}-field-13-${fieldIndex1}`}
                                name="default"
                              />
                              <FieldLabel
                                htmlFor={`${formFieldId}-field-13-${fieldIndex1}`}
                              >
                                <span>同时设为此能力的平台默认</span>
                              </FieldLabel>
                            </Field>
                            <Button disabled={!ready || mutation.busy}>
                              {r.status === "active"
                                ? "重新启用此发布"
                                : r.status === "retired"
                                  ? "恢复此历史发布"
                                  : "激活已验证发布"}
                            </Button>
                          </FieldGroup>
                        </form>
                      ) : null}
                    </article>
                  );
                })}
              </section>
              {model?.status === "active" ? (
                <Button
                  variant="outline"
                  disabled={mutation.busy}
                  onClick={() =>
                    void mutation.run("model.status", id, (requestID) =>
                      setAdminModelStatus(
                        { id },
                        {
                          status: "disabled",
                          expected_revision: model.revision,
                        },
                        { headers: { "Idempotency-Key": requestID } },
                      ),
                    )
                  }
                >
                  停用模型（不接受新任务）
                </Button>
              ) : null}
            </TabsContent>
          </Tabs>
        </>
      )}
    </section>
  );
}
