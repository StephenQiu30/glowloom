"use client";

import {
  AlertCircle,
  CheckCircle2,
  Clock3,
  ExternalLink,
  LoaderCircle,
  RefreshCcw,
  ShieldAlert,
} from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { LayoutContainer } from "@/components/layout/layout-container";
import { PageHeader } from "@/components/studio/page-header";
import { StudioShell } from "@/components/studio/studio-shell";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { useAuthSessionState } from "@/hooks/use-auth-session";
import { cn } from "@/lib/class-names";
import {
  appApiErrorMessage,
  useClaimHumanTaskMutation,
  useDecideHumanTaskMutation,
  useHumanTaskQuery,
  useHumanTasksQuery,
  useMeQuery,
  useProjectQuery,
  useReleaseHumanTaskClaimMutation,
  useRenewHumanTaskClaimMutation,
  useResumeHumanGateMutation,
  useWorkflowRunQuery,
} from "@/lib/server-state";

type TaskFilter = "active" | API.HumanTaskBaseResponse["status"];
type DecisionValue = API.HumanTaskDecisionRequest["decision"];

const taskFilterLabels: Record<TaskFilter, string> = {
  active: "待处理",
  OPEN: "未领取",
  CLAIMED: "处理中",
  COMPLETED: "已决议",
  CANCELLED: "已取消",
  STALE: "已过期",
};

const taskStatusLabels: Record<API.HumanTaskBaseResponse["status"], string> = {
  OPEN: "待领取",
  CLAIMED: "处理中",
  COMPLETED: "已决议",
  CANCELLED: "已取消",
  STALE: "事实已过期",
};

const decisionLabels: Record<DecisionValue, string> = {
  approved: "接受",
  rejected: "拒绝",
  changes_requested: "要求修改",
  selected: "确认选择",
};

const knownSubjectTypes = new Set([
  "workflow_node_output",
  "generation_candidate_selection",
]);

