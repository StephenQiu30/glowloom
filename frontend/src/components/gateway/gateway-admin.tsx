"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { ArrowLeft, Layers, Plus, RefreshCw } from "lucide-react";
import { useOptionalSession } from "@/components/auth/session";
import { ContentState } from "@/components/layout/content-state";
import { useAdminHeadingInNavigation } from "@/components/governance/admin-layout";
import { useSpace } from "@/components/spaces/space-context";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  loadGatewayConfiguration,
  setCredentialState,
  type CredentialState,
  type GatewayConfiguration,
  type GatewayPrice,
  type OnboardingReview,
} from "@/lib/ai-gateway-admin";
import {
  FormDetails,
  SelectField,
  money,
  type RunMutation,
} from "./gateway-fields";
import {
  CreditsForm,
  ModelForm,
  ReconciliationForm,
  RouteForm,
  protocolLabels,
} from "./gateway-forms";
import { OnboardingPanel, onboardingState } from "./gateway-onboarding";
import { modeLabel } from "./gateway-media-fields";

const tabs = [
  ["models", "模型与销售价"],
  ["routes", "供应商与采购价"],
  ["onboarding", "验证与启用"],
  ["credits", "积分与对账"],
] as const;
type GatewayTab = (typeof tabs)[number][0];
type Entry = "models" | "providers";

export function GatewayAdmin({
  entry = "models",
  entityID,
}: {
  entry?: Entry;
  entityID?: string;
}) {
  const session = useOptionalSession();
  if (session?.actor.platform_role !== "platform_admin")
    return (
      <ContentState
        state="error"
        title="暂无访问权限"
        description="此页面仅向平台管理员开放。"
      />
    );
  return (
    <GatewayConfigurationPage
      key={`${session.actor.id}:${entry}:${entityID ?? ""}`}
      entry={entry}
      entityID={entityID}
    />
  );
}

