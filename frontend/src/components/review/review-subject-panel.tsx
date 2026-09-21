"use client";

import { Clock3, ExternalLink } from "lucide-react";
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from "@/components/ui/card";
import { Fact } from "@/components/review/review-fact";
import { subjectLabel, shortId, shortHash } from "@/components/review/review-presentation";
import {
  Field,
  FieldLabel,
  FieldSet,
  FieldLegend,
  FieldDescription,
  FieldContent,
} from "@/components/ui/field";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import {
  Empty,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
  EmptyDescription,
} from "@/components/ui/empty";
import { Button } from "@/components/ui/button";
import Link from "next/link";

export function EmptyDetail() {
  return (
    <Empty className="min-h-96">
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <Clock3 data-icon="inline-start" aria-hidden="true" />
        </EmptyMedia>
        <EmptyTitle>选择一个审核任务</EmptyTitle>
        <EmptyDescription>选择后查看待确认内容和可执行操作。</EmptyDescription>
      </EmptyHeader>
    </Empty>
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
  subject: API.HumanTaskDetailEnvelope["data"]["subject"];
  task: API.HumanTaskResponse;
}) {
  const selectionSubject = task.subject_type === "generation_candidate_selection";
  const structureSubject =
    subject?.schema_version === "structure-identity-human-gate-input-production" ? subject : null;
  return (
    <Card id="subject-fact">
      <CardHeader>
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
            <ExternalLink data-icon="inline-start" aria-hidden="true" />
          </Link>
        </Button>
        {selectionSubject ? (
          <FieldSet disabled={!canDecide}>
            <FieldLegend id={`candidate-label-${task.id}`} variant="label">
              冻结候选
            </FieldLegend>
            <RadioGroup
              aria-labelledby={`candidate-label-${task.id}`}
              disabled={!canDecide}
              value={effectiveCandidate}
              onValueChange={onCandidateChange}
            >
              {task.candidate_ids.map((candidateId) => (
                <Field key={candidateId} orientation="horizontal" data-disabled={!canDecide}>
                  <RadioGroupItem value={candidateId} id={`candidate-${candidateId}`} />
                  <FieldLabel htmlFor={`candidate-${candidateId}`}>{candidateId}</FieldLabel>
                </Field>
              ))}
            </RadioGroup>
          </FieldSet>
        ) : task.candidate_ids.length > 0 ? (
          <div>
            <h3 className="text-sm font-semibold">冻结输入引用</h3>
            <ul className="mt-2 grid gap-2">
              {task.candidate_ids.map((candidateId) => (
                <li className="py-3 font-mono text-xs" key={candidateId}>
                  {candidateId}
                </li>
              ))}
            </ul>
          </div>
        ) : null}
        {task.subject_type === "structure_identity_gate_input" && structureSubject ? (
          <FieldSet className="pt-5" disabled={!canDecide}>
            <FieldLegend id={`repair-label-${task.id}`} variant="label">
              冻结修复选项
            </FieldLegend>
            <p className="text-xs text-muted-foreground">
              只能选择 Backend
              已冻结的错误项、证据、目标和影响场景；提交后会生成新的候选与审核任务。
            </p>
            <RadioGroup
              aria-labelledby={`repair-label-${task.id}`}
              disabled={!canDecide}
              value={repairRequest ? repairChoiceKey(repairRequest) : ""}
              onValueChange={(value) => {
                const request = structureSubject.repair_options
                  .flatMap((option) =>
                    option.allowed_changes.map((change) => repairRequestFrom(option, change)),
                  )
                  .find((candidate) => repairChoiceKey(candidate) === value);
                if (request) onRepairChange(request);
              }}
            >
              {structureSubject.repair_options.map((option) => (
                <div className="grid gap-2 py-3" key={option.issue_key}>
                  <div>
                    <p className="text-sm font-medium">{option.summary}</p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {option.severity === "blocking" ? "阻塞" : "警告"} · {option.scope} ·{" "}
                      {option.code}
                    </p>
                  </div>
                  <ul
                    aria-label={`${option.summary}的冻结证据`}
                    className="grid gap-1 text-xs text-muted-foreground"
                  >
                    {option.evidence_refs.map((evidence) => (
                      <li
                        key={`${evidence.source_version_id}:${evidence.source_start}:${evidence.source_end}`}
                      >
                        原文区间 [{evidence.source_start}, {evidence.source_end}) ·{" "}
                        {shortHash(evidence.text_hash)}
                      </li>
                    ))}
                  </ul>
                  {option.allowed_changes.map((change, index) => {
                    const request = repairRequestFrom(option, change);
                    const choiceKey = repairChoiceKey(request);
                    return (
                      <Field
                        orientation="horizontal"
                        key={`${change.operation}:${index}`}
                        data-disabled={!canDecide}
                      >
                        <RadioGroupItem
                          value={choiceKey}
                          id={`repair-${option.issue_key}-${index}`}
                        />
                        <FieldContent>
                          <FieldLabel htmlFor={`repair-${option.issue_key}-${index}`}>
                            {repairOperationLabel(change.operation)}
                          </FieldLabel>
                          <FieldDescription>
                            {change.target_keys.join("、")} · {change.affected_scope_keys.length}{" "}
                            个场景
                          </FieldDescription>
                        </FieldContent>
                      </Field>
                    );
                  })}
                </div>
              ))}
            </RadioGroup>
          </FieldSet>
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
    reason_code:
      change.operation === "inspect_source"
        ? "source_interpretation_incorrect"
        : change.operation === "adjust_episode_boundary" ||
            change.operation === "adjust_scene_boundary"
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
  return (
    (
      {
        inspect_source: "重新核对原文",
        adjust_episode_boundary: "调整剧集边界",
        adjust_scene_boundary: "调整场景边界",
        separate_identity: "拆分身份",
        merge_identity: "合并身份",
        resolve_mention: "确认提及归属",
        reject_mention: "排除错误提及",
      } as Record<string, string>
    )[value] ?? value
  );
}
