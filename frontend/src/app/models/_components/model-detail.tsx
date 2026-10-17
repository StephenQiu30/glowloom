"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { LoadingState } from "@/components/layout/loading-state";
import { ChevronDown } from "lucide-react";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
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
import { ProviderTrial } from "./provider-trial";
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

export function ModelDetail({
  id,
  capabilities,
  onSaved,
}: {
  id: string;
  capabilities: API.CapabilitySummary[];
  onSaved: () => void;
}) {
  const formFieldId = useId();

  const [sectionOpen, setSectionOpen] = useState<
    Partial<Record<"configuration" | "pricing" | "release", boolean>>
  >({});
  const [detail, setDetail] = useState<API.AdminModelDetail | null>(null);
  const [config, setConfig] = useState<API.ConnectionConfig | null>(null);
  const [releases, setReleases] = useState<API.ModelRelease[]>([]);
  const [defaults, setDefaults] = useState<API.CapabilityDefault[]>([]);
  const [error, setError] = useState("");
  const mutation = useCatalogMutation(onSaved, `model.${id}`);
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
  }, [id]);
  const model = detail?.model;
  const latest = detail?.versions?.[0];
  const latestPrice = detail?.prices?.[0];
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
  return (
    <section
      className="min-w-0 space-y-5 rounded-xl border p-4"
      aria-label="模型详情"
    >
      <h2 className="text-lg font-medium">
        {model?.display_name ?? "模型详情"}
      </h2>
      {error ? <FeedbackToast message={error} /> : null}
      <MutationStatus mutation={mutation} />
      {!detail ? (
        <LoadingState>正在加载配置…</LoadingState>
      ) : (
        <>
          <p className="text-sm text-muted-foreground">
            {model?.status === "active" ? "已启用" : "草稿 / 已停用"} ·
            当前发布：
            {active
              ? `配置 ${detail.versions?.find((v) => v.id === active.model_version_id)?.version_no ?? "—"} / 价格 ${detail.prices?.find((p) => p.id === active.price_version_id)?.version_no ?? "—"}`
              : "尚未发布"}
          </p>
          <Collapsible
            open={sectionOpen.configuration ?? !latest}
            onOpenChange={(open) =>
              setSectionOpen((previous) => ({
                ...previous,
                configuration: open,
              }))
            }
            className="rounded-lg border p-3"
          >
            <CollapsibleTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                className="group/collapsible h-auto w-full justify-between whitespace-normal"
              >
                <span>
                  模型配置 ·{" "}
                  {latest
                    ? `已保存 ${detail.versions?.length} 个版本`
                    : "待配置"}
                </span>
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
                  try {
                    const configuration = modelConfiguration(
                      f,
                      model?.capability ?? "",
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
                <FieldGroup className="mt-4 gap-3">
                  <Field>
                    <FieldLabel htmlFor={`${formFieldId}-field-1`}>
                      <span>上游模型 ID</span>
                    </FieldLabel>
                    <Input
                      id={`${formFieldId}-field-1`}
                      required
                      name="upstream"
                      maxLength={256}
                      defaultValue={latest?.provider_model_id}
                      placeholder="填写账号已开通的模型或推理接入点 ID"
                    />
                  </Field>
                  <FieldSet>
                    <FieldLegend>开放模式（每种模式均需验证）</FieldLegend>
                    <FieldGroup>
                      <div className="mt-2 flex flex-wrap gap-3">
                        {modes.map((mode, fieldIndex0) => (
                          <Field key={mode} orientation="horizontal">
                            <Checkbox
                              id={`${formFieldId}-field-2-${fieldIndex0}`}

                              name="modes"
                              value={mode}
                              defaultChecked={latest?.modes?.includes(mode)}
                            />
                            <FieldLabel
                              htmlFor={`${formFieldId}-field-2-${fieldIndex0}`}
                            >
                              <span>{mode}</span>
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
                  <ModelSetup
                    capability={model?.capability ?? ""}
                    limits={latest?.limits}
                    parameters={latest?.param_schema}
                  />
                  <p className="text-xs text-muted-foreground">
                    参数和限制以账号对应模型文档为准。首轮不开放回调及自动取消。
                  </p>
                  <Button disabled={mutation.busy}>保存新配置版本</Button>
                </FieldGroup>
              </form>
            </CollapsibleContent>
          </Collapsible>
          <Collapsible
            open={sectionOpen.pricing ?? (!!latest && !latestPrice)}
            onOpenChange={(open) =>
              setSectionOpen((previous) => ({ ...previous, pricing: open }))
            }
            className="rounded-lg border p-3"
          >
            <CollapsibleTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                className="group/collapsible h-auto w-full justify-between whitespace-normal"
              >
                <span>
                  平台计费价格 ·{" "}
                  {latestPrice
                    ? `已保存 ${detail.prices?.length} 个版本`
                    : "待配置"}
                </span>
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
                  try {
                    const rule = priceRule(f);
                    await mutation.run("model.price", id, (request) =>
                      publishAdminModelPrice(
                        { id },
                        {
                          expected_revision: model?.revision,
                          version_no: (latestPrice?.version_no ?? 0) + 1,
                          unit: String(f.get("unit")) as API.PriceUnit,
                          rule,
                          currency: "CNY",
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
                <FieldGroup className="mt-4 gap-3">
                  <Field>
                    <FieldLabel htmlFor={`${formFieldId}-field-4`}>
                      <span>计费单位</span>
                    </FieldLabel>
                    <NativeSelect
                      id={`${formFieldId}-field-4`}
                      name="unit"
                      className="h-10 w-full"
                      defaultValue={latestPrice?.unit ?? "per_request"}
                    >
                      <NativeSelectOption value="per_request">
                        每次请求
                      </NativeSelectOption>
                      <NativeSelectOption value="per_image">
                        每张图片
                      </NativeSelectOption>
                      <NativeSelectOption value="per_second">
                        每秒
                      </NativeSelectOption>
                      <NativeSelectOption value="per_1k_tokens">
                        每千 Token
                      </NativeSelectOption>
                    </NativeSelect>
                  </Field>
                  <PriceSetup rule={latestPrice?.rule} />
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
                  <p className="text-xs text-muted-foreground">
                    这是平台向用户收取的价格；上游成本单独审核。零价格表示平台不向用户收费。
                  </p>
                  <Button disabled={mutation.busy || !latest}>
                    保存价格版本
                  </Button>
                </FieldGroup>
              </form>
            </CollapsibleContent>
          </Collapsible>
          <Collapsible
            open={sectionOpen.release ?? !!latestPrice}
            onOpenChange={(open) =>
              setSectionOpen((previous) => ({ ...previous, release: open }))
            }
            className="rounded-lg border p-3"
          >
            <CollapsibleTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                className="group/collapsible h-auto w-full justify-between whitespace-normal"
              >
                <span>创建待验证发布</span>
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
                    <NativeSelect
                      id={`${formFieldId}-field-6`}
                      name="version"
                      required
                      className="h-10 w-full"
                    >
                      {detail.versions?.map((v) => (
                        <NativeSelectOption key={v.id} value={v.id}>
                          v{v.version_no} · {v.provider_model_id}
                        </NativeSelectOption>
                      ))}
                    </NativeSelect>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor={`${formFieldId}-field-7`}>
                      <span>价格版本</span>
                    </FieldLabel>
                    <NativeSelect
                      id={`${formFieldId}-field-7`}
                      name="price"
                      required
                      className="h-10 w-full"
                    >
                      {detail.prices?.map((p) => (
                        <NativeSelectOption key={p.id} value={p.id}>
                          v{p.version_no} · {p.unit} · {p.currency}
                        </NativeSelectOption>
                      ))}
                    </NativeSelect>
                  </Field>
                  <p className="text-sm">
                    绑定连接配置：v{config?.version ?? "—"}
                  </p>
                  <div className="grid gap-3 sm:grid-cols-2">
                    <Field>
                      <FieldLabel htmlFor={`${formFieldId}-field-8`}>
                        <span>上游币种</span>
                      </FieldLabel>
                      <NativeSelect
                        id={`${formFieldId}-field-8`}
                        name="currency"
                        className="h-10 w-full"
                      >
                        <NativeSelectOption value="CNY">
                          人民币
                        </NativeSelectOption>
                        <NativeSelectOption value="USD">
                          美元
                        </NativeSelectOption>
                      </NativeSelect>
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
                  <Button disabled={mutation.busy || !config || !latestPrice}>
                    创建待验证发布
                  </Button>
                </FieldGroup>
              </form>
            </CollapsibleContent>
          </Collapsible>
          <section className="space-y-3" aria-label="发布历史">
            <h3 className="font-medium">发布记录</h3>
            {!releases.length ? (
              <p className="text-sm text-muted-foreground">尚无发布记录。</p>
            ) : null}
            {releases.map((r, fieldIndex1) => {
              const missing =
                r.required_modes?.filter(
                  (mode) => !r.validated_modes?.includes(mode),
                ) ?? [];
              const ready =
                (r.status === "draft" || r.status === "retired") &&
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
                    {currentDefault?.release_id === r.id ? " · 平台默认" : ""}
                  </p>
                  <p className="text-xs break-all text-muted-foreground">
                    {r.id}
                  </p>
                  <p>已验证：{r.validated_modes?.join("、") || "无"}</p>
                  {missing.length ? (
                    <p>尚需真实验证：{missing.join("、")}</p>
                  ) : null}
                  {r.status === "draft" ? (
                    <ProviderTrial release={r} onVerified={onSaved} />
                  ) : null}
                  {r.status === "draft" || r.status === "retired" ? (
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
                          {r.status === "retired"
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
              onClick={async () => {
                try {
                  await setAdminModelStatus(
                    { id },
                    { status: "disabled", expected_revision: model.revision },
                    { headers: { "Idempotency-Key": crypto.randomUUID() } },
                  );
                  onSaved();
                } catch (e) {
                  setError(catalogMessage(e));
                }
              }}
            >
              停用模型（不接受新任务）
            </Button>
          ) : null}
        </>
      )}
    </section>
  );
}
