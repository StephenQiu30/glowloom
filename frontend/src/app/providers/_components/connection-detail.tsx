"use client";
import { ConnectionTests } from "./connection-tests";
import useSWR from "swr";
import { useSession } from "@/components/auth/session";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from "@/components/ui/collapsible";
import { LoadingState } from "@/components/layout/loading-state";
import { ContentState } from "@/components/layout/content-state";
import { ChevronDown, Eye, EyeOff } from "lucide-react";
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
import { Badge } from "@/components/ui/badge";
import { Field, FieldLabel, FieldGroup } from "@/components/ui/field";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useState, useId } from "react";
import { providerConnectionName } from "@/lib/provider-connection-name";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { Separator } from "@/components/ui/separator";
import { isCredentialReceipt } from "@/components/catalog/credential-receipt";
import { RequestError } from "@/lib/request";
import {
  getAdminProvider,
  getProviderConfig,
  setAdminCredential,
  previewProviderUpgrade,
  applyProviderPreset,
  listProviderCredentials,
  disableAdminCredential,
  updateAdminProvider,
} from "@/api/settings";
import {
  catalogMessage,
  MutationStatus,
  useCatalogMutation,
} from "@/components/catalog/mutation";

export function ConnectionDetail({
  id,
  presets,
  onSaved,
  revision = 0,
  initialModel,
  initialRelease,
  initialMode,
}: {
  id: string;
  revision?: number;
  initialModel?: string;
  initialRelease?: string;
  initialMode?: string;
  presets: API.Preset[];
  onSaved: () => void;
}) {
  const formFieldId = useId();
  const { actor } = useSession();
  const [apiKey, setAPIKey] = useState("");
  const [accessKeyID, setAccessKeyID] = useState("");
  const [visible, setVisible] = useState(false);
  const [editKey, setEditKey] = useState(false);
  const [advanced, setAdvanced] = useState(false);
  const [error, setError] = useState("");
  const [target, setTarget] = useState("");
  const [preview, setPreview] = useState<API.ConfigUpgradePreview | null>(null);
  const [resolution, setResolution] = useState("");
  const {
    data: detail,
    error: detailError,
    mutate,
  } = useSWR(["admin-provider", actor.id, id, revision], () =>
    getAdminProvider({ id }),
  );
  const {
    data: settings,
    error: settingsError,
    mutate: reloadSettings,
  } = useSWR(
    advanced ? ["admin-provider-settings", actor.id, id, revision] : null,
    async () => {
      const [config, credentials] = await Promise.all([
        getProviderConfig({ id }).catch((error) => {
          if (error instanceof RequestError && error.status === 404)
            return null;
          throw error;
        }),
        listProviderCredentials({ id }),
      ]);
      return { config, credentials };
    },
  );
  function refresh() {
    void mutate();
    if (advanced) void reloadSettings();
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
  const config = settings?.config;
  const credentials = settings?.credentials ?? [];

  return (
    <section
      className="flex min-h-0 min-w-0 flex-1 flex-col gap-4 overflow-y-auto"
      aria-label="连接详情"
    >
      <header className="flex flex-col gap-2 pr-8">
        <h2
          data-connection-title
          tabIndex={-1}
          className="text-lg font-medium outline-none"
        >
          {providerConnectionName(provider) ?? "连接详情"}
        </h2>
        {provider ? (
          <div className="flex flex-wrap gap-2">
            <Badge variant="secondary">
              {provider.status === "active" ? "已启用" : "未启用"}
            </Badge>
            <Badge variant="outline">
              {credentialID ? "已配置" : "待配置"}
            </Badge>
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
          {onboardingPending ? (
            <MutationStatus mutation={onboarding} inline />
          ) : null}
          {mutation.message || mutation.pending ? (
            <MutationStatus mutation={mutation} inline />
          ) : null}
          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
          {!credentialID || editKey ? (
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
              <span className="text-sm font-medium">API Key</span>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setEditKey(true)}
              >
                更换 Key
              </Button>
            </div>
          )}
          {credentialID ? (
            <>
              <Separator />
              <ConnectionTests
                id={id}
                adapter={provider.adapter_key}
                credentialID={credentialID}
                revision={revision}
                onSaved={refresh}
                initialModel={initialModel}
                initialRelease={initialRelease}
                initialMode={initialMode}
              />
            </>
          ) : null}
          <Separator />
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="outline" asChild>
              <Link href={`/models?provider=${encodeURIComponent(id)}`}>
                模型管理
              </Link>
            </Button>
            {provider?.status === "active" ? (
              <AlertDialog>
                <AlertDialogTrigger asChild>
                  <Button variant="ghost" className="max-md:h-11">
                    停用连接
                  </Button>
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>停用连接？</AlertDialogTitle>
                    <AlertDialogDescription>
                      停止创建新任务。已有任务继续使用绑定的配置和 Key 恢复。
                    </AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>取消</AlertDialogCancel>
                    <AlertDialogAction
                      variant="outline"
                      disabled={mutation.busy}
                      onClick={() =>
                        void mutation.run("provider.update", id, (requestID) =>
                          updateAdminProvider(
                            { id },
                            {
                              expected_revision: provider.revision,
                              status: "disabled",
                            },
                            { headers: { "Idempotency-Key": requestID } },
                          ),
                        )
                      }
                    >
                      停用连接（停止新任务）
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            ) : null}
          </div>
          <Collapsible open={advanced} onOpenChange={setAdvanced}>
            <CollapsibleTrigger asChild>
              <Button
                variant="ghost"
                className="group w-full justify-between px-0"
              >
                连接设置
                <ChevronDown
                  data-icon="inline-end"
                  className="transition-transform group-data-[state=open]:rotate-180"
                />
              </Button>
            </CollapsibleTrigger>
            <CollapsibleContent className="flex flex-col gap-4">
              {settingsError ? (
                <Alert variant="destructive">
                  <AlertDescription>
                    {catalogMessage(settingsError)}
                  </AlertDescription>
                </Alert>
              ) : null}
              {!settings && !settingsError ? (
                <LoadingState>正在加载连接设置…</LoadingState>
              ) : null}
              {config ? (
                <div className="flex flex-col gap-4">
                  <FieldGroup>
                    <Field>
                      <FieldLabel>绑定预设版本</FieldLabel>
                      <p className="text-sm break-all">
                        {presets.find(
                          (p) => p.version_id === config.preset_version_id,
                        )?.document?.name ??
                          config.preset_version_id ??
                          "未知"}{" "}
                        · 配置 v{config.version ?? "未知"}
                      </p>
                    </Field>
                    <Field>
                      <FieldLabel>服务端点</FieldLabel>
                      <p className="text-sm break-all">
                        {config.config?.base_url ?? "未知"}
                      </p>
                    </Field>
                  </FieldGroup>
                  <h3 className="font-medium">配置升级</h3>
                  <p className="text-xs text-muted-foreground">
                    仅升级连接配置。模型模板用于新建连接，已有模型的参数和价格需单独发布。
                  </p>
                  <p className="text-xs text-muted-foreground">
                    保存配置只更新候选快照。现有模型发布和任务继续使用原快照。
                  </p>
                  <Field>
                    <FieldLabel htmlFor={`${formFieldId}-field-2`}>
                      <span>目标预设版本</span>
                    </FieldLabel>
                    <Select
                      value={target}
                      onValueChange={(value) => {
                        setTarget(value);
                        setPreview(null);
                      }}
                    >
                      <SelectTrigger
                        id={`${formFieldId}-field-2`}
                        className="mt-1 h-10 w-full"
                      >
                        <SelectValue placeholder={"选择同一预设的新版本"} />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectGroup>
                          {presets
                            .filter(
                              (p) =>
                                !p.archived &&
                                !p.emergency_disabled &&
                                p.document?.config?.adapter_key ===
                                  provider?.adapter_key,
                            )
                            .map((p) =>
                              p.version_id ? (
                                <SelectItem
                                  key={p.version_id}
                                  value={p.version_id}
                                >
                                  {p.document?.name} v{p.version}
                                </SelectItem>
                              ) : null,
                            )}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </Field>
                  <Button
                    variant="outline"
                    disabled={!target || mutation.busy}
                    onClick={async () => {
                      try {
                        const result = await previewProviderUpgrade(
                          { id },
                          {
                            expected_revision: provider?.revision,
                            expected_config_id: config.id,
                            preset_version_id: target,
                          },
                        );
                        setPreview(result);
                        setResolution(JSON.stringify(result.proposed, null, 2));
                      } catch (error) {
                        setError(catalogMessage(error));
                      }
                    }}
                  >
                    预览差异
                  </Button>
                  {preview ? (
                    <>
                      <p className="text-sm">
                        {preview.conflicts?.length
                          ? `需处理冲突：${preview.conflicts.join("、")}`
                          : "没有覆盖冲突。请确认最终配置。"}
                      </p>
                      <Field>
                        <FieldLabel htmlFor={`${formFieldId}-field-3`}>
                          <span>最终配置</span>
                        </FieldLabel>
                        <Textarea
                          id={`${formFieldId}-field-3`}
                          className="mt-1 min-h-48 w-full"
                          value={resolution}
                          onChange={(e) => setResolution(e.target.value)}
                        />
                      </Field>
                      <Button
                        disabled={mutation.busy}
                        onClick={async () => {
                          try {
                            const resolved: API.ProviderConfig =
                              JSON.parse(resolution);
                            await mutation.run(
                              "config.upgrade",
                              id,
                              (requestID) =>
                                applyProviderPreset(
                                  { id },
                                  {
                                    request_id: requestID,
                                    preview_hash: preview.hash,
                                    upgrade: {
                                      expected_revision: provider?.revision,
                                      expected_config_id: config.id,
                                      preset_version_id: target,
                                    },
                                    resolution: resolved,
                                  },
                                ),
                            );
                          } catch (error) {
                            setError(catalogMessage(error));
                          }
                        }}
                      >
                        应用已确认配置
                      </Button>
                    </>
                  ) : null}
                </div>
              ) : null}

              {credentials.length ? (
                <section
                  className="flex flex-col gap-3"
                  aria-label="Key 轮换记录"
                >
                  <h3 className="font-medium">Key 记录</h3>
                  {credentials.map((key) => (
                    <div
                      key={key.id}
                      className="flex flex-wrap items-center justify-between gap-2 rounded border p-3 text-sm"
                    >
                      <span>
                        {key.status === "active"
                          ? "已配置"
                          : key.status === "draining"
                            ? "仅供已有任务恢复"
                            : "已停用"}
                        {key.create_time
                          ? ` · ${new Date(key.create_time).toLocaleString("zh-CN")}`
                          : ""}
                      </span>
                      {key.status !== "disabled" ? (
                        <AlertDialog>
                          <AlertDialogTrigger asChild>
                            <Button type="button" variant="ghost">
                              紧急停用
                            </Button>
                          </AlertDialogTrigger>
                          <AlertDialogContent>
                            <AlertDialogHeader>
                              <AlertDialogTitle>
                                紧急停用此 Key？
                              </AlertDialogTitle>
                              <AlertDialogDescription>
                                绑定此 Key
                                的历史任务也将无法继续查询。此操作不能撤销，后续需添加新
                                Key。
                              </AlertDialogDescription>
                            </AlertDialogHeader>
                            <AlertDialogFooter>
                              <AlertDialogCancel>取消</AlertDialogCancel>
                              <AlertDialogAction
                                variant="destructive"
                                size="sm"
                                disabled={mutation.busy}
                                onClick={() =>
                                  mutation.run(
                                    "credential.disable",
                                    key.id!,
                                    (request) =>
                                      disableAdminCredential(
                                        { id, credential_id: key.id! },
                                        {
                                          headers: {
                                            "Idempotency-Key": request,
                                          },
                                        },
                                      ),
                                  )
                                }
                              >
                                确认停用此 Key
                              </AlertDialogAction>
                            </AlertDialogFooter>
                          </AlertDialogContent>
                        </AlertDialog>
                      ) : null}
                    </div>
                  ))}
                </section>
              ) : null}
            </CollapsibleContent>
          </Collapsible>
        </>
      )}
    </section>
  );
}
