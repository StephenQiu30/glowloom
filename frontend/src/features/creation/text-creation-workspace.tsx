"use client";

import { useState } from "react";
import Link from "next/link";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { appApiErrorMessage } from "@/lib/server-state";
import {
  useAcceptCreationSourceMutation, useCreationExecutionQuery, useCreationProposalsQuery,
  useCreationRunQuery, useCreationRunsQuery, useCurrentScriptSourceQuery, useRetryCreationDeliveryMutation,
  useStartCreationMutation, useSyncCreationMutation, useResumeCreationMutation,
} from "./endpoints";
import { stageLabels } from "./proposal-content";
import { ProposalReview } from "./proposal-review";

const deliveryLabels: Record<API.CreationRunResponse["status"], string> = {
  queued: "等待交接", delivery_unknown: "正在核对原交接结果", delivery_blocked: "交接受阻",
  accepted: "Agent 已接受，等待执行状态",
};
const executionLabels: Record<API.CreationExecution["status"], string> = {
  queued: "已接受，等待 Worker 开始创作",
  running: "正在创作", waiting_review: "等待人工审阅与采纳", blocked: "执行受阻", rejected: "审阅未通过", completed: "文本分镜已完成",
};
const polling = { pollingInterval: 5000, refetchOnFocus: true, refetchOnReconnect: true };
const executionErrors: Record<string, string> = {
  platform_unavailable: "原稿或采纳服务暂时无法连接。服务恢复后，可继续本次运行。",
  invocation_budget_exhausted: "本次创作的模型调用额度已用完，运行已停止。",
  harness_response_unknown: "模型调用结果尚未确认，已停止重复调用，请联系管理员核查。",
  invocation_outcome_unknown: "模型调用结果尚未确认，已停止重复调用，请联系管理员核查。",
  harness_result_invalid: "模型结果未通过内容校验，已保留现场，请联系管理员核查。",
  review_rejected: "草案未获批准。请修改原稿后发起新的创作。",
};
export type CreationSource = { revisionId: string; contentHash: string; title: string; text: string };

