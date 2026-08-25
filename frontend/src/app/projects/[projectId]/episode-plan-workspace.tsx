"use client";

import { AlertCircle, CheckCircle2, LoaderCircle, Sparkles } from "lucide-react";
import { useState } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  appApiErrorMessage,
  useConfirmEpisodePlanMutation,
  useCreateEpisodePlanMutation,
  useMaterializeEpisodePlanMutation,
  usePublishImportCommitMutation,
} from "@/lib/server-state";

function commandKey(action: string, ...parts: Array<string | number>): string {
  return [action, ...parts].join(":").slice(0, 200);
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
  const [confirmPlan, confirmState] = useConfirmEpisodePlanMutation();
  const [materializePlan, materializeState] = useMaterializeEpisodePlanMutation();
  const [publishCommit, publishState] = usePublishImportCommitMutation();
  const [plan, setPlan] = useState<API.EpisodePlanDetailResponse | null>(null);
  const [commit, setCommit] = useState<API.ImportCommitDetailResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const busy = createState.isLoading || confirmState.isLoading || materializeState.isLoading || publishState.isLoading;

  async function run<T>(operation: () => Promise<T>): Promise<T | null> {
    setError(null);
    setNotice(null);
    try {
      return await operation();
    } catch (cause: unknown) {
      setError(appApiErrorMessage(cause));
      return null;
    }
  }

  async function create(): Promise<void> {
    const created = await run(() =>
      createPlan({
        revisionId: analysis.revision.id,
        body: {
          strategy: "explicit_markers",
          target_duration_ms: targetDurationMs,
          requested_episode_count: null,
          idempotency_key: commandKey("episode-plan", analysis.revision.id, analysis.revision.normalized_hash),
        },
      }).unwrap(),
    );
    if (created) setPlan(created);
  }

  async function confirm(): Promise<void> {
    if (!plan) return;
    const confirmed = await run(() =>
      confirmPlan({
        planId: plan.plan.id,
        body: {
          expected_revision: plan.plan.revision,
          idempotency_key: commandKey("episode-plan-confirm", plan.plan.id, plan.plan.revision),
        },
      }).unwrap(),
    );
    if (confirmed) setPlan(confirmed);
  }

  async function materialize(): Promise<void> {
    if (!plan) return;
    const created = await run(() =>
      materializePlan({
        planId: plan.plan.id,
        body: {
          mode: "append_new",
          expected_plan_revision: plan.plan.revision,
          expected_project_revision: plan.impact.project_revision,
          expected_active_order_hash: plan.impact.active_order_hash,
          idempotency_key: commandKey("episode-plan-materialize", plan.plan.id, plan.plan.revision),
        },
      }).unwrap(),
    );
    if (created) setCommit(created);
  }

  async function publish(): Promise<void> {
    if (!commit || !plan) return;
    const published = await run(() =>
      publishCommit({
        commitId: commit.commit.id,
        body: {
          expected_revision: commit.commit.revision,
          idempotency_key: commandKey("episode-plan-publish", commit.commit.id, commit.commit.revision),
        },
      }).unwrap(),
    );
    if (published) {
      setCommit(published);
      setNotice(`${plan.proposals.length} 集剧本已批量发布；每集均已生成待确认的场景与制作任务。`);
    }
  }

  return (
    <Card aria-label="分集计划与批量创建" className="mt-8" role="region">
      <CardHeader>
        <CardTitle className="flex items-center gap-2"><Sparkles className="size-5" aria-hidden="true" />分集计划与批量创建</CardTitle>
        <CardDescription>按已验证的连续集标记切分不可变原稿，确认前不会创建正式剧集。</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-5 pt-6">
        {error ? <Alert variant="destructive"><AlertCircle aria-hidden="true" /><AlertTitle>分集操作未完成</AlertTitle><AlertDescription>{error}</AlertDescription></Alert> : null}
        {notice ? <Alert role="status"><CheckCircle2 aria-hidden="true" /><AlertTitle>批量发布完成</AlertTitle><AlertDescription>{notice}</AlertDescription></Alert> : null}

        {!plan ? (
          <div className="flex flex-wrap items-center justify-between gap-4 bg-muted/45 p-5">
            <div><p className="font-medium">生成确定性分集计划</p><p className="mt-1 text-sm text-muted-foreground">边界、标题和内容哈希均来自不可变 Revision，不调用模型猜测边界。</p></div>
            <Button disabled={!canWrite || busy} onClick={create}>{busy ? <LoaderCircle className="animate-spin" aria-hidden="true" /> : null}生成确定性分集计划</Button>
          </div>
        ) : (
          <>
            <div className="grid gap-3 bg-muted/45 p-5 sm:grid-cols-3">
              <div><p className="text-xs text-muted-foreground">状态</p><Badge className="mt-2" variant="outline">{plan.plan.status === "confirmed" ? "已确认" : plan.plan.status === "materialized" ? "已创建" : "待人工确认"}</Badge></div>
              <div><p className="text-xs text-muted-foreground">候选集数</p><p className="mt-1 text-xl font-semibold">{plan.proposals.length}</p></div>
              <div><p className="text-xs text-muted-foreground">全文字符</p><p className="mt-1 text-xl font-semibold">{plan.source.codepoint_count}</p></div>
            </div>
            <div className="grid gap-4 md:grid-cols-2">
              {plan.proposals.map((proposal) => (
                <article className="border p-4" key={proposal.id}>
                  <Label htmlFor={`episode-title-${proposal.id}`}>第 {proposal.position} 集标题</Label>
                  <Input className="mt-2" id={`episode-title-${proposal.id}`} readOnly value={proposal.title} />
                  <p className="mt-3 text-sm">{proposal.reason}</p>
                  <p className="mt-2 text-xs text-muted-foreground">置信度 {Math.round(proposal.confidence * 100)}% · 字符 {proposal.source_start}–{proposal.source_end}</p>
                  <pre className="mt-3 max-h-40 overflow-auto whitespace-pre-wrap bg-muted/45 p-3 text-xs">{plan.source.normalized_text.slice(proposal.source_start, proposal.source_end)}</pre>
                </article>
              ))}
            </div>
            <div className="flex flex-wrap justify-end gap-3 border-t pt-5">
              {plan.plan.status === "review_ready" ? <Button disabled={!canWrite || busy} onClick={confirm}>确认分集计划</Button> : null}
              {plan.plan.status === "confirmed" && !commit ? <Button disabled={!canWrite || busy} onClick={materialize}>原子创建 {plan.proposals.length} 集</Button> : null}
              {commit?.commit.status === "materialized" ? <Button disabled={!canWrite || busy} onClick={publish}>发布 {plan.proposals.length} 集剧本</Button> : null}
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}
