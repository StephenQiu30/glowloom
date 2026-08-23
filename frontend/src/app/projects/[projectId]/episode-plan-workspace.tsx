"use client";

import {
  AlertCircle,
  CheckCircle2,
  GitMerge,
  LoaderCircle,
  Scissors,
  Sparkles,
} from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  appApiErrorMessage,
  useConfirmEpisodePlanMutation,
  useCreateEpisodePlanMutation,
  useLazyEpisodePlanQuery,
  useMaterializeEpisodePlanMutation,
  useMergeEpisodeProposalsMutation,
  useMoveEpisodeBoundaryMutation,
  usePublishImportCommitMutation,
  useRenameEpisodeProposalMutation,
  useSplitEpisodeProposalMutation,
} from "@/lib/server-state";

const planStatusLabels: Record<API.EpisodePlanResponse["status"], string> = {
  confirmed: "已确认",
  draft: "AI 生成中",
  materialized: "已物化",
  review_ready: "待人工确认",
  superseded: "已过期",
};

const blockKindLabels: Record<API.NarrativeBlockResponse["kind"], string> = {
  action: "动作",
  dialogue: "对白",
  episode_marker: "集标记",
  narration: "旁白",
  preamble: "前言",
  scene_heading: "场景",
  separator: "空行",
};

function commandKey(action: string, ...parts: Array<string | number>): string {
  return [action, ...parts].join(":").slice(0, 200);
}

function wait(milliseconds: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, milliseconds));
}

