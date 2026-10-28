"use client";

import { useEffect, useId, useRef, useState } from "react";
import { Eye, EyeOff } from "lucide-react";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Empty, EmptyHeader, EmptyDescription } from "@/components/ui/empty";
import { Checkbox } from "@/components/ui/checkbox";
import { Badge } from "@/components/ui/badge";
import { Spinner } from "@/components/ui/spinner";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
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
  initialPreset = "",
  onCancel,
  providers = [],
  onCreated,
  onSaved,
}: {
  presets: API.Preset[];
  initialPreset?: string;
  onCancel?: () => void;
  providers?: API.ProviderListItem[];
  onCreated: (id: string) => void;
  onSaved: (id: string) => void;
}) {
  const fieldID = useId();
  const [presetID, setPresetID] = useState(initialPreset);
  const [candidateKeys, setCandidateKeys] = useState<string[]>([]);
  const [visible, setVisible] = useState(false);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const [apiKey, setAPIKey] = useState("");
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
  const complete =
    confirmed ||
    (!!targetID &&
      !credential.pending &&
      providers.some(
        (p) => p.provider?.id === targetID && p.credential?.status === "active",
      ));
  const busy = create.busy || credential.busy;
  const direct =
    !!preset?.model_catalog?.models?.length &&
    !preset.document?.model_templates?.length;
  const models = preset?.model_catalog?.models ?? [];
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
              (direct && !candidateKeys.length)))
        )
          return;
        const connectionName = "";
        setPresetID(selectedPreset);
        const secret = apiKey;
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
                  direct_use: direct,
                },
              ),
          );
          id = connectionID(result);
          if (!id) return;
        }
        await credential.run("credential.set", id, (requestID) =>
          setAdminCredential(
            { id },
            { label: "平台统一 Key", secret: { api_key: secret } },
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
                setAPIKey("");
                setVisible(false);
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
          <Field>
            <FieldLabel variant="title" htmlFor={`${fieldID}-key`}>
              API Key
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
                  preset?.document?.config?.adapter_key === "volcengine_speech"
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
                  aria-label={visible ? "隐藏 API Key" : "显示 API Key"}
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
              onValueChange={setCategory}
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
                return (
                  <TabsContent
                    key={value}
                    value={value}
                    className="max-h-60 overflow-y-auto py-1"
                  >
                    {visibleModels.length ? (
                      <FieldGroup className="gap-0">
                        {visibleModels.map((candidate) => (
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
                              htmlFor={fieldID + "-candidate-" + candidate.key}
                              className="min-h-10 min-w-0 flex-1 items-center py-2 max-md:min-h-11"
                            >
                              <FieldTitle className="min-w-0 break-words">
                                {candidate.display_name}
                              </FieldTitle>
                              {candidate.requires_model_selection ? (
                                <span>需配置</span>
                              ) : null}
                              {candidate.lifecycle === "sunsetting" ? (
                                <Badge variant="outline">即将下线</Badge>
                              ) : null}
                            </FieldLabel>
                          </Field>
                        ))}
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
