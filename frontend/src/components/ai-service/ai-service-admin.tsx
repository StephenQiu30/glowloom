"use client";

import { AdminHeading } from "@/components/governance/admin-layout";
import { AiServiceAction } from "./admin-action";
import { ArkVideoEstimate } from "./ark-video-estimate";
import { ModelCatalog } from "./model-catalog";
import { CreditGrants } from "./credit-grants";
import { deleteAiRoutes } from "@/api/aiGateway";
import { AiServiceTable } from "./admin-table";
import {
  ModelOnboardingDialog,
  type ModelOnboarding,
} from "./model-onboarding";
import { TooltipProvider } from "@/components/ui/tooltip";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  ArrowLeft,
  Layers,
  Plus,
  RefreshCw,
  Eye,
  Route,
  ClipboardCheck,
  Save,
  Coins,
  X,
  Check,
} from "lucide-react";
import { useOptionalSession } from "@/components/auth/session";
import { ContentState } from "@/components/layout/content-state";
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
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";

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
  loadAiServiceConfiguration,
  setCredentialState,
  type CredentialState,
  type AiServiceConfiguration,
} from "@/lib/ai-service-admin";
import {
  SelectField,
  money,
  priceSummary,
  type RunMutation,
} from "./admin-fields";
import { CreditsForm, ReconciliationForm, protocolLabels } from "./admin-forms";
import { OnboardingPanel, onboardingState } from "./onboarding-verification";
import { modeLabel } from "./media-fields";

const tabs = [
  ["models", "模型"],
  ["routes", "渠道"],
  ["pending", "待接入"],
  ["onboarding", "验证"],
  ["quotas", "配额"],
  ["credentials", "凭据"],
  ["credit-grants", "赠送单"],
  ["credits", "对账"],
] as const;

type Entry = "models" | "providers";
type GatewayTab = (typeof tabs)[number][0];

export function AiServiceAdmin({
  entry = "models",
  entityID,
  initialTab,
}: {
  entry?: Entry;
  entityID?: string;
  initialTab?: string;
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
    <AiServiceConfigurationPage
      key={`${session.actor.id}:${entry}:${entityID ?? ""}:${initialTab ?? ""}`}
      actorID={session.actor.id}
      entry={entry}
      entityID={entityID}
      initialTab={initialTab}
    />
  );
}

