"use client";

import { useEffect, useId, useRef, useState } from "react";
import { ChevronDown, Eye, EyeOff, Globe, Server } from "lucide-react";
import { ConnectionOnboarding } from "./connection-onboarding";
import { Checkbox } from "@/components/ui/checkbox";
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
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
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
import { Input } from "@/components/ui/input";
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
  const [name, setName] = useState("");
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
      if (!direct) onSaved(connectionID(result));
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
  if (complete && targetID && direct)
    return (
      <ConnectionOnboarding
        id={targetID}
        onSaved={onSaved}
        onCancel={onCancel}
      />
    );
  return (
    <form
      className="flex min-h-0 min-w-0 flex-1 flex-col gap-6"
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
        const connectionName = name.trim();
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
      <div className="flex min-h-0 flex-col gap-6 overflow-y-auto">
        <FieldGroup>
          <FieldSet>
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
              }}
              className="grid-cols-1 sm:grid-cols-2"
            >
              {presets
                .filter((p) => !p.archived && !p.emergency_disabled)
                .map((p) => (
                  <FieldLabel key={p.id} htmlFor={`${fieldID}-${p.id}`}>
                    <Field orientation="horizontal">
                      <RadioGroupItem
                        id={`${fieldID}-${p.id}`}
                        value={p.id!}
                        aria-label={p.document?.name}
                      />
                      <FieldContent>
                        <FieldTitle>
                          <span className="flex items-center gap-2">
                            {p.document?.config?.adapter_key ===
                            "volcengine_ark" ? (
                              <Server className="size-4" />
                            ) : (
                              <Globe className="size-4" />
                            )}
                            {p.document?.name}
                          </span>
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
            <FieldLabel htmlFor={`${fieldID}-key`}>API Key</FieldLabel>
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
                placeholder="输入 API Key"
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
            <FieldDescription>加密保存</FieldDescription>
          </Field>
        </FieldGroup>
        {preset?.model_catalog?.models?.length && !targetID ? (
          <FieldSet disabled={busy || !!create.pending} className="gap-4">
            <FieldLegend variant="label">
              选择模型 · 已选 {candidateKeys.length}
            </FieldLegend>
            <FieldDescription>
              勾选要用于创作的模型，保存后只需验证其中一个。
            </FieldDescription>
            {[
              ["text.generate", "文本"],
              ["image.generate", "图像"],
              ["video.generate", "视频"],
            ].map(([capability, label]) => {
              const models = preset.model_catalog!.models!.filter(
                (m) => m.capability === capability,
              );
              if (!models.length) return null;
              return (
                <FieldSet key={capability} className="gap-3">
                  <FieldLegend variant="label" className="mb-0">
                    {label}
                  </FieldLegend>
                  {models.map((candidate) => (
                    <Field
                      key={candidate.key}
                      orientation="horizontal"
                      className="items-start"
                    >
                      <Checkbox
                        id={`${fieldID}-candidate-${candidate.key}`}
                        className="mt-1"
                        disabled={
                          candidate.requires_model_selection ||
                          busy ||
                          !!create.pending
                        }
                        checked={candidateKeys.includes(candidate.key!)}
                        onCheckedChange={(checked) =>
                          setCandidateKeys((previous) =>
                            checked === true
                              ? [...previous, candidate.key!]
                              : previous.filter((key) => key !== candidate.key),
                          )
                        }
                      />
                      <FieldContent className="min-w-0">
                        <FieldLabel
                          htmlFor={`${fieldID}-candidate-${candidate.key}`}
                        >
                          {candidate.display_name}
                        </FieldLabel>
                        <FieldDescription className="break-all">
                          {candidate.provider_model_id ||
                            "此型号需要先补充账号模型配置"}
                        </FieldDescription>
                      </FieldContent>
                    </Field>
                  ))}
                </FieldSet>
              );
            })}
            {preset.document?.config?.adapter_key === "volcengine_ark" ? (
              <FieldDescription>
                <a
                  href="https://console.volcengine.com/ark/region:cn-beijing/docs/model-list"
                  target="_blank"
                  rel="noreferrer"
                  className="underline underline-offset-4"
                >
                  查看火山方舟模型文档
                </a>{" "}
                · 请确保 Key 已获得所选模型的调用权限。
              </FieldDescription>
            ) : null}
          </FieldSet>
        ) : null}
        {!targetID ? (
          <Collapsible>
            <CollapsibleTrigger asChild>
              <Button type="button" variant="ghost" className="max-md:h-11">
                <ChevronDown data-icon="inline-start" />
                连接名称（可选）
              </Button>
            </CollapsibleTrigger>
            <CollapsibleContent>
              <Field className="mt-3">
                <FieldLabel htmlFor={`${fieldID}-name`}>
                  连接名称（可选）
                </FieldLabel>
                <Input
                  id={`${fieldID}-name`}
                  name="name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  maxLength={200}
                  placeholder="留空自动命名"
                  disabled={busy || !!create.pending}
                />
              </Field>
            </CollapsibleContent>
          </Collapsible>
        ) : null}
        {targetID ? (
          <p role="status" className="text-sm">
            {complete
              ? "连接与 Key 已保存。"
              : "连接已创建，请继续保存 Key；结果待确认时先查询回执。"}
          </p>
        ) : null}
        {create.pending || (!targetID && create.message) ? (
          <MutationStatus
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
            mutation={{
              ...credential,
              message: complete ? "" : credential.message,
            }}
          />
        ) : null}
      </div>
      <div className="flex shrink-0 flex-wrap items-center justify-end gap-2 border-t pt-4">
        <span className="mr-auto text-xs text-muted-foreground">
          保存不会产生生成费用
        </span>
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
            ? "正在保存…"
            : complete
              ? "连接与 Key 已保存"
              : targetID
                ? "继续保存 API Key"
                : direct
                  ? "保存并继续验证"
                  : "保存连接"}
        </Button>
        {complete ? (
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              if (onCancel) {
                onSaved(targetID);
                return;
              }
              setCreatedID("");
              setComplete(false);
              setPresetID("");
              setName("");
            }}
          >
            {onCancel ? "查看连接" : "添加另一个连接"}
          </Button>
        ) : null}
      </div>
    </form>
  );
}
