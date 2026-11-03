"use client";
import useSWR from "swr";
import { pageBusinessMemory } from "@/lib/page-business-memory";

import { ProjectPicker } from "@/components/projects/project-picker";
import type { Project } from "@/components/projects/project";
import { GenerationParameters } from "@/components/generation/generation-parameters";
import {
  fieldsForMode,
  initialParams,
} from "@/components/generation/contracts";
import { TrialReferences, trialRoles } from "./trial-references";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { ChevronDown } from "lucide-react";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Field, FieldLabel, FieldGroup, FieldSet } from "@/components/ui/field";
import { QuoteCostDetail } from "@/components/generation/quote-cost-detail";
import { formatBillingMicros } from "@/components/billing/billing-outcome";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

import dynamic from "next/dynamic";
import {
  useEffect,
  useRef,
  useState,
  useSyncExternalStore,
  useId,
} from "react";
import {
  createProviderTrialQuote,
  getConfigReceipt,
  getProviderTrialRecovery,
} from "@/api/settings";
import { listProjects, getProjectBudget } from "@/api/projects";
import { confirmOperationQuote, getOperation } from "@/api/operations";
import { useSession } from "@/components/auth/session";
import { RequestError } from "@/lib/request";
import { catalogMessage } from "@/components/catalog/mutation";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

const TrialResult = dynamic(() =>
  import("@/components/generation/generation-dialog").then((m) => m.TaskDetail),
);

type PendingTrial = {
  request: string;
  confirm: string;
  project: string;
  operation?: string;
  confirmRequested?: boolean;
};
const event = "glowloom:trial-request";
function subscribe(callback: () => void) {
  window.addEventListener("storage", callback);
  window.addEventListener(event, callback);
  return () => {
    window.removeEventListener("storage", callback);
    window.removeEventListener(event, callback);
  };
}
function read(key: string) {
  try {
    return pageBusinessMemory.getItem(key) ?? "";
  } catch {
    return "unavailable";
  }
}
function parse(raw: string): PendingTrial | null {
  try {
    const value = JSON.parse(raw);
    return typeof value.request === "string" &&
      typeof value.confirm === "string" &&
      typeof value.project === "string"
      ? value
      : null;
  } catch {
    return null;
  }
}
const terminal = new Set(["completed", "failed", "cancelled", "expired"]);