export function EpisodePlanWorkspace({
  analysis,
  canWrite,
  targetDurationMs,
}: {
  analysis: API.ScriptDocumentAnalysisResponse;
  canWrite: boolean;
  targetDurationMs: number;
}) {
  const [createPlan, createState] = useCreateEpisodePlanMutation();
  const [loadPlan] = useLazyEpisodePlanQuery();
  const [renameProposal, renameState] = useRenameEpisodeProposalMutation();
  const [moveBoundary, moveState] = useMoveEpisodeBoundaryMutation();
  const [splitProposal, splitState] = useSplitEpisodeProposalMutation();
  const [mergeProposals, mergeState] = useMergeEpisodeProposalsMutation();
  const [confirmPlan, confirmState] = useConfirmEpisodePlanMutation();
  const [materializePlan, materializeState] = useMaterializeEpisodePlanMutation();
  const [publishCommit, publishState] = usePublishImportCommitMutation();
  const [plan, setPlan] = useState<API.EpisodePlanDetailResponse | null>(null);
  const [commit, setCommit] = useState<API.ImportCommitDetailResponse | null>(null);
  const [titles, setTitles] = useState<Record<string, string>>({});
  const [splitOffsets, setSplitOffsets] = useState<Record<string, number>>({});
  const [splitTitles, setSplitTitles] = useState<Record<string, string>>({});
  const [actionError, setActionError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const busy =
    createState.isLoading ||
    renameState.isLoading ||
    moveState.isLoading ||
    splitState.isLoading ||
    mergeState.isLoading ||
    confirmState.isLoading ||
    materializeState.isLoading ||
    publishState.isLoading;
  const strategy: API.EpisodePlanCreateRequest["strategy"] =
    analysis.revision.analysis_status === "deterministic"
      ? "explicit_markers"
      : "target_duration_ai";
  const planButtonLabel =
    strategy === "explicit_markers"
      ? "生成确定性分集计划"
      : "生成 AI 分集候选";

  const blockByPosition = useMemo(
    () => new Map((plan?.source.blocks ?? []).map((block) => [block.position, block])),
    [plan?.source.blocks],
  );

  async function runAction<T>(action: () => Promise<T>): Promise<T | null> {
    setActionError(null);
    setNotice(null);
    try {
      return await action();
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
      return null;
    }
  }

  async function create(): Promise<void> {
    const created = await runAction(() =>
      createPlan({
        revisionId: analysis.revision.id,
        body: {
          strategy,
          target_duration_ms: targetDurationMs,
          requested_episode_count: null,
          idempotency_key: commandKey(
            "episode-plan",
            analysis.revision.id,
            analysis.revision.normalized_hash,
            strategy,
            targetDurationMs,
          ),
        },
      }).unwrap(),
    );
    if (!created) return;
    setPlan(created);
    if (created.plan.status !== "draft") return;
    for (let attempt = 0; attempt < 30; attempt += 1) {
      await wait(1_000);
      const refreshed = await runAction(() => loadPlan(created.plan.id, false).unwrap());
      if (!refreshed) return;
      setPlan(refreshed);
      if (refreshed.plan.planning_error_code) {
        setActionError(`AI 分集未完成：${refreshed.plan.planning_error_code}`);
        return;
      }
      if (refreshed.plan.status !== "draft") return;
    }
    setActionError("AI 分集仍在生成，请稍后重新读取该计划。");
  }

  async function rename(proposal: API.EpisodeProposalResponse): Promise<void> {
    if (!plan) return;
    const title = (titles[proposal.id] ?? "").trim();
    if (!title) {
      setActionError("分集标题不能为空。");
      return;
    }
    const updated = await runAction(() =>
      renameProposal({
        planId: plan.plan.id,
        body: {
          expected_revision: plan.plan.revision,
          idempotency_key: commandKey("rename", plan.plan.id, plan.plan.revision, proposal.id, title),
          proposal_id: proposal.id,
          title,
        },
      }).unwrap(),
    );
    if (updated) setPlan(updated);
  }

  async function move(
    left: API.EpisodeProposalResponse,
    sourceOffset: number,
  ): Promise<void> {
    if (!plan || sourceOffset === plan.proposals[left.position]?.source_start) return;
    const updated = await runAction(() =>
      moveBoundary({
        planId: plan.plan.id,
        body: {
          expected_revision: plan.plan.revision,
          idempotency_key: commandKey(
            "move-boundary",
            plan.plan.id,
            plan.plan.revision,
            left.id,
            sourceOffset,
          ),
          left_proposal_id: left.id,
          source_offset: sourceOffset,
        },
      }).unwrap(),
    );
    if (updated) setPlan(updated);
  }

  async function split(proposal: API.EpisodeProposalResponse): Promise<void> {
    if (!plan) return;
    const sourceOffset = splitOffsets[proposal.id];
    const newTitle = (splitTitles[proposal.id] ?? "").trim();
    if (!sourceOffset || !newTitle) {
      setActionError("请选择拆分结构块并填写新一集标题。");
      return;
    }
    const updated = await runAction(() =>
      splitProposal({
        planId: plan.plan.id,
        body: {
          expected_revision: plan.plan.revision,
          idempotency_key: commandKey(
            "split",
            plan.plan.id,
            plan.plan.revision,
            proposal.id,
            sourceOffset,
            newTitle,
          ),
          proposal_id: proposal.id,
          source_offset: sourceOffset,
          new_title: newTitle,
        },
      }).unwrap(),
    );
    if (updated) {
      setPlan(updated);
      setSplitTitles((current) => ({ ...current, [proposal.id]: "" }));
    }
  }

  async function merge(left: API.EpisodeProposalResponse): Promise<void> {
    if (!plan) return;
    const updated = await runAction(() =>
      mergeProposals({
        planId: plan.plan.id,
        body: {
          expected_revision: plan.plan.revision,
          idempotency_key: commandKey(
            "merge",
            plan.plan.id,
            plan.plan.revision,
            left.id,
          ),
          left_proposal_id: left.id,
        },
      }).unwrap(),
    );
    if (updated) setPlan(updated);
  }

  async function confirm(): Promise<void> {
    if (!plan) return;
    const updated = await runAction(() =>
      confirmPlan({
        planId: plan.plan.id,
        body: {
          expected_revision: plan.plan.revision,
          idempotency_key: commandKey("confirm", plan.plan.id, plan.plan.revision),
        },
      }).unwrap(),
    );
    if (updated) setPlan(updated);
  }

  async function materialize(): Promise<void> {
    if (!plan) return;
    const created = await runAction(() =>
      materializePlan({
        planId: plan.plan.id,
        body: {
          mode: "append_new",
          expected_plan_revision: plan.plan.revision,
          expected_project_revision: plan.impact.project_revision,
          expected_active_order_hash: plan.impact.active_order_hash,
          idempotency_key: commandKey(
            "materialize",
            plan.plan.id,
            plan.plan.revision,
            plan.impact.project_revision,
            plan.impact.active_order_hash,
          ),
        },
      }).unwrap(),
    );
    if (created) setCommit(created);
  }

  async function publish(): Promise<void> {
    if (!commit || !plan) return;
    const published = await runAction(() =>
      publishCommit({
        commitId: commit.commit.id,
        body: {
          expected_revision: commit.commit.revision,
          idempotency_key: commandKey(
            "publish-import",
            commit.commit.id,
            commit.commit.revision,
          ),
        },
      }).unwrap(),
    );
    if (published) {
      setCommit(published);
      setNotice(
        `${plan.proposals.length} 集剧本已批量发布，全部结构 Skill 任务已自动创建。`,
      );
    }
  }

  if (analysis.revision.analysis_status === "rejected") return null;

  return (
    <Card className="mt-8" aria-label="分集计划与批量创建" role="region">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Sparkles className="size-5" aria-hidden="true" />分集计划与批量创建
        </CardTitle>
        <CardDescription>
          先审阅可追溯候选；确认前不会创建正式 Episode，发布前不会切换单集 current。
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-6 pt-6">
        {actionError ? (
          <Alert variant="destructive">
            <AlertCircle aria-hidden="true" />
            <AlertTitle>分集操作未完成</AlertTitle>
            <AlertDescription>{actionError}</AlertDescription>
          </Alert>
        ) : null}
        {notice ? (
          <Alert className="border-0 bg-muted/50" role="status">
            <CheckCircle2 aria-hidden="true" />
            <AlertTitle>批量发布完成</AlertTitle>
            <AlertDescription>{notice}</AlertDescription>
          </Alert>
        ) : null}

        {!plan ? (
          <div className="flex flex-wrap items-center justify-between gap-4 bg-muted/45 p-5">
            <div>
              <p className="font-medium">{planButtonLabel}</p>
              <p className="mt-1 text-sm text-muted-foreground">
                {strategy === "explicit_markers"
                  ? "沿用已验证的连续集标记，不调用模型。"
                  : "本地 Codex 只生成候选边界，服务端仍会校验唯一锚点和全文守恒。"}
              </p>
            </div>
            <Button disabled={!canWrite || busy} onClick={create}>
              {busy ? <LoaderCircle className="animate-spin" aria-hidden="true" /> : null}
              {planButtonLabel}
            </Button>
          </div>
        ) : (
          <>
            <div className="grid gap-3 bg-muted/45 p-5 sm:grid-cols-4">
              <div><p className="text-xs text-muted-foreground">状态</p><Badge className="mt-2" variant="outline">{planStatusLabels[plan.plan.status]}</Badge></div>
              <div><p className="text-xs text-muted-foreground">候选集数</p><p className="mt-1 text-xl font-semibold">{plan.proposals.length}</p></div>
              <div><p className="text-xs text-muted-foreground">预计总时长</p><p className="mt-1 text-xl font-semibold">{Math.round(plan.plan.total_estimated_duration_ms / 1_000)} 秒</p></div>
              <div><p className="text-xs text-muted-foreground">物化后活跃集</p><p className="mt-1 text-xl font-semibold">{plan.impact.projected_episode_count}</p></div>
            </div>

            {plan.plan.status === "draft" ? (
            <p className="flex items-center gap-2 text-sm text-muted-foreground">
                <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
                AI 候选生成后还会经过服务端锚点、边界和全文守恒校验。
              </p>
            ) : null}

            <div className="grid gap-6 xl:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
              <div className="min-w-0 bg-muted/45 p-5">
                <p className="font-medium">不可变原文</p>
                <p className="mt-1 text-xs text-muted-foreground">revision {plan.plan.document_revision_id} · {plan.source.codepoint_count} 字符</p>
                <pre className="mt-4 max-h-[680px] overflow-auto whitespace-pre-wrap rounded-lg bg-slate-950 p-4 font-mono text-xs leading-6 text-slate-100">{plan.source.normalized_text}</pre>
              </div>

              <div className="grid content-start gap-4">
                {plan.proposals.map((proposal, index) => {
                  const editable = plan.plan.status === "review_ready" && canWrite;
                  const innerBlocks = plan.source.blocks.filter(
                    (block) =>
                      block.source_start > proposal.source_start &&
                      block.source_start < proposal.source_end,
                  );
                  const next = plan.proposals[index + 1];
                  const boundaryBlocks = next
                    ? plan.source.blocks.filter(
                        (block) =>
                          block.source_start > proposal.source_start &&
                          block.source_start < next.source_end,
                      )
                    : [];
                  return (
                    <div className="grid gap-3" key={proposal.id}>
                      <article className="bg-muted/30 p-5">
                        <div className="flex flex-wrap items-start justify-between gap-3">
                          <div className="min-w-0 flex-1">
                            <Label htmlFor={`episode-title-${proposal.id}`}>第 {proposal.position} 集标题</Label>
                            <div className="mt-2 flex gap-2">
                              <Input
                                disabled={!editable || busy}
                                id={`episode-title-${proposal.id}`}
                                maxLength={120}
                                value={titles[proposal.id] ?? proposal.title}
                                onChange={(event) =>
                                  setTitles((current) => ({
                                    ...current,
                                    [proposal.id]: event.target.value,
                                  }))
                                }
                              />
                              {editable ? (
                                <Button
                                  aria-label={`保存第 ${proposal.position} 集标题`}
                                  disabled={busy || (titles[proposal.id] ?? proposal.title) === proposal.title}
                                  onClick={() => rename(proposal)}
                                  size="sm"
                                  variant="outline"
                                >保存</Button>
                              ) : null}
                            </div>
                          </div>
                          <Badge variant="secondary">{Math.round(proposal.estimated_duration_ms / 1_000)} 秒</Badge>
                        </div>
                        <p className="mt-4 text-sm leading-6 text-foreground">{proposal.reason}</p>
                        <div className="mt-3 flex flex-wrap gap-2 text-xs text-muted-foreground">
                          <span>置信度 {Math.round(proposal.confidence * 100)}%</span>
                          <span>块 {proposal.start_block_position}–{proposal.end_block_position}</span>
                          <span>字符 {proposal.source_start}–{proposal.source_end}</span>
                        </div>
                        <pre className="mt-4 max-h-56 overflow-auto whitespace-pre-wrap bg-background p-4 text-xs leading-6 text-muted-foreground">{plan.source.normalized_text.slice(proposal.source_start, proposal.source_end)}</pre>

                        {editable && innerBlocks.length ? (
                          <div className="mt-4 grid gap-3 bg-muted/45 p-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] sm:items-end">
                            <div className="grid gap-1.5">
                              <Label htmlFor={`split-offset-${proposal.id}`}>从结构块拆分</Label>
                              <Select
                                value={splitOffsets[proposal.id]?.toString() ?? ""}
                                onValueChange={(value) =>
                                  setSplitOffsets((current) => ({ ...current, [proposal.id]: Number(value) }))
                                }
                              >
                                <SelectTrigger className="w-full" id={`split-offset-${proposal.id}`}><SelectValue placeholder="选择新一集首块" /></SelectTrigger>
                                <SelectContent>
                                  {innerBlocks.map((block) => (
                                    <SelectItem key={block.id} value={String(block.source_start)}>
                                      {block.position}. {blockKindLabels[block.kind]}
                                    </SelectItem>
                                  ))}
                                </SelectContent>
                              </Select>
                            </div>
                            <div className="grid gap-1.5">
                              <Label htmlFor={`split-title-${proposal.id}`}>新一集标题</Label>
                              <Input
                                id={`split-title-${proposal.id}`}
                                value={splitTitles[proposal.id] ?? ""}
                                onChange={(event) =>
                                  setSplitTitles((current) => ({
                                    ...current,
                                    [proposal.id]: event.target.value,
                                  }))
                                }
                              />
                            </div>
                            <Button onClick={() => split(proposal)} size="sm" variant="outline"><Scissors aria-hidden="true" />拆分</Button>
                          </div>
                        ) : null}
                      </article>

                      {next && editable ? (
                        <div className="grid gap-2 bg-muted/45 p-4 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end">
                          <div className="grid gap-1.5">
                            <Label htmlFor={`boundary-${proposal.id}`}>第 {proposal.position}/{next.position} 集边界</Label>
                            <Select
                              value={String(next.source_start)}
                              onValueChange={(value) => move(proposal, Number(value))}
                            >
                              <SelectTrigger className="w-full" id={`boundary-${proposal.id}`}><SelectValue /></SelectTrigger>
                              <SelectContent>
                                {boundaryBlocks.map((block) => (
                                  <SelectItem key={block.id} value={String(block.source_start)}>
                                    块 {block.position} · {blockKindLabels[block.kind]}
                                    {blockByPosition.get(block.position)?.source_start === next.source_start ? "（当前）" : ""}
                                  </SelectItem>
                                ))}
                              </SelectContent>
                            </Select>
                          </div>
                          <Button onClick={() => merge(proposal)} size="sm" variant="outline"><GitMerge aria-hidden="true" />合并相邻两集</Button>
                        </div>
                      ) : null}
                    </div>
                  );
                })}
              </div>
            </div>

            {plan.impact.blockers.length ? (
              <Alert variant="destructive">
                <AlertCircle aria-hidden="true" />
                <AlertTitle>当前不能批量创建</AlertTitle>
                <AlertDescription>{plan.impact.blockers.map((item) => item.summary).join("；")}</AlertDescription>
              </Alert>
            ) : null}

            <div className="flex flex-wrap justify-end gap-3 pt-2">
              {plan.plan.status === "review_ready" ? (
                <Button disabled={!canWrite || busy || !plan.impact.allowed} onClick={confirm}>
                  确认分集计划
                </Button>
              ) : null}
              {plan.plan.status === "confirmed" && !commit ? (
                <Button disabled={!canWrite || busy || !plan.impact.allowed} onClick={materialize}>
                  原子创建 {plan.proposals.length} 集
                </Button>
              ) : null}
              {commit?.commit.status === "materialized" ? (
                <Button disabled={!canWrite || busy} onClick={publish}>
                  发布 {plan.proposals.length} 集剧本
                </Button>
              ) : null}
              {busy ? <LoaderCircle className="size-5 animate-spin self-center" aria-label="正在执行分集操作" /> : null}
            </div>

            {commit?.commit.status === "published" ? (
              <div className="grid gap-3 bg-muted/45 p-5">
                <div>
                  <p className="font-medium">逐集结构解析已启动</p>
                  <p className="mt-1 text-sm text-muted-foreground">
                    每一集都已进入场景、人物、制作任务与镜头候选提取；进入剧本工作台审阅 Skill 结果。
                  </p>
                </div>
                <div className="flex flex-wrap gap-2">
                  {commit.segments.map((segment) => (
                    <Button asChild key={segment.id} size="sm" variant="outline">
                      <Link href={`/studio/${segment.episode_id}/script`}>
                        审阅第 {segment.position} 集结构
                      </Link>
                    </Button>
                  ))}
                </div>
              </div>
            ) : null}
          </>
        )}
      </CardContent>
    </Card>
  );
}
