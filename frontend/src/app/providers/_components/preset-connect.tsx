"use client";

import { useEffect, useId, useRef, useState } from "react";
import { Eye, EyeOff, Plus } from "lucide-react";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Empty, EmptyHeader, EmptyDescription } from "@/components/ui/empty";
import { Checkbox } from "@/components/ui/checkbox";
import { Badge } from "@/components/ui/badge";
import { Spinner } from "@/components/ui/spinner";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Input } from "@/components/ui/input";
import {
  InputGroup,
  InputGroupInput,
  InputGroupAddon,
  InputGroupButton,
} from "@/components/ui/input-group";
import { connectProviderPreset, setAdminCredential } from "@/api/settings";
import { isCredentialReceipt } from "@/components/catalog/credential-receipt";
import { RequestError } from "@/lib/request";
import { ProviderLogo } from "./provider-logo";
import { Button } from "@/components/ui/button";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { getCandidateBrand } from "./provider-label";
import {
  Field,
  FieldGroup,
  FieldLabel,
  FieldDescription,
  FieldSet,
  FieldLegend,
  FieldContent,
  FieldTitle,
} from "@/components/ui/field";
import {
  MutationStatus,
  useCatalogMutation,
} from "@/components/catalog/mutation";

function connectionID(value: unknown) {
  if (
    value &&
    typeof value === "object" &&
    "provider_id" in value &&
    typeof value.provider_id === "string"
  )
    return value.provider_id;
  return "";
}

