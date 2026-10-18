"use client";

import { useEffect, useId, useRef, useState } from "react";
import { ChevronDown, Eye, EyeOff, Globe, Server } from "lucide-react";
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
  return (
    <form
      className="flex min-h-0 min-w-0 flex-1 flex-col gap-6"
      onSubmit={async (event) => {
        event.preventDefault();
        if (
          busy ||
          complete ||
          !apiKey ||
          (!targetID && (!preset?.id || !preset.version_id))
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
              required={!targetID}
              value={selectedPreset}
              disabled={busy || !!targetID || !!create.pending}
              onValueChange={setPresetID}
              className="grid-cols-1 sm:grid-cols-2"
            >
              {presets
                .filter((p) => !p.archived && !p.emergency_disabled)
                .map((p) => (
                  <FieldLabel key={p.id} htmlFor={`${fieldID}-${p.id}`}>
                    <Field orientation="horizontal">
                      <RadioGroupItem id={`${fieldID}-${p.id}`} value={p.id!} />
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
                          · v{p.version ?? "未知"}
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
            <FieldDescription>Key 加密保存，保存后不可查看。</FieldDescription>
          </Field>
        </FieldGroup>
        {!targetID ? (
          <Collapsible>
            <CollapsibleTrigger asChild>
              <Button type="button" variant="ghost" className="max-md:h-11">
                <ChevronDown data-icon="inline-start" />
                更多设置（可选）
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
        {preset ? (
          <p className="text-sm text-muted-foreground">
            {preset.document?.model_templates?.length
              ? `自动创建 ${preset.document.model_templates.length} 个待验证模型，复用预设中的参数和价格。`
              : "此预设包含连接配置，保存后需由管理员补齐模型模板。"}{" "}
            保存不会自动付费试运行或启用模型。
          </p>
        ) : null}
        {targetID ? (
          <p role="status" className="text-sm">
            {complete
              ? "连接与 Key 已保存。请在连接详情检查账号权限与模型状态。"
              : "连接已创建。若 Key 尚未保存，请重新输入后继续；结果未知时先查询回执。"}
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
      <div className="flex shrink-0 flex-wrap justify-end gap-2">
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
          disabled={busy || (!preset && !targetID) || !apiKey || complete}
        >
          {busy ? <Spinner data-icon="inline-start" /> : null}
          {busy
            ? "正在保存…"
            : complete
              ? "连接与 Key 已保存"
              : targetID
                ? "继续保存 API Key"
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
