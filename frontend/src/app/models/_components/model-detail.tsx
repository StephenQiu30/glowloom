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
import { useEffect, useState, useId, useRef, type FormEvent } from "react";
import {
  ModelSetup,
  PriceSetup,
  modelConfiguration,
  priceRule,
} from "./model-setup";
import { useProviderCatalogProbe } from "@/components/catalog/provider-catalog-probe";
import { testErrorMessage } from "@/app/providers/_components/testing-status";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Spinner } from "@/components/ui/spinner";
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
  const [markupMultiplier, setMarkupMultiplier] = useState("1");
  const [selectedPriceCandidate, setSelectedPriceCandidate] =
    useState<API.ModelPriceCandidate | null>(null);
  const priceStart = useRef<{ request: string; effectiveFrom: string } | null>(
    null,
  );
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
          const activeSale = d.prices?.find(
            (price) =>
              typeof price.rule?.procurement_price_version_id === "string",
          );
          setMarkupMultiplier(
            String(activeSale?.rule?.markup_multiplier ?? "1"),
          );
          setSelectedPriceCandidate(null);
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
  const connection = providers.find(
    (item) => item.provider?.id === model?.provider_id,
  );
  const providerStatus = connection?.provider?.status;
  const credentialID =
    connection?.credential?.status === "active"
      ? connection.credential.id
      : undefined;
  const probe = useProviderCatalogProbe(
    model?.provider_id ?? "",
    `model.release.check.${id}`,
  );
  const latest = detail?.versions?.[0];
  const latestPrice = detail?.prices?.[0];
  const procurement = detail?.prices?.find(isProcurementPrice);
  const sale = detail?.prices?.find(
    (price) => typeof price.rule?.procurement_price_version_id === "string",
  );
  const pricePreset = detail?.price_preset;
  const editablePrice = procurement ?? (pricePreset ? undefined : latestPrice);
  const referenceOnly =
    !!pricePreset &&
    !pricePreset.unit &&
    !procurement &&
    !selectedPriceCandidate;
  const currency =
    priceCurrency ??
    editablePrice?.currency ??
    selectedPriceCandidate?.currency ??
    pricePreset?.currency ??
    "CNY";
  const unit =
    priceUnit ??
    editablePrice?.unit ??
    selectedPriceCandidate?.unit ??
    pricePreset?.unit ??
    "";
  const officialRule = selectedPriceCandidate
    ? selectedPriceCandidate.rule
    : editablePrice?.rule
      ? Object.fromEntries(
          Object.entries(editablePrice.rule).filter(
            ([key]) =>
              ![
                "procurement_price_version_id",
                "markup_multiplier",
                "credit_ratio",
              ].includes(key),
          ),
        )
      : pricePreset?.unit === unit && pricePreset.currency === currency
        ? pricePreset?.rule
        : undefined;
  const active = releases.find((r) => r.status === "active");
  const currentDefault = defaults.find(
    (d) => d.capability === model?.capability,
  );
  const versionID = model?.current_version_id ?? latest?.id;
  const priceID =
    sale?.id ??
    (!detail?.candidate?.model && !pricingReview ? latestPrice?.id : undefined);
  const configID = config?.id;
  const check = probe.data;
  const checkID =
    check?.current &&
    check.status === "completed" &&
    check.kind === "credential_auth" &&
    check.auth_result === "ok" &&
    check.adapter_result === "ok" &&
    check.config_version_id === configID &&
    check.credential_id === credentialID
      ? check.id
      : undefined;
  const matchesSavedSettings = (release: API.ModelRelease) =>
    release.model_version_id === versionID &&
    release.price_version_id === priceID &&
    release.config_version_id === configID;
  const published = model?.status === "active" && providerStatus === "active";
  const upToDate = published && !!active && matchesSavedSettings(active);
  const checking = probe.running || probe.mutation.busy;
  let checkMessage = "发布前请验证 Key 和服务可用性。";
  if (providerStatus === "archived") {
    checkMessage = "请先恢复服务连接。";
  } else if (!credentialID) {
    checkMessage = "请先在 AI 服务中保存 Key。";
  } else if (checking) {
    checkMessage = "正在验证 Key 和服务…";
  } else if (probe.error) {
    checkMessage = catalogMessage(probe.error);
  } else if (checkID) {
    checkMessage = "Key 有效，服务可访问。";
  } else if (check && !check.current) {
    checkMessage = "连接配置或 Key 已变更，请重新验证。";
  } else if (check?.auth_result === "unsupported") {
    checkMessage = "该服务暂不支持只读鉴权。";
  } else if (check) {
    checkMessage = testErrorMessage(
      check.error_code ?? check.auth_result ?? "",
    );
  }

  async function publish() {
    if (!versionID || !priceID || !configID || !checkID) return;
    const existing =
      active && matchesSavedSettings(active)
        ? active
        : releases.find(
            (release) =>
              release.status === "draft" && matchesSavedSettings(release),
          );
    const release =
      existing ??
      (await mutation.run("release.create", id, (request) =>
        createModelRelease(
          { id },
          {
            request_id: request,
            model_version_id: versionID,
            price_version_id: priceID,
            config_version_id: configID,
          },
        ),
      ));
    if (!release?.id) return;
    const releaseID = release.id;
    const keepDefault =
      !!active?.id && currentDefault?.release_id === active.id;
    await mutation.run("release.activate", releaseID, (request) =>
      activateModelRelease(
        { id: releaseID },
        {
          request_id: request,
          credential_check_id: checkID,
          expected_revision: release.revision,
          expected_model_revision: model?.revision,
          expected_active_release_id: active?.id,
          enable_provider: providerStatus === "disabled",
          set_default: keepDefault && active?.id !== releaseID,
          expected_default_revision: currentDefault?.revision ?? 0,
        },
      ),
    );
  }

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
  async function savePrice(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    try {
      const rule = priceRule(f);
      await mutation.run("model.price", id, (request) => {
        if (priceStart.current?.request !== request) {
          priceStart.current = {
            request,
            effectiveFrom: new Date().toISOString(),
          };
        }
        return publishAdminModelPrice(
          { id },
          {
            use_official_rate: true,
            expected_revision: model?.revision,
            version_no: (latestPrice?.version_no ?? 0) + 1,
            unit: String(f.get("unit")) as API.PriceUnit,
            source_url: String(f.get("source_url")),
            reviewed_at: priceStart.current.effectiveFrom,
            candidate_id: selectedPriceCandidate?.id,
            rule,
            markup_multiplier: String(f.get("markup_multiplier") ?? "1"),
            currency,
            fx_rate_to_cny:
              currency === "CNY" ? undefined : String(f.get("fx_rate") ?? ""),
            effective_from: priceStart.current.effectiveFrom,
          },
          { headers: { "Idempotency-Key": request } },
        );
      });
    } catch (e) {
      setError(catalogMessage(e));
    }
  }
  const priceReference = pricePreset ? (
    <Alert>
      <AlertTitle>官方计价预设</AlertTitle>
      <AlertDescription>
        <ul className="grid gap-1">
          {pricePreset.reference_lines?.map((line) => (
            <li key={line}>{line}</li>
          ))}
        </ul>
        <a href={pricePreset.source_url} target="_blank" rel="noreferrer">
          查看官方价目
        </a>
        {referenceOnly ? (
          <p>该模型的计量规则尚未接入，暂不能保存价格。</p>
        ) : null}
      </AlertDescription>
    </Alert>
  ) : null;
  const priceCandidates = detail?.price_candidates?.length ? (
    <section className="grid gap-3" aria-label="官方价目同步候选">
      <h3 className="text-sm font-medium">官方价目同步候选</h3>
      {detail.price_candidates.map((candidate, index) => {
        const canLoad =
          candidate.status === "needs_review" ||
          (candidate.status === "pending_review" &&
            !!candidate.unit &&
            !!candidate.rule);
        const reviewLabel =
          candidate.status === "pending_review"
            ? "待人工核验"
            : candidate.status === "published"
              ? "已发布"
              : candidate.status === "rejected"
                ? "已拒绝"
                : "需人工核价";
        return (
          <Alert
            key={
              candidate.id ??
              candidate.source_hash ??
              `${candidate.provider_model_id ?? "candidate"}-${index}`
            }
          >
            <AlertTitle className="flex flex-wrap items-center gap-2">
              <Badge
                variant={
                  candidate.status === "pending_review"
                    ? "secondary"
                    : "outline"
                }
              >
                {reviewLabel}
              </Badge>
              <span>
                {candidate.currency ?? "币种未知"}
                {candidate.unit ? ` · ${candidate.unit}` : ""}
              </span>
              <time
                dateTime={candidate.source_fetched_at ?? undefined}
                className="text-xs font-normal text-muted-foreground"
              >
                {candidate.source_fetched_at
                  ? `同步于 ${new Date(candidate.source_fetched_at).toLocaleString()}`
                  : "同步时间未知"}
              </time>
            </AlertTitle>
            <AlertDescription className="grid gap-2">
              <a href={candidate.source_url} target="_blank" rel="noreferrer">
                查看公开来源
              </a>
              {candidate.rule ? (
                <pre className="overflow-x-auto rounded-md bg-muted p-2 text-xs">
                  {JSON.stringify(candidate.rule, null, 2)}
                </pre>
              ) : (
                <p>
                  来源未提供当前可执行的完整计费规则；核对官方条款后再手动录入。
                </p>
              )}
              <Collapsible>
                <CollapsibleTrigger asChild>
                  <Button
                    type="button"
                    variant="ghost"
                    className="group/candidate-collapsible justify-between"
                  >
                    查看来源计价字段
                    <ChevronDown className="transition-transform group-data-[state=open]/candidate-collapsible:rotate-180" />
                  </Button>
                </CollapsibleTrigger>
                <CollapsibleContent className="pt-2">
                  <pre className="overflow-x-auto rounded-md bg-muted p-2 text-xs">
                    {JSON.stringify(candidate.source_pricing, null, 2)}
                  </pre>
                </CollapsibleContent>
              </Collapsible>
              {canLoad ? (
                <div className="flex flex-wrap gap-2">
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    aria-pressed={selectedPriceCandidate?.id === candidate.id}
                    onClick={() => {
                      setSelectedPriceCandidate(candidate);
                      setPriceUnit(candidate.unit ?? null);
                      setPriceCurrency(candidate.currency ?? null);
                    }}
                  >
                    {selectedPriceCandidate?.id === candidate.id
                      ? "已载入，仍需核验并保存"
                      : candidate.status === "needs_review"
                        ? "载入来源并人工录价"
                        : "载入候选并人工核验"}
                  </Button>
                  {selectedPriceCandidate?.id === candidate.id ? (
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      onClick={() => setSelectedPriceCandidate(null)}
                    >
                      改为手动录入
                    </Button>
                  ) : null}
                </div>
              ) : null}
            </AlertDescription>
          </Alert>
        );
      })}
    </section>
  ) : null;
  const priceForm =
    detail && !referenceOnly ? (
      <form
        key={selectedPriceCandidate?.id ?? editablePrice?.id ?? "official"}
        onSubmit={savePrice}
      >
        <FieldGroup className="gap-3">
          <div>
            <Badge variant="secondary">
              {priceID
                ? active?.price_version_id === priceID
                  ? "已发布"
                  : "待发布"
                : procurement
                  ? "待保存同价"
                  : "待核验"}
            </Badge>
          </div>
          {!pricePreset && !editablePrice ? (
            <p className="text-sm text-muted-foreground">
              按供应商价目填写计费单位、费率和来源。
            </p>
          ) : null}
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
                  {Object.entries(pricingUnits).map(([key, label]) => (
                    <SelectItem key={key} value={key}>
                      {label}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </Field>
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
          <Field>
            <FieldLabel htmlFor={`${formFieldId}-markup`}>
              平台销售倍率
            </FieldLabel>
            <Input
              id={`${formFieldId}-markup`}
              name="markup_multiplier"
              type="number"
              min="0.000001"
              max="1000000"
              step="0.000001"
              required
              value={markupMultiplier}
              onChange={(event) => setMarkupMultiplier(event.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              1 表示与供应商公开价一致；倍率仅影响新销售价格版本。
            </p>
          </Field>
          {unit ? (
            <PriceSetup
              key={`${unit}:${currency}`}
              unit={unit}
              rule={officialRule}
              currency={currency}
            />
          ) : null}
          <PriceEvidenceFields
            sourceURL={
              selectedPriceCandidate?.source_url ??
              editablePrice?.source_url ??
              pricePreset?.source_url
            }
          />
          <Button disabled={archived || mutation.busy || !latest || !unit}>
            核验并保存
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
              {priceCandidates}
              {priceReference}
              {priceForm}
            </TabsContent>
            <TabsContent
              value="release"
              forceMount
              hidden={view !== "release"}
              className="flex flex-col gap-5 data-[state=inactive]:hidden"
            >
              <div className="flex items-center gap-2">
                <Badge variant="secondary">
                  {published ? "已发布" : active ? "已停用" : "未发布"}
                </Badge>
                {published && !upToDate ? (
                  <span className="text-sm text-muted-foreground">
                    有待发布的配置
                  </span>
                ) : null}
              </div>
              {!upToDate ? (
                <>
                  {!checking &&
                  (probe.mutation.pending ||
                    (!check && probe.mutation.message)) ? (
                    <MutationStatus mutation={probe.mutation} />
                  ) : null}
                  <div role="status" className="text-sm text-muted-foreground">
                    {checkMessage}
                  </div>
                  {!versionID || !priceID ? (
                    <p className="text-sm text-muted-foreground">
                      {!versionID
                        ? "请先在「模型设置」保存配置。"
                        : "请先在「价格」保存可发布的价格。"}
                    </p>
                  ) : null}
                  <div className="flex flex-wrap gap-2">
                    <Button
                      variant={checkID ? "outline" : "default"}
                      disabled={
                        checking ||
                        mutation.busy ||
                        !credentialID ||
                        !configID ||
                        providerStatus === "archived"
                      }
                      onClick={() => {
                        if (credentialID) void probe.run(credentialID);
                      }}
                    >
                      {checking ? <Spinner data-icon="inline-start" /> : null}
                      {checkID ? "重新验证" : "验证 Key 和服务"}
                    </Button>
                    <Button
                      variant={checkID ? "default" : "outline"}
                      disabled={
                        !checkID ||
                        checking ||
                        mutation.busy ||
                        !priceID ||
                        !versionID ||
                        providerStatus === "archived"
                      }
                      onClick={() => void publish()}
                    >
                      发布模型
                    </Button>
                  </div>
                </>
              ) : null}
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
