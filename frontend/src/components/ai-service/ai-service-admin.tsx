"use client";

import Link from "next/link";
import { ProviderLogo } from "@/components/providers/provider-logo";
import { providerLabel } from "@/components/providers/provider-label";
import { ModelPrice } from "./model-sale";
import { AdminHeading } from "@/components/governance/admin-layout";
import { toast } from "@/components/feedback/notification";
import { RequestError } from "@/lib/request";
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
  useId,
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
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  loadAiServiceConfiguration,
  setCredentialState,
  type CredentialState,
  type AiServiceConfiguration,
} from "@/lib/ai-service-admin";
import {
  FormDetails,
  SelectField,
  priceSummary,
  type RunMutation,
} from "./admin-fields";
import { CreditsForm, ReconciliationForm, protocolLabels } from "./admin-forms";
import { OnboardingPanel, onboardingState } from "./onboarding-verification";
import { modeLabel } from "./media-fields";

const operationErrors = new Map([
  ["invalid_request_error", "请检查配置"],
  ["permission_error", "当前账号无权执行此操作"],
  ["idempotency_conflict", "数据已更新，请刷新后重试"],
  ["insufficient_credits", "验证空间积分不足"],
  ["provider_key_rejected", "供应商未接受 API Key，请核对后重试"],
  ["provider_key_scope", "API Key 缺少只读接口权限"],
  ["provider_key_check_unsupported", "该地址不支持只读鉴权，可选择生成试跑"],
  ["trial_confirmation_required", "请先确认试跑费用"],
  ["budget_exceeded", "超出预算上限，请核对后重试"],
  ["model_unavailable", "接入服务暂时不可用"],
  ["ai_result_unknown", "提交结果待确认，请核对原记录"],
  ["request_unconfirmed", "提交结果待确认，请核对原记录"],
]);

const billingTabs = [
  ["credit-grants", "赠送单"],
  ["credits", "对账"],
] as const;
type Entry = "models" | "providers" | "billing";
type BillingTab = (typeof billingTabs)[number][0];
const serviceLinks = [
  ["models", "/models", "模型"],
  ["providers", "/providers", "渠道"],
] as const;

export function AiServiceAdmin({
  entry = "models",
  entityID,
  initialProvider,
}: {
  entry?: Entry;
  entityID?: string;
  initialProvider?: string;
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
      key={`${session.actor.id}:${entry}:${entityID ?? ""}:${initialProvider ?? ""}`}
      actorID={session.actor.id}
      entry={entry}
      entityID={entityID}
      initialProvider={initialProvider}
    />
  );
}