function GatewayConfigurationPage({
  entry,
  entityID,
}: {
  entry: Entry;
  entityID?: string;
}) {
  const { selected: space } = useSpace();
  const defaultScope = space?.writable ? space.id : undefined;
  const [configuration, setConfiguration] =
    useState<GatewayConfiguration | null>(null);
  const [revision, setRevision] = useState(0);
  const [tab, setTab] = useState<GatewayTab>(
    entry === "providers" ? "routes" : "models",
  );
  const [stage, setStage] = useState<"model" | "route">();
  const [discard, setDiscard] = useState<GatewayTab>();
  const [selectedModel, setSelectedModel] = useState<string>();
  const [candidate, setCandidate] = useState<string>();
  const [prepared, setPrepared] = useState<OnboardingReview>();
  const [inDetail, setInDetail] = useState(Boolean(entityID));
  const [readError, setReadError] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const submitting = useRef(false);
  const dirty = useRef(false);
  const heading = useRef<HTMLHeadingElement>(null);
  const refresh = useCallback(() => setRevision((value) => value + 1), []);

  useEffect(() => {
    const controller = new AbortController();
    loadGatewayConfiguration(controller.signal).then(
      (next) => {
        if (controller.signal.aborted) return;
        setConfiguration(next);
        setReadError(false);
        setLoading(false);
      },
      () => {
        if (controller.signal.aborted) return;
        setReadError(true);
        setLoading(false);
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
      refresh();
      return true;
    } catch (cause) {
      // Provider response bodies are never shown as form feedback.
      setError(
        cause instanceof Error &&
          cause.name === "Error" &&
          !cause.message.startsWith("Request failed")
          ? cause.message
          : "保存未完成。请核对当前状态后重试。",
      );
      requestAnimationFrame(() =>
        document.getElementById("gateway-operation-error")?.focus(),
      );
      return false;
    } finally {
      submitting.current = false;
      setBusy(false);
    }
  };

  function go(next: GatewayTab) {
    dirty.current = false;
    setTab(next);
    setStage(undefined);
    setInDetail(false);
    setError("");
  }

  function requestNavigation(next: GatewayTab) {
    if (busy) return;
    if (stage && dirty.current) {
      setDiscard(next);
      return;
    }
    go(next);
  }

  function startModel() {
    go("models");
    setStage("model");
    requestAnimationFrame(() => heading.current?.focus());
  }

  function startRoute(id?: string) {
    go("routes");
    setSelectedModel(id);
    setStage("route");
    requestAnimationFrame(() => heading.current?.focus());
  }

  function openCandidate(id: string) {
    go("onboarding");
    setCandidate(id);
    requestAnimationFrame(() => heading.current?.focus());
  }

  const isProviders = entry === "providers";
  const title = isProviders ? "AI 服务接入" : "模型管理";
  const titleInNavigation = useAdminHeadingInNavigation(title);
  const showListing =
    !inDetail && !stage && (tab !== "onboarding" || !candidate);
  const currentStep =
    stage === "model"
      ? 0
      : stage === "route"
        ? 1
        : tab === "onboarding" && candidate
          ? 2
          : undefined;
  const waitingForNewDraft =
    stage === "route" &&
    selectedModel &&
    !configuration?.models.some(({ model }) => model.id === selectedModel);

  return (
    <div
      className="mx-auto flex w-full max-w-6xl min-w-0 flex-col gap-5"
      onChangeCapture={() => {
        if (stage) dirty.current = true;
      }}
    >
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0 flex-1">
          <h1
            ref={heading}
            tabIndex={-1}
            className={
              titleInNavigation
                ? "sr-only"
                : "text-2xl font-semibold tracking-tight"
            }
          >
            {title}
          </h1>
          {!titleInNavigation ? (
            <p className="mt-1 text-sm text-muted-foreground">
              {isProviders
                ? "管理供应商路由、采购费用和上线验证。"
                : "管理模型预设、支持能力与销售定价。"}
            </p>
          ) : null}
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            variant="outline"
            disabled={busy || loading}
            className="max-md:min-h-11"
            onClick={() => {
              setLoading(true);
              refresh();
            }}
          >
            <RefreshCw data-icon="inline-start" aria-hidden="true" />
            刷新配置
          </Button>
          {showListing && configuration && (
            <Button
              type="button"
              disabled={busy}
              className="max-md:min-h-11"
              onClick={startModel}
            >
              <Plus data-icon="inline-start" aria-hidden="true" />
              {isProviders ? "新建接入" : "新建模型预设"}
            </Button>
          )}
        </div>
      </div>
      {readError && (
        <Alert variant="destructive">
          <AlertTitle>网关配置暂时无法读取</AlertTitle>
          <AlertDescription>请刷新配置后继续。</AlertDescription>
        </Alert>
      )}
      {error && (
        <Alert id="gateway-operation-error" tabIndex={-1} variant="destructive">
          <AlertTitle>操作未完成</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      {notice && (
        <Alert role="status">
          <AlertDescription>{notice}</AlertDescription>
        </Alert>
      )}
      {!configuration && loading && (
        <Skeleton className="h-48 w-full" aria-label="读取网关配置" />
      )}
      {configuration && (
        <Tabs
          value={tab}
          onValueChange={(value) => requestNavigation(value as GatewayTab)}
          className="min-w-0 gap-5"
        >
          <TabsList
            aria-label="网关管理"
            variant="line"
            className="grid w-full grid-cols-2 justify-items-stretch gap-y-2 max-md:h-auto! sm:flex sm:w-fit sm:gap-4"
          >
            {tabs.map(([value, label]) => (
              <TabsTrigger
                key={value}
                value={value}
                className="min-h-10 px-2 max-md:min-h-11"
              >
                {label}
              </TabsTrigger>
            ))}
          </TabsList>
          {currentStep !== undefined && (
            <OnboardingSteps current={currentStep} />
          )}
          {inDetail && entityID ? (
            <TabsContent value={tab} className="min-w-0">
              <EntityDetail
                entry={entry}
                id={entityID}
                configuration={configuration}
                startRoute={startRoute}
                openCandidate={openCandidate}
              />
            </TabsContent>
          ) : (
            <>
              <TabsContent value="models" className="min-w-0">
                {stage === "model" ? (
                  <Card>
                    <CardHeader>
                      <CardTitle>模型预设与销售价</CardTitle>
                      <CardDescription>
                        定义创作者可选的模型能力及计价方式。
                      </CardDescription>
                      <CardAction>
                        <Button
                          type="button"
                          variant="ghost"
                          disabled={busy}
                          className="max-md:min-h-11"
                          onClick={() => requestNavigation("models")}
                        >
                          返回列表
                        </Button>
                      </CardAction>
                    </CardHeader>
                    <CardContent>
                      <ModelForm
                        busy={busy}
                        run={run}
                        onPrepared={startRoute}
                      />
                    </CardContent>
                  </Card>
                ) : (
                  <ModelList
                    configuration={configuration}
                    startRoute={startRoute}
                  />
                )}
              </TabsContent>
              <TabsContent
                value="routes"
                className="flex min-w-0 flex-col gap-5"
              >
                {stage === "route" ? (
                  <Card>
                    <CardHeader>
                      <CardTitle>供应商与采购价</CardTitle>
                      <CardDescription>
                        选择接入的精确型号、验证范围及实际采购定价。
                      </CardDescription>
                      <CardAction>
                        <Button
                          type="button"
                          variant="ghost"
                          disabled={busy}
                          className="max-md:min-h-11"
                          onClick={() => requestNavigation("routes")}
                        >
                          返回列表
                        </Button>
                      </CardAction>
                    </CardHeader>
                    <CardContent>
                      {waitingForNewDraft ? (
                        <p
                          role="status"
                          className="text-sm text-muted-foreground"
                        >
                          正在读取刚保存的模型预设…
                        </p>
                      ) : (
                        <RouteForm
                          key={selectedModel ?? "choose"}
                          configuration={configuration}
                          busy={busy}
                          run={run}
                          initialModelID={selectedModel}
                          defaultScope={defaultScope}
                          onPrepared={(review) => {
                            setPrepared(review);
                            openCandidate(review.id);
                          }}
                        />
                      )}
                    </CardContent>
                  </Card>
                ) : (
                  <>
                    <RoutesList
                      configuration={configuration}
                      startRoute={startRoute}
                      openCandidate={openCandidate}
                    />
                    <Card>
                      <CardHeader>
                        <CardTitle>共享配额与凭据</CardTitle>
                        <CardDescription>
                          同一账号下的路由共享并发及请求频率。
                        </CardDescription>
                      </CardHeader>
                      <CardContent className="flex flex-col gap-4">
                        {configuration.quotas.length > 0 ? (
                          <ul className="flex flex-col gap-2">
                            {configuration.quotas.map((quota) => (
                              <li
                                key={quota.name}
                                className="flex flex-wrap justify-between gap-2 text-sm"
                              >
                                <span className="font-medium wrap-anywhere">
                                  {quota.name}
                                </span>
                                <span className="text-muted-foreground">
                                  并发 {quota.concurrency} · {quota.rpm} 请求 /
                                  分钟
                                </span>
                              </li>
                            ))}
                          </ul>
                        ) : (
                          <p className="text-sm text-muted-foreground">
                            暂无共享配额组。
                          </p>
                        )}
                        <FormDetails label="管理凭据状态" keepMounted={false}>
                          <CredentialList
                            configuration={configuration}
                            busy={busy}
                            run={run}
                          />
                        </FormDetails>
                      </CardContent>
                    </Card>
                  </>
                )}
              </TabsContent>
              <TabsContent value="onboarding" className="min-w-0">
                {candidate ? (
                  <OnboardingPanel
                    key={candidate}
                    id={candidate}
                    initial={prepared?.id === candidate ? prepared : undefined}
                    run={run}
                    busy={busy}
                    refreshConfiguration={refresh}
                    back={() => setCandidate(undefined)}
                    retryRoute={startRoute}
                  />
                ) : (
                  <Card>
                    <CardHeader>
                      <CardTitle>接入候选</CardTitle>
                      <CardDescription>
                        查看待验证候选及其原始回执。
                      </CardDescription>
                    </CardHeader>
                    <CardContent>
                      {configuration.onboarding.length ? (
                        <ul className="flex flex-col divide-y divide-border">
                          {configuration.onboarding.map((item) => (
                            <ListRow
                              key={item.id}
                              name={
                                configuration.models.find(
                                  ({ model }) => model.id === item.model_id,
                                )?.model.name ?? item.upstream_model
                              }
                              description={item.upstream_model}
                              status={onboardingState[item.state]}
                            >
                              <Button
                                type="button"
                                variant="outline"
                                className="max-md:min-h-11"
                                onClick={() => openCandidate(item.id)}
                              >
                                {item.state === "prepared"
                                  ? "继续验证"
                                  : "查看回执"}
                              </Button>
                            </ListRow>
                          ))}
                        </ul>
                      ) : (
                        <ListEmpty
                          title="还没有待验证候选"
                          description="保存采购路由后可在这里查看预算和验证内容。"
                        />
                      )}
                    </CardContent>
                  </Card>
                )}
              </TabsContent>
              <TabsContent
                value="credits"
                className="grid min-w-0 items-start gap-5 xl:grid-cols-2"
              >
                <Card>
                  <CardHeader>
                    <CardTitle>管理员增加积分</CardTitle>
                    <CardDescription>
                      按唯一业务凭据记入积分账本。
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <CreditsForm
                      run={run}
                      busy={busy}
                      defaultScope={defaultScope}
                    />
                  </CardContent>
                </Card>
                <Card>
                  <CardHeader>
                    <CardTitle>供应商账单对账</CardTitle>
                    <CardDescription>
                      待核对请求 {configuration.unknown.length} 条
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <ReconciliationForm
                      run={run}
                      busy={busy}
                      unknown={configuration.unknown}
                    />
                  </CardContent>
                </Card>
              </TabsContent>
            </>
          )}
        </Tabs>
      )}
      <AlertDialog
        open={discard !== undefined}
        onOpenChange={(open) => {
          if (!open) setDiscard(undefined);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>离开当前表单？</AlertDialogTitle>
            <AlertDialogDescription>
              本步骤尚未保存，离开后需重新填写。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>继续填写</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                if (discard) go(discard);
              }}
            >
              离开表单
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function OnboardingSteps({ current }: { current: number }) {
  return (
    <ol
      aria-label="接入进度"
      className="flex flex-wrap items-center gap-x-4 gap-y-2"
    >
      {tabs.slice(0, 3).map(([, label], index) => (
        <li
          key={label}
          aria-current={index === current ? "step" : undefined}
          className="flex items-center gap-2 text-sm text-muted-foreground aria-[current=step]:font-medium aria-[current=step]:text-foreground"
        >
          <Badge variant={index === current ? "default" : "secondary"}>
            {index + 1}
          </Badge>
          {label}
        </li>
      ))}
    </ol>
  );
}

function ListEmpty({
  title,
  description,
}: {
  title: string;
  description: string;
}) {
  return (
    <Empty>
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <Layers aria-hidden="true" />
        </EmptyMedia>
        <EmptyTitle>{title}</EmptyTitle>
        <EmptyDescription>{description}</EmptyDescription>
      </EmptyHeader>
    </Empty>
  );
}

function ListRow({
  name,
  description,
  status,
  children,
}: {
  name: string;
  description?: string;
  status: string;
  children: React.ReactNode;
}) {
  return (
    <li className="grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-2 py-3 md:grid-cols-[minmax(0,1fr)_auto_auto]">
      <div className="min-w-0">
        <p className="font-medium wrap-anywhere">{name}</p>
        {description && (
          <p className="mt-1 text-sm wrap-anywhere text-muted-foreground">
            {description}
          </p>
        )}
      </div>
      <Badge variant="secondary" className="justify-self-end">
        {status}
      </Badge>
      <div className="col-span-2 flex flex-wrap items-center gap-2 md:col-span-1">
        {children}
      </div>
    </li>
  );
}

function priceSummary(price: GatewayPrice, media: boolean) {
  return media
    ? `${money(price.request, price.currency)} / 次`
    : `输入 ${money(price.input, price.currency)} · 输出 ${money(price.output, price.currency)} / 百万 token`;
}

function ModelList({
  configuration,
  startRoute,
}: {
  configuration: GatewayConfiguration;
  startRoute: (id: string) => void;
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>销售模型版本</CardTitle>
        <CardDescription>
          查看已启用模型、待验证草稿和历史版本。
        </CardDescription>
      </CardHeader>
      <CardContent>
        {configuration.models.length ? (
          <ul className="flex flex-col divide-y divide-border">
            {configuration.models.map(({ model, enabled, draft }) => (
              <ListRow
                key={model.id}
                name={model.name}
                description={`${protocolLabels[model.protocol] ?? model.protocol}${model.protocol === "local" ? "" : ` · ${priceSummary(model.price, model.protocol === "media")}`}`}
                status={enabled ? "已启用" : draft ? "待验证" : "历史版本"}
              >
                <Button asChild variant="outline" className="max-md:min-h-11">
                  <Link href={`/models/${model.id}`}>查看版本</Link>
                </Button>
                {(enabled || draft) && model.protocol !== "local" && (
                  <Button
                    type="button"
                    variant="ghost"
                    className="max-md:min-h-11"
                    onClick={() => startRoute(model.id)}
                  >
                    准备采购
                  </Button>
                )}
              </ListRow>
            ))}
          </ul>
        ) : (
          <ListEmpty
            title="还没有销售模型"
            description="新建模型预设后可以准备采购路由。"
          />
        )}
      </CardContent>
    </Card>
  );
}

function RoutesList({
  configuration,
  startRoute,
  openCandidate,
}: {
  configuration: GatewayConfiguration;
  startRoute: () => void;
  openCandidate: (id: string) => void;
}) {
  const pending = configuration.onboarding.filter(
    ({ state }) => state === "prepared" || state === "verifying",
  );
  return (
    <Card>
      <CardHeader>
        <CardTitle>供应商路由</CardTitle>
        <CardDescription>按供应商账号和上游型号维护采购路由。</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {configuration.routes.length || pending.length ? (
          <ul className="flex flex-col divide-y divide-border">
            {configuration.routes.map(({ route, enabled }) => (
              <ListRow
                key={route.id ?? route.model_id}
                name={route.upstream_model}
                description={`${route.group}${route.protocol === "local" ? "" : ` · ${priceSummary(route.purchase, route.protocol === "media")}`}`}
                status={enabled ? "已启用" : "未启用"}
              >
                {route.id && (
                  <Button asChild variant="outline" className="max-md:min-h-11">
                    <Link href={`/providers/${route.id}`}>查看路由</Link>
                  </Button>
                )}
              </ListRow>
            ))}
            {pending
              .filter(
                ({ id }) =>
                  !configuration.routes.some(({ route }) => route.id === id),
              )
              .map((item) => (
                <ListRow
                  key={item.id}
                  name={item.upstream_model}
                  description={
                    configuration.models.find(
                      ({ model }) => model.id === item.model_id,
                    )?.model.name
                  }
                  status={onboardingState[item.state]}
                >
                  <Button
                    type="button"
                    variant="outline"
                    className="max-md:min-h-11"
                    onClick={() => openCandidate(item.id)}
                  >
                    查看验证
                  </Button>
                </ListRow>
              ))}
          </ul>
        ) : (
          <ListEmpty
            title="还没有采购路由"
            description="选择已保存的模型版本，准备供应商和采购价。"
          />
        )}
        <Button
          type="button"
          variant="outline"
          className="w-fit max-md:min-h-11"
          onClick={() => startRoute()}
        >
          准备采购路由
        </Button>
      </CardContent>
    </Card>
  );
}

function EntityDetail({
  entry,
  id,
  configuration,
  startRoute,
  openCandidate,
}: {
  entry: Entry;
  id: string;
  configuration: GatewayConfiguration;
  startRoute: (id: string) => void;
  openCandidate: (id: string) => void;
}) {
  const modelRow =
    entry === "models"
      ? configuration.models.find(({ model }) => model.id === id)
      : undefined;
  const routeRow =
    entry === "providers"
      ? configuration.routes.find(({ route }) => route.id === id)
      : undefined;
  const candidate = configuration.onboarding.find((item) => item.id === id);
  const model = modelRow?.model;
  const route = routeRow?.route;
  const linkedModel =
    model ??
    configuration.models.find((row) => row.model.id === route?.model_id)?.model;
  return (
    <div className="flex flex-col gap-4">
      <Button asChild variant="ghost" className="w-fit max-md:min-h-11">
        <Link href={entry === "models" ? "/models" : "/providers"}>
          <ArrowLeft data-icon="inline-start" aria-hidden="true" />
          返回列表
        </Link>
      </Button>
      {model || route ? (
        <Card>
          <CardHeader>
            <CardTitle className="wrap-anywhere">
              {model?.name ?? route?.upstream_model}
            </CardTitle>
            <CardDescription>
              {protocolLabels[linkedModel?.protocol ?? route?.protocol ?? ""] ??
                "供应商路由"}
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <Badge variant="secondary" className="w-fit">
              {modelRow?.enabled || routeRow?.enabled
                ? "已启用"
                : modelRow?.draft
                  ? "待验证"
                  : "未启用"}
            </Badge>
            {linkedModel?.modes?.length ? (
              <Detail
                label="支持模式"
                value={linkedModel.modes.map(modeLabel).join("、")}
              />
            ) : null}
            {model && (
              <Detail
                label="销售定价"
                value={priceSummary(model.price, model.protocol === "media")}
              />
            )}
            {model && (
              <Detail label="单次采购上限" value={money(model.purchase_cap)} />
            )}
            {route && (
              <>
                <Detail label="供应商地址" value={route.base_url} />
                <Detail
                  label="采购定价"
                  value={priceSummary(
                    route.purchase,
                    route.protocol === "media",
                  )}
                />
                <Detail label="共享配额组" value={route.group} />
              </>
            )}
            {model?.parameters &&
              Object.entries(model.parameters).map(([key, p]) => (
                <Detail
                  key={key}
                  label={key}
                  value={[
                    p.type,
                    p.min !== undefined ? `最小 ${p.min}` : "",
                    p.max !== undefined ? `最大 ${p.max}` : "",
                    p.enum?.join(" / "),
                    p.default !== undefined ? `默认 ${p.default}` : "",
                  ]
                    .filter(Boolean)
                    .join(" · ")}
                />
              ))}
            <Separator />
            <Detail label="版本标识" value={id} />
            <div className="flex flex-wrap gap-2">
              {model &&
                (modelRow?.draft || modelRow?.enabled) &&
                model.protocol !== "local" && (
                  <Button
                    type="button"
                    onClick={() => startRoute(model.id)}
                    className="max-md:min-h-11"
                  >
                    准备采购
                  </Button>
                )}
              {candidate && (
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => openCandidate(candidate.id)}
                  className="max-md:min-h-11"
                >
                  查看验证
                </Button>
              )}
            </div>
          </CardContent>
        </Card>
      ) : candidate && entry === "providers" ? (
        <Card>
          <CardHeader>
            <CardTitle>{candidate.upstream_model}</CardTitle>
            <CardDescription>
              {onboardingState[candidate.state]}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Button type="button" onClick={() => openCandidate(candidate.id)}>
              查看验证
            </Button>
          </CardContent>
        </Card>
      ) : (
        <ListEmpty
          title="未找到此版本"
          description="返回列表查看可用模型或供应商路由。"
        />
      )}
    </div>
  );
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <dl className="flex flex-col gap-1 text-sm">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="wrap-anywhere">{value}</dd>
    </dl>
  );
}

const credentialOptions = [
  ["active", "可用"],
  ["draining", "排空"],
  ["revoked", "撤销（不可恢复）"],
] as const;

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
  const [revoke, setRevoke] = useState<string>();
  if (!configuration.credentials.length)
    return <p className="text-sm text-muted-foreground">暂无供应商凭据。</p>;
  function save(id: string) {
    const state = states[id];
    if (!state) return;
    void run(() => setCredentialState(id, state), "凭据状态已更新。").then(
      (saved) => {
        if (!saved) return;
        setStates((current) =>
          Object.fromEntries(
            Object.entries(current).filter(([key]) => key !== id),
          ),
        );
        setRevoke(undefined);
      },
    );
  }
  return (
    <>
      <ul className="flex flex-col divide-y divide-border">
        {configuration.credentials.map((credential) => (
          <li
            key={credential.id}
            className="grid items-end gap-3 py-3 sm:grid-cols-[minmax(0,1fr)_12rem_auto]"
          >
            <div className="min-w-0">
              <p className="text-sm font-medium break-all">{credential.id}</p>
              <p className="text-sm break-all text-muted-foreground">
                供应商 {credential.supplier_id}
              </p>
            </div>
            <SelectField
              label={`凭据 ${credential.id} 状态`}
              options={credentialOptions}
              value={states[credential.id] ?? credential.state}
              disabled={busy || credential.state === "revoked"}
              onChange={(event) =>
                setStates((current) => ({
                  ...current,
                  [credential.id]: event.currentTarget.value as CredentialState,
                }))
              }
            />
            <Button
              type="button"
              variant="outline"
              disabled={
                busy ||
                !states[credential.id] ||
                states[credential.id] === credential.state ||
                credential.state === "revoked"
              }
              onClick={() =>
                states[credential.id] === "revoked"
                  ? setRevoke(credential.id)
                  : save(credential.id)
              }
              className="max-md:min-h-11"
            >
              保存状态
            </Button>
          </li>
        ))}
      </ul>
      <AlertDialog
        open={Boolean(revoke)}
        onOpenChange={(open) => {
          if (!open) setRevoke(undefined);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>撤销这个凭据？</AlertDialogTitle>
            <AlertDialogDescription>
              凭据撤销后无法恢复。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              disabled={busy}
              onClick={() => {
                if (revoke) save(revoke);
              }}
            >
              确认撤销
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