export function PresetConnect({
  presets,
  adapters,
  initialPreset = "",
  onCancel,
  providers = [],
  onCreated,
  onSaved,
}: {
  presets: API.Preset[];
  adapters?: API.AdapterMetadata[];
  initialPreset?: string;
  onCancel?: () => void;
  providers?: API.ProviderListItem[];
  onCreated: (id: string) => void;
  onSaved: (id: string) => void;
}) {
  const fieldID = useId();
  const [presetID, setPresetID] = useState(initialPreset);
  const [candidateKeys, setCandidateKeys] = useState<string[]>([]);
  const [selectedBrand, setSelectedBrand] = useState<string>("all");
  const [customCandidates, setCustomCandidates] = useState<
    API.ModelCandidatePreset[]
  >([]);
  const [showAddCustom, setShowAddCustom] = useState(false);
  const [customName, setCustomName] = useState("");
  const [customID, setCustomID] = useState("");
  const [visible, setVisible] = useState(false);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const [apiKey, setAPIKey] = useState("");
  const [accessKeyID, setAccessKeyID] = useState("");
  const [category, setCategory] = useState(
    () =>
      presets.find((p) => p.id === initialPreset)?.model_catalog?.models?.[0]
        ?.capability ?? "text.generate",
  );
  const [createdID, setCreatedID] = useState("");
  const [confirmed, setComplete] = useState(false);
  const create = useCatalogMutation(
    (result) => {
      const id = connectionID(result);
      if (id) {
        setCreatedID(id);
        if (mounted.current) onCreated(id);
      }
    },
    "preset.onboarding.create",
    (result) => !!connectionID(result),
  );
  const credential = useCatalogMutation(
    (result) => {
      if (!mounted.current) return;
      setComplete(true);
      onSaved(connectionID(result));
    },
    "preset.onboarding.credential",
    isCredentialReceipt,
  );
  const targetID =
    createdID ||
    (credential.pending?.action === "credential.set"
      ? credential.pending.target
      : "");
  const selectedPreset =
    create.pending?.action === "preset.connect"
      ? create.pending.target
      : presetID;
  const preset = presets.find((p) => p.id === selectedPreset);
  const metadata = adapters?.find(
    (item) => item.adapter_key === preset?.document?.config?.adapter_key,
  );
  const musicCredential = metadata
    ? metadata.credential_fields?.some(
        (field) => field.name === "secret_access_key",
      )
    : preset?.document?.config?.adapter_key === "volcengine_music";
  const complete =
    confirmed ||
    (!!targetID &&
      !credential.pending &&
      providers.some(
        (p) => p.provider?.id === targetID && p.credential?.status === "active",
      ));
  const busy = create.busy || credential.busy;
  const isMultiBrand =
    preset?.document?.config?.adapter_key === "openrouter" ||
    preset?.document?.config?.adapter_key === "vercel";
  const allModels = [
    ...(preset?.model_catalog?.models ?? []),
    ...customCandidates,
  ];
  const models = isMultiBrand
    ? allModels.filter((m) => getCandidateBrand(m) !== undefined)
    : allModels;
  const direct =
    (candidateKeys.length > 0 || customCandidates.length > 0) &&
    !preset?.document?.model_templates?.length;
  const selectable = models.filter((m) => m.key && !m.requires_model_selection);
  const allSelected =
    selectable.length > 0 &&
    selectable.every((m) => candidateKeys.includes(m.key!));
  return (
    <form
      className="flex min-h-0 min-w-0 flex-1 flex-col gap-4"
      onSubmit={async (event) => {
        event.preventDefault();
        if (
          busy ||
          complete ||
          !apiKey ||
          (!targetID &&
            (!preset?.id ||
              !preset.version_id ||
              (direct && !candidateKeys.length && !customCandidates.length)))
        )
          return;
        const connectionName = "";
        setPresetID(selectedPreset);
        const secret: Record<string, string> = musicCredential
          ? { access_key_id: accessKeyID, secret_access_key: apiKey }
          : { api_key: apiKey };
        setAccessKeyID("");
        setAPIKey("");
        setVisible(false);
        let id = targetID;
        if (!id) {
          if (!preset?.id || !preset.version_id) return;
          const result = await create.run(
            "preset.connect",
            preset.id,
            (requestID) =>
              connectProviderPreset(
                { id: preset.id! },
                {
                  request_id: requestID,
                  version_id: preset.version_id,
                  name: connectionName,
                  catalog_version: preset.model_catalog?.version,
                  candidate_keys: candidateKeys,
                  custom_candidates:
                    customCandidates.length > 0 ? customCandidates : undefined,
                  direct_use: direct,
                },
              ),
          );
          id = connectionID(result);
          if (!id || !mounted.current) return;
        }
        await credential.run("credential.set", id, (requestID) =>
          setAdminCredential(
            { id },
            { label: "平台统一 Key", secret },
            { headers: { "Idempotency-Key": requestID } },
          ).then((result) => {
            if (
              !result.id ||
              result.provider_id !== id ||
              result.status !== "active" ||
              result.last4?.length !== 4
            )
              throw new RequestError("request_unconfirmed");
            return result;
          }),
        );
      }}
    >
      <div className="flex min-h-0 flex-col gap-4 overflow-y-auto">
        <FieldGroup className="gap-4">
          <FieldSet className="gap-2">
            <FieldLegend variant="label">平台预设</FieldLegend>
            <RadioGroup
              aria-label="平台预设"
              name={`${fieldID}-preset`}
              required={!targetID}
              value={selectedPreset}
              disabled={busy || !!targetID || !!create.pending}
              onValueChange={(id) => {
                setPresetID(id);
                setCandidateKeys([]);
                setCustomCandidates([]);
                setShowAddCustom(false);
                setCustomName("");
                setCustomID("");
                setAccessKeyID("");
                setAPIKey("");
                setVisible(false);
                setSelectedBrand("all");
                setCategory(
                  presets.find((p) => p.id === id)?.model_catalog?.models?.[0]
                    ?.capability ?? "text.generate",
                );
              }}
              className="grid-cols-[repeat(auto-fit,minmax(min(100%,9rem),1fr))] gap-3"
            >
              {presets
                .filter((p) => !p.archived && !p.emergency_disabled)
                .map((p) => (
                  <FieldLabel key={p.id} htmlFor={`${fieldID}-${p.id}`}>
                    <Field orientation="horizontal" className="min-h-16">
                      <RadioGroupItem
                        id={`${fieldID}-${p.id}`}
                        value={p.id!}
                        aria-label={p.document?.name}
                      />
                      <FieldContent>
                        <FieldTitle>
                          <ProviderLogo
                            adapterKey={p.document?.config?.adapter_key}
                            metadata={adapters?.find(
                              (item) =>
                                item.adapter_key ===
                                p.document?.config?.adapter_key,
                            )}
                          />
                          {p.document?.name}
                        </FieldTitle>
                        <FieldDescription>
                          {p.document?.config?.region === "domestic"
                            ? "国内"
                            : p.document?.config?.region === "overseas"
                              ? "海外"
                              : "区域未知"}{" "}
                        </FieldDescription>
                      </FieldContent>
                    </Field>
                  </FieldLabel>
                ))}
            </RadioGroup>
            {!presets.some((p) => !p.archived && !p.emergency_disabled) ? (
              <FieldDescription>暂无可用预设，请先新建预设。</FieldDescription>
            ) : null}
          </FieldSet>
          {musicCredential && (
            <Field>
              <FieldLabel htmlFor={`${fieldID}-access-key`}>
                Access Key ID
              </FieldLabel>
              <InputGroup>
                <InputGroupInput
                  id={`${fieldID}-access-key`}
                  type="password"
                  required
                  autoComplete="off"
                  value={accessKeyID}
                  disabled={complete || busy}
                  onChange={(event) => setAccessKeyID(event.target.value)}
                />
              </InputGroup>
            </Field>
          )}
          <Field>
            <FieldLabel variant="title" htmlFor={`${fieldID}-key`}>
              {musicCredential ? "Secret Access Key" : "API Key"}
            </FieldLabel>
            <InputGroup className="h-10 max-md:h-11">
              <InputGroupInput
                className="h-full"
                id={`${fieldID}-key`}
                type={visible ? "text" : "password"}
                required
                value={apiKey}
                onChange={(event) => setAPIKey(event.target.value)}
                autoComplete="off"
                spellCheck={false}
                disabled={complete || busy || (!preset && !targetID)}
                placeholder={
                  musicCredential
                    ? "输入 Secret Access Key"
                    : preset?.document?.config?.adapter_key ===
                        "volcengine_speech"
                      ? "输入豆包语音 API Key"
                      : "输入 API Key"
                }
              />
              <InputGroupAddon
                align="inline-end"
                className="py-0 has-[>button]:mr-0"
              >
                <InputGroupButton
                  type="button"
                  size="icon-sm"
                  className="max-md:size-11"
                  aria-label={
                    musicCredential
                      ? visible
                        ? "隐藏 Secret Access Key"
                        : "显示 Secret Access Key"
                      : visible
                        ? "隐藏 API Key"
                        : "显示 API Key"
                  }
                  aria-pressed={visible}
                  disabled={complete || busy || (!preset && !targetID)}
                  onClick={() => setVisible((v) => !v)}
                >
                  {visible ? (
                    <EyeOff data-icon="inline-start" />
                  ) : (
                    <Eye data-icon="inline-start" />
                  )}
                </InputGroupButton>
              </InputGroupAddon>
            </InputGroup>
            {preset?.document?.config?.adapter_key === "volcengine_speech" ? (
              <FieldDescription>使用豆包语音 API Key</FieldDescription>
            ) : null}
          </Field>
        </FieldGroup>
        {models.length ? (
          <FieldSet
            disabled={busy || complete || !!targetID || !!create.pending}
            className="min-w-0 gap-1"
          >
            <FieldLegend variant="label" className="sr-only">
              接入模型
            </FieldLegend>
            <div className="flex min-h-8 flex-wrap items-center gap-2">
              <FieldTitle aria-hidden="true">接入模型</FieldTitle>
              <span
                role="status"
                className="ml-auto text-xs text-muted-foreground"
              >
                已选 {candidateKeys.length}
              </span>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="max-md:h-11"
                disabled={
                  busy ||
                  complete ||
                  !!targetID ||
                  !!create.pending ||
                  !selectable.length
                }
                onClick={() =>
                  setCandidateKeys(
                    allSelected ? [] : selectable.map((m) => m.key!),
                  )
                }
              >
                {allSelected ? "取消全选" : "全选"}
              </Button>
            </div>
            <Tabs
              value={category}
              onValueChange={(val) => {
                setCategory(val);
                setSelectedBrand("all");
              }}
              className="min-w-0 gap-1"
            >
              <TabsList
                className="w-full group-data-horizontal/tabs:h-10 max-md:group-data-horizontal/tabs:h-12"
                aria-label="模型类型"
              >
                {[
                  ["text.generate", "文本"],
                  ["video.generate", "视频"],
                  ["audio.generate", "音频"],
                  ["image.generate", "图片"],
                ].map(([value, label]) => (
                  <TabsTrigger key={value} value={value}>
                    {label}
                  </TabsTrigger>
                ))}
              </TabsList>
              {[
                ["text.generate", "文本"],
                ["video.generate", "视频"],
                ["audio.generate", "音频"],
                ["image.generate", "图片"],
              ].map(([value, label]) => {
                const visibleModels = models.filter((m) =>
                  value === "audio.generate"
                    ? m.capability?.startsWith("audio.")
                    : m.capability === value,
                );
                const availableBrands = (
                  ["ChatGPT", "Anthropic", "Gemini", "Grok"] as const
                ).filter((b) =>
                  visibleModels.some((m) => getCandidateBrand(m) === b),
                );
                const hasMultipleBrands = availableBrands.length > 1;
                const displayedModels =
                  hasMultipleBrands && selectedBrand !== "all"
                    ? visibleModels.filter(
                        (m) => getCandidateBrand(m) === selectedBrand,
                      )
                    : visibleModels;
                return (
                  <TabsContent
                    key={value}
                    value={value}
                    className="max-h-60 overflow-y-auto py-1"
                  >
                    {isMultiBrand || hasMultipleBrands ? (
                      <ToggleGroup
                        type="single"
                        value={selectedBrand}
                        onValueChange={(value) => {
                          if (value) setSelectedBrand(value);
                        }}
                        variant="pill"
                        size="sm"
                        aria-label="按模型品牌筛选"
                        className="sticky top-0 z-10 mb-1.5 flex-wrap bg-background/95 px-2 py-1.5 backdrop-blur"
                      >
                        <ToggleGroupItem value="all">
                          全部 ({visibleModels.length})
                        </ToggleGroupItem>
                        {(
                          ["ChatGPT", "Anthropic", "Gemini", "Grok"] as const
                        ).map((brand) => (
                          <ToggleGroupItem key={brand} value={brand}>
                            {brand} (
                            {
                              visibleModels.filter(
                                (model) => getCandidateBrand(model) === brand,
                              ).length
                            }
                            )
                          </ToggleGroupItem>
                        ))}
                      </ToggleGroup>
                    ) : null}
                    {displayedModels.length ? (
                      <FieldGroup className="gap-0">
                        {displayedModels.map((candidate) => {
                          const brand = getCandidateBrand(candidate);
                          return (
                            <Field
                              key={candidate.key}
                              orientation="horizontal"
                              data-disabled={
                                candidate.requires_model_selection || undefined
                              }
                              className="min-h-10 gap-2 px-2 max-md:min-h-11"
                            >
                              <Checkbox
                                id={fieldID + "-candidate-" + candidate.key}
                                disabled={
                                  candidate.requires_model_selection ||
                                  busy ||
                                  complete ||
                                  !!targetID ||
                                  !!create.pending
                                }
                                checked={candidateKeys.includes(candidate.key!)}
                                onCheckedChange={(checked) =>
                                  setCandidateKeys((previous) =>
                                    checked === true
                                      ? [
                                          ...new Set([
                                            ...previous,
                                            candidate.key!,
                                          ]),
                                        ]
                                      : previous.filter(
                                          (key) => key !== candidate.key,
                                        ),
                                  )
                                }
                              />
                              <FieldLabel
                                htmlFor={
                                  fieldID + "-candidate-" + candidate.key
                                }
                                className="min-h-10 min-w-0 flex-1 items-center py-2 max-md:min-h-11"
                              >
                                <FieldTitle className="flex min-w-0 items-center gap-1.5 break-words">
                                  <span>{candidate.display_name}</span>
                                  {brand ? (
                                    <Badge
                                      variant="secondary"
                                      className="px-1.5 py-0 text-[10px] font-normal"
                                    >
                                      {brand}
                                    </Badge>
                                  ) : null}
                                </FieldTitle>
                                {candidate.requires_model_selection ? (
                                  <span>需配置</span>
                                ) : null}
                                {candidate.lifecycle === "sunsetting" ? (
                                  <Badge variant="outline">即将下线</Badge>
                                ) : null}
                              </FieldLabel>
                            </Field>
                          );
                        })}
                      </FieldGroup>
                    ) : (
                      <Empty className="min-h-24 p-4">
                        <EmptyHeader>
                          <EmptyDescription>
                            暂无可接入的{label}模型
                          </EmptyDescription>
                        </EmptyHeader>
                      </Empty>
                    )}
                  </TabsContent>
                );
              })}
            </Tabs>
            <div className="mt-1 border-t border-dashed pt-2">
              <Button
                type="button"
                variant="link"
                size="sm"
                aria-expanded={showAddCustom}
                onClick={() => setShowAddCustom((value) => !value)}
              >
                <Plus data-icon="inline-start" />
                {showAddCustom ? "收起添加模型" : "添加自定义候选模型"}
              </Button>
              {showAddCustom ? (
                <div className="mt-2 flex flex-col gap-2 rounded-md border bg-muted/20 p-2.5">
                  <div className="grid grid-cols-2 gap-2 max-sm:grid-cols-1">
                    <Input
                      placeholder="模型名称（如 Claude Sonnet 5.5）"
                      value={customName}
                      onChange={(e) => setCustomName(e.target.value)}
                      className="h-8 text-xs"
                    />
                    <Input
                      placeholder="上游标识（如 anthropic/claude-sonnet-5.5）"
                      value={customID}
                      onChange={(e) => setCustomID(e.target.value)}
                      className="h-8 text-xs"
                    />
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-xs text-muted-foreground">
                      分类：
                      {category === "text.generate"
                        ? "文本"
                        : category === "image.generate"
                          ? "图片"
                          : category === "video.generate"
                            ? "视频"
                            : "音频"}
                    </span>
                    <Button
                      type="button"
                      size="sm"
                      variant="secondary"
                      className="h-7 text-xs"
                      disabled={!customName.trim() || !customID.trim()}
                      onClick={() => {
                        const key = `custom-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 6)}`;
                        const newModel: API.ModelCandidatePreset = {
                          key,
                          adapter_key: preset?.document?.config?.adapter_key,
                          display_name: customName.trim(),
                          provider_model_id: customID.trim(),
                          capability: category,
                          modes:
                            category === "image.generate"
                              ? ["text2image"]
                              : category === "video.generate"
                                ? ["text2video"]
                                : ["text_to_text"],
                          lifecycle: "active",
                        };
                        setCustomCandidates((prev) => [...prev, newModel]);
                        setCandidateKeys((prev) => [...prev, key]);
                        setCustomName("");
                        setCustomID("");
                        setShowAddCustom(false);
                      }}
                    >
                      确认添加并勾选
                    </Button>
                  </div>
                </div>
              ) : null}
            </div>
          </FieldSet>
        ) : null}
        {targetID ? (
          <p role="status" className="text-sm">
            {complete
              ? "服务已接入。"
              : "连接已创建，请继续保存 Key；结果待确认时先查询回执。"}
          </p>
        ) : null}
        {create.pending || (!targetID && create.message) ? (
          <MutationStatus
            inline
            mutation={{
              ...create,
              message: targetID ? "" : create.message,
              recover: async () => {
                setPresetID(selectedPreset);
                await create.recover();
              },
            }}
          />
        ) : null}
        {credential.pending || (!complete && credential.message) ? (
          <MutationStatus
            inline
            mutation={{
              ...credential,
              message: complete ? "" : credential.message,
            }}
          />
        ) : null}
      </div>
      <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
        {onCancel ? (
          <Button
            type="button"
            variant="ghost"
            className="max-md:h-11"
            onClick={onCancel}
          >
            取消
          </Button>
        ) : null}
        {!complete ? (
          <Button
            className="max-md:h-11"
            disabled={
              busy ||
              (!preset && !targetID) ||
              !apiKey ||
              complete ||
              (direct && !targetID && !candidateKeys.length)
            }
          >
            {busy ? <Spinner data-icon="inline-start" /> : null}
            {busy
              ? "接入中…"
              : complete
                ? "连接与 Key 已保存"
                : targetID
                  ? "继续保存 API Key"
                  : direct
                    ? "接入服务"
                    : "保存连接"}
          </Button>
        ) : null}
      </div>
    </form>
  );
}
