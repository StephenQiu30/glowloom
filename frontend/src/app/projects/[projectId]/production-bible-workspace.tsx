"use client";

import {
  AlertCircle,
  BookOpenCheck,
  CheckCircle2,
  LoaderCircle,
  RotateCcw,
  Sparkles,
} from "lucide-react";
import { useState } from "react";

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
import {
  appApiErrorMessage,
  useCreateProductionBibleMutation,
  useCurrentProductionBibleQuery,
  useDecideProductionBibleReviewIssueMutation,
  useProductionBibleQuery,
  useResumeProductionBibleMutation,
  type ProductionBibleWithDecisions,
} from "@/lib/server-state";

const statusLabels: Record<API.ProductionBibleResponse["status"], string> = {
  cancelled: "已取消",
  confirmed: "已确认",
  failed: "生成失败",
  needs_review: "待人工确认",
  queued: "等待生成",
  running: "正在生成",
  superseded: "已被新版本替代",
  unknown: "状态待恢复",
};

const kindLabels: Record<API.ProductionBibleEntityResponse["kind"], string> = {
  character: "角色",
  costume: "服装",
  location: "场景",
  prop: "道具",
  visual_style: "视觉风格",
  voice: "声音",
};

function actionKey(action: string, ...parts: Array<string | number>): string {
  return [action, ...parts].join(":").slice(0, 200);
}

