"use client";

import { useId, useState } from "react";
import useSWR from "swr";
import { ChevronDown } from "lucide-react";
import {
  listProviderChecks,
  testProviderReachability,
  testAdminCredential,
  queryProviderModelCheck,
} from "@/api/settings";
import { useSession } from "@/components/auth/session";
import {
  useCatalogMutation,
  catalogMessage,
} from "@/components/catalog/mutation";
import { RequestError } from "@/lib/request";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Spinner } from "@/components/ui/spinner";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from "@/components/ui/collapsible";
import { ConnectionModels } from "./connection-models";
import { checkRunning, testErrorMessage } from "./testing-status";

const time = (value?: string) =>
  value && Number.isFinite(Date.parse(value))
    ? new Date(value).toLocaleString()
    : "尚无记录";
function resultLabel(check?: API.ProviderCheck) {
  if (!check) return "未检查";
  if (checkRunning(check)) return "检查中";
  if (check.kind === "reachability")
    return check.network_result === "reachable" ? "网络可达" : "无法连接";
  if (check.model_result)
    return (
      {
        completed: "已通过",
        accepted: "已受理",
        unknown: "待确认",
        rejected: "未通过",
      }[check.model_result] ?? "结果未知"
    );
  return check.auth_result === "ok" ? "鉴权通过" : "未通过";
}
function checkKind(kind?: string) {
  return kind === "reachability"
    ? "网络连通"
    : kind === "model_access" || kind === "model_query"
      ? "模型验证"
      : "凭据检查";
}

