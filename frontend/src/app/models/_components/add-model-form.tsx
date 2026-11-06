"use client";

import Link from "next/link";
import { Select as SelectPrimitive } from "radix-ui";
import { providerLabel } from "@/app/providers/_components/provider-label";
import { useId, useState } from "react";
import useSWR from "swr";
import {
  addProviderPresetModel,
  getProviderConfig,
  setAdminCredential,
} from "@/api/settings";
import { useSession } from "@/components/auth/session";
import { isCredentialReceipt } from "@/components/catalog/credential-receipt";
import {
  catalogMessage,
  MutationStatus,
  useCatalogMutation,
} from "@/components/catalog/mutation";
import { useProviderCatalogProbe } from "@/components/catalog/provider-catalog-probe";
import { providerConnectionName } from "@/lib/provider-connection-name";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { DialogClose, DialogFooter } from "@/components/ui/dialog";
import { Separator } from "@/components/ui/separator";
import { ModelCatalogChoices } from "@/components/catalog/model-catalog-choices";
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
import { Spinner } from "@/components/ui/spinner";

const capabilityLabels: Record<string, string> = {
  "text.generate": "文本生成",
  "image.generate": "图片生成",
  "video.generate": "视频生成",
  "audio.generate": "音频生成",
};

const directoryAdapters = new Set([
  "openai",
  "openrouter",
  "vercel",
  "deepseek",
]);

