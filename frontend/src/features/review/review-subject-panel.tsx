"use client";

import { Clock3, ExternalLink } from "lucide-react";
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from "@/components/ui/card";
import { Fact } from "./review-fact";
import { subjectLabel, shortId, shortHash } from "./review-presentation";
import { Button } from "@/components/ui/button";
import Link from "next/link";

export function EmptyDetail() {
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

export function SubjectPanel({
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
