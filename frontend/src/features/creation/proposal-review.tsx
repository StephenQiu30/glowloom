"use client";

import { useState } from "react";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useClaimHumanTaskMutation, useDecideHumanTaskMutation, useHumanTaskQuery, useRenewHumanTaskClaimMutation } from "@/features/review/endpoints";
import { appApiErrorMessage } from "@/lib/server-state";
import { useAdoptCreationMutation } from "./endpoints";
import { ProposalContent, stageLabels } from "./proposal-content";

export function ProposalReview({ proposal, proposals, projectId, canWrite }: {
  proposal: API.CreationProposal; proposals: API.CreationProposal[]; projectId: string; canWrite: boolean;
}) {
  const detail = useHumanTaskQuery(proposal.human_task_id, { pollingInterval: 5000, refetchOnFocus: true });
  const [claim, claimState] = useClaimHumanTaskMutation();
  const [renew, renewState] = useRenewHumanTaskClaimMutation();
  const [decide, decisionState] = useDecideHumanTaskMutation();
  const [adopt, adoptionState] = useAdoptCreationMutation();
  const [reasons, setReasons] = useState<Record<string, string>>({});
  const [error, setError] = useState<string>();
  const [notice, setNotice] = useState<string>();
  const task = detail.data?.task;
  const decision = detail.data?.decision;
  const blockers = proposal.issues.filter((issue) => issue.severity === "blocker");
  const riskKey = (issue: API.CreationTextIssue) => JSON.stringify([issue.code, issue.scope]);
  const resolutions = blockers.map((issue) => ({ code: issue.code, scope: issue.scope, reason: reasons[riskKey(issue)]?.trim() ?? "" }));
  const risksReady = resolutions.every((item) => item.reason.length > 0);
  const accepted = proposal.acceptance ?? adoptionState.data;
  const busy = claimState.isLoading || decisionState.isLoading || adoptionState.isLoading || renewState.isLoading;

  async function command(action: () => Promise<unknown>, message: string) {
    setError(undefined); setNotice(undefined);
    try { await action(); setNotice(message); }
    catch (cause) { setError(appApiErrorMessage(cause)); }
    finally { void detail.refetch(); }
  }

  function claimTask() {
    if (!task) return;
    void command(() => claim({ projectId, taskId: task.id, body: { expected_revision: task.revision, idempotency_key: `creation-claim:${task.id}:${task.revision}` } }).unwrap(), "审核已领取，请核对正文、来源和待确认问题。");
  }
  function renewTask() {
    const token = task?.claim?.claim_token;
    if (!task || !token) return;
    void command(() => renew({ projectId, taskId: task.id, body: { claim_token: token, expected_revision: task.revision, idempotency_key: `creation-renew:${task.id}:${task.revision}` } }).unwrap(), "审核领取已续期。");
  }
  function decideTask(value: "approved" | "rejected") {
    const token = task?.claim?.claim_token;
    if (!task || !token) return;
    void command(() => decide({ projectId, taskId: task.id, workflowRunId: proposal.run_id, body: {
      claim_token: token, expected_task_revision: task.revision,
      expected_subject_revision: proposal.revision, expected_subject_hash: proposal.result_hash,
      decision: value, selected_candidate_id: null, idempotency_key: `creation-review:${task.id}:${proposal.revision}:${value}`,
    } }).unwrap(), value === "approved" ? "草案已批准；正式采纳后才会进入下一阶段。" : "草案已拒绝，本次创作将停止。修改原稿后可发起新的创作。");
  }
  function adoptProposal() {
    if (!decision || decision.decision !== "approved") return;
    void command(() => adopt({ projectId, runId: proposal.run_id, proposalId: proposal.id, body: {
      expected_revision: proposal.revision, decision_id: decision.id,
      idempotency_key: `creation-adopt:${proposal.id}:${proposal.revision}`,
      risk_resolutions: resolutions,
    } }).unwrap(), "已正式采纳。系统会在本阶段所有提案采纳后继续。");
  }

  return <article aria-label="提案审阅" className="min-w-0 space-y-5 rounded-lg border bg-card p-5 sm:p-6">
    <header className="flex flex-wrap items-center justify-between gap-3"><h2 className="text-xl font-semibold">{stageLabels[proposal.stage]}</h2><Badge variant={accepted ? "default" : "outline"}>{accepted ? "已正式采纳" : "待人工审阅"}</Badge></header>
    <ProposalContent proposal={proposal} proposals={proposals} />
    <details className="rounded-md border p-4"><summary className="cursor-pointer text-sm font-medium">原文来源与固定版本 · {proposal.evidence.length} 处引用</summary><p className="mt-3 break-all text-xs text-muted-foreground">原稿版本：{proposal.source_revision_id}<br />原稿摘要：{proposal.source_hash}<br />草案版本：{proposal.revision} · {proposal.result_hash}</p><ul className="mt-4 space-y-3">{proposal.evidence.map((item, index) => <li className="text-sm" key={index}><blockquote className="whitespace-pre-wrap border-l-2 pl-3">{item.quote}</blockquote><p className="mt-1 text-xs text-muted-foreground">原文块 {item.block + 1} · 字符 {item.start}–{item.end}（Unicode 码点，右侧不含）</p></li>)}</ul></details>
    {proposal.issues.length > 0 && <section aria-label="待确认问题" className="space-y-4 border-t pt-5"><h3 className="font-semibold">待确认问题</h3>{proposal.issues.map((issue, index) => <div className="space-y-2" key={riskKey(issue)}><p className="text-sm"><Badge variant="outline">{issue.severity === "blocker" ? "采纳前必需处理" : "提示"}</Badge> {issue.summary}</p><p className="text-xs text-muted-foreground">范围：{issue.scope}</p>{issue.severity === "blocker" && !accepted && canWrite && <div className="space-y-2"><Label htmlFor={`resolution-${proposal.id}-${index}`}>处理说明：{issue.summary}</Label><Textarea id={`resolution-${proposal.id}-${index}`} maxLength={4000} value={reasons[riskKey(issue)] ?? ""} onChange={(event) => setReasons((prior) => ({ ...prior, [riskKey(issue)]: event.target.value }))} placeholder="写明核对依据、状态与披露处理结果。" /></div>}</div>)}</section>}
    {accepted ? <section aria-label="正式采纳回执" className="space-y-3 border-t pt-5"><h3 className="font-semibold">正式采纳回执</h3><p className="break-all text-xs text-muted-foreground">{accepted.submission_id}</p><ul className="space-y-2 text-sm">{accepted.formal_refs.map((ref) => <li className="break-all" key={`${ref.type}:${ref.id}`}>{ref.type} · {ref.id} · 版本 {ref.revision}</li>)}</ul>{accepted.risk_resolutions.map((item) => <p className="whitespace-pre-wrap text-sm" key={`${item.code}:${item.scope}`}>已处置 {item.scope}：{item.reason}</p>)}{proposal.stage === "direct_scene" && <p className="text-sm text-muted-foreground">文字导演意图已保存，视觉参考尚待制作。</p>}</section> : canWrite ? <section className="space-y-3 border-t pt-5">
      {detail.error && <Alert variant="destructive"><AlertDescription>{appApiErrorMessage(detail.error)}</AlertDescription></Alert>}
      {task && <div className="flex flex-wrap gap-2">
        {!decision && !task.claim?.claim_token && (task.status === "OPEN" || task.status === "CLAIMED") && <Button disabled={busy} onClick={claimTask}>领取审阅</Button>}
        {!decision && task.claim?.claim_token && <><Button disabled={busy || !risksReady} onClick={() => decideTask("approved")}>批准草案</Button><Button disabled={busy} variant="outline" onClick={() => decideTask("rejected")}>拒绝草案</Button><Button disabled={busy} variant="ghost" onClick={renewTask}>延长审阅时间</Button></>}
        {decision?.decision === "approved" && <Button disabled={busy || !risksReady} onClick={adoptProposal}>正式采纳并继续</Button>}
      </div>}
      {decision?.decision === "rejected" && <p className="text-sm">本提案已拒绝。请修改原稿后发起新的创作。</p>}
      {!risksReady && <p className="text-sm text-muted-foreground">填写全部必需问题的处理说明后才能批准和采纳。</p>}
    </section> : <p className="border-t pt-4 text-sm text-muted-foreground">当前为只读权限，可查看候选和已保存回执。</p>}
    {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}
    {notice && <p role="status" className="text-sm">{notice}</p>}
  </article>;
}
