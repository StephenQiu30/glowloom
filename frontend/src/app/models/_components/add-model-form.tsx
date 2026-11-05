"use client";

import Link from "next/link";
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
import { providerConnectionName } from "@/lib/provider-connection-name";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
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
  const [candidateKey, setCandidateKey] = useState("");
  const [configured, setConfigured] = useState("");
  const [apiKey, setAPIKey] = useState("");
  const [accessKey, setAccessKey] = useState("");
  const provider = providers.find((item) => item.provider?.id === providerID);
  const keyRequired =
    !!provider &&
    provider.credential?.status !== "active" &&
    configured !== providerID;
  const signedCredential =
    provider?.provider?.adapter_key === "volcengine_music";
  const {
    data: config,
    error,
    isLoading,
  } = useSWR(provider ? ["add-model-config", actor.id, providerID] : null, () =>
    getProviderConfig({ id: providerID }),
  );
  const presets =
    config?.model_catalog?.models?.filter(
      (model) => !model.requires_model_selection,
    ) ?? [];
  const selected = presets.find((model) => model.key === candidateKey);
  const credential = useCatalogMutation(
    (result) => {
      const { provider_id } = result as API.ProviderCredentialSummary;
      setConfigured(provider_id!);
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
  const busy = credential.busy || addition.busy;
  const locked = busy || !!addition.pending || !!credential.pending;

  return (
    <form
      onSubmit={async (event) => {
        event.preventDefault();
        if (
          locked ||
          !selected?.key ||
          !config?.id ||
          !config.model_catalog?.version
        )
          return;
        if (keyRequired) {
          const secret = signedCredential
            ? { access_key_id: accessKey, secret_access_key: apiKey }
            : { api_key: apiKey };
          setAPIKey("");
          setAccessKey("");
          const saved = await credential.run(
            "credential.set",
            providerID,
            (id) =>
              setAdminCredential(
                { id: providerID },
                { label: "平台统一 Key", secret },
                { headers: { "Idempotency-Key": id } },
              ),
          );
          if (!saved) return;
        }
        await addition.run("model.preset.add", providerID, (requestID) =>
          addProviderPresetModel(
            { id: providerID },
            {
              candidate_key: selected.key,
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
              setAPIKey("");
              setAccessKey("");
            }}
          >
            <SelectTrigger id={`${field}-provider`} className="w-full">
              <SelectValue placeholder="选择供应商" />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {providers.map(({ provider }) =>
                  provider?.id ? (
                    <SelectItem key={provider.id} value={provider.id}>
                      {providerConnectionName(provider)}
                    </SelectItem>
                  ) : null,
                )}
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor={`${field}-preset`}>模型预设</FieldLabel>
          <Select
            value={selected?.key ?? ""}
            disabled={locked || !presets.length}
            onValueChange={setCandidateKey}
          >
            <SelectTrigger id={`${field}-preset`} className="w-full">
              <SelectValue
                placeholder={isLoading ? "正在读取预设…" : "选择模型预设"}
              />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {presets.map((model) => (
                  <SelectItem key={model.key} value={model.key!}>
                    {model.display_name}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
        {keyRequired ? (
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
        {config && !presets.length ? (
          <p className="text-sm text-muted-foreground">
            该连接暂无可直接添加的模型预设。
          </p>
        ) : null}
        <Button
          className="w-fit"
          disabled={
            locked ||
            !selected ||
            (keyRequired && (!apiKey || (signedCredential && !accessKey)))
          }
        >
          {busy ? <Spinner data-icon="inline-start" /> : null}
          保存模型
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
