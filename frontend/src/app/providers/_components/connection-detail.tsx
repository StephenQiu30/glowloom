"use client";
import { LoadingState } from "@/components/layout/loading-state";
import { ChevronDown } from "lucide-react";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Field, FieldLabel, FieldGroup } from "@/components/ui/field";
import { Textarea } from "@/components/ui/textarea";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { useEffect, useState, useId } from "react";
import { Button } from "@/components/ui/button";
import { RequestError } from "@/lib/request";
import { Input } from "@/components/ui/input";
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
}: {
  id: string;
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
  const [error, setError] = useState("");
  const [check, setCheck] = useState<API.ProviderCheck | null>(null);
  const [checkID, setCheckID] = useState("");
  const [checking, setChecking] = useState(false);
  const [target, setTarget] = useState("");
  const [preview, setPreview] = useState<API.ConfigUpgradePreview | null>(null);
  const [resolution, setResolution] = useState("");
  const mutation = useCatalogMutation(onSaved, `connection.${id}`);
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
  }, [id]);
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
  return (
    <section
      className="min-w-0 space-y-4 rounded-xl border p-4"
      aria-label="连接详情"
    >
      <h2 className="text-lg font-medium">{provider?.name ?? "连接详情"}</h2>
      {error ? (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
      {!detail ? (
        <LoadingState>正在加载连接…</LoadingState>
      ) : (
        <>
          <p className="text-sm text-muted-foreground">
            {provider?.status === "active" ? "已启用" : "已禁用"} · 配置 v
            {config?.version ?? "—"} ·{" "}
            {provider?.region === "domestic" ? "国内" : "海外"}
          </p>
          <form
            onSubmit={async (event) => {
              event.preventDefault();
              const key = apiKey;
              setAPIKey("");
              await mutation.run("credential.set", id, (requestID) =>
                setAdminCredential(
                  { id },
                  { label: "平台统一 Key", secret: { api_key: key } },
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
              <Field>
                <FieldLabel htmlFor={`${formFieldId}-field-1`}>
                  <span>平台 API Key</span>
                </FieldLabel>
                <Input
                  id={`${formFieldId}-field-1`}
                  type="password"
                  value={apiKey}
                  onChange={(event) => setAPIKey(event.target.value)}
                  autoComplete="off"
                  spellCheck={false}
                  required
                  placeholder={
                    detail.credential?.last4
                      ? `已配置 ••••${detail.credential.last4}，输入以轮换`
                      : "输入 API Key"
                  }
                />
              </Field>
              <p className="text-xs text-muted-foreground">
                Key 仅用于本次保存，不会保存在浏览器。轮换后旧 Key
                只用于已有任务恢复。
              </p>
              <Button disabled={mutation.busy || !apiKey}>
                {detail.credential ? "轮换 Key" : "保存 Key"}
              </Button>
            </FieldGroup>
          </form>
          <MutationStatus mutation={mutation} />
          <section className="space-y-2" aria-label="Key 轮换记录">
            <h3 className="font-medium">Key 记录</h3>
            {credentials.map((key) => (
              <div
                key={key.id}
                className="flex flex-wrap items-center justify-between gap-2 rounded border p-3 text-sm"
              >
                <span>
                  ••••{key.last4} ·{" "}
                  {key.status === "active"
                    ? "使用中"
                    : key.status === "draining"
                      ? "仅供已有任务恢复"
                      : "已停用"}
                </span>
                {key.status !== "disabled" ? (
                  <Collapsible>
                    <CollapsibleTrigger asChild>
                      <Button
                        type="button"
                        variant="ghost"
                        className="group/collapsible h-auto w-full justify-between whitespace-normal"
                      >
                        <span>紧急停用</span>
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
                      <p className="my-2 max-w-xs text-xs">
                        停用此 Key
                        后，绑定它的任务也无法继续查询。此操作不能撤销，后续需要添加新
                        Key。
                      </p>
                      <Button
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
                                { headers: { "Idempotency-Key": request } },
                              ),
                          )
                        }
                      >
                        确认停用此 Key
                      </Button>
                    </CollapsibleContent>
                  </Collapsible>
                ) : null}
              </div>
            ))}
          </section>
          {provider?.status === "active" ? (
            <Button
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
                    { headers: { "Idempotency-Key": crypto.randomUUID() } },
                  );
                  onSaved();
                } catch (error) {
                  setError(catalogMessage(error));
                }
              }}
            >
              停用连接（停止新任务）
            </Button>
          ) : null}
          <div className="space-y-2 border-t pt-4">
            <h3 className="font-medium">连接检查与模型发现</h3>
            <p className="text-xs text-muted-foreground">
              仅执行读取请求。发现模型不代表账号具备权限，也不代表生成已通过。
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
                    { headers: { "Idempotency-Key": crypto.randomUUID() } },
                  );
                  if (result.check_id) setCheckID(result.check_id);
                  else throw new Error("检查回执不完整。");
                } catch (error) {
                  setError(catalogMessage(error));
                  setChecking(false);
                }
              }}
            >
              {checking ? "检查中…" : "检查连接并发现模型"}
            </Button>
            {check ? (
              <div className="space-y-2 text-sm">
                <p>
                  鉴权：{check.auth_result} · 适配：{check.adapter_result} ·
                  发现：{check.discovery_result}
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
                        <span>查看 {check.candidates.length} 个候选模型</span>
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
                      <ul className="mt-2 max-h-64 space-y-2 overflow-auto">
                        {check.candidates.map((item) => (
                          <li
                            key={`${item.capability}:${item.provider_model_id}`}
                            className="rounded border p-2 break-all"
                          >
                            <span>
                              {item.display_name || item.provider_model_id}
                            </span>
                            <code className="block text-xs">
                              {item.provider_model_id}
                            </code>
                            <span className="text-xs text-muted-foreground">
                              {item.capability}
                            </span>
                          </li>
                        ))}
                      </ul>
                    </CollapsibleContent>
                  </Collapsible>
                ) : null}
              </div>
            ) : null}
          </div>
          {config ? (
            <div className="space-y-3 border-t pt-4">
              <h3 className="font-medium">配置升级</h3>
              <p className="text-xs text-muted-foreground">
                保存配置只更新候选快照。现有模型发布和任务继续使用原快照。
              </p>
              <Field>
                <FieldLabel htmlFor={`${formFieldId}-field-2`}>
                  <span>目标预设版本</span>
                </FieldLabel>
                <NativeSelect
                  id={`${formFieldId}-field-2`}
                  className="mt-1 h-10 w-full"
                  value={target}
                  onChange={(e) => {
                    setTarget(e.target.value);
                    setPreview(null);
                  }}
                >
                  <NativeSelectOption value="">
                    选择同一预设的新版本
                  </NativeSelectOption>
                  {presets
                    .filter(
                      (p) =>
                        !p.archived &&
                        !p.emergency_disabled &&
                        p.document?.config?.adapter_key ===
                          provider?.adapter_key,
                    )
                    .map((p) => (
                      <NativeSelectOption
                        key={p.version_id}
                        value={p.version_id}
                      >
                        {p.document?.name} v{p.version}
                      </NativeSelectOption>
                    ))}
                </NativeSelect>
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
                        await mutation.run("config.upgrade", id, (requestID) =>
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
        </>
      )}
    </section>
  );
}