export function ProductionBibleWorkspace({
  analysis,
  canWrite,
  projectId,
}: {
  analysis: API.ScriptDocumentAnalysisResponse;
  canWrite: boolean;
  projectId: string;
}) {
  const currentQuery = useCurrentProductionBibleQuery(projectId, {
    pollingInterval: 5_000,
  });
  const [activeBibleId, setActiveBibleId] = useState<string | null>(null);
  const detailQuery = useProductionBibleQuery(activeBibleId ?? "", {
    pollingInterval: 3_000,
    skip: !activeBibleId,
  });
  const [createBible, createState] = useCreateProductionBibleMutation();
  const [decideReviewIssue, decideState] = useDecideProductionBibleReviewIssueMutation();
  const [resumeBible, resumeState] = useResumeProductionBibleMutation();
  const [actionError, setActionError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const queriedBible = detailQuery.data ?? currentQuery.data;
  const outdatedBible =
    queriedBible &&
    queriedBible.document_revision_id !== analysis.revision.id
      ? queriedBible
      : undefined;
  const bible = outdatedBible ? undefined : queriedBible;
  const busy = createState.isLoading || resumeState.isLoading || decideState.isLoading;
  const reviewDecisions = (bible as ProductionBibleWithDecisions | undefined)?.review_decisions ?? {};
  const blockingIssues =
    bible?.review_issues.filter((issue) => issue.severity === "blocking") ?? [];
  const unresolvedBlockingIssues = blockingIssues.filter(
    (issue) => reviewDecisions[issue.issue_key] !== "accepted",
  );
  const unavailableError = (currentQuery.error as { code?: string } | undefined);
  const queryError =
    detailQuery.error ??
    (unavailableError?.code && unavailableError.code !== "not_found"
      ? currentQuery.error
      : undefined);

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
      createBible({
        projectId,
        revisionId: analysis.revision.id,
        body: {
          idempotency_key: actionKey(
            "production-bible",
            analysis.revision.id,
            analysis.revision.normalized_hash,
          ),
        },
      }).unwrap(),
    );
    if (!created) return;
    setActiveBibleId(created.id);
    setNotice("制作圣经任务已创建；页面会从服务端恢复生成进度。");
  }

  async function resume(): Promise<void> {
    if (!bible) return;
    const resumed = await runAction(() =>
      resumeBible({
        projectId,
        bibleId: bible.id,
        body: {
          expected_revision: bible.revision,
          idempotency_key: actionKey(
            "resume-production-bible",
            bible.id,
            bible.revision,
          ),
        },
      }).unwrap(),
    );
    if (!resumed) return;
    setActiveBibleId(resumed.id);
    setNotice("已从最近的安全检查点恢复制作圣经任务。");
  }

  async function decideIssue(
    issueKey: string,
    action: "accepted" | "rejected",
  ): Promise<void> {
    if (!bible) return;
    const decided = await runAction(() =>
      decideReviewIssue({
        projectId,
        bibleId: bible.id,
        body: {
          issue_key: issueKey,
          action,
          expected_revision: bible.revision,
          idempotency_key: actionKey(
            "decide-production-bible-issue",
            bible.id,
            issueKey,
            action,
            bible.revision,
          ),
        },
      }).unwrap(),
    );
    if (!decided) return;
    setActiveBibleId(decided.id);
    setNotice(
      action === "accepted"
        ? "已明确接受该审阅风险；决议会与制作圣经一起保留。"
        : "该问题继续阻断确认，后续可以重新审阅并更改决议。",
    );
  }

  return (
    <Card aria-label="项目制作圣经" className="mt-8" role="region">
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle className="flex items-center gap-2">
              <BookOpenCheck className="size-5" aria-hidden="true" />项目制作圣经
            </CardTitle>
            <CardDescription className="mt-1">
              本地 Codex 从不可变整剧原稿提取统一角色、场景、道具和世界观；Workflow 审核通过后冻结为不可变版本。
            </CardDescription>
          </div>
          {bible ? <Badge variant="outline">{statusLabels[bible.status]}</Badge> : null}
        </div>
      </CardHeader>
      <CardContent className="grid gap-5 pt-6">
        {queryError || actionError ? (
          <Alert variant="destructive">
            <AlertCircle aria-hidden="true" />
            <AlertTitle>制作圣经操作未完成</AlertTitle>
            <AlertDescription>
              {actionError ?? appApiErrorMessage(queryError)}
            </AlertDescription>
          </Alert>
        ) : null}
        {notice ? (
          <Alert className="border-0 bg-muted/50" role="status">
            <CheckCircle2 aria-hidden="true" />
            <AlertTitle>制作圣经状态已更新</AlertTitle>
            <AlertDescription>{notice}</AlertDescription>
          </Alert>
        ) : null}

        {!bible ? (
          <div className="flex flex-wrap items-center justify-between gap-4 bg-muted/45 p-5">
            <div>
              <p className="font-medium">先建立跨剧集统一事实</p>
              <p className="mt-1 text-sm text-muted-foreground">
                {outdatedBible
                  ? "项目已有旧原稿的制作圣经；本次原稿必须生成并确认自己的新版本。"
                  : "分集计划可以先审阅，但正式发布前必须确认本次原稿对应的制作圣经。"}
              </p>
            </div>
            <Button disabled={!canWrite || busy} onClick={create}>
              {busy ? <LoaderCircle className="animate-spin" aria-hidden="true" /> : <Sparkles aria-hidden="true" />}
              生成项目制作圣经
            </Button>
          </div>
        ) : ["queued", "running"].includes(bible.status) ? (
          <div aria-live="polite" className="flex items-center gap-3 bg-muted/45 p-5">
            <LoaderCircle className="size-5 animate-spin" aria-hidden="true" />
            <div>
              <p className="font-medium">本地 Codex 正在分析整剧原稿</p>
              <p className="mt-1 text-sm text-muted-foreground">
                当前阶段：{bible.checkpoint_stage ?? "等待任务执行"}；刷新页面不会丢失任务。
              </p>
            </div>
          </div>
        ) : ["failed", "unknown", "cancelled"].includes(bible.status) ? (
          <div className="flex flex-wrap items-center justify-between gap-4 bg-destructive/5 p-5">
            <div>
              <p className="font-medium">{statusLabels[bible.status]}</p>
              <p className="mt-1 text-sm text-muted-foreground">
                {bible.generation_error?.summary ??
                  "原任务保持可追溯；从已保存检查点创建一次受控恢复。"}
              </p>
              {bible.generation_error ? (
                <p
                  aria-label="制作圣经生成错误"
                  className="mt-2 text-xs text-muted-foreground"
                  role="alert"
                >
                  {bible.generation_error.code} ·{" "}
                  {bible.generation_error.retryable ? "可恢复" : "不可恢复"}
                </p>
              ) : null}
            </div>
            <Button disabled={!canWrite || busy} onClick={resume} variant="outline">
              <RotateCcw aria-hidden="true" />恢复生成
            </Button>
          </div>
        ) : (
          <>
            <dl className="grid gap-3 sm:grid-cols-3">
              <div className="bg-muted/45 p-4">
                <dt className="text-xs text-muted-foreground">统一实体</dt>
                <dd className="mt-1 text-2xl font-semibold">{bible.entities?.length ?? 0}</dd>
              </div>
              <div className="bg-muted/45 p-4">
                <dt className="text-xs text-muted-foreground">世界观条目</dt>
                <dd className="mt-1 text-2xl font-semibold">{bible.world_entries?.length ?? 0}</dd>
              </div>
              <div className="bg-muted/45 p-4">
                <dt className="text-xs text-muted-foreground">审阅问题</dt>
                <dd className="mt-1 text-2xl font-semibold">{bible.review_issues.length}</dd>
              </div>
            </dl>

            {(bible.entities?.length ?? 0) > 0 ? (
              <section aria-label="制作圣经实体" className="grid gap-3 sm:grid-cols-2">
                {bible.entities?.map((entity) => (
                  <article className="border p-4" key={entity.id}>
                    <div className="flex items-center justify-between gap-3">
                      <p className="font-medium">{entity.canonical_name}</p>
                      <Badge variant="secondary">{kindLabels[entity.kind]}</Badge>
                    </div>
                    <p className="mt-2 text-xs text-muted-foreground">
                      出现于第 {entity.episode_numbers.join("、") || "未标注"} 集 · {entity.states?.length ?? 0} 个状态
                    </p>
                  </article>
                ))}
              </section>
            ) : null}

            {bible.review_issues.length ? (
              <section aria-label="制作圣经审阅问题" className="grid gap-3">
                {bible.review_issues.map((issue) => (
                  <Alert key={issue.issue_key} variant={issue.severity === "blocking" ? "destructive" : "default"}>
                    <AlertCircle aria-hidden="true" />
                    <AlertTitle>{issue.code}</AlertTitle>
                    <AlertDescription>
                      <p>{issue.summary}</p>
                      {issue.severity === "blocking" ? (
                        <div className="mt-3 flex flex-wrap items-center gap-2">
                          <Badge variant="outline">
                            {reviewDecisions[issue.issue_key] === "accepted"
                              ? "已接受风险"
                              : reviewDecisions[issue.issue_key] === "rejected"
                                ? "继续阻断"
                                : "待人工决议"}
                          </Badge>
                          {reviewDecisions[issue.issue_key] !== "accepted" ? (
                            <Button
                              disabled={!canWrite || busy}
                              onClick={() => void decideIssue(issue.issue_key, "accepted")}
                              size="sm"
                              type="button"
                            >
                              接受风险并继续
                            </Button>
                          ) : null}
                          {reviewDecisions[issue.issue_key] !== "rejected" ? (
                            <Button
                              disabled={!canWrite || busy}
                              onClick={() => void decideIssue(issue.issue_key, "rejected")}
                              size="sm"
                              type="button"
                              variant="outline"
                            >
                              保留阻断
                            </Button>
                          ) : null}
                        </div>
                      ) : null}
                    </AlertDescription>
                  </Alert>
                ))}
              </section>
            ) : null}

            {bible.status === "needs_review" ? (
              <div className="border-t pt-5">
                <p className="text-sm text-muted-foreground">
                  {unresolvedBlockingIssues.length
                    ? `仍有 ${unresolvedBlockingIssues.length} 个阻断问题需要明确人工决议。`
                    : "候选已具备审核条件；请在项目 HumanTask 审核队列中批准，系统会冻结精确 Candidate Revision 并恢复 Workflow。"}
                </p>
              </div>
            ) : (
              <Alert className="border-emerald-200 bg-emerald-50" role="status">
                <CheckCircle2 aria-hidden="true" />
                <AlertTitle>制作圣经已确认</AlertTitle>
                <AlertDescription>分集发布与后续场景、任务、分镜都将固定引用此版本。</AlertDescription>
              </Alert>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}
