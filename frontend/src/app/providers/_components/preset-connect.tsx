"use client";

import { useEffect, useRef, useState } from "react";

import {
  connectProviderPreset,
  getConfigReceipt,
  readPresetModelDirectory,
  setAdminCredential,
} from "@/api/settings";
import { useSession } from "@/components/auth/session";
import { RequestError } from "@/lib/request";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { ModelCatalogChoices } from "@/components/catalog/model-catalog-choices";
import {
  Field,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";

type ConnectInput = Parameters<typeof connectProviderPreset>[1];
type Model = API.ModelCandidatePreset;

const capabilities: Record<string, string> = {
  "text.generate": "文本",
  "image.generate": "图片",
  "video.generate": "视频",
  "audio.generate": "音频",
};
const credentialLabels: Record<string, string> = {
  api_key: "API Key",
  access_key_id: "Access Key ID",
  secret_access_key: "Secret Access Key",
};

function connectionID(result: unknown) {
  if (
    typeof result === "object" &&
    result !== null &&
    "provider_id" in result
  ) {
    const id = result.provider_id;
    return typeof id === "string" && id ? id : null;
  }
  return null;
}

function savedCredential(result: unknown, provider: string) {
  return (
    typeof result === "object" &&
    result !== null &&
    "id" in result &&
    typeof result.id === "string" &&
    !!result.id &&
    "status" in result &&
    result.status === "active" &&
    connectionID(result) === provider
  );
}

type Save = {
  input: ConnectInput & { request_id: string };
  presetID: string;
  credentialRequest: string;
  providerID?: string;
};

// Keep only the nonsecret request coordinates when the dialog is reopened.
let outstanding: { session: string; save: Save } | undefined;

function recordPending(session: string, save: Save | null, previous?: string) {
  if (
    previous &&
    (outstanding?.session !== session ||
      outstanding.save.input.request_id !== previous)
  )
    return;
  outstanding = save ? { session, save } : undefined;
}

async function findReceipt(requestID: string, action: string, target: string) {
  try {
    const receipt = await getConfigReceipt({
      id: requestID,
      action,
      target_id: target,
    });
    if (
      receipt.request_id === requestID &&
      receipt.action === action &&
      receipt.target_id === target
    ) {
      return receipt.result;
    }
    throw new Error("receipt does not match the pending request");
  } catch (error) {
    if (error instanceof RequestError && error.status === 404) return undefined;
    throw error;
  }
}

type Props = {
  presets: API.Preset[];
  adapters?: API.AdapterMetadata[];
  initialPreset?: string;
  providers?: API.ProviderListItem[];
  onCancel?: () => void;
  onCreated: (id: string) => void;
  onSaved: (id: string) => void;
};

export function PresetConnect(props: Props) {
  const session = useSession().session_id;
  return <ConnectWizard key={session} session={session} {...props} />;
}

function ConnectWizard({
  session,
  presets,
  adapters = [],
  initialPreset,
  onCancel,
  onCreated,
  onSaved,
}: Props & { session: string }) {
  const [pending, setPending] = useState<Save | null>(() =>
    outstanding && outstanding.session === session ? outstanding.save : null,
  );
  const [step, setStep] = useState<1 | 2>(1);
  const [presetID, setPresetID] = useState(initialPreset ?? "");
  const [secret, setSecret] = useState<Record<string, string>>({});
  const [picked, setPicked] = useState<string[]>([]);
  const [custom, setCustom] = useState<Model[]>([]);
  const [online, setOnline] = useState<string[]>([]);
  const [manualID, setManualID] = useState("");
  const [showManual, setShowManual] = useState(false);
  const [manualCapability, setManualCapability] = useState("");
  const [notice, setNotice] = useState(
    pending ? "此接入有待确认的请求，可继续核对。" : "",
  );
  const [busy, setBusy] = useState<"read" | "save" | null>(null);
  const inFlight = useRef(false);
  const mounted = useRef(false);
  const searchInput = useRef<HTMLInputElement>(null);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  useEffect(() => {
    if (step === 2) searchInput.current?.focus();
  }, [step]);

  function remember(save: Save | null, previous?: string) {
    recordPending(session, save, previous);
    if (mounted.current) setPending(save);
  }

  const available = presets.filter(
    (item) =>
      item.id && item.version_id && !item.archived && !item.emergency_disabled,
  );
  const preset = pending
    ? presets.find((item) => item.id === pending.presetID)
    : (available.find((item) => item.id === presetID) ?? available[0]);
  const config = preset?.document?.config;
  const adapter = adapters.find(
    (item) => item.adapter_key === config?.adapter_key,
  );
  const fields = adapter?.credential_fields?.length
    ? adapter.credential_fields
    : [{ name: "api_key", required: true }];
  const canRead =
    adapter?.check_capabilities?.includes("model_discovery") &&
    fields.some((field) => field.name === "api_key");
  const hasCredentials =
    !!preset &&
    fields.every(
      (field) =>
        !!field.name &&
        (field.required === false || !!secret[field.name]?.trim()),
    );
  const modes = (config?.capabilities ?? []).map(
    (capability) => capability + ".generate",
  );
  const mode =
    modes.length === 1
      ? modes[0]
      : modes.includes(manualCapability)
        ? manualCapability
        : "";
  const models = (preset?.model_catalog?.models ?? []).filter(
    (model) =>
      model.key &&
      model.provider_model_id &&
      model.capability &&
      !model.requires_model_selection,
  );
  const selectionCount = picked.length + custom.length;

  function changeSupplier(id: string) {
    setPresetID(id);
    setSecret({});
    setPicked([]);
    setCustom([]);
    setOnline([]);
    setManualID("");
    setShowManual(false);
    setManualCapability("");
    setNotice("");
  }

  function enterSelection() {
    if (hasCredentials) {
      if (!models.length) setShowManual(true);
      setStep(2);
      setNotice("");
    }
  }

  async function readDirectory() {
    if (
      inFlight.current ||
      !preset?.id ||
      !preset.version_id ||
      !canRead ||
      !hasCredentials
    )
      return;
    inFlight.current = true;
    setBusy("read");
    setNotice("");
    try {
      const result = await readPresetModelDirectory(
        { id: preset.id },
        { version_id: preset.version_id, api_key: secret.api_key },
      );
      const ids = result.model_ids ?? [];
      setOnline(ids);
      setStep(2);
      if (!ids.length)
        setNotice("目录没有返回模型，可以使用预置模型或手动添加。");
    } catch {
      setNotice(
        "暂时无法读取目录。可核对凭据后重试，或继续使用预置模型和手动添加。",
      );
    } finally {
      inFlight.current = false;
      setBusy(null);
    }
  }

  function addManualModel() {
    const id = manualID.trim();
    if (!id || !mode || /\s/u.test(id) || selectionCount >= 64) return;

    const presetModel = models.find(
      (model) => model.provider_model_id === id && model.capability === mode,
    );
    if (presetModel?.key) {
      const key = presetModel.key;
      setPicked((current) =>
        current.includes(key) ? current : [...current, key],
      );
    } else if (
      !custom.some(
        (model) => model.provider_model_id === id && model.capability === mode,
      )
    ) {
      setCustom((current) => [
        ...current,
        {
          key: crypto.randomUUID(),
          provider_model_id: id,
          display_name: id,
          capability: mode,
        },
      ]);
    }
    setManualID("");
  }

  async function saveConnection(onlyConnection: boolean) {
    if (inFlight.current) return;
    let attempt = pending;
    if (!attempt) {
      if (
        !hasCredentials ||
        !preset?.id ||
        !preset.version_id ||
        (!onlyConnection && selectionCount === 0)
      )
        return;
      attempt = {
        presetID: preset.id,
        input: {
          request_id: crypto.randomUUID(),
          version_id: preset.version_id,
          catalog_version: preset.model_catalog?.version,
          candidate_keys: onlyConnection ? [] : picked,
          custom_candidates: onlyConnection ? [] : custom,
          selected_models_only: true,
        },
        credentialRequest: crypto.randomUUID(),
      };
    }

    inFlight.current = true;
    setBusy("save");
    remember(attempt, pending?.input.request_id);
    setNotice("");
    let id = attempt.providerID;
    let writingCredential = false;
    try {
      if (!id) {
        const existing =
          pending &&
          (await findReceipt(
            attempt.input.request_id,
            "preset.connect",
            attempt.presetID,
          ));
        id = connectionID(existing) ?? undefined;
        if (!id && mounted.current) {
          id =
            connectionID(
              await connectProviderPreset(
                { id: attempt.presetID },
                attempt.input,
              ),
            ) ?? undefined;
        }
        if (!id) throw new Error("connection receipt missing");
        remember({ ...attempt, providerID: id }, attempt.input.request_id);
      }
      if (!mounted.current) return;

      const existing =
        pending &&
        (await findReceipt(attempt.credentialRequest, "credential.set", id));
      if (existing && !savedCredential(existing, id)) {
        throw new Error(
          "credential receipt does not match the pending connection",
        );
      }
      if (!existing) {
        if (!mounted.current) return;
        if (!hasCredentials) {
          setStep(1);
          setNotice("连接和模型草稿已保存。请重新填写凭据以完成接入。");
          return;
        }
        writingCredential = true;
        const result = await setAdminCredential(
          { id },
          {
            label: "平台凭据",
            secret: Object.fromEntries(
              fields.flatMap((field) => {
                const name = field.name;
                const value = name && secret[name]?.trim();
                return name && value ? [[name, value]] : [];
              }),
            ),
          },
          { headers: { "Idempotency-Key": attempt.credentialRequest } },
        );
        if (!savedCredential(result, id))
          throw new Error("credential receipt missing");
      }
      remember(null, attempt.input.request_id);
    } catch (error) {
      if (mounted.current) {
        if (
          writingCredential &&
          id &&
          error instanceof RequestError &&
          error.status === 422
        ) {
          remember(
            {
              ...attempt,
              providerID: id,
              credentialRequest: crypto.randomUUID(),
            },
            attempt.input.request_id,
          );
          setSecret({});
          setStep(1);
          setNotice("连接和模型草稿已保存。凭据未通过校验，请重新填写。");
        } else if (
          !id &&
          error instanceof RequestError &&
          [
            "revision_conflict",
            "preset_unavailable",
            "invalid_preset",
          ].includes(error.code)
        ) {
          remember(null, attempt.input.request_id);
          setOnline([]);
          setStep(1);
          setNotice("预设或选模已失效，请重新确认后保存。");
        } else {
          setNotice(
            id
              ? "连接和所选模型草稿已保存，凭据结果待确认。可核对原请求并继续。"
              : "接入结果待确认。可核对原请求并继续。",
          );
        }
      }
      return;
    } finally {
      inFlight.current = false;
      if (mounted.current) setBusy(null);
    }
    if (id && mounted.current) {
      setSecret({});
      onCreated(id);
      onSaved(id);
    }
  }

  return (
    <form
      className="flex min-h-0 min-w-0 flex-col gap-4"
      onSubmit={(event) => {
        event.preventDefault();
        if (step === 1 && !pending) enterSelection();
        else void saveConnection(false);
      }}
    >
      <ol aria-label="接入步骤" className="grid grid-cols-2 gap-2 text-sm">
        <li
          aria-current={step === 1 ? "step" : undefined}
          className="rounded-md bg-muted px-3 py-2 aria-[current=step]:font-semibold"
        >
          1. 连接服务
        </li>
        <li
          aria-current={step === 2 ? "step" : undefined}
          className="rounded-md bg-muted px-3 py-2 aria-[current=step]:font-semibold"
        >
          2. 选择模型
        </li>
      </ol>
      {notice && (
        <Alert role="status">
          <AlertDescription>{notice}</AlertDescription>
        </Alert>
      )}
      {step === 1 ? (
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="connect-supplier">供应商</FieldLabel>
            <NativeSelect
              id="connect-supplier"
              value={preset?.id ?? ""}
              onChange={(event) => changeSupplier(event.target.value)}
              disabled={!!busy || !!pending || available.length === 0}
            >
              {!available.length && (
                <NativeSelectOption value="">
                  暂无可用供应商预设
                </NativeSelectOption>
              )}
              {available.map((item) => (
                <NativeSelectOption key={item.id} value={item.id}>
                  {item.document?.name ?? item.slug ?? item.id}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </Field>
          {config?.base_url && (
            <p className="text-sm break-all text-muted-foreground">
              {config.base_url}
            </p>
          )}
          {fields.map((field) => {
            const name = field.name ?? "api_key";
            return (
              <Field key={name}>
                <FieldLabel htmlFor={"connect-" + name}>
                  {credentialLabels[name] ?? name}
                </FieldLabel>
                <Input
                  id={"connect-" + name}
                  type="password"
                  autoComplete="off"
                  spellCheck={false}
                  value={secret[name] ?? ""}
                  disabled={!!busy || !preset}
                  onChange={(event) => {
                    setSecret({ ...secret, [name]: event.target.value });
                    setCustom((current) =>
                      current.filter(
                        (model) =>
                          !online.includes(model.provider_model_id ?? ""),
                      ),
                    );
                    if (online.includes(manualID)) setManualID("");
                    setOnline([]);
                  }}
                />
              </Field>
            );
          })}
        </FieldGroup>
      ) : (
        <FieldSet
          disabled={!!busy || !!pending}
          className="flex min-h-0 min-w-0 flex-col gap-4 overflow-y-auto"
        >
          <FieldLegend className="sr-only">选择模型</FieldLegend>
          <ModelCatalogChoices
            models={models}
            online={online}
            picked={picked}
            searchRef={searchInput}
            canSelect={(model) =>
              picked.includes(model.key!) || selectionCount < 64
            }
            onPreset={(model) => {
              const key = model.key!;
              setPicked((current) =>
                current.includes(key)
                  ? current.filter((item) => item !== key)
                  : [...current, key],
              );
            }}
            onOnline={(id) => {
              setManualID(id);
              setShowManual(true);
            }}
          />
          {modes.length > 0 && !showManual && (
            <Button
              type="button"
              variant="outline"
              className="self-start"
              onClick={() => setShowManual(true)}
            >
              手动添加模型
            </Button>
          )}
          {modes.length > 0 && showManual && (
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="connect-manual-model">
                  手动添加模型 ID
                </FieldLabel>
                <Input
                  id="connect-manual-model"
                  maxLength={200}
                  value={manualID}
                  onChange={(event) => setManualID(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key === "Enter") {
                      event.preventDefault();
                      addManualModel();
                    }
                  }}
                />
              </Field>
              <div className="flex flex-wrap items-end gap-2">
                <Field className="w-auto">
                  <FieldLabel htmlFor="connect-capability">能力</FieldLabel>
                  <NativeSelect
                    id="connect-capability"
                    value={mode}
                    onChange={(event) =>
                      setManualCapability(event.target.value)
                    }
                  >
                    {modes.length > 1 && (
                      <NativeSelectOption value="" disabled>
                        选择能力
                      </NativeSelectOption>
                    )}
                    {modes.map((value) => (
                      <NativeSelectOption key={value} value={value}>
                        {capabilities[value] ?? value}
                      </NativeSelectOption>
                    ))}
                  </NativeSelect>
                </Field>
                <Button
                  type="button"
                  variant="outline"
                  disabled={
                    !mode ||
                    !manualID.trim() ||
                    /\s/u.test(manualID.trim()) ||
                    selectionCount >= 64
                  }
                  onClick={addManualModel}
                >
                  加入所选
                </Button>
              </div>
            </FieldGroup>
          )}
          {custom.length > 0 && (
            <ul
              className="flex flex-col gap-1 text-sm"
              aria-label="手动添加的模型"
            >
              {custom.map((model) => (
                <li
                  key={model.key}
                  className="flex items-center justify-between gap-2"
                >
                  <span className="min-w-0 break-all">
                    {model.provider_model_id} ·{" "}
                    {capabilities[model.capability ?? ""]} · 待配置
                  </span>
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    aria-label={"移除 " + model.provider_model_id}
                    onClick={() =>
                      setCustom((current) =>
                        current.filter((item) => item.key !== model.key),
                      )
                    }
                  >
                    移除
                  </Button>
                </li>
              ))}
            </ul>
          )}
          <p className="text-xs text-muted-foreground" aria-live="polite">
            已选 {selectionCount} 个模型。保存后可在模型管理中配置和验证。
          </p>
        </FieldSet>
      )}
      <div className="flex flex-wrap justify-end gap-2 border-t pt-3">
        {pending ? (
          <Button
            type="button"
            disabled={!!busy}
            onClick={() => void saveConnection(false)}
          >
            {busy === "save" ? "正在保存…" : "核对并继续保存"}
          </Button>
        ) : step === 1 ? (
          <>
            <Button
              type="button"
              variant="ghost"
              disabled={!!busy}
              onClick={() => {
                setSecret({});
                onCancel?.();
              }}
            >
              取消
            </Button>
            {canRead && (
              <Button
                type="button"
                variant="outline"
                disabled={!hasCredentials || !!busy}
                onClick={() => void readDirectory()}
              >
                {busy === "read" ? "正在读取…" : "读取模型"}
              </Button>
            )}
            <Button type="submit" disabled={!hasCredentials || !!busy}>
              {models.length ? "查看预置模型" : "手动添加模型"}
            </Button>
          </>
        ) : (
          <>
            <Button
              type="button"
              variant="ghost"
              disabled={!!busy}
              onClick={() => {
                setStep(1);
                setNotice("");
              }}
            >
              上一步
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={!!busy || selectionCount > 0}
              onClick={() => void saveConnection(true)}
            >
              仅保存连接
            </Button>
            <Button type="submit" disabled={!!busy || selectionCount === 0}>
              保存接入
            </Button>
          </>
        )}
      </div>
    </form>
  );
}
