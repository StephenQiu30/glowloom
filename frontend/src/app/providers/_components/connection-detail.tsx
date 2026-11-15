"use client";
import { ConnectionLogoEditor } from "./connection-logo-editor";
import useSWR from "swr";
import { useSession } from "@/components/auth/session";
import { LoadingState } from "@/components/layout/loading-state";
import { ContentState } from "@/components/layout/content-state";
import { Eye, EyeOff } from "lucide-react";
import Link from "next/link";
import {
  InputGroup,
  InputGroupInput,
  InputGroupAddon,
  InputGroupButton,
} from "@/components/ui/input-group";
import {
  AlertDialog,
  AlertDialogTrigger,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
  AlertDialogAction,
} from "@/components/ui/alert-dialog";
import type { DialogTitle } from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import { Field, FieldLabel, FieldGroup } from "@/components/ui/field";
import { useState, useId } from "react";
import { providerConnectionName } from "@/lib/provider-connection-name";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Toolbar } from "@/components/ui/toolbar";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import { isCredentialReceipt } from "@/components/catalog/credential-receipt";
import { RequestError } from "@/lib/request";
import {
  getAdminProvider,
  setAdminCredential,
  updateAdminProvider,
} from "@/api/settings";
import {
  MutationStatus,
  useCatalogMutation,
} from "@/components/catalog/mutation";

