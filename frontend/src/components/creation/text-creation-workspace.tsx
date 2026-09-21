"use client";
import { toast } from "sonner";
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from "@/components/ui/collapsible";

import { Empty, EmptyHeader, EmptyTitle, EmptyDescription } from "@/components/ui/empty";
import { PageLoading } from "@/components/system/page-loading";
import { useState } from "react";
import { Field, FieldLabel } from "@/components/ui/field";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import Link from "next/link";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { appApiErrorMessage } from "@/lib/server-state";
import {
  useAcceptCreationSourceMutation,
  useCreationExecutionQuery,
  useCreationProposalsQuery,
  useCreationRunQuery,
  useCreationRunsQuery,
  useCurrentScriptSourceQuery,
  useRetryCreationDeliveryMutation,
  useStartCreationMutation,
  useSyncCreationMutation,
  useResumeCreationMutation,
} from "@/components/creation/endpoints";
import { stageLabels } from "@/components/creation/proposal-content";
import { ProposalReview } from "@/components/creation/proposal-review";
import { ProposalNavigation } from "@/components/creation/proposal-navigation";

const deliveryLabels: Record<API.CreationRunResponse["status"], string> = {
  queued: "正在准备解析",
  delivery_unknown: "正在确认本次创作是否启动",
  delivery_blocked: "暂时无法开始解析",
  accepted: "已接收剧本，正在准备创作",
};
const executionLabels: Record<API.CreationExecution["status"], string> = {
  queued: "已接收剧本，等待开始解析",
  running: "正在创作",
  waiting_review: "等待人工审阅与采纳",
  blocked: "执行受阻",
  rejected: "审阅未通过",
  completed: "文本分镜已完成",
};
const polling = { pollingInterval: 5000, refetchOnFocus: true, refetchOnReconnect: true };
const executionErrors: Record<string, string> = {
  context_insufficient: "本阶段上下文超过限制，已在模型调用前停止。请联系管理员核查范围。",
  skill_release_unavailable: "本次运行固定的专业能力版本不可用，请联系管理员恢复原版本。",
  input_contract_invalid: "本阶段输入未通过校验，已在模型调用前停止。",
  candidate_contract_invalid: "模型候选未通过来源或结构校验，失败候选和诊断已保存，需核查后恢复。",
  structured_output_invalid: "模型返回的结构化内容无效，已保存可用的原始输出供核查。",
  execution_output_budget_exceeded: "模型输出超过本次限制，调用已停止，需核查后恢复。",
  execution_deadline_exceeded: "本次模型执行超时并已结束，需核查后恢复。",
  platform_unavailable: "原稿或采纳服务暂时无法连接。服务恢复后，可继续本次运行。",
  invocation_budget_exhausted: "本次创作的模型调用额度已用完，运行已停止。",
  harness_response_unknown: "模型调用结果尚未确认，已停止重复调用，请联系管理员核查。",
  invocation_outcome_unknown: "模型调用结果尚未确认，已停止重复调用，请联系管理员核查。",
  harness_result_invalid: "模型结果未通过内容校验，已保留现场，请联系管理员核查。",
  review_rejected: "草案未获批准。请修改原稿后发起新的创作。",
};
export type CreationSource = {
  revisionId: string;
  contentHash: string;
  title: string;
  text: string;
};

