"use client";
import { ConnectionOnboarding } from "./connection-onboarding";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { LoadingState } from "@/components/layout/loading-state";
import { ChevronDown, Eye, EyeOff } from "lucide-react";
import Link from "next/link";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
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
import {
  Empty,
  EmptyHeader,
  EmptyTitle,
  EmptyContent,
} from "@/components/ui/empty";
import {
  Item,
  ItemGroup,
  ItemContent,
  ItemTitle,
  ItemActions,
} from "@/components/ui/item";
import { Spinner } from "@/components/ui/spinner";
import { Badge } from "@/components/ui/badge";
import { regionLabel } from "./connection-list";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
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
import { useEffect, useState, useId } from "react";
import { Button } from "@/components/ui/button";
import { isCredentialReceipt } from "@/components/catalog/credential-receipt";
import { RequestError } from "@/lib/request";
import {
  getAdminProvider,
  getProviderConfig,
  setAdminCredential,
  testAdminCredential,
  getProviderCheck,
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
}: {
  id: string;
  revision?: number;
  presets: API.Preset[];
  onSaved: () => void;
}) {
  const formFieldId = useId();

  const [detail, setDetail] = useState<API.ProviderDetail | null>(null);
  const [config, setConfig] = useState<API.ConnectionConfig | null>(null);
  const [credentials, setCredentials] = useState<
    API.ProviderCredentialSummary[]
  >([]);
  const [apiKey, setAPIKey] = useState("");
  const [accessKeyID, setAccessKeyID] = useState("");
  const [visible, setVisible] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [error, setError] = useState("");
  const [check, setCheck] = useState<API.ProviderCheck | null>(null);
  const [checkID, setCheckID] = useState("");
  const [checking, setChecking] = useState(false);
  const [target, setTarget] = useState("");
  const [preview, setPreview] = useState<API.ConfigUpgradePreview | null>(null);
  const [resolution, setResolution] = useState("");
  const mutation = useCatalogMutation(onSaved, `connection.${id}`);
  const onboarding = useCatalogMutation(
    onSaved,
    "preset.onboarding.credential",
    isCredentialReceipt,
  );
  const onboardingPending =
    onboarding.pending?.action === "credential.set" &&
    onboarding.pending.target === id;
  useEffect(() => {
    const abort = new AbortController();
    Promise.all([
      getAdminProvider({ id }, { signal: abort.signal }),
      getProviderConfig({ id }, { signal: abort.signal }).catch((error) => {
        if (error instanceof RequestError && error.status === 404) return null;
        throw error;
      }),
      listProviderCredentials({ id }, { signal: abort.signal }),
    ])
      .then(([d, c, keys]) => {
        if (!abort.signal.aborted) {
          setDetail(d);
          setConfig(c);
          setCredentials(keys);
        }
      })
      .catch((error) => {
        if (!abort.signal.aborted) setError(catalogMessage(error));
      });
    return () => abort.abort();
  }, [id, revision, attempt]);
  useEffect(() => {
    if (!checkID) return;
    const abort = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      try {
        const result = await getProviderCheck(
          { id: checkID },
          { signal: abort.signal },
        );
        if (abort.signal.aborted) return;
        setCheck(result);
        if (result.status === "pending" || result.status === "running")
          timer = setTimeout(poll, 3000);
        else setChecking(false);
      } catch (error) {
        if (!abort.signal.aborted) {
          setError(catalogMessage(error));
          setChecking(false);
        }
      }
    }
    void poll();
    return () => {
      abort.abort();
      clearTimeout(timer);
    };
  }, [checkID]);
  const provider = detail?.provider;
  const musicCredential = provider?.adapter_key === "volcengine_music";
  return (
    <section className="flex min-w-0 flex-col gap-6" aria-label="连接详情">
      <h2 className="text-lg font-medium">{provider?.name ?? "连接详情"}</h2>
      {error ? <FeedbackToast message={error} /> : null}
      {!detail ? (
        error ? (
          <Empty>
            <EmptyHeader>
              <EmptyTitle>暂时无法加载连接</EmptyTitle>
            </EmptyHeader>
            <EmptyContent>
              <Button
                variant="outline"
                onClick={() => {
                  setError("");
                  setAttempt((v) => v + 1);
                }}
              >
                重新加载
              </Button>
            </EmptyContent>
          </Empty>
        ) : (
          <LoadingState>正在加载连接…</LoadingState>
        )
      ) : (
        <>
          <p className="text-sm text-muted-foreground">
            {provider?.status === "active"
              ? "已启用"
              : provider?.status === "disabled"
                ? "未启用"
                : "状态未知"}{" "}
            · {regionLabel(provider?.region)}
          </p>
          {onboardingPending ? (
            <>
              <p className="text-sm">
                添加连接时的 Key
                写入尚未确认，请先查询原回执，再进行新的保存或轮换。
              </p>
              <MutationStatus mutation={onboarding} />
            </>
          ) : null}
          {mutation.message || mutation.pending ? (
            <MutationStatus mutation={mutation} />
          ) : null}
          <Tabs defaultValue="overview" className="gap-6">
            <TabsList className="w-full max-md:h-11" aria-label="连接详情视图">
              <TabsTrigger value="overview">概览</TabsTrigger>
              <TabsTrigger value="key">API Key</TabsTrigger>
              <TabsTrigger value="config">高级配置</TabsTrigger>
            </TabsList>
            <TabsContent value="overview" className="flex flex-col gap-6">
              <Item size="xs" className="px-0">
                <ItemContent>
                  <ItemTitle>密钥配置</ItemTitle>
                </ItemContent>
                <ItemActions>
                  <Badge variant="secondary">
                    {detail.credential?.status === "active"
                      ? "已配置"
                      : detail.credential?.status === "draining"
                        ? "仅供恢复"
                        : detail.credential?.status === "disabled"
                          ? "已停用"
                          : "未配置"}
                  </Badge>
                </ItemActions>
              </Item>
              {[
                "volcengine_music",
                "volcengine_avatar",
                "volcengine_motion",
              ].includes(provider?.adapter_key ?? "") ? (
                <p className="text-sm text-muted-foreground">
                  请在模型管理中逐项试运行。完成生成与归档后，可启用已验证的发布。
                </p>
              ) : (
                <ConnectionOnboarding
                  id={id}
                  revision={revision + attempt}
                  onSaved={() => {
                    onSaved();
                    setAttempt((v) => v + 1);
                  }}
                />
              )}
              <Button
                variant="outline"
                asChild
                className="self-start max-md:h-11"
              >
                <Link href={`/models?provider=${encodeURIComponent(id)}`}>
                  模型管理
                </Link>
              </Button>
              <Collapsible className="flex flex-col gap-3">
                <CollapsibleTrigger asChild>
                  <Button type="button" variant="ghost" className="self-start">
                    <ChevronDown data-icon="inline-start" />
                    连接检查
                  </Button>
                </CollapsibleTrigger>
                <CollapsibleContent className="flex flex-col gap-4">
                  <p className="text-xs text-muted-foreground">
                    只读检查，不生成内容。
                  </p>
                  <Button
                    variant="outline"
                    disabled={!detail.credential?.id || checking}
                    onClick={async () => {
                      setChecking(true);
                      setError("");
                      try {
                        const result = await testAdminCredential(
                          { id, credential_id: detail.credential!.id! },
                          {
                            headers: { "Idempotency-Key": crypto.randomUUID() },
                          },
                        );
                        if (result.check_id) setCheckID(result.check_id);
                        else throw new Error("检查回执不完整。");
                      } catch (error) {
                        setError(catalogMessage(error));
                        setChecking(false);
                      }
                    }}
                  >
                    {checking ? <Spinner data-icon="inline-start" /> : null}
                    {checking ? "正在检查" : "检查连接"}
                  </Button>
                  {check ? (
                    <div className="flex flex-col gap-3 text-sm">
                      <p>
                        鉴权：{check.auth_result} · 适配：{check.adapter_result}{" "}
                        · 发现：{check.discovery_result}
                      </p>
                      {check.discovery_result === "unsupported" ? (
                        <p>
                          此平台不提供已接入的模型发现接口，请在模型管理中填写账号可用的模型
                          ID。
                        </p>
                      ) : null}
                      {check.discovery_result === "partial" ? (
                        <p>候选列表已截断，可在模型管理中手动补充模型 ID。</p>
                      ) : null}
                      {check.candidates?.length ? (
                        <Collapsible>
                          <CollapsibleTrigger asChild>
                            <Button
                              type="button"
                              variant="ghost"
                              className="group/collapsible h-auto w-full justify-between whitespace-normal"
                            >
                              <span>
                                查看 {check.candidates.length} 个候选模型
                              </span>
                              <ChevronDown
                                data-icon="inline-end"
                                className="transition-transform group-data-[state=open]/collapsible:rotate-180"
                              />
                            </Button>
                          </CollapsibleTrigger>
                          <CollapsibleContent
                            forceMount
                            className="pt-3 data-[state=closed]:hidden"
                          >
                            <ItemGroup className="mt-2 max-h-64 overflow-auto">
                              {check.candidates.map((item) => (
                                <Item
                                  key={`${item.capability}:${item.provider_model_id}`}
                                  variant="outline"
                                  className="break-all"
                                >
                                  <span>
                                    {item.display_name ||
                                      item.provider_model_id}
                                  </span>
                                  <code className="block text-xs">
                                    {item.provider_model_id}
                                  </code>
                                  <span className="text-xs text-muted-foreground">
                                    {item.capability}
                                  </span>
                                </Item>
                              ))}
                            </ItemGroup>
                          </CollapsibleContent>
                        </Collapsible>
                      ) : null}
                    </div>
                  ) : null}
                </CollapsibleContent>
              </Collapsible>
              {provider?.status === "active" ? (
                <AlertDialog>
                  <AlertDialogTrigger asChild>
                    <Button variant="outline">停用连接</Button>
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
                        onClick={async () => {
                          try {
                            await updateAdminProvider(
                              { id },
                              {
                                expected_revision: provider.revision,
                                status: "disabled",
                              },
                              {
                                headers: {
                                  "Idempotency-Key": crypto.randomUUID(),
                                },
                              },
                            );
                            onSaved();
                          } catch (error) {
                            setError(catalogMessage(error));
                          }
                        }}
                      >
                        停用连接（停止新任务）
                      </AlertDialogAction>
                    </AlertDialogFooter>
                  </AlertDialogContent>
                </AlertDialog>
              ) : null}
            </TabsContent>
            <TabsContent value="key" className="flex flex-col gap-6">
              {" "}
              <form
                onSubmit={async (event) => {
                  event.preventDefault();
                  if (onboardingPending) return;
                  const secret: Record<string, string> = musicCredential
                    ? { access_key_id: accessKeyID, secret_access_key: apiKey }
                    : { api_key: apiKey };
                  setAccessKeyID("");
                  setAPIKey("");
                  setVisible(false);
                  await mutation.run("credential.set", id, (requestID) =>
                    setAdminCredential(
                      { id },
                      { label: "平台统一 Key", secret },
                      { headers: { "Idempotency-Key": requestID } },
                    ).then((result) => {
                      if (
                        !result.id ||
                        result.provider_id !== id ||
                        result.status !== "active" ||
                        typeof result.last4 !== "string" ||
                        result.last4.length !== 4
                      )
                        throw new RequestError("request_unconfirmed");
                      return result;
                    }),
                  );
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
                          disabled={onboardingPending || mutation.busy}
                          onChange={(event) =>
                            setAccessKeyID(event.target.value)
                          }
                        />
                      </InputGroup>
                    </Field>
                  )}
                  <Field>
                    <FieldLabel htmlFor={`${formFieldId}-field-1`}>
                      <span>
                        {musicCredential ? "Secret Access Key" : "平台 API Key"}
                      </span>
                    </FieldLabel>
                    <InputGroup className="h-10 max-md:h-11">
                      <InputGroupInput
                        className="h-full"
                        id={`${formFieldId}-field-1`}
                        type={visible ? "text" : "password"}
                        disabled={onboardingPending || mutation.busy}
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
                          disabled={onboardingPending || mutation.busy}
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
                  <p className="text-xs text-muted-foreground">
                    Key 仅用于本次保存，不会保存在浏览器。轮换后旧 Key
                    只用于已有任务恢复。
                  </p>
                  <Button
                    disabled={mutation.busy || onboardingPending || !apiKey}
                  >
                    {detail.credential ? "轮换 Key" : "保存 Key"}
                  </Button>
                </FieldGroup>
              </form>
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
            </TabsContent>
            <TabsContent value="config">
              {" "}
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
            </TabsContent>
          </Tabs>
        </>
      )}
    </section>
  );
}
