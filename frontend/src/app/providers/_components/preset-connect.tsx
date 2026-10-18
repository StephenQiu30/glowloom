"use client";

import { useId, useState } from "react";
import { connectProviderPreset, setAdminCredential } from "@/api/settings";
import { isCredentialReceipt } from "@/components/catalog/credential-receipt";
import { RequestError } from "@/lib/request";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Button } from "@/components/ui/button";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
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
  providers = [],
  onCreated,
  onSaved,
}: {
  presets: API.Preset[];
  providers?: API.ProviderListItem[];
  onCreated: (id: string) => void;
  onSaved: () => void;
}) {
  const fieldID = useId();
  const [presetID, setPresetID] = useState("");
  const [apiKey, setAPIKey] = useState("");
  const [name, setName] = useState("");
  const [createdID, setCreatedID] = useState("");
  const [confirmed, setComplete] = useState(false);
  const create = useCatalogMutation(
    (result) => {
      const id = connectionID(result);
      if (id) {
        setCreatedID(id);
        onCreated(id);
      }
    },
    "preset.onboarding.create",
    (result) => !!connectionID(result),
  );
  const credential = useCatalogMutation(
    () => {
      setComplete(true);
      onSaved();
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
    presetID ||
    (create.pending?.action === "preset.connect" ? create.pending.target : "");
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
      className="flex flex-col gap-3 rounded-xl border p-4"
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
        const secret = apiKey;
        setAPIKey("");
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
      <h2 className="font-medium">从预设接入 AI 服务</h2>
      <FieldGroup className="grid gap-3 sm:grid-cols-2">
        <Field>
          <FieldLabel htmlFor={`${fieldID}-preset`}>平台预设</FieldLabel>
          <NativeSelect
            id={`${fieldID}-preset`}
            required
            value={selectedPreset}
            disabled={busy || !!targetID || !!create.pending}
            onChange={(event) => setPresetID(event.target.value)}
          >
            <NativeSelectOption value="">选择预设</NativeSelectOption>
            {presets
              .filter((p) => !p.archived && !p.emergency_disabled)
              .map((p) => (
                <NativeSelectOption key={p.id} value={p.id}>
                  {p.document?.name} · v{p.version}
                  {p.document?.model_templates?.length
                    ? " · 含模型模板"
                    : " · 连接配置"}
                </NativeSelectOption>
              ))}
          </NativeSelect>
        </Field>
        <Field>
          <FieldLabel htmlFor={`${fieldID}-key`}>API Key</FieldLabel>
          <Input
            id={`${fieldID}-key`}
            type="password"
            required
            value={apiKey}
            onChange={(event) => setAPIKey(event.target.value)}
            autoComplete="off"
            spellCheck={false}
            disabled={complete || busy}
          />
        </Field>
      </FieldGroup>
      {!targetID ? (
        <Collapsible>
          <CollapsibleTrigger asChild>
            <Button type="button" variant="ghost">
              可选设置
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
      <p className="text-xs text-muted-foreground">
        Key 只发送到后台加密保存，不写入浏览器存储。名称和连接标识自动生成。
      </p>
      {targetID ? (
        <p role="status" className="text-sm">
          {complete
            ? "连接与 Key 已保存。请在连接详情检查账号权限与模型状态。"
            : "连接已创建。若 Key 尚未保存，请重新输入后继续；结果未知时先查询回执。"}
        </p>
      ) : null}
      <MutationStatus mutation={create} />
      <MutationStatus mutation={credential} />
      <div className="flex flex-wrap gap-2">
        <Button
          disabled={busy || (!preset && !targetID) || !apiKey || complete}
        >
          {busy
            ? "正在保存…"
            : complete
              ? "连接与 Key 已保存"
              : targetID
                ? "继续保存 API Key"
                : "保存连接与 API Key"}
        </Button>
        {complete ? (
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setCreatedID("");
              setComplete(false);
              setPresetID("");
              setName("");
            }}
          >
            添加另一个连接
          </Button>
        ) : null}
      </div>
    </form>
  );
}