export function ProviderTrial({
  release,
  onVerified,
  version,
  embedded = false,
  initialMode,
}: {
  release: API.ModelRelease;
  version?: API.AdminModelVersion;
  embedded?: boolean;
  initialMode?: string;
  onVerified: () => void;
}) {
  const formFieldId = useId();

  const { actor } = useSession();
  const storageKey = `glowloom.trial.v1.${actor.id}.${release.id}`;
  const snapshot = useSyncExternalStore(
    subscribe,
    () => read(storageKey),
    () => "server",
  );
  const memoryPending = parse(snapshot);
  const [dismissed, setDismissed] = useState("");
  const {
    data: recovery,
    error: recoveryError,
    isLoading: recoveryLoading,
    mutate: reloadRecovery,
  } = useSWR(
    ["provider-trial-recovery", actor.id, release.id],
    () => getProviderTrialRecovery({ id: release.id! }),
    { errorRetryCount: 0, revalidateOnFocus: true },
  );
  const pending: PendingTrial | null =
    memoryPending ??
    (recovery?.found &&
    recovery.operation_id !== dismissed &&
    recovery.operation_id &&
    recovery.project_id &&
    recovery.request_id &&
    recovery.confirm_request_id
      ? {
          request: recovery.request_id,
          confirm: recovery.confirm_request_id,
          project: recovery.project_id,
          operation: recovery.operation_id,
          confirmRequested: recovery.status !== "quoted",
        }
      : null);
  const [selectedMode, setSelectedMode] = useState(
    (initialMode && release.required_modes?.includes(initialMode)
      ? initialMode
      : release.required_modes?.[0]) ?? "",
  );
  const [project, setProject] = useState<Project | null>(null);
  const [params, setParams] = useState<
    Record<string, string | number | boolean | null>
  >(() =>
    version
      ? initialParams(fieldsForMode(version.param_schema ?? [], selectedMode))
      : {},
  );
  const [references, setReferences] = useState<API.TrialMediaInput[]>([]);
  const [charge, setCharge] = useState("");
  const {
    data: budget,
    error: budgetError,
    isLoading: budgetLoading,
  } = useSWR(
    version && project ? ["provider-trial-budget", actor.id, project.id] : null,
    () => getProjectBudget({ pid: project!.id }),
    { errorRetryCount: 0 },
  );
  const [prompt, setPrompt] = useState("");
  const fields = version
    ? fieldsForMode(version.param_schema ?? [], selectedMode)
    : [];
  const roles = trialRoles(version, selectedMode);
  const missingRole = roles.some(
    (role) =>
      references.filter((r) => r.role === role.role).length <
      (role.min_count ?? 0),
  );
  const missingParam = fields.some(
    (f) =>
      f.required &&
      (params[f.field] === undefined ||
        params[f.field] === null ||
        params[f.field] === ""),
  );
  const [projects, setProjects] = useState<API.ProjectResponse[]>([]);
  const [task, setTask] = useState<API.TaskDetail | null>(null);
  const [preparedBudget, setPreparedBudget] = useState<{
    supplier: number;
    currency: string;
    customer: number;
  } | null>(null);
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const lock = useRef(false);
  useEffect(() => {
    const abort = new AbortController();
    if (version) return;
    listProjects({ status: "active", limit: 200 }, { signal: abort.signal })
      .then((r) => {
        if (!abort.signal.aborted) setProjects(r.items ?? []);
      })
      .catch((e) => {
        if (!abort.signal.aborted) setMessage(catalogMessage(e));
      });
    return () => abort.abort();
  }, [version]);
  const pendingOperation = pending?.operation,
    pendingProject = pending?.project;
  useEffect(() => {
    if (!pendingOperation || !pendingProject) return;
    const abort = new AbortController();
    getOperation(
      { id: pendingOperation, project_id: pendingProject },
      { signal: abort.signal },
    )
      .then((loaded) => {
        if (
          loaded.id !== pendingOperation ||
          loaded.project_id !== pendingProject
        )
          throw new Error("试运行任务不匹配。");
        if (!abort.signal.aborted) setTask(loaded);
      })
      .catch((error) => {
        if (!abort.signal.aborted) setMessage(catalogMessage(error));
      });
    return () => abort.abort();
  }, [pendingOperation, pendingProject]);
  function save(value: PendingTrial) {
    pageBusinessMemory.setItem(storageKey, JSON.stringify(value));
    window.dispatchEvent(new Event(event));
  }
  async function run(action: () => Promise<void>, quoteRequest = false) {
    if (lock.current) return;
    lock.current = true;
    setBusy(true);
    setMessage("");
    try {
      await action();
    } catch (e) {
      // A transactional validation refusal has no operation to recover. Keep
      // ambiguous transport/5xx/idempotency outcomes pinned to their request.
      if (
        quoteRequest &&
        e instanceof RequestError &&
        ((e.status >= 400 && e.status < 500 && e.status !== 409) ||
          [
            "invalid_request",
            "model_unavailable",
            "budget_insufficient",
            "input_not_ready",
          ].includes(e.code))
      ) {
        pageBusinessMemory.removeItem(storageKey);
        window.dispatchEvent(new Event(event));
      }
      setMessage(catalogMessage(e));
    } finally {
      lock.current = false;
      setBusy(false);
    }
  }
  async function refresh(value: PendingTrial) {
    let operation = value.operation;
    if (!operation) {
      const receipt = await getConfigReceipt({
        id: value.request,
        action: "trial.quote",
        target_id: release.id!,
      });
      const result = receipt.result;
      if (
        typeof result?.operation_id !== "string" ||
        result.project_id !== value.project
      )
        throw new Error("试运行回执不匹配。");
      operation = result.operation_id;
      save({ ...value, operation });
    }
    const loaded = await getOperation({
      id: operation,
      project_id: value.project,
    });
    if (loaded.id !== operation || loaded.project_id !== value.project)
      throw new Error("试运行任务不匹配。");
    setTask(loaded);
    if (loaded.status === "completed") {
      setMessage("试运行已完成。刷新模型验证记录后即可检查上线条件。");
    }
  }
  const frozenBudget =
    recovery?.found && recovery.operation_id === task?.id
      ? {
          supplier: recovery.supplier_max_micros ?? 0,
          currency: recovery.supplier_currency ?? "",
          customer: recovery.max_charge_micros ?? 0,
        }
      : preparedBudget;
  const disabled =
    busy ||
    snapshot === "server" ||
    snapshot === "unavailable" ||
    (!memoryPending && (recoveryLoading || !!recoveryError));
  if (
    recoveryError instanceof RequestError &&
    [401, 403].includes(recoveryError.status)
  )
    return (
      <Alert>
        <AlertDescription>
          管理员权限或项目权限已失效，无法继续查看原试运行。
        </AlertDescription>
      </Alert>
    );
  return (
    <Collapsible
      defaultOpen={embedded}
      className={embedded ? "space-y-3" : "space-y-3 rounded-lg border p-3"}
    >
      {!embedded ? (
        <CollapsibleTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            className="group/collapsible h-auto w-full justify-between whitespace-normal"
          >
            <span>试运行此发布</span>
            <ChevronDown
              data-icon="inline-end"
              className="transition-transform group-data-[state=open]/collapsible:rotate-180"
            />
          </Button>
        </CollapsibleTrigger>
      ) : null}
      <CollapsibleContent
        forceMount
        className="pt-3 data-[state=closed]:hidden"
      >
        <p className="text-sm text-muted-foreground">
          试运行生成一个结果，费用记入所选项目。
        </p>
        {snapshot === "unavailable" && (
          <Alert variant="destructive">
            <AlertDescription>
              请允许浏览器保存请求标识，以便恢复未确认的操作。这里不会保存 Key
              或提示词。
            </AlertDescription>
          </Alert>
        )}
        {recoveryError ? (
          <Alert variant="destructive">
            <AlertDescription>
              无法确认是否存在原试运行，暂不创建新请求。
              <Button variant="outline" onClick={() => void reloadRecovery()}>
                重新读取试运行
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}
        {recoveryLoading && !memoryPending ? (
          <p role="status" className="text-xs text-muted-foreground">
            正在恢复试运行记录…
          </p>
        ) : null}
        {message && <FeedbackToast message={message} tone="info" />}
        {!pending?.operation && version ? (
          <Field>
            <FieldLabel>扣费项目（必选）</FieldLabel>
            <ProjectPicker
              value={project}
              disabled={disabled || !!pending}
              compact
              onChange={(value) => {
                setProject(value);
                setReferences([]);
                setCharge("");
              }}
            />
            <p className="text-xs text-muted-foreground">
              {budgetLoading
                ? "正在读取项目预算…"
                : budgetError
                  ? "项目预算暂时无法读取，请重新选择或刷新后再试。"
                  : budget
                    ? `平台可用预算 ¥${((budget.available_micros ?? 0) / 1e6).toFixed(6)} / 总预算 ¥${((budget.limit_micros ?? 0) / 1e6).toFixed(6)}。供应商预算单独确认，后台会重新核验。`
                    : "请选择扣费项目。"}
            </p>
          </Field>
        ) : null}
        {!pending?.operation && (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              const form = new FormData(e.currentTarget);
              void run(async () => {
                const projectID = version
                  ? project?.id
                  : String(form.get("project"));
                if (!projectID) throw new Error("请选择扣费项目。");
                if (pending && pending.project !== projectID)
                  throw new Error("请先恢复现有试运行，不能更换项目后重试。");
                const state = pending ?? {
                  request: crypto.randomUUID(),
                  confirm: crypto.randomUUID(),
                  project: projectID,
                };
                const preparedParams = version
                  ? Object.fromEntries(
                      Object.entries(params).filter(
                        ([key]) => key !== "output_count",
                      ),
                    )
                  : JSON.parse(String(form.get("params") || "{}"));
                const preparedReferences = version
                  ? references
                  : JSON.parse(String(form.get("references") || "[]"));
                if (!Array.isArray(preparedReferences))
                  throw new Error("参考素材需要使用列表格式。");
                const charge = Math.round(
                  Number(form.get("charge")) * 1_000_000,
                );
                const supplier = Math.round(
                  Number(form.get("supplier")) * 1_000_000,
                );
                if (
                  !Number.isSafeInteger(charge) ||
                  charge < 0 ||
                  !Number.isSafeInteger(supplier) ||
                  supplier < 0
                )
                  throw new Error("预算必须是有效的非负金额。");
                save(state);
                const quote = await createProviderTrialQuote(
                  { id: release.id! },
                  {
                    project_id: projectID,
                    request_id: state.request,
                    mode: String(form.get("mode")),
                    prompt: String(form.get("prompt") ?? ""),
                    params: preparedParams,
                    media_inputs: preparedReferences,
                    max_charge_micros: charge,
                    supplier_max_micros: supplier,
                    supplier_currency: release.upstream_cost?.currency,
                  },
                );
                if (!quote.operation_id)
                  throw new Error("报价响应尚未确认，请查询回执。");
                setPreparedBudget({
                  supplier,
                  currency: release.upstream_cost?.currency ?? "",
                  customer: charge,
                });
                const saved = {
                  ...state,
                  operation: quote.operation_id,
                  confirm: quote.confirm_key ?? state.confirm,
                };
                save(saved);
                await refresh(saved);
                void reloadRecovery();
              }, true);
            }}
          >
            <FieldSet disabled={disabled || !!pending} className="min-w-0">
              <FieldGroup className="grid gap-3">
                {!version ? (
                  <Field>
                    <FieldLabel htmlFor={`${formFieldId}-field-1`}>
                      <span>扣费项目</span>
                    </FieldLabel>
                    <Select
                      required
                      name="project"
                      defaultValue={pending?.project ?? ""}
                    >
                      <SelectTrigger
                        id={`${formFieldId}-field-1`}
                        className="mt-1 w-full"
                      >
                        <SelectValue placeholder={"选择有权访问的项目"} />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectGroup>
                          {projects.map((p) =>
                            p.id ? (
                              <SelectItem key={p.id} value={p.id}>
                                {p.name}
                              </SelectItem>
                            ) : null,
                          )}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </Field>
                ) : null}
                <Field>
                  <FieldLabel htmlFor={`${formFieldId}-field-2`}>
                    <span>验证模式</span>
                  </FieldLabel>
                  <Select
                    name="mode"
                    required
                    value={selectedMode}
                    onValueChange={(value) => {
                      if (!release.required_modes?.includes(value)) return;
                      setSelectedMode(value);
                      setReferences([]);
                      if (version)
                        setParams(
                          initialParams(
                            fieldsForMode(version.param_schema ?? [], value),
                          ),
                        );
                    }}
                  >
                    <SelectTrigger
                      id={`${formFieldId}-field-2`}
                      className="mt-1 w-full"
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        {release.required_modes?.map((mode) => (
                          <SelectItem key={mode} value={mode}>
                            {mode}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </Field>
                <Field>
                  <FieldLabel htmlFor={`${formFieldId}-field-3`}>
                    <span>提示词</span>
                  </FieldLabel>
                  <Textarea
                    id={`${formFieldId}-field-3`}
                    name="prompt"
                    value={prompt}
                    onChange={(e) => setPrompt(e.target.value)}
                    required={!["avatar", "motion"].includes(selectedMode)}
                    disabled={selectedMode === "motion"}
                    maxLength={65536}
                    className="mt-1 min-h-24 w-full"
                  />
                </Field>
                <div className="grid gap-3 sm:grid-cols-2">
                  <Field>
                    <FieldLabel htmlFor={`${formFieldId}-field-4`}>
                      <span>本次平台费用上限（元）</span>
                    </FieldLabel>
                    <Input
                      id={`${formFieldId}-field-4`}
                      name="charge"
                      value={charge}
                      onChange={(e) => setCharge(e.target.value)}
                      type="number"
                      min="0"
                      step="0.000001"
                      required
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor={`${formFieldId}-field-5`}>
                      <span>
                        供应商预算（{release.upstream_cost?.currency}）
                      </span>
                    </FieldLabel>
                    <Input
                      id={`${formFieldId}-field-5`}
                      name="supplier"
                      type="number"
                      min="0"
                      step="0.000001"
                      required
                      defaultValue={
                        (release.upstream_cost?.max_micros ?? 0) / 1_000_000
                      }
                    />
                  </Field>
                </div>
                {version ? (
                  <>
                    <GenerationParameters
                      fields={fields}
                      params={params}
                      maxOutputs={1}
                      disabled={disabled || !!pending}
                      compact
                      onChange={setParams}
                    />
                    <TrialReferences
                      projectID={project?.id}
                      roles={roles}
                      value={references}
                      disabled={disabled || !!pending}
                      onChange={setReferences}
                    />
                  </>
                ) : (
                  <Collapsible>
                    <CollapsibleTrigger asChild>
                      <Button
                        type="button"
                        variant="ghost"
                        className="group/collapsible h-auto w-full justify-between whitespace-normal"
                      >
                        <span>模型参数与参考素材</span>
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
                      <Field>
                        <FieldLabel htmlFor={`${formFieldId}-field-6`}>
                          <span>参数 JSON</span>
                        </FieldLabel>
                        <Textarea
                          id={`${formFieldId}-field-6`}
                          name="params"
                          defaultValue="{}"
                          className="mt-1 w-full"
                        />
                      </Field>
                      <Field>
                        <FieldLabel htmlFor={`${formFieldId}-field-7`}>
                          <span>参考素材列表（role、media_asset_id）</span>
                        </FieldLabel>
                        <Textarea
                          id={`${formFieldId}-field-7`}
                          name="references"
                          defaultValue="[]"
                          className="mt-1 w-full"
                        />
                      </Field>
                    </CollapsibleContent>
                  </Collapsible>
                )}
                <Button
                  disabled={
                    disabled ||
                    !!pending ||
                    (!!version &&
                      (!project ||
                        !budget ||
                        !!budgetError ||
                        Math.round(Number(charge) * 1e6) >
                          (budget?.available_micros ?? 0) ||
                        charge === "" ||
                        missingRole ||
                        missingParam ||
                        (!["avatar", "motion"].includes(selectedMode) &&
                          !prompt.trim())))
                  }
                >
                  {embedded ? "准备验证" : "准备免费报价"}
                </Button>
              </FieldGroup>
            </FieldSet>
          </form>
        )}
        {pending && (
          <Button
            type="button"
            variant="outline"
            disabled={disabled}
            onClick={() => void run(() => refresh(pending))}
          >
            查询报价回执 / 刷新任务
          </Button>
        )}
        {task && (
          <div className="space-y-2 text-sm">
            <p className="text-muted-foreground">
              任务状态：{task.status}
              {task.settled_micros !== undefined &&
                ` · 已结算 ${formatBillingMicros(task.settled_micros)}`}
            </p>
            {task.status === "quoted" ? (
              <QuoteCostDetail amount={task.quote_micros} />
            ) : null}
            {task.failure_code && (
              <Alert variant="destructive">
                <AlertDescription>{task.failure_code}</AlertDescription>
              </Alert>
            )}
            {task.status === "quoted" ? (
              <Collapsible>
                <CollapsibleTrigger asChild>
                  <Button type="button" variant="ghost">
                    费用与输入详情
                  </Button>
                </CollapsibleTrigger>
                <CollapsibleContent
                  className="flex flex-col gap-2 pt-3 text-xs text-muted-foreground"
                  aria-label="本次验证摘要"
                >
                  <p>
                    模型：{task.model_name ?? task.model_key ?? "已冻结模型"} ·
                    模式：{task.mode ?? "见已冻结报价"} · 输出：
                    {task.output_count ?? 1} 个
                  </p>
                  <p>
                    参考素材：
                    {task.inputs?.filter((input) => input.media_asset_id)
                      .length ?? 0}{" "}
                    项；提示词：
                    {task.inputs
                      ?.filter((input) => input.text)
                      .map((input) => input.text)
                      .join(" ")
                      .slice(0, 120) || "以已冻结输入为准"}
                  </p>
                  {Object.entries(task.params ?? {})
                    .filter(([name]) => fields.some((f) => f.field === name))
                    .map(([name, value]) => (
                      <p key={name}>
                        {fields.find((f) => f.field === name)?.label ?? name}：
                        {typeof value === "string" ||
                        typeof value === "number" ||
                        typeof value === "boolean"
                          ? String(value)
                          : "已冻结"}
                      </p>
                    ))}
                  <p>
                    供应商预算：{frozenBudget?.currency ?? "读取中"}{" "}
                    {frozenBudget
                      ? (frozenBudget.supplier / 1e6).toFixed(6)
                      : "待确认"}
                    ；平台报价{" "}
                    {task.quote_micros === undefined
                      ? "待核验"
                      : formatBillingMicros(task.quote_micros)}
                    。
                  </p>
                  <p>
                    以上为已冻结的原请求。修改输入需重新准备报价，确认前不会发起生成。
                  </p>
                </CollapsibleContent>
              </Collapsible>
            ) : null}
            {task.status === "quoted" && pending && (
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  void run(async () => {
                    save({ ...pending, confirmRequested: true });
                    await confirmOperationQuote(
                      { id: task.id! },
                      { project_id: pending.project },
                      { headers: { "Idempotency-Key": pending.confirm } },
                    );
                    await refresh(pending);
                  });
                }}
              >
                <FieldGroup className="gap-2">
                  <Button
                    disabled={
                      disabled ||
                      !frozenBudget ||
                      typeof task.quote_micros !== "number"
                    }
                  >
                    确认并试运行
                  </Button>
                </FieldGroup>
              </form>
            )}
            {task.id && task.project_id && task.status !== "quoted" && (
              <Collapsible>
                <CollapsibleTrigger asChild>
                  <Button
                    type="button"
                    variant="ghost"
                    className="group/collapsible h-auto w-full justify-between whitespace-normal"
                  >
                    <span>查看试运行结果与执行记录</span>
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
                  <TrialResult id={task.id} projectID={task.project_id} />
                </CollapsibleContent>
              </Collapsible>
            )}
            {task.status === "completed" && (
              <Button type="button" variant="outline" onClick={onVerified}>
                刷新模型验证记录
              </Button>
            )}
            {(terminal.has(task.status ?? "") ||
              (task.status === "quoted" && !pending?.confirmRequested)) && (
              <Button
                type="button"
                variant="outline"
                disabled={disabled}
                onClick={() => {
                  setDismissed(
                    pending?.operation ?? recovery?.operation_id ?? "",
                  );
                  pageBusinessMemory.removeItem(storageKey);
                  window.dispatchEvent(new Event(event));
                  setTask(null);
                  setPreparedBudget(null);
                  setMessage("");
                }}
              >
                准备另一项试运行
              </Button>
            )}
          </div>
        )}
      </CollapsibleContent>
    </Collapsible>
  );
}