function AiServiceConfigurationPage({
  actorID,
  entry,
  entityID,
  initialTab,
}: {
  actorID: string;
  entry: Entry;
  entityID?: string;
  initialTab?: string;
}) {
  const { selected: space } = useSpace();
  const defaultScope = space?.writable ? space.id : undefined;
  const [configuration, setConfiguration] =
    useState<AiServiceConfiguration | null>(null);
  const [revision, setRevision] = useState(0);
  const [tab, setTab] = useState<GatewayTab>(
    tabs.find(([key]) => key === initialTab)?.[0] ??
      (entry === "providers" ? "routes" : "models"),
  );
  const [billingAction, setBillingAction] = useState<"fund" | "reconcile">();
  const [setup, setSetup] = useState<ModelOnboarding>();
  const [discard, setDiscard] = useState<() => void>();
  const [candidate, setCandidate] = useState<string>();
  const [inDetail, setInDetail] = useState(Boolean(entityID));
  const [readError, setReadError] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const submitting = useRef(false);
  const dirty = useRef(false);
  const heading = useRef<HTMLDivElement>(null);
  const setupTrigger = useRef<HTMLElement>(null);
  const refresh = useCallback(() => setRevision((value) => value + 1), []);

  useEffect(() => {
    const controller = new AbortController();
    loadAiServiceConfiguration(controller.signal).then(
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

  async function deleteChannels(ids: string[]) {
    await deleteAiRoutes({ ids });
    refresh();
  }

  const [selectedAttempt, setSelectedAttempt] = useState("");

  function go(next: GatewayTab) {
    dirty.current = false;
    setTab(next);
    if (!entityID && next !== tab && window.location.pathname === "/models") {
      const url = new URL(window.location.href);
      if (next === "models") url.searchParams.delete("tab");
      else url.searchParams.set("tab", next);
      window.history.replaceState(null, "", url);
    }
    setCandidate(undefined);
    setBillingAction(undefined);
    setSetup(undefined);
    setInDetail(false);
    setError("");
  }

  function requestChange(action: () => void) {
    if (busy) return;
    if (dirty.current) {
      setDiscard(() => () => {
        dirty.current = false;
        setError("");
        action();
      });
      return;
    }
    setError("");
    action();
  }

  function requestNavigation(next: GatewayTab) {
    requestChange(() => go(next));
  }

  function openSetup(next: ModelOnboarding) {
    const trigger = document.activeElement;
    setupTrigger.current =
      trigger instanceof HTMLElement && trigger !== document.body
        ? trigger
        : null;
    go(tab);
    setSetup(next);
  }

  function startModel() {
    openSetup({});
  }

  function startRoute(id?: string) {
    openSetup({ existing: true, modelID: id });
  }

  function openCandidate(id: string) {
    go(tab === "pending" ? "pending" : "onboarding");
    setCandidate(id);
    requestAnimationFrame(() => heading.current?.focus());
  }

  function openBilling(action: "fund" | "reconcile", attempt = "") {
    go(action === "fund" ? "credit-grants" : "credits");
    setSelectedAttempt(attempt);
    setBillingAction(action);
    requestAnimationFrame(() => heading.current?.focus());
  }
  const title = "AI 服务";
  const showListing = !inDetail && !billingAction && !candidate;

  const operationError = error && (
    <Alert id="gateway-operation-error" tabIndex={-1} variant="destructive">
      <AlertTitle>操作未完成</AlertTitle>
      <AlertDescription>{error}</AlertDescription>
    </Alert>
  );
  function reload() {
    setLoading(true);
    refresh();
  }

  const toolbarActions = showListing && (
    <>
      {!readError && (
        <AiServiceAction
          label="刷新配置"
          icon={RefreshCw}
          disabled={busy || loading}
          onClick={reload}
        />
      )}
      {tab === "models" && (
        <ArkVideoEstimate presets={configuration?.model_presets} />
      )}
      {(tab === "models" || tab === "routes") && (
        <AiServiceAction
          icon={Plus}
          variant="default"
          label="新增接入"
          disabled={busy}
          onClick={startModel}
        />
      )}
      {tab === "credit-grants" && (
        <AiServiceAction
          icon={Coins}
          label="赠送积分"
          disabled={busy}
          onClick={() => openBilling("fund")}
        />
      )}
      {tab === "credits" && (
        <AiServiceAction
          icon={ClipboardCheck}
          label="手动对账"
          disabled={busy}
          onClick={() => openBilling("reconcile")}
        />
      )}
    </>
  );

  return (
    <TooltipProvider>
      <div
        className="admin-page"
        onChangeCapture={() => {
          if (billingAction) dirty.current = true;
        }}
      >
        {inDetail ? (
          <AdminHeading
            title={entry === "providers" ? "供应商路由详情" : "模型详情"}
            detail
          />
        ) : (
          <h1 className="sr-only">{title}</h1>
        )}
        <Tabs
          value={tab}
          onValueChange={(value) => requestNavigation(value as GatewayTab)}
          className="admin-page"
        >
          <div
            ref={heading}
            tabIndex={-1}
            aria-label={title}
            role="group"
            className="admin-toolbar"
          >
            <TabsList variant="secondary" aria-label="AI 服务管理">
              {tabs.map(([value, label]) => (
                <TabsTrigger
                  key={value}
                  value={value}
                  disabled={!configuration}
                >
                  {label}
                </TabsTrigger>
              ))}
            </TabsList>
          </div>
          {readError && (
            <Alert variant="destructive">
              <AlertTitle>AI 服务配置暂时无法读取</AlertTitle>
              <AlertDescription>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={loading}
                  onClick={reload}
                >
                  刷新配置
                </Button>
              </AlertDescription>
            </Alert>
          )}
          {!setup && operationError}
          {notice && (
            <Alert role="status">
              <AlertDescription>{notice}</AlertDescription>
            </Alert>
          )}
          {!configuration && loading && (
            <Skeleton className="h-48 w-full" aria-label="读取AI 服务配置" />
          )}
          {configuration && (
            <>
              <TabsContent key={tab} value={tab} className="min-w-0">
                {inDetail && entityID ? (
                  <EntityDetail
                    entry={entry}
                    id={entityID}
                    configuration={configuration}
                    startRoute={startRoute}
                    openCandidate={openCandidate}
                  />
                ) : candidate ? (
                  <OnboardingPanel
                    key={candidate}
                    id={candidate}
                    run={run}
                    busy={busy}
                    refreshConfiguration={refresh}
                    back={() => go(tab)}
                    retryRoute={startRoute}
                  />
                ) : tab === "onboarding" || tab === "pending" ? (
                  <AiServiceTable
                    toolbarActions={toolbarActions}
                    label={tab === "pending" ? "待接入渠道" : "接入验证"}
                    onDelete={tab === "pending" ? deleteChannels : undefined}
                    columns={["模型 / 上游型号", "状态", "操作"]}
                    empty={
                      tab === "pending" ? "暂无待接入渠道" : "暂无验证记录"
                    }
                    rows={configuration.onboarding
                      .filter(
                        (item) =>
                          tab !== "pending" ||
                          item.state === "prepared" ||
                          item.state === "verifying",
                      )
                      .map((item) => {
                        const name =
                          configuration.models.find(
                            ({ model }) => model.id === item.model_id,
                          )?.model.name ?? item.upstream_model;
                        return {
                          id: item.id,
                          deleteID: item.id,
                          label: tab === "pending" ? item.upstream_model : name,
                          terms: [name, item.upstream_model],
                          status: onboardingState[item.state],
                          cells: [
                            <RouteModel
                              key="model"
                              name={name}
                              upstream={item.upstream_model}
                            />,
                            <Badge key="status" variant="secondary">
                              {onboardingState[item.state]}
                            </Badge>,
                            <AiServiceAction
                              key="review"
                              onClick={() => openCandidate(item.id)}
                              label={
                                item.state === "prepared"
                                  ? "继续验证"
                                  : "查看回执"
                              }
                              icon={ClipboardCheck}
                            />,
                          ],
                        };
                      })}
                  />
                ) : tab === "models" ? (
                  <ModelCatalog
                    toolbarActions={toolbarActions}
                    configuration={configuration}
                    startRoute={startRoute}
                  />
                ) : tab === "routes" ? (
                  <RoutesList
                    toolbarActions={toolbarActions}
                    configuration={configuration}
                    onDelete={deleteChannels}
                  />
                ) : tab === "quotas" ? (
                  <AiServiceTable
                    toolbarActions={toolbarActions}
                    label="共享配额"
                    columns={["配额组", "并发", "请求／分钟"]}
                    empty="暂无共享配额组"
                    rows={configuration.quotas.map((q) => ({
                      id: q.name,
                      label: q.name,
                      cells: [q.name, q.concurrency, q.rpm],
                    }))}
                  />
                ) : tab === "credentials" ? (
                  <CredentialList
                    toolbarActions={toolbarActions}
                    configuration={configuration}
                    busy={busy}
                    run={run}
                  />
                ) : billingAction === "fund" ||
                  billingAction === "reconcile" ? (
                  <Card>
                    <CardHeader>
                      <CardTitle>
                        {billingAction === "fund"
                          ? "提交积分赠送单"
                          : "供应商账单对账"}
                      </CardTitle>
                      <CardAction>
                        <AiServiceAction
                          label="返回列表"
                          icon={ArrowLeft}
                          disabled={busy}
                          onClick={() => requestNavigation(tab)}
                        />
                      </CardAction>
                    </CardHeader>
                    <CardContent>
                      {billingAction === "fund" ? (
                        <CreditsForm
                          run={run}
                          busy={busy}
                          defaultScope={defaultScope}
                        />
                      ) : (
                        <ReconciliationForm
                          run={run}
                          busy={busy}
                          unknown={configuration.unknown}
                          initialAttempt={selectedAttempt}
                        />
                      )}
                    </CardContent>
                  </Card>
                ) : tab === "credit-grants" ? (
                  <CreditGrants
                    actorID={actorID}
                    orders={configuration.credit_grants ?? []}
                    busy={busy}
                    run={run}
                    toolbarActions={toolbarActions}
                  />
                ) : (
                  <AiServiceTable
                    toolbarActions={toolbarActions}
                    label="待对账请求"
                    columns={["原请求", "原尝试", "状态", "操作"]}
                    empty="暂无待对账请求"
                    rows={configuration.unknown.map((row) => ({
                      id: row.attempt_id,
                      label: row.request_id,
                      terms: [row.attempt_id],
                      cells: [
                        row.request_id,
                        row.attempt_id,
                        "待核对",
                        <AiServiceAction
                          key="reconcile"
                          label="核对账单"
                          icon={ClipboardCheck}
                          onClick={() =>
                            openBilling("reconcile", row.attempt_id)
                          }
                        />,
                      ],
                    }))}
                  />
                )}
              </TabsContent>
            </>
          )}
        </Tabs>
        {configuration && setup && (
          <ModelOnboardingDialog
            setup={setup}
            configuration={configuration}
            scope={defaultScope}
            busy={busy}
            error={operationError}
            run={run}
            refresh={refresh}
            allowExistingModel={
              tab === "routes" &&
              configuration.models.some(
                ({ enabled, draft }) => enabled || draft,
              )
            }
            onDirty={(value) => {
              dirty.current = value;
            }}
            onChangeRequest={requestChange}
            onClose={() => requestNavigation(tab)}
            restoreFocus={() => {
              const trigger = setupTrigger.current;
              (trigger?.isConnected ? trigger : heading.current)?.focus();
            }}
          />
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
                尚未保存的内容需要重新填写。
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>继续填写</AlertDialogCancel>
              <AlertDialogAction
                onClick={() => {
                  discard?.();
                }}
              >
                离开表单
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </div>
    </TooltipProvider>
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

function RouteModel({ name, upstream }: { name?: string; upstream: string }) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="font-medium">{name || upstream}</span>
      {name && name !== upstream && (
        <span className="text-xs text-muted-foreground">{upstream}</span>
      )}
    </div>
  );
}

function RoutesList({
  configuration,
  toolbarActions,
  onDelete,
}: {
  configuration: AiServiceConfiguration;
  toolbarActions: ReactNode;
  onDelete: (ids: string[]) => Promise<void>;
}) {
  return (
    <AiServiceTable
      toolbarActions={toolbarActions}
      label="供应商路由"
      onDelete={onDelete}
      columns={["渠道 / 上游型号", "配额组", "采购价", "状态", "操作"]}
      empty="暂无渠道"
      rows={configuration.routes.map(({ route, enabled }) => {
        const modelName = configuration.models.find(
          ({ model }) => model.id === route.model_id,
        )?.model.name;
        return {
          id:
            route.id ??
            JSON.stringify([
              route.model_id,
              route.supplier,
              route.credential,
              route.group,
              route.upstream_model,
              route.base_url,
              route.priority,
            ]),
          deleteID: route.id,
          label: route.upstream_model,
          terms: [modelName ?? "", route.group],
          status: enabled ? "已启用" : "未启用",
          cells: [
            <RouteModel
              key="model"
              name={modelName}
              upstream={route.upstream_model}
            />,
            route.group,
            priceSummary(route.purchase, route.protocol),
            <Badge key="status" variant="secondary">
              {enabled ? "已启用" : "未启用"}
            </Badge>,
            route.id && (
              <AiServiceAction
                label="查看路由"
                icon={Eye}
                href={`/providers/${route.id}`}
              />
            ),
          ],
        };
      })}
    />
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
  configuration: AiServiceConfiguration;
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
      <AiServiceAction
        label="返回列表"
        icon={ArrowLeft}
        href={entry === "models" ? "/models" : "/models?tab=routes"}
      />
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
                value={priceSummary(model.price, model.protocol)}
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
                  value={priceSummary(route.purchase, route.protocol)}
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
              {model && (modelRow?.draft || modelRow?.enabled) && (
                <AiServiceAction
                  onClick={() => startRoute(model.id)}
                  label="准备采购"
                  icon={Route}
                />
              )}
              {candidate && (
                <AiServiceAction
                  onClick={() => openCandidate(candidate.id)}
                  label="查看验证"
                  icon={ClipboardCheck}
                />
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
            <AiServiceAction
              label="查看验证"
              icon={ClipboardCheck}
              onClick={() => openCandidate(candidate.id)}
            />
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
  toolbarActions,
  run,
  busy,
}: {
  configuration: AiServiceConfiguration;
  toolbarActions: ReactNode;
  run: RunMutation;
  busy: boolean;
}) {
  const [states, setStates] = useState<Record<string, CredentialState>>({});
  const [revoke, setRevoke] = useState<string>();

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
      <AiServiceTable
        toolbarActions={toolbarActions}
        label="供应商凭据"
        columns={["凭据", "状态", "操作"]}
        empty="暂无供应商凭据"
        rows={configuration.credentials.map((credential) => ({
          id: credential.id,
          label: credential.id,
          terms: [credential.supplier_id],
          status: credentialOptions.find(
            ([key]) => key === credential.state,
          )?.[1],
          cells: [
            <>
              <p className="text-sm font-medium break-all">{credential.id}</p>
              <p className="text-sm break-all text-muted-foreground">
                供应商 {credential.supplier_id}
              </p>
            </>,
            <SelectField
              key="state"
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
            />,
            <AiServiceAction
              key="save"
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
              label="保存状态"
              icon={Save}
            />,
          ],
        }))}
      />
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
            <AlertDialogCancel aria-label="取消" title="取消" size="icon">
              <X aria-hidden="true" />
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={busy}
              onClick={() => {
                if (revoke) save(revoke);
              }}
              aria-label="确认撤销"
              title="确认撤销"
              size="icon"
            >
              <Check aria-hidden="true" />
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