export function ReviewWorkbench({
  initialTaskId,
  projectId,
}: {
  initialTaskId?: string;
  projectId: string;
}) {
  const sessionState = useAuthSessionState();
  const authenticated = sessionState === "authenticated";
  const me = useMeQuery(undefined, { skip: !authenticated });
  const projectQuery = useProjectQuery(projectId, { skip: !authenticated });
  const [statusFilter, setStatusFilter] = useState<TaskFilter>("active");
  const [selectedTaskId, setSelectedTaskId] = useState("");
  const [selectedCandidateId, setSelectedCandidateId] = useState("");
  const [commandMessage, setCommandMessage] = useState<string>();
  const [commandFailed, setCommandFailed] = useState(false);

  const listQuery = useHumanTasksQuery(
    { projectId, status: statusFilter },
    {
      skip: !authenticated,
      pollingInterval: 10_000,
      refetchOnFocus: true,
      refetchOnReconnect: true,
    },
  );
  const requestedTaskId = selectedTaskId
    || initialTaskId?.trim()
    || listQuery.data?.items[0]?.id
    || "";
  const detailQuery = useHumanTaskQuery(requestedTaskId, {
    skip: !authenticated || !requestedTaskId,
    pollingInterval: 5_000,
    refetchOnFocus: true,
    refetchOnReconnect: true,
  });
  const detail = detailQuery.data;
  const task = detail?.task;
  const workflowQuery = useWorkflowRunQuery(task?.workflow_run_id ?? "", {
    skip: !authenticated || !task,
    pollingInterval: 5_000,
    refetchOnFocus: true,
    refetchOnReconnect: true,
  });

  const [claimTask, claimState] = useClaimHumanTaskMutation();
  const [renewClaim, renewState] = useRenewHumanTaskClaimMutation();
  const [releaseClaim, releaseState] = useReleaseHumanTaskClaimMutation();
  const [decideTask, decideState] = useDecideHumanTaskMutation();
  const [resumeHumanGate, resumeState] = useResumeHumanGateMutation();

  const canWrite = projectQuery.data?.status === "active"
    && me.data?.workspace.role !== "viewer";
  const knownSubject = Boolean(task && knownSubjectTypes.has(task.subject_type));
  const claimToken = task?.claim?.claim_token;
  const effectiveCandidate = task?.candidate_ids.includes(selectedCandidateId)
    ? selectedCandidateId
    : "";
  const busy = claimState.isLoading
    || renewState.isLoading
    || releaseState.isLoading
    || decideState.isLoading
    || resumeState.isLoading;
  const gateNode = workflowQuery.data?.nodes.find((node) => node.id === task?.node_run_id);
  const coordination = detail?.coordination;
  const ownerEvidenceReady = coordination?.owner_apply_status === "not_required"
    || (coordination?.owner_apply_status === "completed"
      && Boolean(coordination.owner_receipt_id));
  const workflowFactVerified = coordination?.workflow_resume_status === "completed"
    && ownerEvidenceReady
    && Boolean(gateNode)
    && gateNode?.status !== "WAITING_HUMAN"
    && gateNode?.status !== "QUEUED"
    && gateNode?.status !== "RUNNING"
    && gateNode?.status !== "RETRYING"
    && Boolean(gateNode?.output_hash.trim());

  function commandKey(action: string, identity: string): string {
    return `${action}:${identity}`;
  }

  async function runCommand(
    operation: () => Promise<unknown>,
    successMessage: string,
  ) {
    setCommandMessage(undefined);
    setCommandFailed(false);
    try {
      await operation();
      setCommandMessage(successMessage);
    } catch (error: unknown) {
      setCommandFailed(true);
      setCommandMessage(appApiErrorMessage(error));
      await detailQuery.refetch();
    }
  }

  async function handleClaim() {
    if (!task) return;
    await runCommand(
      () => claimTask({
        projectId,
        taskId: task.id,
        body: {
          expected_revision: task.revision,
          idempotency_key: commandKey(
            "human-task-claim",
            `${task.id}:${task.revision}`,
          ),
        },
      }).unwrap(),
      "审核已领取；租约只保存在当前受保护详情中。",
    );
  }

  async function handleClaimCommand(action: "renew" | "release") {
    if (!task || !claimToken) return;
    const mutation = action === "renew" ? renewClaim : releaseClaim;
    await runCommand(
      () => mutation({
        projectId,
        taskId: task.id,
        body: {
          claim_token: claimToken,
          expected_revision: task.revision,
          idempotency_key: commandKey(
            `human-task-${action}`,
            `${task.id}:${task.revision}`,
          ),
        },
      }).unwrap(),
      action === "renew" ? "审核租约已续期。" : "审核已释放。",
    );
  }

  async function handleDecision(decision: DecisionValue) {
    if (!task || !claimToken || detail?.decision) return;
    if (decision === "selected" && !effectiveCandidate) return;
    await runCommand(
      () => decideTask({
        projectId,
        taskId: task.id,
        workflowRunId: task.workflow_run_id,
        body: {
          claim_token: claimToken,
          expected_task_revision: task.revision,
          expected_subject_revision: task.subject_revision,
          expected_subject_hash: task.subject_hash,
          decision,
          selected_candidate_id: decision === "selected"
            ? effectiveCandidate
            : null,
          idempotency_key: commandKey(
            "human-task-decision",
            `${task.id}:${task.revision}:${decision}:${effectiveCandidate}`,
          ),
        },
      }).unwrap(),
      "决议已记录；页面会继续核对业务应用和工作流恢复。",
    );
  }

  async function handleResume() {
    if (!task || !detail?.decision) return;
    await runCommand(
      () => resumeHumanGate({
        projectId,
        taskId: task.id,
        decisionId: detail.decision!.id,
        workflowRunId: task.workflow_run_id,
      }).unwrap(),
      "已按原决议恢复；页面会从服务端重取运行事实。",
    );
  }

  const pageError = me.error ?? projectQuery.error ?? listQuery.error;

  if (sessionState === "checking") {
    return (
      <StudioShell active="projects">
        <div className="grid min-h-[70dvh] place-items-center">
          <LoaderCircle
            aria-label="正在读取审核权限"
            className="size-5 animate-spin"
          />
        </div>
      </StudioShell>
    );
  }

  return (
    <StudioShell
      active="projects"
      projectName={projectQuery.data?.name}
      viewer={me.data ? {
        displayName: me.data.user.display_name?.trim() || me.data.user.email,
        workspaceName: me.data.workspace.name,
      } : undefined}
    >
      <LayoutContainer className="py-8 sm:py-10">
        <PageHeader
          actions={(
            <Button asChild variant="outline">
              <Link href={`/projects/${projectId}`}>返回项目</Link>
            </Button>
          )}
          badges={[{ label: "Backend 事实" }, { label: "自动刷新" }]}
          breadcrumbs={[
            { label: "项目", href: "/projects" },
            { label: projectQuery.data?.name ?? "项目", href: `/projects/${projectId}` },
            { label: "审核工作台" },
          ]}
          description="领取冻结任务、记录不可变决议，并分别确认业务应用与工作流恢复。"
          note="Claim Token 不进入地址、浏览器存储或列表；结果未知时只按原决议恢复。"
          title="审核工作台"
        />

        {!authenticated ? (
          <Alert className="mt-6">
            <AlertCircle aria-hidden="true" />
            <AlertTitle>需要登录</AlertTitle>
            <AlertDescription><Link href="/login">登录后查看审核队列</Link></AlertDescription>
          </Alert>
        ) : pageError ? (
          <Alert className="mt-6" variant="destructive">
            <AlertCircle aria-hidden="true" />
            <AlertTitle>审核事实暂时无法读取</AlertTitle>
            <AlertDescription>{appApiErrorMessage(pageError)}</AlertDescription>
          </Alert>
        ) : (
          <div className="mt-7 grid gap-6 lg:grid-cols-[minmax(17rem,0.72fr)_minmax(0,1.6fr)]">
            <TaskQueue
              isLoading={listQuery.isLoading}
              onFilterChange={(value) => {
                setStatusFilter(value);
                setSelectedTaskId("");
                setSelectedCandidateId("");
              }}
              onSelect={(taskId) => {
                setSelectedTaskId(taskId);
                setSelectedCandidateId("");
                setCommandMessage(undefined);
              }}
              selectedTaskId={requestedTaskId}
              statusFilter={statusFilter}
              tasks={listQuery.data?.items ?? []}
            />

            <section aria-label="审核任务详情" className="min-w-0">
              {!requestedTaskId ? (
                <EmptyDetail />
              ) : detailQuery.isLoading && !detail ? (
                <div className="grid min-h-96 place-items-center border bg-card">
                  <LoaderCircle
                    aria-label="正在加载审核详情"
                    className="size-5 animate-spin"
                  />
                </div>
              ) : detailQuery.error || !task ? (
                <Alert variant="destructive">
                  <AlertCircle aria-hidden="true" />
                  <AlertTitle>审核详情无法读取</AlertTitle>
                  <AlertDescription>{appApiErrorMessage(detailQuery.error)}</AlertDescription>
                </Alert>
              ) : (
                <div className="space-y-5">
                  {!canWrite ? (
                    <Alert>
                      <ShieldAlert aria-hidden="true" />
                      <AlertTitle>当前身份为只读</AlertTitle>
                      <AlertDescription>
                        你可以检查冻结事实和恢复状态，但不能领取或提交决议。
                      </AlertDescription>
                    </Alert>
                  ) : null}
                  {!knownSubject ? (
                    <Alert>
                      <ShieldAlert aria-hidden="true" />
                      <AlertTitle>当前 Subject 类型仅支持只读查看</AlertTitle>
                      <AlertDescription>
                        未注册的 Subject 不会猜测渲染器或允许动作，请等待对应 Owner 接入。
                      </AlertDescription>
                    </Alert>
                  ) : null}

                  <TaskStatusPanel
                    coordination={coordination}
                    decision={detail.decision}
                    task={task}
                    workflowFactVerified={Boolean(workflowFactVerified)}
                  />

                  <SubjectPanel
                    canDecide={Boolean(
                      canWrite
                      && knownSubject
                      && claimToken
                      && task.status === "CLAIMED"
                      && !detail.decision,
                    )}
                    effectiveCandidate={effectiveCandidate}
                    onCandidateChange={setSelectedCandidateId}
                    projectId={projectId}
                    task={task}
                  />

                  <ReviewActions
                    busy={busy}
                    canWrite={Boolean(canWrite && knownSubject)}
                    claimToken={claimToken}
                    decision={detail.decision}
                    effectiveCandidate={effectiveCandidate}
                    onClaim={handleClaim}
                    onDecision={handleDecision}
                    onRelease={() => handleClaimCommand("release")}
                    onRenew={() => handleClaimCommand("renew")}
                    onResume={handleResume}
                    task={task}
                    coordination={coordination}
                  />

                  {commandMessage ? (
                    <Alert variant={commandFailed ? "destructive" : "default"}>
                      {commandFailed
                        ? <AlertCircle aria-hidden="true" />
                        : <CheckCircle2 aria-hidden="true" />}
                      <AlertTitle>{commandFailed ? "命令未完成" : "服务端事实已更新"}</AlertTitle>
                      <AlertDescription>{commandMessage}</AlertDescription>
                    </Alert>
                  ) : null}

                  <WorkflowFactPanel
                    coordination={coordination}
                    error={workflowQuery.error}
                    gateNode={gateNode}
                    isFetching={workflowQuery.isFetching}
                    run={workflowQuery.data?.run}
                    verified={Boolean(workflowFactVerified)}
                  />
                </div>
              )}
            </section>
          </div>
        )}
      </LayoutContainer>
    </StudioShell>
  );
}

