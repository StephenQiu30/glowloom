"use client";

import { useAuthSessionState } from "@/hooks/use-auth-session";
import { useMeQuery } from "@/features/identity/endpoints";
import { useProjectQuery } from "@/features/project/endpoints";
import { useState } from "react";
import { type TaskFilter, knownSubjectTypes, type DecisionValue } from "./review-presentation";
import {
  useHumanTasksQuery,
  useHumanTaskQuery,
  useClaimHumanTaskMutation,
  useRenewHumanTaskClaimMutation,
  useReleaseHumanTaskClaimMutation,
  useDecideHumanTaskMutation,
  useResumeHumanGateMutation,
  useStructureIdentityQuery,
} from "@/features/review/endpoints";
import { useWorkflowRunQuery } from "@/features/workflow/endpoints";
import { appApiErrorMessage } from "@/lib/server-state";
import { StudioShell } from "@/components/studio/studio-shell";
import { LoaderCircle, AlertCircle, ShieldAlert, CheckCircle2 } from "lucide-react";
import { LayoutContainer } from "@/components/layout/layout-container";
import { PageHeader } from "@/components/studio/page-header";
import { Button } from "@/components/ui/button";
import Link from "next/link";
import { Alert, AlertTitle, AlertDescription } from "@/components/ui/alert";
import { TaskQueue } from "./task-queue";
import { EmptyDetail, SubjectPanel } from "./review-subject-panel";
import { TaskStatusPanel } from "./review-status-panel";
import { ReviewActions } from "./review-actions";
import { WorkflowFactPanel } from "./workflow-fact-panel";
import { StructureIdentityResultPanel } from "./structure-identity-result-panel";

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
  const [repairRequest, setRepairRequest] = useState<API.HumanGateChangeRequest | null>(null);
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
  const structureIdentityRequired = task?.subject_type === "structure_identity_gate_input"
    && detail?.decision?.decision === "approved";
  const structureIdentityReady = structureIdentityRequired
    && coordination?.owner_apply_status === "completed"
    && Boolean(coordination.owner_receipt_id);
  const structureIdentityQuery = useStructureIdentityQuery(projectId, {
    skip: !authenticated || !structureIdentityReady,
    pollingInterval: 5_000,
    refetchOnFocus: true,
    refetchOnReconnect: true,
  });
  const structureIdentityVerified = Boolean(
    !structureIdentityRequired
    || (structureIdentityQuery.data
      && detail?.decision
      && structureIdentityQuery.data.command_receipt_id === coordination?.owner_receipt_id
      && structureIdentityQuery.data.version.review_decision_id === detail.decision.id
      && structureIdentityQuery.data.version.gate_input_id === task?.subject_id
      && structureIdentityQuery.data.version.gate_input_hash === task?.subject_hash),
  );
  const ownerEvidenceReady = coordination?.owner_apply_status === "not_required"
    || (coordination?.owner_apply_status === "completed"
      && Boolean(coordination.owner_receipt_id));
  const workflowFactVerified = coordination?.workflow_resume_status === "completed"
    && ownerEvidenceReady
    && (detail?.decision?.decision === "changes_requested"
      ? Boolean(coordination.repair_workflow_run_id)
      : Boolean(gateNode)
        && gateNode?.status !== "WAITING_HUMAN"
        && gateNode?.status !== "QUEUED"
        && gateNode?.status !== "RUNNING"
        && gateNode?.status !== "RETRYING"
        && Boolean(gateNode?.output_hash.trim()))
    && structureIdentityVerified;

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
    if (decision === "changes_requested" && !repairRequest) return;
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
          ...(decision === "changes_requested" && repairRequest
            ? { change_request: repairRequest }
            : {}),
          idempotency_key: commandKey(
            "human-task-decision",
            `${task.id}:${task.revision}:${decision}:${effectiveCandidate}:${repairRequest?.change_spec.operation ?? ""}`,
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
                setRepairRequest(null);
              }}
              onSelect={(taskId) => {
                setSelectedTaskId(taskId);
                setSelectedCandidateId("");
                setRepairRequest(null);
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
                    onRepairChange={setRepairRequest}
                    projectId={projectId}
                    repairRequest={repairRequest}
                    subject={detail.subject}
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
                    repairRequest={repairRequest}
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

                  {structureIdentityReady ? (
                    <StructureIdentityResultPanel
                      data={structureIdentityQuery.data}
                      error={structureIdentityQuery.error}
                      isFetching={structureIdentityQuery.isFetching}
                      verified={structureIdentityVerified}
                    />
                  ) : null}
                </div>
              )}
            </section>
          </div>
        )}
      </LayoutContainer>
    </StudioShell>
  );
}