export function ConnectionDetail({
  id,
  onSaved,
  onAddModel,
  revision = 0,
  initialRename = false,
  Title = "h2",
}: {
  id: string;
  revision?: number;
  initialRename?: boolean;
  Title?: typeof DialogTitle | "h2";
  onSaved: () => void;
  onAddModel?: (row: API.ProviderListItem, trigger: HTMLButtonElement) => void;
}) {
  const formFieldId = useId();
  const { actor } = useSession();
  const [apiKey, setAPIKey] = useState("");
  const [accessKeyID, setAccessKeyID] = useState("");
  const [visible, setVisible] = useState(false);
  const [renaming, setRenaming] = useState(initialRename);
  const [editKey, setEditKey] = useState(false);
  const {
    data: detail,
    error: detailError,
    mutate,
  } = useSWR(
    ["admin-provider", actor.id, id, revision],
    () => getAdminProvider({ id }),
    { keepPreviousData: true },
  );
  function refresh() {
    void mutate();
    onSaved();
  }
  const mutation = useCatalogMutation(refresh, `connection.${id}`);
  const onboarding = useCatalogMutation(
    refresh,
    "preset.onboarding.credential",
    isCredentialReceipt,
  );
  const onboardingPending =
    onboarding.pending?.action === "credential.set" &&
    onboarding.pending.target === id;
  const keyBusy = onboardingPending || mutation.busy || !!mutation.pending;
  const provider = detail?.provider;
  const credentialID =
    detail?.credential?.status === "active" ? detail.credential.id : undefined;
  const musicCredential = provider?.adapter_key === "volcengine_music";
  const archived = provider?.status === "archived";
  const name = providerConnectionName(provider) ?? "";

  const archiveAction = provider ? (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button
          variant={archived ? "outline" : "ghost"}
          size="toolbar"
          disabled={keyBusy}
        >
          {archived ? "恢复连接" : "归档连接"}
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {archived ? "恢复连接？" : "归档连接？"}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {archived
              ? "连接将恢复为未启用。启用后才能接收新任务。"
              : "归档后停止接收新任务，保留模型、凭据和任务记录。"}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>取消</AlertDialogCancel>
          <AlertDialogAction
            variant={archived ? "default" : "destructive"}
            disabled={keyBusy}
            onClick={() =>
              void mutation.run("provider.update", id, (requestID) =>
                updateAdminProvider(
                  { id },
                  {
                    expected_revision: provider.revision,
                    status: archived ? "disabled" : "archived",
                  },
                  { headers: { "Idempotency-Key": requestID } },
                ),
              )
            }
          >
            {archived ? "确认恢复" : "确认归档"}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  ) : null;
  return (
    <div className="flex min-h-0 min-w-0 flex-col gap-4 overflow-y-auto">
      <header className="flex flex-col gap-2 pr-8">
        <Title
          data-connection-title
          tabIndex={-1}
          className="break-words outline-none"
        >
          {providerConnectionName(provider) ?? "连接详情"}
        </Title>
        {provider ? (
          <div className="flex flex-wrap items-center gap-2">
            {archived ? <Badge variant="secondary">已归档</Badge> : null}
            {!renaming ? (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                disabled={keyBusy}
                onClick={() => setRenaming(true)}
              >
                编辑名称
              </Button>
            ) : null}
          </div>
        ) : null}
      </header>
      {detailError ? (
        <ContentState
          state="error"
          title="暂时无法加载连接"
          actions={
            <Button variant="outline" onClick={() => void mutate()}>
              重新加载
            </Button>
          }
        />
      ) : !provider ? (
        <LoadingState>正在加载连接…</LoadingState>
      ) : (
        <>
          <ConnectionLogoEditor
            provider={provider}
            disabled={keyBusy || archived}
            onSaved={refresh}
          />
          {onboardingPending ? (
            <MutationStatus mutation={onboarding} inline />
          ) : null}
          {mutation.message || mutation.pending ? (
            <MutationStatus mutation={mutation} inline />
          ) : null}
          {renaming ? (
            <form
              key={provider.revision}
              className="flex flex-col gap-2"
              onSubmit={async (event) => {
                event.preventDefault();
                const value = new FormData(event.currentTarget).get("name");
                if (
                  keyBusy ||
                  typeof value !== "string" ||
                  !value.trim() ||
                  value.trim() === name
                )
                  return;
                const saved = await mutation.run(
                  "provider.update",
                  id,
                  (requestID) =>
                    updateAdminProvider(
                      { id },
                      {
                        name: value.trim(),
                        expected_revision: provider.revision,
                      },
                      { headers: { "Idempotency-Key": requestID } },
                    ),
                );
                if (saved) setRenaming(false);
              }}
            >
              <Field>
                <FieldLabel htmlFor={`${formFieldId}-name`}>
                  连接名称
                </FieldLabel>
                <Input
                  id={`${formFieldId}-name`}
                  name="name"
                  defaultValue={name}
                  required
                  maxLength={100}
                  disabled={keyBusy}
                />
              </Field>
              <div className="flex justify-end gap-2">
                <Button
                  type="button"
                  variant="ghost"
                  disabled={mutation.busy}
                  onClick={() => setRenaming(false)}
                >
                  取消
                </Button>
                <Button
                  variant="outline"
                  className="self-end"
                  disabled={keyBusy}
                >
                  保存连接名称
                </Button>
              </div>
            </form>
          ) : null}
          {archived ? null : !credentialID || editKey ? (
            <form
              onSubmit={async (event) => {
                event.preventDefault();
                if (keyBusy) return;
                const secret: Record<string, string> = musicCredential
                  ? { access_key_id: accessKeyID, secret_access_key: apiKey }
                  : { api_key: apiKey };
                setAccessKeyID("");
                setAPIKey("");
                setVisible(false);
                const saved = await mutation.run(
                  "credential.set",
                  id,
                  (requestID) =>
                    setAdminCredential(
                      { id },
                      { label: "平台统一 Key", secret },
                      { headers: { "Idempotency-Key": requestID } },
                    ).then((result) => {
                      if (!isCredentialReceipt(result, id))
                        throw new RequestError("request_unconfirmed");
                      return result;
                    }),
                );
                if (saved) setEditKey(false);
              }}
            >
              <FieldGroup className="gap-3">
                {musicCredential && (
                  <Field>
                    <FieldLabel htmlFor={`${formFieldId}-access-key`}>
                      Access Key ID
                    </FieldLabel>
                    <InputGroup>
                      <InputGroupInput
                        id={`${formFieldId}-access-key`}
                        type="password"
                        required
                        autoComplete="off"
                        value={accessKeyID}
                        disabled={keyBusy}
                        onChange={(event) => setAccessKeyID(event.target.value)}
                      />
                    </InputGroup>
                  </Field>
                )}
                <Field>
                  <FieldLabel htmlFor={`${formFieldId}-field-1`}>
                    <span>
                      {musicCredential ? "Secret Access Key" : "API Key"}
                    </span>
                  </FieldLabel>
                  <InputGroup className="h-10 max-md:h-11">
                    <InputGroupInput
                      className="h-full"
                      id={`${formFieldId}-field-1`}
                      type={visible ? "text" : "password"}
                      disabled={keyBusy}
                      value={apiKey}
                      onChange={(event) => setAPIKey(event.target.value)}
                      autoComplete="off"
                      spellCheck={false}
                      required
                      placeholder={
                        detail.credential?.status === "active"
                          ? "已配置，输入新密钥以更换"
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
                        disabled={keyBusy}
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
                </Field>
                <div className="flex flex-wrap justify-end gap-2">
                  {credentialID ? (
                    <Button
                      type="button"
                      variant="ghost"
                      onClick={() => setEditKey(false)}
                    >
                      取消
                    </Button>
                  ) : null}
                  <Button disabled={keyBusy || !apiKey}>
                    {mutation.busy ? (
                      <Spinner data-icon="inline-start" />
                    ) : null}
                    {credentialID ? "保存新 Key" : "保存 API Key"}
                  </Button>
                </div>
              </FieldGroup>
            </form>
          ) : (
            <div className="flex items-center justify-between gap-2">
              <span className="text-sm font-medium">
                {musicCredential ? "密钥已配置" : "API Key 已配置"}
              </span>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setEditKey(true)}
              >
                更换 Key
              </Button>
            </div>
          )}
          <Field
            orientation="horizontal"
            className="pr-3"
            data-disabled={keyBusy || archived || undefined}
            aria-busy={mutation.busy}
          >
            <FieldLabel
              htmlFor={`${formFieldId}-enabled`}
              className="min-h-11 flex-1"
            >
              启用连接
            </FieldLabel>
            <Switch
              id={`${formFieldId}-enabled`}
              checked={provider.status === "active"}
              disabled={keyBusy || archived}
              onCheckedChange={(checked) =>
                void mutation.run("provider.update", id, (requestID) =>
                  updateAdminProvider(
                    { id },
                    {
                      expected_revision: provider.revision,
                      status: checked ? "active" : "disabled",
                    },
                    { headers: { "Idempotency-Key": requestID } },
                  ),
                )
              }
            />
          </Field>
          <Toolbar aria-label="连接操作" className="flex-wrap">
            <Button variant="outline" size="toolbar" asChild>
              <Link href={`/models?provider=${encodeURIComponent(id)}`}>
                模型管理
              </Link>
            </Button>
            {!archived ? (
              onAddModel ? (
                <Button
                  variant="outline"
                  size="toolbar"
                  onClick={(event) =>
                    onAddModel(
                      { provider, credential: detail?.credential },
                      event.currentTarget,
                    )
                  }
                >
                  接入模型
                </Button>
              ) : (
                <Button variant="outline" size="toolbar" asChild>
                  <Link
                    href={`/models?provider=${encodeURIComponent(id)}&add=1`}
                  >
                    添加模型
                  </Link>
                </Button>
              )
            ) : null}
            {archived ? archiveAction : null}
          </Toolbar>
        </>
      )}
    </div>
  );
}