function AiServiceConfigurationPage({
  actorID,
  entry,
  entityID,
  initialProvider,
}: {
  actorID: string;
  entry: Entry;
  entityID?: string;
  initialProvider?: string;
}) {
  const { selected: space } = useSpace();
  const defaultScope = space?.writable ? space.id : undefined;
  const [configuration, setConfiguration] =
    useState<AiServiceConfiguration | null>(null);
  const [revision, setRevision] = useState(0);
  const [tab, setTab] = useState<BillingTab>("credit-grants");
  const [billingAction, setBillingAction] = useState<"fund" | "reconcile">();
  const [setup, setSetup] = useState<ModelOnboarding>();
  const [discard, setDiscard] = useState<() => void>();
  const [candidate, setCandidate] = useState<string>();
  const [inDetail, setInDetail] = useState(Boolean(entityID));
  const [readError, setReadError] = useState(false);
  const [error, setError] = useState("");
  const feedbackID = useId();
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

  const run: RunMutation = async (action, success, reload = true) => {
    if (submitting.current) return false;
    submitting.current = true;
    setBusy(true);
    setError("");
    try {
      await action();
      if (success) toast.success(success, { id: feedbackID });
      else toast.dismiss(feedbackID);
      if (reload) refresh();
      return true;
    } catch (cause) {
      const fallback = "保存失败，请重试";
      const message =
        cause instanceof RequestError
          ? (operationErrors.get(cause.code) ?? fallback)
          : cause instanceof Error &&
              cause.name === "Error" &&
              !cause.message.startsWith("Request failed")
            ? cause.message || fallback
            : fallback;
      setError(message);
      toast.error(message, { id: feedbackID });
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

  function go(next: BillingTab = tab) {
    dirty.current = false;
    setTab(next);
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

  function requestNavigation(next: BillingTab = tab) {
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
    go();
    setCandidate(id);
    requestAnimationFrame(() => heading.current?.focus());
  }

  function openBilling(action: "fund" | "reconcile", attempt = "") {
    go(action === "fund" ? "credit-grants" : "credits");
    setSelectedAttempt(attempt);
    setBillingAction(action);
    requestAnimationFrame(() => heading.current?.focus());
  }
  const title = entry === "billing" ? "平台账务" : "AI 服务";
  const showListing = !inDetail && !billingAction && !candidate;

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
      {entry === "models" && (
        <ArkVideoEstimate presets={configuration?.model_presets} />
      )}
      {entry === "providers" && (
        <AiServiceAction
          icon={Plus}
          variant="default"
          label="新增接入"
          disabled={busy}
          onClick={startModel}
        />
      )}
      {entry === "billing" && tab === "credit-grants" && (
        <AiServiceAction
          icon={Coins}
          label="赠送积分"
          disabled={busy}
          onClick={() => openBilling("fund")}
        />
      )}
      {entry === "billing" && tab === "credits" && (
        <AiServiceAction
          icon={ClipboardCheck}
          label="手动对账"
          disabled={busy}
          onClick={() => openBilling("reconcile")}
        />
      )}
    </>
  );

  const wrapContent = (body: ReactNode) =>
    entry === "billing" ? (
      <TabsContent value={tab} className="min-w-0">
        {body}
      </TabsContent>
    ) : (
      <div className="min-w-0">{body}</div>
    );
  const content = (
    <>
      <div
        ref={heading}
        tabIndex={-1}
        aria-label={title}
        role="group"
        className="admin-toolbar"
      >
        {entry === "billing" ? (
          <TabsList variant="secondary" aria-label="账务管理">
            {billingTabs.map(([value, label]) => (
              <TabsTrigger key={value} value={value} disabled={!configuration}>
                {label}
              </TabsTrigger>
            ))}
          </TabsList>
        ) : (
          <nav aria-label="AI 服务管理" className="flex gap-1">
            {serviceLinks.map(([key, href, label]) => (
              <Button
                key={key}
                variant={key === entry ? "secondary" : "ghost"}
                size="sm"
                asChild
              >
                <Link
                  href={href}
                  aria-current={key === entry ? "page" : undefined}
                >
                  {label}
                </Link>
              </Button>
            ))}
          </nav>
        )}
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
      {!configuration && loading && (
        <Skeleton className="h-48 w-full" aria-label="读取AI 服务配置" />
      )}
      {configuration &&
        wrapContent(
          inDetail && entityID ? (
            <EntityDetail
              entry={entry}
              id={entityID}
              configuration={configuration}
              openCandidate={openCandidate}
              run={run}
              busy={busy}
            />
          ) : candidate ? (
            <OnboardingPanel
              key={candidate}
              id={candidate}
              models={configuration.models}
              run={run}
              busy={busy}
              refreshConfiguration={refresh}
              back={() => go(tab)}
              retryRoute={startRoute}
            />
          ) : entry === "models" ? (
            <ModelCatalog
              toolbarActions={toolbarActions}
              configuration={configuration}
            />
          ) : entry === "providers" ? (
            <Channels
              toolbarActions={toolbarActions}
              configuration={configuration}
              provider={initialProvider}
              openCandidate={openCandidate}
              startRoute={startRoute}
              onDelete={deleteChannels}
              busy={busy}
              run={run}
            />
          ) : billingAction === "fund" || billingAction === "reconcile" ? (
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
                    onClick={() => openBilling("reconcile", row.attempt_id)}
                  />,
                ],
              }))}
            />
          ),
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
        {entry === "billing" ? (
          <Tabs
            value={tab}
            onValueChange={(value) => requestNavigation(value as BillingTab)}
            className="admin-page"
          >
            {content}
          </Tabs>
        ) : (
          <div className="admin-page">{content}</div>
        )}
        {configuration && setup && (
          <ModelOnboardingDialog
            setup={setup}
            configuration={configuration}
            scope={defaultScope}
            busy={busy}
            error={error}
            run={run}
            refresh={refresh}
            allowExistingModel={
              entry === "providers" &&
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

function Channels({
  configuration,
  toolbarActions,
  provider,
  openCandidate,
  startRoute,
  onDelete,
  busy,
  run,
}: {
  configuration: AiServiceConfiguration;
  toolbarActions: ReactNode;
  provider?: string;
  openCandidate: (id: string) => void;
  startRoute: (id: string) => void;
  onDelete: (ids: string[]) => Promise<void>;
  busy: boolean;
  run: RunMutation;
}) {
  const { routes, onboarding, models, quotas, credentials } = configuration;
  const providerOf = (key?: string) => key ?? "custom";
  const nameOf = (key: string) =>
    configuration.model_presets?.find(({ adapter }) => adapter === key)
      ?.provider ?? providerLabel(key);
  const providers = [
    ...new Set([
      ...routes.map(({ provider_key }) => providerOf(provider_key)),
      ...onboarding
        .filter(({ supplier_id }) => supplier_id)
        .map(({ provider_key }) => providerOf(provider_key)),
    ]),
  ].sort((a, b) => nameOf(a).localeCompare(nameOf(b), "zh-CN"));
  const hasRecord = new Set(onboarding.map(({ model_id }) => model_id));
  const withoutAccount = onboarding.filter(
    ({ supplier_id, state }) =>
      !supplier_id && ["prepared", "verifying", "failed"].includes(state),
  );
  const drafts = models.filter(
    ({ draft, enabled, model }) =>
      (draft || enabled) &&
      !hasRecord.has(model.id) &&
      !routes.some(({ route }) => route.model_id === model.id),
  );
  const modelNames = new Map(models.map(({ model }) => [model.id, model.name]));
  const records = (
    candidates: AiServiceConfiguration["onboarding"],
    rawDrafts: typeof drafts = [],
  ) => (
    <AiServiceTable
      label="接入记录"
      columns={["模型", "状态", "操作"]}
      empty="暂无接入记录"
      onDelete={onDelete}
      rows={[
        ...candidates.map((candidate) => ({
          id: candidate.id,
          deleteID:
            candidate.state === "prepared" || candidate.state === "failed"
              ? candidate.id
              : undefined,
          label: candidate.upstream_model,
          cells: [
            <RouteModel
              key="model"
              name={modelNames.get(candidate.model_id)}
              upstream={candidate.upstream_model}
            />,
            onboardingState[candidate.state],
            <AiServiceAction
              key="open"
              label={candidate.state === "prepared" ? "继续接入" : "查看回执"}
              icon={ClipboardCheck}
              onClick={() => openCandidate(candidate.id)}
            />,
          ],
        })),
        ...rawDrafts.map(({ model }) => ({
          id: model.id,
          label: model.name,
          cells: [
            model.name,
            "待接入",
            <AiServiceAction
              key="continue"
              label="继续接入"
              icon={Route}
              onClick={() => startRoute(model.id)}
            />,
          ],
        })),
      ]}
    />
  );

  if (!provider || !providers.includes(provider)) {
    return (
      <div className="flex flex-col gap-6">
        <AiServiceTable
          label="供应商"
          columns={["供应商", "绑定渠道", "待处理", "操作"]}
          empty="暂无供应商"
          toolbarActions={toolbarActions}
          rows={providers.map((key) => ({
            id: key,
            label: nameOf(key),
            cells: [
              <span
                key="provider"
                className="flex items-center gap-2 font-medium"
              >
                <ProviderLogo adapterKey={key} iconOnly />
                {nameOf(key)}
              </span>,
              routes.filter(
                ({ provider_key }) => providerOf(provider_key) === key,
              ).length,
              onboarding.filter(
                ({ provider_key, state }) =>
                  providerOf(provider_key) === key &&
                  ["prepared", "verifying", "failed"].includes(state),
              ).length,
              <AiServiceAction
                key="open"
                label={`查看 ${nameOf(key)}`}
                icon={Eye}
                href={`/providers?provider=${encodeURIComponent(key)}`}
              />,
            ],
          }))}
        />
        {(withoutAccount.length > 0 || drafts.length > 0) &&
          records(withoutAccount, drafts)}
      </div>
    );
  }

  const supplierRoutes = routes.filter(
    ({ provider_key }) => providerOf(provider_key) === provider,
  );
  const supplierRecords = onboarding.filter(
    ({ provider_key, supplier_id }) =>
      supplier_id && providerOf(provider_key) === provider,
  );
  const accounts = [
    ...new Set([
      ...supplierRoutes.map(({ route }) => route.supplier),
      ...supplierRecords.flatMap(({ supplier_id }) =>
        supplier_id ? [supplier_id] : [],
      ),
    ]),
  ];
  return (
    <section
      aria-label={nameOf(provider)}
      className="flex min-w-0 flex-col gap-6"
    >
      <div className="flex flex-wrap items-center gap-3">
        <AiServiceAction
          label="返回供应商"
          icon={ArrowLeft}
          href="/providers"
        />
        <h2 className="font-medium">{nameOf(provider)}</h2>
        <div className="ml-auto flex items-center gap-2">{toolbarActions}</div>
      </div>
      {accounts.map((account) => {
        const accountRoutes = supplierRoutes.filter(
          ({ route }) => route.supplier === account,
        );
        const accountRecords = supplierRecords.filter(
          ({ supplier_id }) => supplier_id === account,
        );
        const groups = new Set([
          ...accountRoutes.map(({ route }) => route.group),
          ...accountRecords.map(({ group }) => group),
        ]);
        const accountQuotas = quotas.filter(({ name }) => groups.has(name));
        const accountCredentials = credentials.filter(
          ({ supplier_id }) => supplier_id === account,
        );
        const accountName =
          accountRoutes[0]?.route.group ??
          accountRecords[0]?.group ??
          account.slice(0, 8);
        return (
          <section
            key={account}
            aria-label={`账号 ${accountName}`}
            className="flex min-w-0 flex-col gap-3"
          >
            {accounts.length > 1 && (
              <h3 className="text-sm font-medium">{accountName}</h3>
            )}
            {accountRoutes.length > 0 && (
              <RoutesList
                configuration={configuration}
                routes={accountRoutes}
                onDelete={onDelete}
              />
            )}
            {accountRecords.some(({ state }) =>
              ["prepared", "verifying", "failed"].includes(state),
            ) &&
              records(
                accountRecords.filter(({ state }) =>
                  ["prepared", "verifying", "failed"].includes(state),
                ),
              )}
            <FormDetails label="账号设置" keepMounted={false}>
              {accountCredentials.length > 0 && (
                <CredentialList
                  credentials={accountCredentials}
                  busy={busy}
                  run={run}
                />
              )}
              {accountQuotas.length > 0 && (
                <AiServiceTable
                  label="共享配额"
                  columns={["配额组", "并发", "请求／分钟"]}
                  empty="暂无共享配额"
                  rows={accountQuotas.map((quota) => ({
                    id: quota.name,
                    label: quota.name,
                    cells: [quota.name, quota.concurrency, quota.rpm],
                  }))}
                />
              )}
            </FormDetails>
            {accountRecords.some(
              ({ state }) =>
                !["prepared", "verifying", "failed"].includes(state),
            ) && (
              <FormDetails label="验证记录" keepMounted={false}>
                {records(
                  accountRecords.filter(
                    ({ state }) =>
                      !["prepared", "verifying", "failed"].includes(state),
                  ),
                )}
              </FormDetails>
            )}
          </section>
        );
      })}
    </section>
  );
}

function RoutesList({
  configuration,
  routes,
  onDelete,
}: {
  configuration: AiServiceConfiguration;
  routes: AiServiceConfiguration["routes"];
  onDelete: (ids: string[]) => Promise<void>;
}) {
  return (
    <AiServiceTable
      label="供应商路由"
      onDelete={onDelete}
      columns={["渠道 / 上游型号", "配额组", "采购价", "状态", "操作"]}
      empty="暂无渠道"
      rows={routes.map(({ route, enabled }) => {
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
  openCandidate,
  run,
  busy,
}: {
  entry: Entry;
  id: string;
  configuration: AiServiceConfiguration;
  openCandidate: (id: string) => void;
  run: RunMutation;
  busy: boolean;
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
        href={
          entry === "models"
            ? "/models"
            : routeRow?.provider_key
              ? `/providers?provider=${encodeURIComponent(routeRow.provider_key)}`
              : "/providers"
        }
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
            {model &&
              modelRow?.enabled &&
              configuration.routes.some(
                ({ route, enabled }) =>
                  enabled &&
                  route.model_id === model.id &&
                  configuration.credentials.some(
                    ({ id, state }) =>
                      id === route.credential && state === "active",
                  ),
              ) && (
                <ModelPrice
                  key={JSON.stringify([model.id, model.sale_version])}
                  model={model}
                  run={run}
                  busy={busy}
                />
              )}
            {route && (
              <>
                <Detail label="供应商地址" value={route.base_url} />
                {linkedModel && route.id && (
                  <ModelPrice
                    key={JSON.stringify([route.id, route.purchase_version])}
                    model={linkedModel}
                    route={route}
                    run={run}
                    busy={busy}
                  />
                )}
                <Detail label="共享配额组" value={route.group} />
              </>
            )}
            <div className="flex flex-wrap gap-2">
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
  credentials,
  run,
  busy,
}: {
  credentials: AiServiceConfiguration["credentials"];
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
        label="供应商凭据"
        columns={["凭据", "状态", "操作"]}
        empty="暂无供应商凭据"
        rows={credentials.map((credential) => ({
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