export function AddModelForm({
  providers,
  initialProvider,
  onAdded,
  onCredentialSaved,
}: {
  providers: API.ProviderListItem[];
  initialProvider: string;
  onAdded: (id: string) => void;
  onCredentialSaved: () => void;
}) {
  const field = useId();
  const { actor } = useSession();
  const [providerID, setProviderID] = useState(initialProvider);
  const [savedCredential, setSavedCredential] =
    useState<API.ProviderCredentialSummary>();
  const [candidateKey, setCandidateKey] = useState("");
  const [directorySelection, setDirectorySelection] = useState<string>();
  const [manual, setManual] = useState(false);
  const [customID, setCustomID] = useState("");
  const [capability, setCapability] = useState("");
  const [apiKey, setAPIKey] = useState("");
  const [accessKey, setAccessKey] = useState("");
  const connection = providers.find((item) => item.provider?.id === providerID);
  const adapter = connection?.provider?.adapter_key ?? "";
  const credentialID =
    savedCredential?.provider_id === providerID
      ? savedCredential?.id
      : connection?.credential?.status === "active"
        ? connection.credential.id
        : undefined;
  const keyRequired = !!connection && !credentialID;
  const signedCredential = adapter === "volcengine_music";
  const hasKeyInput =
    !!apiKey.trim() && (!signedCredential || !!accessKey.trim());

  const {
    data: config,
    error,
    isLoading,
  } = useSWR(
    connection ? ["add-model-config", actor.id, providerID] : null,
    () => getProviderConfig({ id: providerID }),
  );
  const presets = config?.model_catalog?.models ?? [];
  const selected = presets.find((model) => model.key === candidateKey);
  const capabilities = (config?.config?.capabilities ?? []).map(
    (item) => item + ".generate",
  );
  const matchingModel = (modelID?: string, kind?: string) =>
    modelID &&
    config?.configured_models?.find(
      (model) =>
        model.provider_model_id === modelID && model.capability === kind,
    );
  const existing = manual
    ? matchingModel(customID.trim(), capability)
    : matchingModel(selected?.provider_model_id, selected?.capability);

  const credential = useCatalogMutation(
    (result) => {
      setSavedCredential(result as API.ProviderCredentialSummary);
      onCredentialSaved();
    },
    "preset.onboarding.credential",
    isCredentialReceipt,
  );
  const addition = useCatalogMutation(
    (result) => onAdded((result as API.CreatedModel).id!),
    "model.preset.add",
    (result, target) => {
      const model = result as API.CreatedModel | undefined;
      return !!model?.id && model.provider_id === target;
    },
  );
  const probe = useProviderCatalogProbe(
    providerID,
    `add-model.catalog.${providerID}`,
    true,
  );
  const check =
    probe.data?.kind === "discovery" &&
    probe.data.current &&
    probe.data.config_version_id === config?.id &&
    probe.data.credential_id === credentialID
      ? probe.data
      : undefined;
  const online = check?.candidates ?? [];
  const readingDirectory = probe.mutation.busy || probe.running;
  const savingModel = addition.busy || credential.busy;
  const busy = savingModel || readingDirectory;
  const locked =
    busy ||
    !!addition.pending ||
    !!credential.pending ||
    !!probe.mutation.pending;
  const selectable = manual
    ? !!customID.trim() && !!capability
    : !!selected?.key;

  async function ensureCredential() {
    if (credentialID) return credentialID;
    const secret = signedCredential
      ? { access_key_id: accessKey, secret_access_key: apiKey }
      : { api_key: apiKey };
    setAPIKey("");
    setAccessKey("");
    const saved = await credential.run("credential.set", providerID, (id) =>
      setAdminCredential(
        { id: providerID },
        { label: "平台统一 Key", secret },
        { headers: { "Idempotency-Key": id } },
      ),
    );
    return saved?.id;
  }

  function choosePreset(model: API.ModelCandidatePreset) {
    setCandidateKey(
      model.key === candidateKey && !manual ? "" : (model.key ?? ""),
    );
    setDirectorySelection(undefined);
    setManual(!!model.requires_model_selection);
    setCustomID("");
    setCapability(model.capability ?? "");
  }

  function chooseManual(modelID = "", directoryCheckID?: string) {
    setDirectorySelection(directoryCheckID);
    setCandidateKey("");
    setManual(true);
    setCustomID(modelID);
    setCapability(capabilities.length === 1 ? capabilities[0] : "");
  }

  const directoryMessage = readingDirectory
    ? "正在读取模型目录…"
    : probe.error
      ? catalogMessage(probe.error)
      : check?.auth_result === "auth_failed"
        ? "供应商拒绝了凭据，请检查 Key。"
        : check?.discovery_result === "ok" ||
            check?.discovery_result === "partial"
          ? `目录${check.discovery_result === "partial" ? "部分" : ""}返回 ${online.length} 个候选。`
          : check
            ? "目录暂不可读取，可以使用推荐预设或手动输入模型 ID。"
            : "";
  const adapters = [
    ...new Set(
      providers.flatMap(({ provider }) =>
        provider?.id ? [provider.adapter_key ?? ""] : [],
      ),
    ),
  ];
  return (
    <form
      className="flex min-h-0 flex-col gap-4"
      onSubmit={async (event) => {
        event.preventDefault();
        if (
          locked ||
          !selectable ||
          (!!directorySelection && directorySelection !== check?.id) ||
          !config?.id ||
          !config.model_catalog?.version
        )
          return;
        if (existing && existing.id) {
          onAdded(existing.id);
          return;
        }
        const savedID = await ensureCredential();
        if (!savedID) return;
        await addition.run("model.preset.add", providerID, (requestID) =>
          addProviderPresetModel(
            { id: providerID },
            {
              ...(manual
                ? { custom_model_id: customID.trim(), capability }
                : { candidate_key: selected?.key }),
              config_id: config.id,
              credential_id: savedID,
              directory_check_id: directorySelection,
              draft_only: true,
              catalog_version: config.model_catalog!.version,
              request_id: requestID,
            },
          ),
        );
      }}
    >
      <FieldGroup className="min-h-0 gap-4 overflow-y-auto p-px">
        {initialProvider && connection ? (
          <p className="text-sm font-medium">
            {providerConnectionName(connection.provider)}
          </p>
        ) : (
          <Field orientation="horizontal" className="justify-between">
            <FieldLabel htmlFor={`${field}-provider`} className="sr-only">
              供应商
            </FieldLabel>
            <Select
              value={providerID}
              disabled={locked}
              onValueChange={(value) => {
                setProviderID(value);
                setDirectorySelection(undefined);
                setCandidateKey("");
                setManual(false);
                setCustomID("");
                setCapability("");
                setAPIKey("");
                setAccessKey("");
              }}
            >
              <SelectTrigger
                id={`${field}-provider`}
                className="w-full max-w-[220px]"
              >
                <SelectValue placeholder="选择供应商连接">
                  {providerConnectionName(connection?.provider)}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {adapters.map((group) => {
                  const items = providers.filter(
                    ({ provider }) =>
                      provider?.id && (provider.adapter_key ?? "") === group,
                  );
                  return (
                    <SelectGroup key={group}>
                      <SelectPrimitive.Label className="px-2 py-1.5 text-xs text-muted-foreground">
                        {providerLabel(group)}
                      </SelectPrimitive.Label>
                      {items.map(({ provider, credential }) => (
                        <SelectItem key={provider!.id} value={provider!.id!}>
                          {providerConnectionName(provider)}
                          {credential?.status === "active" ||
                          savedCredential?.provider_id === provider?.id
                            ? " · 已配置 Key"
                            : " · 未配置 Key"}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  );
                })}
              </SelectContent>
            </Select>
          </Field>
        )}
        <div className="flex min-w-0 flex-col gap-3">
          {directoryAdapters.has(adapter) && credentialID ? (
            <div className="flex flex-wrap items-center gap-2">
              <Button
                type="button"
                variant="outline"
                className="max-md:min-h-11"
                disabled={!config || locked}
                onClick={() => void probe.run(credentialID)}
              >
                {readingDirectory && <Spinner data-icon="inline-start" />}
                {readingDirectory ? "正在读取目录…" : "读取在线目录"}
              </Button>
              {directoryMessage && (
                <p role="status" className="text-xs text-muted-foreground">
                  {directoryMessage}
                </p>
              )}
            </div>
          ) : null}
          {isLoading ? (
            <p role="status">正在读取预设…</p>
          ) : config ? (
            <ModelCatalogChoices
              key={config.id}
              models={presets}
              online={online.flatMap((item) =>
                item.provider_model_id ? [item.provider_model_id] : [],
              )}
              picked={candidateKey ? [candidateKey] : []}
              disabled={locked}
              canSelect={(model) =>
                !model.requires_model_selection ||
                capabilities.includes(model.capability ?? "")
              }
              isConfigured={(model) =>
                !!matchingModel(model.provider_model_id, model.capability)
              }
              onPreset={choosePreset}
              onOnline={(id) => chooseManual(id, check?.id)}
            />
          ) : null}
          {capabilities.length ? (
            <Button
              type="button"
              variant="ghost"
              className="w-fit max-md:min-h-11"
              disabled={locked}
              onClick={() => chooseManual()}
            >
              手动输入模型 ID
            </Button>
          ) : null}
          {selected && !manual ? (
            <p className="sr-only" role="status">
              已选择 {selected.display_name}
            </p>
          ) : null}
          {!probe.mutation.busy && probe.mutation.pending ? (
            <MutationStatus mutation={probe.mutation} inline />
          ) : null}
          {!probe.mutation.pending &&
          probe.mutation.message &&
          !probe.data &&
          !readingDirectory &&
          probe.mutation.message !== "已保存。" ? (
            <p className="text-sm text-muted-foreground">
              {probe.mutation.message}
            </p>
          ) : null}
        </div>
        {manual ? (
          <>
            <Field>
              <FieldLabel htmlFor={`${field}-model-id`}>
                上游模型或接入点 ID
              </FieldLabel>
              <Input
                id={`${field}-model-id`}
                value={customID}
                maxLength={200}
                className="max-md:min-h-11 max-md:text-base"
                disabled={locked}
                required
                onChange={(e) => {
                  setCustomID(e.target.value);
                  setDirectorySelection(undefined);
                }}
              />
            </Field>
            {capabilities.length > 1 ? (
              <Field>
                <FieldLabel htmlFor={`${field}-capability`}>
                  模型能力
                </FieldLabel>
                <Select
                  value={capability}
                  onValueChange={setCapability}
                  disabled={locked}
                >
                  <SelectTrigger id={`${field}-capability`} className="w-full">
                    <SelectValue placeholder="选择模型能力" />
                  </SelectTrigger>
                  <SelectContent>
                    {capabilities.map((item) => (
                      <SelectItem key={item} value={item}>
                        {capabilityLabels[item]}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            ) : null}
          </>
        ) : null}
        {keyRequired && !existing ? (
          <>
            {signedCredential ? (
              <Field>
                <FieldLabel htmlFor={`${field}-access`}>
                  Access Key ID
                </FieldLabel>
                <Input
                  id={`${field}-access`}
                  type="password"
                  autoComplete="off"
                  value={accessKey}
                  disabled={locked}
                  required
                  onChange={(e) => setAccessKey(e.target.value)}
                />
              </Field>
            ) : null}
            <Field>
              <FieldLabel htmlFor={`${field}-key`}>
                {signedCredential ? "Secret Access Key" : "API Key"}
              </FieldLabel>
              <Input
                id={`${field}-key`}
                type="password"
                autoComplete="off"
                value={apiKey}
                disabled={locked}
                required
                onChange={(e) => setAPIKey(e.target.value)}
              />
            </Field>
          </>
        ) : null}
        {error ? (
          <Alert variant="destructive">
            <AlertDescription>{catalogMessage(error)}</AlertDescription>
          </Alert>
        ) : null}
        {!providers.length ? (
          <Button asChild variant="link" className="w-fit px-0">
            <Link href="/providers">接入供应商</Link>
          </Button>
        ) : null}
        {credential.pending || credential.message ? (
          <MutationStatus mutation={credential} inline />
        ) : null}
        {addition.pending || addition.message ? (
          <MutationStatus mutation={addition} inline />
        ) : null}
      </FieldGroup>
      <Separator />
      <DialogFooter className="shrink-0 pt-0 max-md:[&_button]:min-h-11">
        <DialogClose asChild>
          <Button type="button" variant="ghost">
            取消
          </Button>
        </DialogClose>
        <Button
          disabled={
            locked ||
            !selectable ||
            (!!directorySelection && directorySelection !== check?.id) ||
            (!existing && keyRequired && !hasKeyInput)
          }
        >
          {savingModel ? <Spinner data-icon="inline-start" /> : null}
          {existing ? "查看模型" : "保存模型"}
        </Button>
      </DialogFooter>
    </form>
  );
}
