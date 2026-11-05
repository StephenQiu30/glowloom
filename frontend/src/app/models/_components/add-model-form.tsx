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
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
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
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

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
  const [requestingDirectory, setRequestingDirectory] = useState(false);
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
  const capabilities = (config?.config?.capabilities ?? [])
    .map((item) => `${item}.generate`)
    .filter(
      (item) =>
        item === "text.generate" ||
        item === "image.generate" ||
        (item === "video.generate" && adapter === "volcengine_ark"),
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
  );
  const check =
    probe.data?.current &&
    probe.data.config_version_id === config?.id &&
    probe.data.credential_id === credentialID
      ? probe.data
      : undefined;
  const online = check?.candidates ?? [];
  const readingDirectory = requestingDirectory || probe.running;
  const savingModel =
    addition.busy || (credential.busy && !requestingDirectory);
  const busy = savingModel || probe.mutation.busy || readingDirectory;
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
    setCandidateKey(model.key ?? "");
    setManual(!!model.requires_model_selection);
    setCustomID("");
    setCapability(model.capability ?? "");
  }

  function chooseManual(modelID = "") {
    setCandidateKey("");
    setManual(true);
    setCustomID(modelID);
    setCapability(capabilities.length === 1 ? capabilities[0] : "");
  }

  async function readDirectory() {
    setRequestingDirectory(true);
    try {
      const id = await ensureCredential();
      if (id) await probe.run(id);
    } finally {
      setRequestingDirectory(false);
    }
  }

  const directoryMessage = readingDirectory
    ? "正在读取模型目录…"
    : probe.error
      ? catalogMessage(probe.error)
      : check?.auth_result === "auth_failed"
        ? "供应商拒绝了凭据，请检查 Key。"
        : check?.discovery_result === "ok" ||
            check?.discovery_result === "partial"
          ? `目录${check.discovery_result === "partial" ? "部分" : ""}返回 ${online.length} 个候选。已有预设及模型配置继续保留。`
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
      onSubmit={async (event) => {
        event.preventDefault();
        if (
          locked ||
          !selectable ||
          !config?.id ||
          !config.model_catalog?.version
        )
          return;
        if (existing && existing.id) {
          onAdded(existing.id);
          return;
        }
        if (!(await ensureCredential())) return;
        await addition.run("model.preset.add", providerID, (requestID) =>
          addProviderPresetModel(
            { id: providerID },
            {
              ...(manual
                ? { custom_model_id: customID.trim(), capability }
                : { candidate_key: selected?.key }),
              config_id: config.id,
              catalog_version: config.model_catalog!.version,
              request_id: requestID,
            },
          ),
        );
      }}
    >
      <FieldGroup>
        <Field>
          <FieldLabel htmlFor={`${field}-provider`}>供应商</FieldLabel>
          <Select
            value={providerID}
            disabled={locked}
            onValueChange={(value) => {
              setProviderID(value);
              setCandidateKey("");
              setManual(false);
              setCustomID("");
              setCapability("");
              setAPIKey("");
              setAccessKey("");
            }}
          >
            <SelectTrigger id={`${field}-provider`} className="w-full">
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
        <Field>
          <Tabs defaultValue="presets" className="gap-3">
            <TabsList aria-label="模型来源" className="w-full">
              <TabsTrigger value="presets">推荐预设</TabsTrigger>
              <TabsTrigger value="directory">在线目录</TabsTrigger>
            </TabsList>
            <TabsContent value="presets">
              {isLoading ? (
                <p role="status" className="text-sm text-muted-foreground">
                  正在读取预设…
                </p>
              ) : !config ? (
                <p className="text-sm text-muted-foreground">
                  请先选择供应商连接。
                </p>
              ) : (
                <Command label="搜索预设" className="border">
                  <CommandInput placeholder="搜索预设名称或模型 ID" />
                  <CommandList className="max-h-44">
                    <CommandEmpty>没有匹配的预设</CommandEmpty>
                    {[
                      ...new Set(
                        presets.map((model) => model.capability ?? ""),
                      ),
                    ].map((kind) => (
                      <CommandGroup
                        key={kind}
                        heading={capabilityLabels[kind] ?? kind}
                      >
                        {presets
                          .filter((model) => model.capability === kind)
                          .map((model) => (
                            <CommandItem
                              key={model.key}
                              value={model.key}
                              keywords={[
                                model.display_name ?? "",
                                model.provider_model_id ?? "",
                              ]}
                              onSelect={() => choosePreset(model)}
                              disabled={
                                locked ||
                                (model.requires_model_selection &&
                                  !capabilities.includes(kind))
                              }
                            >
                              <span className="min-w-0 flex-1 truncate">
                                {model.display_name}
                              </span>
                              {model.key === candidateKey ? (
                                <Badge variant="secondary">已选</Badge>
                              ) : null}
                              {matchingModel(
                                model.provider_model_id,
                                model.capability,
                              ) ? (
                                <Badge variant="secondary">已添加</Badge>
                              ) : model.requires_model_selection ? (
                                <Badge variant="outline">需填写接入点</Badge>
                              ) : null}
                            </CommandItem>
                          ))}
                      </CommandGroup>
                    ))}
                  </CommandList>
                </Command>
              )}
            </TabsContent>
            <TabsContent
              value="directory"
              className="flex flex-col items-start gap-2"
            >
              {directoryAdapters.has(adapter) ? (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={!config || locked || (keyRequired && !hasKeyInput)}
                  onClick={() => void readDirectory()}
                >
                  {readingDirectory ? (
                    <Spinner data-icon="inline-start" />
                  ) : null}
                  {readingDirectory
                    ? "正在读取目录…"
                    : keyRequired
                      ? "保存 Key 并读取目录"
                      : "读取在线目录"}
                </Button>
              ) : (
                <p className="text-sm text-muted-foreground">
                  {connection
                    ? "此连接未提供在线目录，可从推荐预设添加。"
                    : "请先选择供应商连接。"}
                </p>
              )}
              {directoryMessage ? (
                <p className="text-xs text-muted-foreground" role="status">
                  {directoryMessage}
                </p>
              ) : null}
              {online.length ? (
                <Command label="搜索在线目录" className="w-full border">
                  <CommandInput placeholder="搜索在线模型" />
                  <CommandList className="max-h-44">
                    <CommandEmpty>没有匹配的在线模型</CommandEmpty>
                    <CommandGroup heading="在线模型">
                      {online.map((model) => (
                        <CommandItem
                          key={model.provider_model_id}
                          value={model.provider_model_id}
                          keywords={[model.display_name ?? ""]}
                          disabled={locked}
                          onSelect={() => {
                            const preset = presets.find(
                              (item) =>
                                item.provider_model_id ===
                                model.provider_model_id,
                            );
                            if (preset) choosePreset(preset);
                            else chooseManual(model.provider_model_id);
                          }}
                        >
                          <span className="min-w-0 flex-1 truncate">
                            {model.display_name || model.provider_model_id}
                          </span>
                          {config?.configured_models?.some(
                            (item) =>
                              item.provider_model_id ===
                              model.provider_model_id,
                          ) ? (
                            <Badge variant="secondary">已添加</Badge>
                          ) : null}
                        </CommandItem>
                      ))}
                    </CommandGroup>
                  </CommandList>
                </Command>
              ) : null}
            </TabsContent>
          </Tabs>
          {capabilities.length ? (
            <Button
              type="button"
              variant="link"
              className="h-auto w-fit px-0"
              disabled={locked}
              onClick={() => chooseManual()}
            >
              手动输入模型 ID
            </Button>
          ) : null}
          {selected && !manual ? (
            <p className="text-xs text-muted-foreground" role="status">
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
        </Field>
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
                disabled={locked}
                required
                onChange={(e) => setCustomID(e.target.value)}
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
        <Button
          className="w-fit"
          disabled={
            locked || !selectable || (!existing && keyRequired && !hasKeyInput)
          }
        >
          {savingModel ? <Spinner data-icon="inline-start" /> : null}
          {existing ? "查看模型" : "保存模型"}
        </Button>
        {credential.pending || credential.message ? (
          <MutationStatus mutation={credential} inline />
        ) : null}
        {addition.pending || addition.message ? (
          <MutationStatus mutation={addition} inline />
        ) : null}
      </FieldGroup>
    </form>
  );
}