function TaskQueue({
  isLoading,
  onFilterChange,
  onSelect,
  selectedTaskId,
  statusFilter,
  tasks,
}: {
  isLoading: boolean;
  onFilterChange: (value: TaskFilter) => void;
  onSelect: (taskId: string) => void;
  selectedTaskId: string;
  statusFilter: TaskFilter;
  tasks: API.HumanTaskListItemResponse[];
}) {
  return (
    <aside aria-label="审核任务队列" className="h-fit border bg-card">
      <div className="flex items-center justify-between gap-3 border-b p-4">
        <div>
          <h2 className="font-semibold">项目任务</h2>
          <p className="mt-1 text-xs text-muted-foreground">服务端稳定排序，10 秒自动刷新</p>
        </div>
        <Badge variant="outline">{tasks.length}</Badge>
      </div>
      <div className="border-b p-4">
        <label className="grid gap-1.5 text-xs font-medium" htmlFor="review-task-status">
          任务状态筛选
          <select
            className="h-9 rounded-lg border border-input bg-background px-3 text-sm"
            id="review-task-status"
            onChange={(event) => onFilterChange(event.target.value as TaskFilter)}
            value={statusFilter}
          >
            {Object.entries(taskFilterLabels).map(([value, label]) => (
              <option key={value} value={value}>{label}</option>
            ))}
          </select>
        </label>
      </div>
      {isLoading ? (
        <div className="grid min-h-32 place-items-center">
          <LoaderCircle aria-label="正在加载审核队列" className="size-4 animate-spin" />
        </div>
      ) : tasks.length === 0 ? (
        <p className="p-6 text-center text-sm text-muted-foreground">当前筛选下没有审核任务。</p>
      ) : (
        <ul className="divide-y">
          {tasks.map((task) => (
            <li key={task.id}>
              <button
                aria-pressed={selectedTaskId === task.id}
                className={cn(
                  "w-full p-4 text-left transition-colors hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset",
                  selectedTaskId === task.id && "bg-muted/70",
                )}
                onClick={() => onSelect(task.id)}
                type="button"
              >
                <span className="flex items-center justify-between gap-3">
                  <span className="truncate text-sm font-medium">{subjectLabel(task.subject_type)}</span>
                  <Badge variant={task.status === "STALE" ? "destructive" : "outline"}>
                    {taskStatusLabels[task.status]}
                  </Badge>
                </span>
                <span className="mt-2 block truncate font-mono text-[11px] text-muted-foreground">
                  {shortId(task.id)} · revision {task.revision}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </aside>
  );
}

function EmptyDetail() {
  return (
    <div className="grid min-h-96 place-items-center border bg-card p-8 text-center">
      <div>
        <Clock3 aria-hidden="true" className="mx-auto size-6 text-muted-foreground" />
        <h2 className="mt-3 font-semibold">选择一个审核任务</h2>
        <p className="mt-1 text-sm text-muted-foreground">详情只读取 Backend 已冻结的事实。</p>
      </div>
    </div>
  );
}

function TaskStatusPanel({
  coordination,
  decision,
  task,
  workflowFactVerified,
}: {
  coordination: API.HumanGateCoordinationResponse | null | undefined;
  decision: API.ReviewDecisionResponse | null;
  task: API.HumanTaskResponse;
  workflowFactVerified: boolean;
}) {
  const ownerText = !coordination
    ? "尚未开始"
    : coordination.owner_apply_status === "completed"
      ? coordination.workflow_resume_status === "completed"
        ? "业务应用已完成"
        : "业务应用完成，正在恢复工作流"
      : coordination.owner_apply_status === "not_required"
        ? "此决议无需业务应用"
        : coordination.owner_apply_status === "conflict"
          ? "业务应用冲突"
          : "等待业务应用";
  const workflowText = !coordination
    ? "尚未开始"
    : coordination.workflow_resume_status === "unknown"
      ? "结果未知，可安全恢复"
      : coordination.workflow_resume_status === "completed"
        ? workflowFactVerified
          ? "工作流已继续"
          : "恢复已确认，正在核对运行事实"
        : coordination.workflow_resume_status === "conflict"
          ? "工作流恢复冲突"
          : "等待恢复";

  return (
    <section aria-label="审核状态" className="grid gap-px overflow-hidden border bg-border sm:grid-cols-2 xl:grid-cols-4" role="region">
      <StatusCell label="任务状态" value={taskStatusLabels[task.status]} />
      <StatusCell
        label="决议状态"
        meta={decision ? shortId(decision.id) : undefined}
        value={decision ? "决议已记录" : "尚未记录"}
      />
      <StatusCell
        label="业务应用"
        meta={coordination?.owner_receipt_id ? shortId(coordination.owner_receipt_id) : undefined}
        value={ownerText}
      />
      <StatusCell
        label="工作流恢复"
        meta={coordination?.workflow_signal_receipt_id
          ? shortId(coordination.workflow_signal_receipt_id)
          : undefined}
        value={workflowText}
      />
    </section>
  );
}

function StatusCell({ label, meta, value }: { label: string; meta?: string; value: string }) {
  return (
    <div className="min-h-28 bg-card p-4">
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <p className="mt-3 text-sm font-semibold">{value}</p>
      {meta ? <p className="mt-1 font-mono text-[11px] text-muted-foreground">{meta}</p> : null}
    </div>
  );
}

function SubjectPanel({
  canDecide,
  effectiveCandidate,
  onCandidateChange,
  projectId,
  task,
}: {
  canDecide: boolean;
  effectiveCandidate: string;
  onCandidateChange: (value: string) => void;
  projectId: string;
  task: API.HumanTaskResponse;
}) {
  const selectionSubject = task.subject_type === "generation_candidate_selection";
  return (
    <Card className="border" id="subject-fact">
      <CardHeader className="border-b">
        <CardTitle>冻结 Subject</CardTitle>
        <CardDescription>
          revision、hash、候选和 rubric 均来自 HumanTask，页面不能改写。
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-5 pt-1">
        <div className="grid gap-4 text-sm sm:grid-cols-2">
          <Fact label="类型" value={subjectLabel(task.subject_type)} />
          <Fact label="Subject ID" value={shortId(task.subject_id)} mono />
          <Fact label="Subject revision" value={String(task.subject_revision)} />
          <Fact label="Task revision" value={String(task.revision)} />
          <Fact label="Rubric" value={task.rubric_version} mono />
          <Fact label="Subject hash" value={shortHash(task.subject_hash)} mono />
        </div>
        <Button asChild className="w-fit" size="sm" variant="outline">
          <Link href={`/projects/${projectId}/reviews?task=${task.id}#subject-fact`}>
            打开固定 Subject 链接
            <ExternalLink aria-hidden="true" />
          </Link>
        </Button>
        {selectionSubject ? (
          <fieldset className="grid gap-2" disabled={!canDecide}>
            <legend className="mb-2 text-sm font-semibold">冻结候选</legend>
            {task.candidate_ids.map((candidateId) => (
              <label className="flex cursor-pointer items-center gap-3 border p-3 text-sm has-checked:border-foreground has-checked:bg-muted/50" key={candidateId}>
                <input
                  checked={effectiveCandidate === candidateId}
                  className="size-4"
                  name={`candidate-${task.id}`}
                  onChange={() => onCandidateChange(candidateId)}
                  type="radio"
                  value={candidateId}
                />
                <span className="font-mono text-xs">{candidateId}</span>
              </label>
            ))}
          </fieldset>
        ) : task.candidate_ids.length > 0 ? (
          <div>
            <h3 className="text-sm font-semibold">冻结输入引用</h3>
            <ul className="mt-2 grid gap-2">
              {task.candidate_ids.map((candidateId) => (
                <li className="border p-3 font-mono text-xs" key={candidateId}>{candidateId}</li>
              ))}
            </ul>
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}

function Fact({ label, mono = false, value }: { label: string; mono?: boolean; value: string }) {
  return (
    <div>
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <p className={cn("mt-1 break-all", mono && "font-mono text-xs")}>{value}</p>
    </div>
  );
}

function ReviewActions({
  busy,
  canWrite,
  claimToken,
  coordination,
  decision,
  effectiveCandidate,
  onClaim,
  onDecision,
  onRelease,
  onRenew,
  onResume,
  task,
}: {
  busy: boolean;
  canWrite: boolean;
  claimToken?: string;
  coordination: API.HumanGateCoordinationResponse | null | undefined;
  decision: API.ReviewDecisionResponse | null;
  effectiveCandidate: string;
  onClaim: () => void;
  onDecision: (decision: DecisionValue) => void;
  onRelease: () => void;
  onRenew: () => void;
  onResume: () => void;
  task: API.HumanTaskResponse;
}) {
  const canClaim = canWrite
    && !decision
    && (task.status === "OPEN" || (task.status === "CLAIMED" && !claimToken));
  const canUseClaim = canWrite && !decision && task.status === "CLAIMED" && Boolean(claimToken);
  const canResume = canWrite
    && Boolean(decision)
    && coordination?.workflow_resume_status !== "completed"
    && coordination?.workflow_resume_status !== "conflict"
    && coordination?.owner_apply_status !== "conflict";

  return (
    <Card className="border">
      <CardHeader className="border-b">
        <CardTitle>可执行动作</CardTitle>
        <CardDescription>所有写入都重新校验服务端 revision、租约和冻结事实。</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 pt-1">
        {task.claim ? (
          <div aria-label="审核租约" className="grid gap-2 border p-3 text-sm sm:grid-cols-2">
            <Fact
              label={claimToken ? "当前租约" : "租约所有者"}
              value={claimToken ? "当前账号持有" : shortId(task.claim.claimed_by)}
            />
            <div>
              <p className="text-xs font-medium text-muted-foreground">服务端到期时间</p>
              <time className="mt-1 block" dateTime={task.claim.expires_at}>
                {formatTimestamp(task.claim.expires_at)}
              </time>
            </div>
          </div>
        ) : null}
        {canClaim ? (
          <div className="flex flex-wrap items-center gap-3">
            <Button disabled={busy} onClick={onClaim}>
              {task.status === "CLAIMED" ? "尝试接管审核" : "领取审核"}
            </Button>
            {task.status === "CLAIMED" ? (
              <p className="text-xs text-muted-foreground">服务端仅在原租约过期后允许接管。</p>
            ) : null}
          </div>
        ) : null}
        {canUseClaim ? (
          <div className="flex flex-wrap gap-3">
            <Button disabled={busy} onClick={onRenew} variant="outline">续期租约</Button>
            <Button disabled={busy} onClick={onRelease} variant="outline">释放审核</Button>
          </div>
        ) : null}
        {canUseClaim ? (
          <div className="flex flex-wrap gap-3 border-t pt-4">
            {task.allowed_decisions.map((value) => (
              <Button
                disabled={busy || (value === "selected" && !effectiveCandidate)}
                key={value}
                onClick={() => onDecision(value)}
                variant={value === "rejected" ? "outline" : "default"}
              >
                {decisionLabels[value]}
              </Button>
            ))}
          </div>
        ) : null}
        {canResume ? (
          <Button className="w-fit" disabled={busy} onClick={onResume}>
            <RefreshCcw aria-hidden="true" />
            按原决议恢复工作流
          </Button>
        ) : null}
        {!canClaim && !canUseClaim && !canResume ? (
          <p className="text-sm text-muted-foreground">
            当前状态没有可执行写命令；仍可查看全部持久化状态。
          </p>
        ) : null}
      </CardContent>
    </Card>
  );
}

function WorkflowFactPanel({
  coordination,
  error,
  gateNode,
  isFetching,
  run,
  verified,
}: {
  coordination: API.HumanGateCoordinationResponse | null | undefined;
  error: unknown;
  gateNode: API.WorkflowNodeRunResponse | undefined;
  isFetching: boolean;
  run: API.WorkflowRunResponse | undefined;
  verified: boolean;
}) {
  return (
    <Card className="border" id="workflow-run">
      <CardHeader className="border-b">
        <CardTitle>WorkflowRun 复核</CardTitle>
        <CardDescription>
          Resume 完成后仍需重取匹配的 NodeRun 和 Gate Output，不能用本地成功代替。
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 pt-1">
        {error ? (
          <Alert variant="destructive">
            <AlertCircle aria-hidden="true" />
            <AlertTitle>运行事实暂时无法读取</AlertTitle>
            <AlertDescription>{appApiErrorMessage(error)}</AlertDescription>
          </Alert>
        ) : !run || !gateNode ? (
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            {isFetching ? <LoaderCircle aria-hidden="true" className="size-4 animate-spin" /> : null}
            正在重取 WorkflowRun 与审核节点事实。
          </p>
        ) : (
          <div className="grid gap-4 text-sm sm:grid-cols-2">
            <Fact label="WorkflowRun" value={shortId(run.id)} mono />
            <Fact label="Run 状态" value={run.status} />
            <Fact label="审核 NodeRun" value={shortId(gateNode.id)} mono />
            <Fact label="Node 状态" value={gateNode.status} />
            <Fact label="Gate Output hash" value={shortHash(gateNode.output_hash)} mono />
            <Fact
              label="复核结论"
              value={verified
                ? "工作流已继续"
                : coordination?.workflow_resume_status === "completed"
                  ? "恢复已确认，运行事实尚未收敛"
                  : "等待 Workflow Resume 完成"}
            />
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function subjectLabel(value: string): string {
  switch (value) {
    case "workflow_node_output":
      return "工作流节点输出";
    case "generation_candidate_selection":
      return "生成候选选择";
    default:
      return value;
  }
}

function shortId(value: string): string {
  return value.length > 16 ? `${value.slice(0, 8)}…${value.slice(-4)}` : value;
}

function shortHash(value: string): string {
  return value ? `${value.slice(0, 12)}…${value.slice(-8)}` : "尚未生成";
}

function formatTimestamp(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
    hour12: false,
  }).format(new Date(value));
}