export function TextCreationWorkspace({
  projectId,
  source,
  canWrite,
  initialRunId = "",
}: {
  projectId: string;
  source?: CreationSource;
  canWrite: boolean;
  initialRunId?: string;
}) {
  const runs = useCreationRunsQuery(projectId, polling);
  const head = useCurrentScriptSourceQuery(projectId);
  const [chosenRunId, setChosenRunId] = useState(initialRunId);
  const exactRun = useCreationRunQuery(chosenRunId, { ...polling, skip: !chosenRunId });
  const [acceptSource, acceptState] = useAcceptCreationSourceMutation();
  const [start, startState] = useStartCreationMutation();
  const [retryDelivery, retryState] = useRetryCreationDeliveryMutation();
  const [sync, syncState] = useSyncCreationMutation();
  const [resume, resumeState] = useResumeCreationMutation();
  const [chosenProposalId, setChosenProposalId] = useState("");
  const [error, setError] = useState<string>();

  const run = chosenRunId
    ? (exactRun.currentData ??
      runs.data?.find((item) => item.id === chosenRunId) ??
      (startState.data?.id === chosenRunId ? startState.data : undefined))
    : runs.data?.[0];
  const execution = useCreationExecutionQuery(run?.id ?? "", {
    ...polling,
    skip: !run || run.status !== "accepted",
  });
  const proposalsQuery = useCreationProposalsQuery(run?.id ?? "", { ...polling, skip: !run });
  const proposals = proposalsQuery.currentData ?? [];
  const selected =
    proposals.find((item) => item.id === chosenProposalId) ??
    proposals.find((item) => item.status === "needs_review") ??
    proposals.at(-1);
  const existing =
    source &&
    runs.data?.find(
      (item) =>
        item.source.revision_id === source.revisionId &&
        item.source.content_hash === source.contentHash,
    );
  const busy =
    acceptState.isLoading ||
    startState.isLoading ||
    retryState.isLoading ||
    syncState.isLoading ||
    resumeState.isLoading;
  const state = execution.currentData;
  const failureCode = state?.last_error || run?.last_error;

  function chooseRun(id: string) {
    setChosenRunId(id);
    setChosenProposalId("");
    setError(undefined);

    const url = new URL(window.location.href);
    url.searchParams.set("run", id);
    window.history.replaceState(null, "", url);
  }
  async function startCreation() {
    if (!source || head.isLoading || head.error) return;
    if (existing) {
      chooseRun(existing.id);
      return;
    }
    setError(undefined);

    try {
      if (head.data?.identity.version_id !== source.revisionId) {
        const accepted = await acceptSource({
          projectId,
          body: {
            document_revision_id: source.revisionId,
            expected_head_revision: head.data?.head_revision ?? 0,
            expected_head_hash: head.data?.head_hash ?? null,
            idempotency_key: `creation-source:${source.revisionId}:${head.data?.head_revision ?? 0}`,
          },
        }).unwrap();
        if (
          accepted.identity.version_id !== source.revisionId ||
          accepted.identity.content_hash !== source.contentHash
        )
          throw new Error("正式源与当前原稿不一致，请刷新核对。");
      } else if (head.data.identity.content_hash !== source.contentHash)
        throw new Error("正式源摘要不一致，已停止启动。");
      const created = await start({
        projectId,
        body: {
          document_revision_id: source.revisionId,
          source_hash: source.contentHash,
          idempotency_key: `text-creation:${source.revisionId}:${source.contentHash}`,
        },
      }).unwrap();
      chooseRun(created.id);
      toast.success("已创建运行。每个阶段审阅并正式采纳后，系统才会继续。");
    } catch (cause) {
      setError(appApiErrorMessage(cause));
      void head.refetch();
      void runs.refetch();
    }
  }
  async function synchronize() {
    if (!run) return;
    setError(undefined);
    try {
      await sync(run.id).unwrap();
      toast.success("已同步执行状态与提案。");
    } catch (cause) {
      setError(appApiErrorMessage(cause));
    }
  }
  async function retry() {
    if (!run) return;
    setError(undefined);
    try {
      await retryDelivery({ projectId, runId: run.id, revision: run.revision }).unwrap();
      toast.success("已按原运行恢复交接。");
    } catch (cause) {
      setError(appApiErrorMessage(cause));
    }
  }

  async function resumeRun() {
    if (!run || !state?.can_resume) return;
    setError(undefined);
    try {
      await resume({ runId: run.id, revision: run.revision }).unwrap();
      toast.success("恢复请求已接受，将沿用本次原稿、草案和剩余额度。");
    } catch (cause) {
      setError(appApiErrorMessage(cause));
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <section aria-label="固定原稿与启动" className="flex flex-col gap-4 bg-transparent py-4">
        <h2 className="text-lg font-semibold">固定原稿</h2>
        {source ? (
          <>
            <p className="font-medium">{source.title}</p>
            <Collapsible>
              <CollapsibleTrigger asChild>
                <Button
                  variant="ghost"
                  type="button"
                  className="h-auto justify-start px-0 text-left whitespace-normal"
                >
                  查看此次原稿
                </Button>
              </CollapsibleTrigger>
              <CollapsibleContent>
                <pre className="mt-3 max-h-72 overflow-auto whitespace-pre-wrap break-words rounded-md bg-muted p-4 font-sans text-sm">
                  {source.text}
                </pre>
              </CollapsibleContent>
            </Collapsible>
            <p className="break-all text-xs text-muted-foreground">版本：{source.revisionId}</p>
            {canWrite && (
              <Button
                disabled={busy || head.isLoading || Boolean(head.error) || runs.isLoading}
                onClick={() => void startCreation()}
              >
                {existing ? "查看当前原稿的创作" : "固定原稿并开始创作"}
              </Button>
            )}
          </>
        ) : (
          <p className="text-sm">
            先
            <Link className="underline" href={`/projects/${projectId}#script-import`}>
              导入剧本
            </Link>
            ，再开始文本创作。
          </p>
        )}
        {head.error && (
          <Alert variant="destructive">
            <AlertDescription>{appApiErrorMessage(head.error)}</AlertDescription>
          </Alert>
        )}
        <p className="text-sm text-muted-foreground">
          依次确认分集、剧稿结构、世界设定和导演分镜。修改原稿将保留已有运行和回执。
        </p>
      </section>
      {runs.error && (
        <Alert variant="destructive">
          <AlertDescription>{appApiErrorMessage(runs.error)}</AlertDescription>
        </Alert>
      )}
      {(runs.data?.length ?? 0) > 0 && (
        <Field>
          <FieldLabel htmlFor="creation-run">创作记录</FieldLabel>
          <Select value={run?.id ?? ""} onValueChange={chooseRun}>
            <SelectTrigger id="creation-run" className="w-full">
              <SelectValue placeholder="选择创作记录" />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {runs.data!.map((item) => (
                  <SelectItem key={item.id} value={item.id}>
                    {new Date(item.created_at).toLocaleString("zh-CN")} ·{" "}
                    {item.source.revision_id.slice(0, 8)} · {deliveryLabels[item.status]}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
      )}
      {chosenRunId && exactRun.error && (
        <Alert variant="destructive">
          <AlertDescription>{appApiErrorMessage(exactRun.error)}</AlertDescription>
        </Alert>
      )}
      {run && (
        <section aria-label="创作状态" className="flex flex-col gap-4 bg-transparent py-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <h2 className="text-lg font-semibold">
              {state ? executionLabels[state.status] : deliveryLabels[run.status]}
            </h2>
            {canWrite && (
              <div className="flex flex-wrap gap-2">
                {run.status === "delivery_blocked" && (
                  <Button disabled={busy} variant="outline" onClick={() => void retry()}>
                    继续本次创作
                  </Button>
                )}
                {run.status === "accepted" && (
                  <Button disabled={busy} variant="outline" onClick={() => void synchronize()}>
                    刷新创作内容
                  </Button>
                )}
              </div>
            )}
          </div>
          <ol aria-label="创作阶段" className="grid gap-2 sm:grid-cols-4">
            {Object.entries(stageLabels).map(([stage, label]) => {
              const items = proposals.filter((item) => item.stage === stage);
              return (
                <li className="flex flex-col gap-1 py-3 text-sm" key={stage}>
                  <p className="font-medium">{label}</p>
                  <p className="text-xs text-muted-foreground">
                    {state?.stage === stage && state.status === "running"
                      ? `已生成 ${state.steps.filter((step) => step.step_key.split("/")[0] === stage && step.state === "needs_review").length} 份草案，正在分析`
                      : items.length
                        ? `${items.filter((item) => item.status === "accepted").length}/${items.length} 已采纳`
                        : "尚未开始"}
                  </p>
                  {state?.stage === stage && <Badge variant="outline">当前阶段</Badge>}
                </li>
              );
            })}
          </ol>
          {state && state.status !== "queued" && (
            <p className="text-xs text-muted-foreground">
              已预留模型调用 {state.reserved_calls}/{state.call_limit} 次；额度在原运行内固定。
            </p>
          )}
          {canWrite && state?.status === "blocked" && state.can_resume && (
            <Button disabled={busy} onClick={() => void resumeRun()}>
              恢复原运行
            </Button>
          )}
          {failureCode && (
            <Alert variant="destructive">
              <AlertDescription>
                {executionErrors[failureCode] ??
                  "本次创作已暂停，请联系管理员核查；原稿和已保存结果仍可查回。"}
                <Collapsible className="mt-2">
                  <CollapsibleTrigger asChild>
                    <Button
                      variant="ghost"
                      type="button"
                      className="h-auto justify-start px-0 text-left whitespace-normal"
                    >
                      故障信息
                    </Button>
                  </CollapsibleTrigger>
                  <CollapsibleContent>{failureCode}</CollapsibleContent>
                </Collapsible>
              </AlertDescription>
            </Alert>
          )}
          {state?.steps.some((step) => step.state === "unknown") && (
            <p className="text-sm">
              模型调用结果尚未确认，系统已停止重复调用。保留本次运行，核查原调用结果后再恢复。
            </p>
          )}
          {state?.status === "completed" && (
            <p className="text-sm">
              所有文本阶段已正式采纳，可随时查回下方内容和回执。视觉素材与视频制作尚未开始。
            </p>
          )}
          {execution.error && (execution.error as { code?: string }).code !== "not_found" && (
            <p role="status" className="text-sm text-muted-foreground">
              执行状态暂未读到：{appApiErrorMessage(execution.error)}
            </p>
          )}
          <Collapsible>
            <CollapsibleTrigger asChild>
              <Button
                variant="ghost"
                type="button"
                className="h-auto justify-start px-0 text-left whitespace-normal"
              >
                运行与原稿身份
              </Button>
            </CollapsibleTrigger>
            <CollapsibleContent>
              <p className="mt-2 break-all text-xs text-muted-foreground">
                运行：{run.id}
                <br />
                原稿：{run.source.revision_id}
                <br />
                摘要：{run.source.content_hash}
              </p>
            </CollapsibleContent>
          </Collapsible>
        </section>
      )}
      {proposalsQuery.error && (
        <Alert variant="destructive">
          <AlertDescription>{appApiErrorMessage(proposalsQuery.error)}</AlertDescription>
        </Alert>
      )}
      {run && proposalsQuery.isLoading && <PageLoading label="正在读取剧集、设定与分镜" />}
      {run && !proposalsQuery.isLoading && !proposalsQuery.error && proposals.length === 0 && (
        <Empty className="py-8">
          <EmptyHeader>
            <EmptyTitle>创作内容将在这里呈现</EmptyTitle>
            <EmptyDescription>
              生成后可逐步查看分集、场景、人物与分镜。
              {state?.status === "blocked"
                ? "当前创作受阻，请先查看上方说明。"
                : "每个阶段确认并采纳后，才会进入下一阶段。"}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}
      {proposals.length > 0 && (
        <div className="grid items-start gap-8 py-6 lg:grid-cols-[15rem_minmax(0,1fr)]">
          <ProposalNavigation
            key={run?.id}
            proposals={proposals}
            selectedId={selected?.id}
            onSelect={setChosenProposalId}
          />
          {selected && (
            <ProposalReview
              key={selected.id}
              proposal={selected}
              proposals={proposals}
              projectId={projectId}
              canWrite={canWrite}
            />
          )}
        </div>
      )}
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
    </div>
  );
}
