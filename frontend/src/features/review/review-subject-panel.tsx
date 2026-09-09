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
  onRepairChange,
  projectId,
  repairRequest,
  subject,
  task,
}: {
  canDecide: boolean;
  effectiveCandidate: string;
  onCandidateChange: (value: string) => void;
  onRepairChange: (value: API.HumanGateChangeRequest) => void;
  projectId: string;
  repairRequest: API.HumanGateChangeRequest | null;
  subject: API.StructureIdentityReviewSubjectResponse | null;
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
        {task.subject_type === "structure_identity_gate_input" && subject ? (
          <fieldset className="grid gap-3 border-t pt-5" disabled={!canDecide}>
            <legend className="text-sm font-semibold">冻结修复选项</legend>
            <p className="text-xs text-muted-foreground">
              只能选择 Backend 已冻结的错误项、证据、目标和影响场景；提交后会生成新的候选与审核任务。
            </p>
            {subject.repair_options.map((option) => (
              <div className="grid gap-2 border p-3" key={option.issue_key}>
                <div>
                  <p className="text-sm font-medium">{option.summary}</p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {option.severity === "blocking" ? "阻塞" : "警告"} · {option.scope} · {option.code}
                  </p>
                </div>
                <ul aria-label={`${option.summary}的冻结证据`} className="grid gap-1 text-xs text-muted-foreground">
                  {option.evidence_refs.map((evidence) => (
                    <li key={`${evidence.source_version_id}:${evidence.source_start}:${evidence.source_end}`}>
                      原文区间 [{evidence.source_start}, {evidence.source_end}) · {shortHash(evidence.text_hash)}
                    </li>
                  ))}
                </ul>
                {option.allowed_changes.map((change, index) => {
                  const request = repairRequestFrom(option, change);
                  const choiceKey = repairChoiceKey(request);
                  return (
                    <label className="flex cursor-pointer items-start gap-3 border p-3 text-sm has-checked:border-foreground has-checked:bg-muted/50" key={`${change.operation}:${index}`}>
                      <input
                        checked={repairRequest ? repairChoiceKey(repairRequest) === choiceKey : false}
                        className="mt-0.5 size-4"
                        name={`repair-${task.id}`}
                        onChange={() => onRepairChange(request)}
                        type="radio"
                      />
                      <span>
                        <span className="block font-medium">{repairOperationLabel(change.operation)}</span>
                        <span className="mt-1 block text-xs text-muted-foreground">
                          {change.target_keys.join("、")} · {change.affected_scope_keys.length} 个场景
                        </span>
                      </span>
                    </label>
                  );
                })}
              </div>
            ))}
          </fieldset>
        ) : null}
      </CardContent>
    </Card>
  );
}

function repairRequestFrom(
  option: API.StructureIdentityRepairOptionResponse,
  change: API.HumanGateChangeSpec,
): API.HumanGateChangeRequest {
  return {
    issue_refs: [option.issue_key],
    evidence_refs: option.evidence_refs,
    change_spec: change,
    reason_code: change.operation === "inspect_source"
      ? "source_interpretation_incorrect"
      : change.operation === "adjust_episode_boundary" || change.operation === "adjust_scene_boundary"
        ? "structure_boundary_incorrect"
        : "identity_resolution_incorrect",
  };
}

function repairChoiceKey(value: API.HumanGateChangeRequest): string {
  return [
    ...value.issue_refs,
    value.change_spec.operation,
    ...value.change_spec.target_keys,
    ...value.change_spec.affected_scope_keys,
  ].join("\u0000");
}

function repairOperationLabel(value: string): string {
  return ({
    inspect_source: "重新核对原文",
    adjust_episode_boundary: "调整剧集边界",
    adjust_scene_boundary: "调整场景边界",
    separate_identity: "拆分身份",
    merge_identity: "合并身份",
    resolve_mention: "确认提及归属",
    reject_mention: "排除错误提及",
  } as Record<string, string>)[value] ?? value;
}
