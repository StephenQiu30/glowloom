"use client";

import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import {
  connectProviderPreset,
  createCustomProviderConnection,
  getConfigReceipt,
  setAdminCredential,
} from "@/api/settings";
import { useSession } from "@/components/auth/session";
import { zeroID, catalogMessage } from "@/components/catalog/mutation";
import { ProviderLogoPicker } from "@/components/providers/provider-logo-picker";
import { RequestError } from "@/lib/request";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { DialogFooter } from "@/components/ui/dialog";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

type PresetInput = Parameters<typeof connectProviderPreset>[1];
type CustomInput = Parameters<typeof createCustomProviderConnection>[0];
type Save = (
  | {
      kind: "preset";
      presetID: string;
      input: PresetInput & { request_id: string };
    }
  | { kind: "custom"; input: CustomInput & { request_id: string } }
) & { credentialRequest: string; providerID?: string };
const credentialLabels: Record<string, string> = {
  api_key: "API Key",
  access_key_id: "Access Key ID",
  secret_access_key: "Secret Access Key",
};
// Reopening retains only an immutable nonsecret request, scoped to this login session.
let outstanding: { session: string; save: Save } | undefined;
const listeners = new Set<() => void>();
function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
export function usePendingProviderConnection() {
  const session = useSession().session_id;
  return useSyncExternalStore(
    subscribe,
    () =>
      outstanding && outstanding.session === session ? outstanding.save : null,
    () => null,
  );
}
function recordPending(session: string, save: Save | null, previous?: string) {
  if (
    previous &&
    (outstanding?.session !== session ||
      outstanding.save.input.request_id !== previous)
  )
    return;
  outstanding = save ? { session, save } : undefined;
  for (const listener of listeners) listener();
}
function connectionID(result: unknown) {
  if (
    typeof result === "object" &&
    result !== null &&
    "provider_id" in result &&
    typeof result.provider_id === "string" &&
    result.provider_id
  )
    return result.provider_id;
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
async function findReceipt(requestID: string, action: string, target: string) {
  try {
    const receipt = await getConfigReceipt({
      id: requestID,
      action,
      target_id: target,
    });
    if (
      receipt.request_id !== requestID ||
      receipt.action !== action ||
      receipt.target_id !== target ||
      typeof receipt.result !== "object" ||
      receipt.result === null
    )
      throw new RequestError("request_unconfirmed");
    return receipt.result;
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
  return <ConnectForm key={session} session={session} {...props} />;
}
function ConnectForm({
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
  const [kind, setKind] = useState<"preset" | "custom">(
    pending?.kind ?? "preset",
  );
  const [presetID, setPresetID] = useState(
    pending?.kind === "preset" ? pending.presetID : (initialPreset ?? ""),
  );
  const [name, setName] = useState<string | undefined>(pending?.input.name);
  const [baseURL, setBaseURL] = useState(
    pending?.kind === "custom" ? (pending.input.config?.base_url ?? "") : "",
  );
  const [logo, setLogo] = useState<string | undefined>(
    pending?.input.logo_asset_id,
  );
  const [uploading, setUploading] = useState(false);
  const [secret, setSecret] = useState<Record<string, string>>({});
  const [notice, setNotice] = useState(
    pending ? "此接入有待确认的请求，可继续核对。" : "",
  );
  const [busy, setBusy] = useState(false);
  const inFlight = useRef(false);
  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const available = presets.filter(
    (item) =>
      item.id && item.version_id && !item.archived && !item.emergency_disabled,
  );
  const preset =
    pending?.kind === "preset"
      ? presets.find((item) => item.id === pending.presetID)
      : (available.find((item) => item.id === presetID) ?? available[0]);
  const config = preset?.document?.config;
  const adapter = adapters.find(
    (item) =>
      item.adapter_key === (kind === "custom" ? "openai" : config?.adapter_key),
  );
  const fields =
    kind === "preset" && adapter?.credential_fields?.length
      ? adapter.credential_fields
      : [{ name: "api_key", required: true }];
  const displayName =
    name ??
    (kind === "preset" ? (preset?.document?.name ?? preset?.slug ?? "") : "");
  const hasCredentials = fields.every(
    (field) =>
      !!field.name &&
      (field.required === false || !!secret[field.name]?.trim()),
  );
  const canSave =
    hasCredentials &&
    !!displayName.trim() &&
    (kind === "custom"
      ? !!baseURL.trim()
      : !!preset?.id && !!preset.version_id);
  function remember(save: Save | null, previous?: string) {
    recordPending(session, save, previous);
    if (mounted.current) setPending(save);
  }
  function changeKind(next: "preset" | "custom") {
    setKind(next);
    setSecret({});
    setName(undefined);
    setLogo(undefined);
    setNotice("");
  }
  async function saveConnection() {
    if (inFlight.current || uploading) return;
    let attempt = pending;
    if (!attempt) {
      if (!canSave) return;
      const request_id = crypto.randomUUID();
      const common = {
        name: displayName.trim(),
        logo_asset_id: logo,
        request_id,
      };
      attempt =
        kind === "custom"
          ? {
              kind,
              input: {
                ...common,
                config: {
                  adapter_key: "openai",
                  base_url: baseURL.trim(),
                  region: "overseas",
                  capabilities: ["text", "image"],
                  timeout_seconds: 60,
                  concurrency_limit: 10,
                  rate_limit_per_min: 60,
                },
              },
              credentialRequest: crypto.randomUUID(),
            }
          : {
              kind,
              presetID: preset!.id!,
              input: {
                ...common,
                version_id: preset!.version_id,
                candidate_keys: [],
                custom_candidates: [],
                selected_models_only: true,
              },
              credentialRequest: crypto.randomUUID(),
            };
    }
    inFlight.current = true;
    setBusy(true);
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
            attempt.kind === "preset"
              ? "preset.connect"
              : "provider.custom.connect",
            attempt.kind === "preset" ? attempt.presetID : zeroID,
          ));
        id = connectionID(existing) ?? undefined;
        if (existing && !id) throw new RequestError("request_unconfirmed");
        if (!id && mounted.current)
          id =
            connectionID(
              attempt.kind === "preset"
                ? await connectProviderPreset(
                    { id: attempt.presetID },
                    attempt.input,
                  )
                : await createCustomProviderConnection(attempt.input),
            ) ?? undefined;
        if (!id) throw new RequestError("request_unconfirmed");
        remember({ ...attempt, providerID: id }, attempt.input.request_id);
      }
      if (!mounted.current) return;
      const existing =
        pending &&
        (await findReceipt(attempt.credentialRequest, "credential.set", id));
      if (existing && !savedCredential(existing, id))
        throw new RequestError("request_unconfirmed");
      if (!existing) {
        if (!mounted.current) return;
        if (!hasCredentials) {
          setNotice("供应商已保存。请重新填写凭据以完成接入。");
          return;
        }
        writingCredential = true;
        const result = await setAdminCredential(
          { id },
          {
            label: "平台凭据",
            secret: Object.fromEntries(
              fields.flatMap((field) => {
                const value = field.name && secret[field.name]?.trim();
                return field.name && value ? [[field.name, value]] : [];
              }),
            ),
          },
          { headers: { "Idempotency-Key": attempt.credentialRequest } },
        );
        if (!savedCredential(result, id))
          throw new RequestError("request_unconfirmed");
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
          setNotice("供应商已保存。凭据未通过校验，请重新填写。");
        } else if (
          !id &&
          error instanceof RequestError &&
          ((error.status >= 400 &&
            error.status < 500 &&
            error.status !== 409) ||
            [
              "revision_conflict",
              "preset_unavailable",
              "invalid_preset",
            ].includes(error.code))
        ) {
          remember(null, attempt.input.request_id);
          setNotice(catalogMessage(error));
        } else
          setNotice(
            id
              ? "供应商已保存，凭据结果待确认。可核对原请求并继续。"
              : "接入结果待确认。可核对原请求并继续。",
          );
      }
      return;
    } finally {
      inFlight.current = false;
      if (mounted.current) setBusy(false);
    }
    if (id && mounted.current) {
      setSecret({});
      onCreated(id);
      onSaved(id);
    }
  }
  const credentialFields = fields.map((field) => {
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
          required={field.required !== false}
          value={secret[name] ?? ""}
          disabled={busy}
          onChange={(event) =>
            setSecret({ ...secret, [name]: event.target.value })
          }
        />
      </Field>
    );
  });
  return (
    <form
      className="flex min-h-0 min-w-0 flex-col gap-4"
      onSubmit={(event) => {
        event.preventDefault();
        void saveConnection();
      }}
    >
      <Tabs
        value={kind}
        onValueChange={(value) => changeKind(value as "preset" | "custom")}
        className="min-h-0"
      >
        <TabsList aria-label="供应商接入方式">
          <TabsTrigger value="preset" disabled={busy || uploading || !!pending}>
            预设
          </TabsTrigger>
          <TabsTrigger value="custom" disabled={busy || uploading || !!pending}>
            自定义
          </TabsTrigger>
        </TabsList>
        {notice ? (
          <Alert role="status">
            <AlertDescription>{notice}</AlertDescription>
          </Alert>
        ) : null}
        <div className="min-h-0 overflow-y-auto p-px">
          <TabsContent value="preset">
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="connect-supplier">供应商</FieldLabel>
                <Select
                  value={preset?.id ?? ""}
                  onValueChange={(id) => {
                    setPresetID(id);
                    setName(undefined);
                    setSecret({});
                    setLogo(undefined);
                    setNotice("");
                  }}
                  disabled={
                    busy || uploading || !!pending || available.length === 0
                  }
                >
                  <SelectTrigger id="connect-supplier" className="w-full">
                    <SelectValue placeholder="暂无可用供应商预设">
                      {preset?.document?.name ?? preset?.slug}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {available.map((item) => (
                        <SelectItem key={item.id} value={item.id!}>
                          {item.document?.name ?? item.slug ?? item.id}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </Field>
              <Field>
                <FieldLabel htmlFor="connect-name">名称</FieldLabel>
                <Input
                  id="connect-name"
                  value={displayName}
                  required
                  maxLength={100}
                  disabled={busy || uploading || !!pending}
                  onChange={(event) => setName(event.target.value)}
                />
              </Field>
              {credentialFields}
            </FieldGroup>
          </TabsContent>
          <TabsContent value="custom">
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="custom-name">名称</FieldLabel>
                <Input
                  id="custom-name"
                  value={name ?? ""}
                  required
                  maxLength={100}
                  disabled={busy || uploading || !!pending}
                  onChange={(event) => setName(event.target.value)}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="custom-protocol">协议</FieldLabel>
                <Select value="openai" disabled>
                  <SelectTrigger id="custom-protocol" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem value="openai">OpenAI 兼容</SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </Field>
              <Field>
                <FieldLabel htmlFor="custom-base-url">请求地址</FieldLabel>
                <Input
                  id="custom-base-url"
                  type="url"
                  value={baseURL}
                  required
                  placeholder="https://api.example.com/v1"
                  disabled={busy || uploading || !!pending}
                  onChange={(event) => setBaseURL(event.target.value)}
                />
              </Field>
              {credentialFields}
            </FieldGroup>
          </TabsContent>
          <ProviderLogoPicker
            key={kind + (preset?.id ?? "")}
            adapterKey={kind === "custom" ? "openai" : config?.adapter_key}
            value={logo}
            onChange={setLogo}
            disabled={busy || uploading || !!pending}
            onUploadingChange={setUploading}
          />
        </div>
      </Tabs>
      <Separator />
      <DialogFooter className="shrink-0 pt-0 max-md:[&_button]:min-h-11">
        <Button
          type="button"
          variant="ghost"
          disabled={busy}
          onClick={() => {
            setSecret({});
            onCancel?.();
          }}
        >
          取消
        </Button>
        <Button
          type="submit"
          disabled={busy || uploading || (!pending && !canSave)}
        >
          {busy ? "正在保存…" : pending ? "核对并继续保存" : "保存供应商"}
        </Button>
      </DialogFooter>
    </form>
  );
}