export function TextCreationWorkspace({ projectId, source, canWrite, initialRunId = "" }: {
  projectId: string; source?: CreationSource; canWrite: boolean; initialRunId?: string;
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
  const [notice, setNotice] = useState<string>();
  const run = chosenRunId ? exactRun.currentData ?? runs.data?.find((item) => item.id === chosenRunId) ?? (startState.data?.id === chosenRunId ? startState.data : undefined) : runs.data?.[0];
  const execution = useCreationExecutionQuery(run?.id ?? "", { ...polling, skip: !run || run.status !== "accepted" });
  const proposalsQuery = useCreationProposalsQuery(run?.id ?? "", { ...polling, skip: !run });
  const proposals = proposalsQuery.currentData ?? [];
  const selected = proposals.find((item) => item.id === chosenProposalId) ?? proposals.find((item) => item.status === "needs_review") ?? proposals.at(-1);
  const existing = source && runs.data?.find((item) => item.source.revision_id === source.revisionId && item.source.content_hash === source.contentHash);
  const busy = acceptState.isLoading || startState.isLoading || retryState.isLoading || syncState.isLoading || resumeState.isLoading;
  const state = execution.currentData;
  const failureCode = state?.last_error || run?.last_error;

  function chooseRun(id: string) {
    setChosenRunId(id); setChosenProposalId(""); setError(undefined); setNotice(undefined);
    const url = new URL(window.location.href); url.searchParams.set("run", id); window.history.replaceState(null, "", url);
  }
  async function startCreation() {
    if (!source || head.isLoading || head.error) return;
    if (existing) { chooseRun(existing.id); return; }
    setError(undefined); setNotice(undefined);
    try {
      if (head.data?.identity.version_id !== source.revisionId) {
        const accepted = await acceptSource({ projectId, body: {
          document_revision_id: source.revisionId, expected_head_revision: head.data?.head_revision ?? 0,
          expected_head_hash: head.data?.head_hash ?? null,
          idempotency_key: `creation-source:${source.revisionId}:${head.data?.head_revision ?? 0}`,
        } }).unwrap();
        if (accepted.identity.version_id !== source.revisionId || accepted.identity.content_hash !== source.contentHash) throw new Error("正式源与当前原稿不一致，请刷新核对。");
      } else if (head.data.identity.content_hash !== source.contentHash) throw new Error("正式源摘要不一致，已停止启动。");
      const created = await start({ projectId, body: { document_revision_id: source.revisionId, source_hash: source.contentHash, idempotency_key: `text-creation:${source.revisionId}:${source.contentHash}` } }).unwrap();
      chooseRun(created.id); setNotice("已创建运行。每个阶段审阅并正式采纳后，系统才会继续。");
    } catch (cause) { setError(appApiErrorMessage(cause)); void head.refetch(); void runs.refetch(); }
  }
  async function synchronize() {
    if (!run) return;
    setError(undefined);
    try { await sync(run.id).unwrap(); setNotice("已同步执行状态与提案。"); }
    catch (cause) { setError(appApiErrorMessage(cause)); }
  }
  async function retry() {
    if (!run) return;
    setError(undefined);
    try { await retryDelivery({ projectId, runId: run.id, revision: run.revision }).unwrap(); setNotice("已按原运行恢复交接。"); }
    catch (cause) { setError(appApiErrorMessage(cause)); }
  }

  async function resumeRun() {
    if (!run || !state?.can_resume) return;
    setError(undefined);
    try { await resume({ runId: run.id, revision: run.revision }).unwrap(); setNotice("恢复请求已接受，将沿用本次原稿、草案和剩余额度。"); }
    catch (cause) { setError(appApiErrorMessage(cause)); }
  }

  return <div className="space-y-6">
    <section aria-label="固定原稿与启动" className="space-y-4 rounded-lg border bg-card p-5">
      <h2 className="text-lg font-semibold">固定原稿</h2>
      {source ? <><p className="font-medium">{source.title}</p><details><summary className="cursor-pointer text-sm text-muted-foreground">查看此次原稿</summary><pre className="mt-3 max-h-72 overflow-auto whitespace-pre-wrap break-words rounded-md bg-muted p-4 font-sans text-sm">{source.text}</pre></details><p className="break-all text-xs text-muted-foreground">版本：{source.revisionId}</p>{canWrite && <Button disabled={busy || head.isLoading || Boolean(head.error) || runs.isLoading} onClick={() => void startCreation()}>{existing ? "查看当前原稿的创作" : "固定原稿并开始创作"}</Button>}</> : <p className="text-sm">先<Link className="underline" href={`/projects/${projectId}#script-import`}>导入剧本</Link>，再开始文本创作。</p>}
      {head.error && <Alert variant="destructive"><AlertDescription>{appApiErrorMessage(head.error)}</AlertDescription></Alert>}
      <p className="text-sm text-muted-foreground">依次确认分集、剧稿结构、世界设定和导演分镜。修改原稿将保留已有运行和回执。</p>
    </section>
    {runs.error && <Alert variant="destructive"><AlertDescription>{appApiErrorMessage(runs.error)}</AlertDescription></Alert>}
    {(runs.data?.length ?? 0) > 0 && <div className="space-y-2"><Label htmlFor="creation-run">创作记录</Label><select id="creation-run" className="h-10 w-full rounded-md border bg-background px-3 text-sm" value={run?.id ?? ""} onChange={(event) => chooseRun(event.target.value)}>{runs.data!.map((item) => <option key={item.id} value={item.id}>{new Date(item.created_at).toLocaleString("zh-CN")} · {item.source.revision_id.slice(0, 8)} · {deliveryLabels[item.status]}</option>)}</select></div>}
    {chosenRunId && exactRun.error && <Alert variant="destructive"><AlertDescription>{appApiErrorMessage(exactRun.error)}</AlertDescription></Alert>}
    {run && <section aria-label="创作状态" className="space-y-4 rounded-lg border bg-card p-5">
      <div className="flex flex-wrap items-center justify-between gap-3"><h2 className="text-lg font-semibold">{state ? executionLabels[state.status] : deliveryLabels[run.status]}</h2>{canWrite && <div className="flex flex-wrap gap-2">{run.status === "delivery_blocked" && <Button disabled={busy} variant="outline" onClick={() => void retry()}>恢复原运行交接</Button>}{run.status === "accepted" && <Button disabled={busy} variant="outline" onClick={() => void synchronize()}>同步执行与提案</Button>}</div>}</div>
      <ol aria-label="创作阶段" className="grid gap-2 sm:grid-cols-4">{Object.entries(stageLabels).map(([stage, label]) => { const items = proposals.filter((item) => item.stage === stage); return <li className="space-y-1 rounded-md border p-3 text-sm" key={stage}><p className="font-medium">{label}</p><p className="text-xs text-muted-foreground">{state?.stage === stage && state.status === "running" ? `已生成 ${state.steps.filter((step) => step.step_key.split("/")[0] === stage && step.state === "needs_review").length} 份草案，正在分析` : items.length ? `${items.filter((item) => item.status === "accepted").length}/${items.length} 已采纳` : "尚未开始"}</p>{state?.stage === stage && <Badge variant="outline">当前阶段</Badge>}</li>; })}</ol>
      {state && state.status !== "queued" && <p className="text-xs text-muted-foreground">已预留模型调用 {state.reserved_calls}/{state.call_limit} 次；额度在原运行内固定。</p>}
      {canWrite && state?.status === "blocked" && state.can_resume && <Button disabled={busy} onClick={() => void resumeRun()}>恢复原运行</Button>}
      {failureCode && <Alert variant="destructive"><AlertDescription>{executionErrors[failureCode] ?? "本次创作已暂停，请联系管理员核查；原稿和已保存结果仍可查回。"}<details className="mt-2"><summary>故障信息</summary>{failureCode}</details></AlertDescription></Alert>}
      {state?.steps.some((step) => step.state === "unknown") && <p className="text-sm">模型调用结果尚未确认，系统已停止重复调用。保留本次运行，核查原调用结果后再恢复。</p>}
      {state?.status === "completed" && <p className="text-sm">所有文本阶段已正式采纳，可随时查回下方内容和回执。视觉素材与视频制作尚未开始。</p>}
      {execution.error && (execution.error as { code?: string }).code !== "not_found" && <p role="status" className="text-sm text-muted-foreground">执行状态暂未读到：{appApiErrorMessage(execution.error)}</p>}
      <details><summary className="cursor-pointer text-xs text-muted-foreground">运行与原稿身份</summary><p className="mt-2 break-all text-xs text-muted-foreground">运行：{run.id}<br />原稿：{run.source.revision_id}<br />摘要：{run.source.content_hash}</p></details>
    </section>}
    {proposalsQuery.error && <Alert variant="destructive"><AlertDescription>{appApiErrorMessage(proposalsQuery.error)}</AlertDescription></Alert>}
    {proposals.length > 0 && <div className="grid items-start gap-5 lg:grid-cols-[15rem_minmax(0,1fr)]"><nav aria-label="阶段提案" className="flex gap-2 overflow-auto lg:flex-col">{proposals.map((proposal, index) => <button type="button" aria-current={selected?.id === proposal.id ? "true" : undefined} className={`min-w-36 rounded-md border p-3 text-left text-sm ${selected?.id === proposal.id ? "border-primary bg-accent" : "bg-card"}`} key={proposal.id} onClick={() => setChosenProposalId(proposal.id)}><span className="block font-medium">{index + 1}. {stageLabels[proposal.stage]}</span><span className="mt-1 block text-xs text-muted-foreground">{proposal.status === "accepted" ? "已采纳" : "待审阅"} · {proposal.step_key.split("/").slice(1).join(" / ")}</span></button>)}</nav>{selected && <ProposalReview key={selected.id} proposal={selected} proposals={proposals} projectId={projectId} canWrite={canWrite} />}</div>}
    {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}
    {notice && <p role="status" className="text-sm">{notice}</p>}
  </div>;
}