export function ConnectionTests({
  id,
  adapter,
  credentialID,
  revision,
  onSaved,
  initialModel,
  initialRelease,
  initialMode,
}: {
  id: string;
  adapter?: string;
  credentialID: string;
  revision: number;
  onSaved: () => void;
  initialModel?: string;
  initialRelease?: string;
  initialMode?: string;
}) {
  const { actor } = useSession();
  const fieldID = useId();
  const [offset, setOffset] = useState(0);
  const [expanded, setExpanded] = useState(false);
  const { data, error, isLoading, mutate } = useSWR(
    ["provider-tests", actor.id, id, revision, offset],
    () => listProviderChecks({ id, offset }),
    {
      refreshInterval: (state) => (state?.items?.some(checkRunning) ? 2500 : 0),
      errorRetryCount: 0,
    },
  );
  const mutation = useCatalogMutation(
    () => {
      setOffset(0);
      void mutate();
    },
    `provider.tests.readonly.${id}`,
    (value, target) => {
      const r = value as API.CredentialTestAccepted | undefined;
      return r?.accepted === true && r.provider_id === target && !!r.check_id;
    },
  );
  const current =
    data?.current_checks ?? data?.items?.filter((c) => c.current) ?? [];
  const network = current.find((c) => c.kind === "reachability");
  const auth = current.find((c) =>
    ["credential_auth", "connectivity", "discovery"].includes(c.kind ?? ""),
  );
  const unsupported = [
    "volcengine_speech",
    "volcengine_music",
    "volcengine_avatar",
    "volcengine_motion",
  ].includes(adapter ?? "");
  if (
    error instanceof RequestError &&
    (error.status === 401 || error.status === 403)
  )
    return (
      <Alert>
        <AlertDescription>
          管理员权限或会话已失效，无法继续查看测试记录。
        </AlertDescription>
      </Alert>
    );
  return (
    <div className="flex min-w-0 flex-col gap-4" aria-label="服务验证">
      {error ? (
        <Alert variant="destructive">
          <AlertDescription>
            {catalogMessage(error)}
            <Button variant="outline" onClick={() => void mutate()}>
              重新读取
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}
      <ConnectionModels
        adapter={adapter}
        id={id}
        revision={revision}
        onSaved={() => {
          onSaved();
          void mutate();
        }}
        checks={current}
        onChecked={() => void mutate()}
        initialModel={initialModel}
        initialRelease={initialRelease}
        initialMode={initialMode}
      />
      <Collapsible>
        <CollapsibleTrigger asChild>
          <Button variant="ghost" className="group w-full justify-between px-0">
            连接诊断
            <ChevronDown
              data-icon="inline-end"
              className="transition-transform group-data-[state=open]:rotate-180"
            />
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <section
            className="flex min-w-0 flex-col gap-2 py-2"
            aria-labelledby={`${fieldID}-network`}
          >
            <div className="flex items-center gap-2">
              <h3
                id={`${fieldID}-network`}
                className="min-w-0 flex-1 text-sm font-medium"
              >
                网络连通
              </h3>
              <Badge variant="secondary" role="status">
                {checkRunning(network) ? <Spinner /> : null}
                {network ? resultLabel(network) : "未检测"}
              </Badge>
              <Button
                variant="outline"
                className="h-11 shrink-0"
                disabled={
                  mutation.busy || !!mutation.pending || checkRunning(network)
                }
                onClick={() =>
                  void mutation.run("credential.test", id, (requestID) =>
                    testProviderReachability(
                      { id },
                      { headers: { "Idempotency-Key": requestID } },
                    ),
                  )
                }
              >
                检测连通
              </Button>
            </div>
            {network &&
            network.network_result !== "reachable" &&
            !checkRunning(network) ? (
              <p className="text-xs text-destructive">
                {testErrorMessage(
                  network.error_code ?? network.network_result ?? "",
                )}
              </p>
            ) : null}
          </section>
          <section
            className="flex min-w-0 flex-col gap-2 py-2"
            aria-labelledby={`${fieldID}-auth`}
          >
            <div className="flex items-center gap-2">
              <h3
                id={`${fieldID}-auth`}
                className="min-w-0 flex-1 text-sm font-medium"
              >
                凭据检查
              </h3>
              <Badge variant="secondary" role="status">
                {unsupported
                  ? "需模型验证"
                  : auth
                    ? resultLabel(auth)
                    : "未检查"}
              </Badge>
              {!unsupported ? (
                <Button
                  variant="outline"
                  className="h-11 shrink-0"
                  disabled={
                    mutation.busy || !!mutation.pending || checkRunning(auth)
                  }
                  onClick={() =>
                    void mutation.run("credential.test", id, (requestID) =>
                      testAdminCredential(
                        { id, credential_id: credentialID },
                        { headers: { "Idempotency-Key": requestID } },
                      ),
                    )
                  }
                >
                  检查凭据
                </Button>
              ) : null}
            </div>
            {auth &&
            auth.auth_result !== "ok" &&
            !checkRunning(auth) &&
            !unsupported ? (
              <p className="text-xs text-destructive">
                {testErrorMessage(auth.auth_result ?? "")}
              </p>
            ) : null}
            {adapter === "volcengine_ark" && auth?.auth_result === "ok" ? (
              <p className="text-xs text-muted-foreground">
                仅检查视频任务权限
              </p>
            ) : null}
          </section>
          {data?.items?.length || network || auth ? (
            <Collapsible className="flex flex-col gap-2">
              <CollapsibleTrigger asChild>
                <Button
                  variant="ghost"
                  className="group w-full justify-between px-0 max-md:h-11"
                >
                  检测详情
                  <ChevronDown className="transition-transform group-data-[state=open]:rotate-180" />
                </Button>
              </CollapsibleTrigger>
              <CollapsibleContent className="flex flex-col gap-3">
                {[network, auth].map((check) =>
                  check ? (
                    <div
                      key={check.id}
                      className="space-y-1 text-xs text-muted-foreground"
                    >
                      <p>
                        {checkKind(check.kind)} · {check.elapsed_ms ?? 0} ms
                        {check.http_status
                          ? ` · HTTP ${check.http_status}`
                          : ""}
                        {check.attempts ? ` · ${check.attempts} 次尝试` : ""}
                      </p>
                      <p>{time(check.completed_at ?? check.created_at)}</p>
                    </div>
                  ) : null,
                )}
                {auth?.candidates?.length ? (
                  <div className="max-h-48 overflow-y-auto text-xs text-muted-foreground">
                    <h4 className="mb-1 font-medium">
                      候选模型 · {auth.candidates.length}
                    </h4>
                    {auth.candidates.map((m) => (
                      <p className="py-1 break-all" key={m.provider_model_id}>
                        {m.display_name ?? m.provider_model_id}
                      </p>
                    ))}
                  </div>
                ) : null}
                {data?.items?.length ? (
                  <h4 className="text-xs font-medium">最近记录</h4>
                ) : null}
                {isLoading ? (
                  <p role="status" className="text-xs text-muted-foreground">
                    正在读取测试记录…
                  </p>
                ) : !data?.items?.length ? (
                  <p className="text-xs text-muted-foreground">
                    暂无测试记录。
                  </p>
                ) : (
                  (expanded ? data.items : data.items.slice(0, 3)).map(
                    (check) => (
                      <Collapsible
                        key={check.id}
                        className="border-b pb-2 text-xs"
                      >
                        <CollapsibleTrigger asChild>
                          <Button
                            variant="ghost"
                            className="h-auto w-full flex-col items-stretch gap-1 p-0 text-left whitespace-normal max-md:min-h-11"
                          >
                            <span className="flex flex-wrap items-center justify-between gap-2">
                              <span>
                                {checkKind(check.kind)} · {resultLabel(check)}
                              </span>
                              <Badge variant="secondary">
                                {check.current ? "当前版本" : "历史版本"}
                              </Badge>
                            </span>
                            <span className="mt-1 block text-muted-foreground">
                              {time(check.created_at)}
                            </span>
                          </Button>
                        </CollapsibleTrigger>
                        <CollapsibleContent className="mt-2 space-y-1 break-all text-muted-foreground">
                          <p>检查标识：{check.id}</p>
                          <p>配置版本：{check.config_version_id ?? "未知"}</p>
                          {check.model_id ? (
                            <p>模型版本：{check.model_version_id ?? "未知"}</p>
                          ) : null}
                          <p>
                            结果：
                            {check.current
                              ? "当前证据"
                              : "历史证据，不参与当前启用判定"}
                          </p>
                          {check.kind === "model_access" &&
                          check.task_id &&
                          ["accepted", "unknown"].includes(
                            check.model_result ?? "",
                          ) ? (
                            <Button
                              variant="outline"
                              className="h-11"
                              disabled={
                                mutation.busy ||
                                !!mutation.pending ||
                                checkRunning(check)
                              }
                              onClick={() =>
                                void mutation.run(
                                  "credential.test",
                                  id,
                                  (requestID) =>
                                    queryProviderModelCheck(
                                      { id: check.id! },
                                      {
                                        headers: {
                                          "Idempotency-Key": requestID,
                                        },
                                      },
                                    ),
                                )
                              }
                            >
                              查询原任务
                            </Button>
                          ) : null}
                          {check.tracking_note ? (
                            <p>核查说明：{check.tracking_note}</p>
                          ) : null}
                          {check.tracking_ended_at ? (
                            <p>已结束追踪，结果仍为未知。</p>
                          ) : null}
                        </CollapsibleContent>
                      </Collapsible>
                    ),
                  )
                )}
                {(data?.items?.length ?? 0) > 3 && !expanded ? (
                  <Button variant="outline" onClick={() => setExpanded(true)}>
                    更多记录
                  </Button>
                ) : expanded ? (
                  <div className="flex justify-between gap-2">
                    <Button
                      variant="outline"
                      disabled={offset === 0}
                      onClick={() => setOffset((v) => Math.max(0, v - 20))}
                    >
                      上一页记录
                    </Button>
                    <Button
                      variant="outline"
                      disabled={!data?.has_more}
                      onClick={() => setOffset((v) => v + 20)}
                    >
                      下一页记录
                    </Button>
                  </div>
                ) : null}
              </CollapsibleContent>
            </Collapsible>
          ) : null}
        </CollapsibleContent>
      </Collapsible>
      {mutation.pending ? (
        <Button
          variant="outline"
          className="self-start max-md:h-11"
          disabled={mutation.busy}
          onClick={mutation.recover}
        >
          查询检查回执
        </Button>
      ) : null}
      {mutation.message && mutation.message !== "已保存。" ? (
        <p role="status" className="text-sm">
          {mutation.message}
        </p>
      ) : null}
    </div>
  );
}
