"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useOptionalSession } from "@/components/auth/session";
import { ContentState } from "@/components/layout/content-state";
import { Button } from "@/components/ui/button";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from "@/components/ui/collapsible";
import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  loadGatewayConfiguration,
  setCredentialState,
  type CredentialState,
  type GatewayConfiguration,
  type OnboardingReview,
} from "@/lib/ai-gateway-admin";
import {
  CreditsForm,
  ModelForm,
  ReconciliationForm,
  RouteForm,
  type RunMutation,
} from "./gateway-forms";

import { OnboardingPanel, onboardingState } from "./gateway-onboarding";

export function GatewayAdmin() {
  const session = useOptionalSession();
  if (session?.actor.platform_role !== "platform_admin")
    return (
      <ContentState
        state="error"
        title="暂无访问权限"
        description="此页面仅向平台管理员开放。"
      />
    );
  return <GatewayConfigurationPage key={session.actor.id} />;
}

function GatewayConfigurationPage() {
  const [configuration, setConfiguration] =
    useState<GatewayConfiguration | null>(null);
  const [revision, setRevision] = useState(0);
  const [tab, setTab] = useState("models");
  const [candidate, setCandidate] = useState<string>();
  const [prepared, setPrepared] = useState<OnboardingReview>();
  const refresh = useCallback(() => setRevision((value) => value + 1), []);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const submitting = useRef(false);
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    const controller = new AbortController();
    loadGatewayConfiguration(controller.signal).then(
      (next) => {
        if (!controller.signal.aborted) {
          setConfiguration(next);
          setLoading(false);
        }
      },
      () => {
        if (!controller.signal.aborted) {
          setError("网关配置暂时无法读取，请检查会话与管理员权限后重试。");
          setLoading(false);
        }
      },
    );
    return () => controller.abort();
  }, [revision]);
  const run: RunMutation = async (action, success) => {
    if (submitting.current) return false;
    submitting.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await action();
      setNotice(success);
      setRevision((value) => value + 1);
      return true;
    } catch (cause) {
      // Never display upstream error bodies: a provider error may include a secret.
      setError(
        cause instanceof Error &&
          cause.name === "Error" &&
          !cause.message.startsWith("Request failed")
          ? cause.message
          : "保存未完成，请读取当前配置核对后重试。",
      );
      return false;
    } finally {
      submitting.current = false;
      setBusy(false);
    }
  };
  return (
    <div className="flex min-w-0 flex-col gap-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-xl font-semibold">模型与供应商</h1>
        <Button
          variant="outline"
          disabled={busy || loading}
          onClick={() => {
            setLoading(true);
            setError("");
            setRevision((value) => value + 1);
          }}
        >
          刷新配置
        </Button>
      </div>
      {error ? (
        <Alert variant="destructive">
          <AlertTitle>操作未完成</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
      {notice ? (
        <Alert role="status">
          <AlertTitle>已保存</AlertTitle>
          <AlertDescription>{notice}</AlertDescription>
        </Alert>
      ) : null}
      {loading ? <Skeleton className="h-32 w-full" /> : null}
      {configuration ? (
        <Tabs value={tab} onValueChange={setTab} className="min-w-0 gap-5">
          <TabsList aria-label="网关管理" className="h-auto flex-wrap">
            <TabsTrigger value="models">销售模型</TabsTrigger>
            <TabsTrigger value="routes">采购路由</TabsTrigger>
            <TabsTrigger value="onboarding">验证与启用</TabsTrigger>
            <TabsTrigger value="credits">积分与对账</TabsTrigger>
          </TabsList>
          <TabsContent value="models" className="flex flex-col gap-5">
            <Card>
              <CardHeader>
                <CardTitle>销售模型版本</CardTitle>
                <CardDescription>
                  创作者按已启用的销售合同计费。
                </CardDescription>
              </CardHeader>
              <CardContent>
                {configuration.models.length ? (
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>模型</TableHead>
                        <TableHead>协议 / 状态</TableHead>
                        <TableHead>销售价（微货币）</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {configuration.models.map(({ model, enabled, draft }) => (
                        <TableRow key={model.id}>
                          <TableCell>
                            <p>{model.name}</p>
                            <p className="text-xs break-all text-muted-foreground">
                              {model.id}
                            </p>
                          </TableCell>
                          <TableCell>
                            {model.protocol}{" "}
                            <Badge variant="secondary">
                              {enabled
                                ? "已启用"
                                : draft
                                  ? "待验证"
                                  : "历史版本"}
                            </Badge>
                          </TableCell>
                          <TableCell>
                            {model.price.currency} · 输入 {model.price.input} /
                            输出 {model.price.output} / 按次{" "}
                            {model.price.request}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                ) : (
                  <p>请先准备模型预设和销售价。</p>
                )}
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle>准备模型与销售价</CardTitle>
                <CardDescription>
                  文本模型填写名称、协议和价格即可。媒体模型在高级配置中声明能力与参数。
                </CardDescription>
              </CardHeader>
              <CardContent>
                <ModelForm run={run} busy={busy} />
              </CardContent>
            </Card>
          </TabsContent>
          <TabsContent value="routes" className="flex flex-col gap-5">
            <Card>
              <CardHeader>
                <CardTitle>供应商路由</CardTitle>
                <CardDescription>
                  采购价与销售价独立，验证通过后路由生效。
                </CardDescription>
              </CardHeader>
              <CardContent>
                {configuration.routes.length ? (
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>供应商模型 / 配额组</TableHead>
                        <TableHead>销售模型版本</TableHead>
                        <TableHead>采购价（微货币）</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {configuration.routes.map(({ route, enabled }) => (
                        <TableRow key={route.id}>
                          <TableCell>
                            {route.upstream_model}
                            <p className="text-xs text-muted-foreground">
                              {route.group} · {enabled ? "已启用" : "未启用"}
                            </p>
                          </TableCell>
                          <TableCell className="break-all">
                            {route.model_id}
                          </TableCell>
                          <TableCell>
                            {route.purchase.currency} · 输入{" "}
                            {route.purchase.input} / 输出{" "}
                            {route.purchase.output} / 按次{" "}
                            {route.purchase.request}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                ) : (
                  <p>暂无供应商路由。</p>
                )}
                {configuration.quotas.map((quota) => (
                  <p
                    key={quota.name}
                    className="mt-3 text-xs break-all text-muted-foreground"
                  >
                    {quota.name}：共享并发 {quota.concurrency} / RPM {quota.rpm}
                  </p>
                ))}
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle>准备供应商路由与采购价</CardTitle>
                <CardDescription>
                  选择已配置的模型版本，核定采购价和测试付款空间。
                </CardDescription>
              </CardHeader>
              <CardContent>
                <RouteForm
                  configuration={configuration}
                  run={run}
                  busy={busy}
                  onPrepared={(review) => {
                    setCandidate(review.id);
                    setPrepared(review);
                    setTab("onboarding");
                  }}
                />
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle>供应商凭据</CardTitle>
                <CardDescription>
                  排空后不接收新请求；撤销不可恢复。密钥只写入，不提供回显。
                </CardDescription>
              </CardHeader>
              <CardContent>
                <CredentialList
                  configuration={configuration}
                  run={run}
                  busy={busy}
                />
              </CardContent>
            </Card>
          </TabsContent>
          <TabsContent value="onboarding" className="flex flex-col gap-5">
            <Card>
              <CardHeader>
                <CardTitle>接入候选</CardTitle>
              </CardHeader>
              <CardContent className="flex flex-wrap gap-2">
                {configuration.onboarding.length === 0 && !prepared && (
                  <p>请先保存采购价与验证计划。</p>
                )}
                {configuration.onboarding.map((item) => (
                  <Button
                    key={item.id}
                    type="button"
                    variant={candidate === item.id ? "default" : "outline"}
                    onClick={() => setCandidate(item.id)}
                    className="h-auto max-w-full break-all whitespace-normal"
                  >
                    {configuration.models.find(
                      ({ model }) => model.id === item.model_id,
                    )?.model.name ?? item.upstream_model}{" "}
                    · {onboardingState[item.state]}
                  </Button>
                ))}
              </CardContent>
            </Card>
            {candidate && (
              <OnboardingPanel
                key={candidate}
                id={candidate}
                initial={prepared?.id === candidate ? prepared : undefined}
                run={run}
                busy={busy}
                refreshConfiguration={refresh}
              />
            )}
          </TabsContent>
          <TabsContent value="credits" className="flex flex-col gap-5">
            <Card>
              <CardHeader>
                <CardTitle>管理员增加积分</CardTitle>
                <CardDescription>
                  手工入账使用唯一业务凭据，进入同一个积分账本并保留操作审计。
                </CardDescription>
              </CardHeader>
              <CardContent>
                <CreditsForm run={run} busy={busy} />
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle>供应商账单对账</CardTitle>
                <CardDescription>
                  待核对请求 {configuration.unknown.length}{" "}
                  条。仅凭确切供应商证据结算原请求。
                </CardDescription>
              </CardHeader>
              <CardContent>
                <Collapsible>
                  <CollapsibleTrigger asChild>
                    <Button variant="outline">展开证据对账</Button>
                  </CollapsibleTrigger>
                  <CollapsibleContent>
                    <ReconciliationForm
                      run={run}
                      busy={busy}
                      unknown={configuration.unknown}
                    />
                  </CollapsibleContent>
                </Collapsible>
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      ) : null}
    </div>
  );
}

function CredentialList({
  configuration,
  run,
  busy,
}: {
  configuration: GatewayConfiguration;
  run: RunMutation;
  busy: boolean;
}) {
  const [states, setStates] = useState<Record<string, CredentialState>>({});
  if (!configuration.credentials.length) return <p>暂无供应商凭据。</p>;
  return (
    <ul className="flex flex-col gap-4">
      {configuration.credentials.map((credential) => (
        <li key={credential.id} className="flex flex-wrap items-center gap-3">
          <div className="min-w-0 flex-1">
            <p className="break-all">{credential.id}</p>
            <p className="text-xs break-all text-muted-foreground">
              供应商 {credential.supplier_id} · {credential.state}
            </p>
          </div>
          <NativeSelect
            aria-label={`凭据 ${credential.id} 状态`}
            value={states[credential.id] ?? credential.state}
            disabled={busy || credential.state === "revoked"}
            onChange={(event) =>
              setStates((current) => ({
                ...current,
                [credential.id]: event.target.value as CredentialState,
              }))
            }
          >
            <NativeSelectOption value="active">可用</NativeSelectOption>
            <NativeSelectOption value="draining">排空</NativeSelectOption>
            <NativeSelectOption value="revoked">
              撤销（不可恢复）
            </NativeSelectOption>
          </NativeSelect>
          <Button
            variant="outline"
            disabled={
              busy ||
              credential.state === "revoked" ||
              !states[credential.id] ||
              states[credential.id] === credential.state
            }
            onClick={() =>
              void run(
                () => setCredentialState(credential.id, states[credential.id]),
                "凭据状态已更新。",
              )
            }
          >
            保存状态
          </Button>
        </li>
      ))}
    </ul>
  );
}
